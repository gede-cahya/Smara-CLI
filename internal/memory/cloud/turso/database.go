// Package turso — EnsureDatabase implementation (task 8.2).
//
// This file implements TursoProvider.EnsureDatabase, the idempotent
// per-workspace remote-database provisioning step required by
// requirements 5.1, 5.2, 6.1, 6.5, and 9.1.
//
// Flow summary:
//
//  1. Resolve the per-workspace database name by applying the configured
//     pattern (cfg.DBNamePattern, default "smara-{workspace}") to the
//     workspaceName argument via applyDBNamePattern. The result is
//     validated against the libSQL/Turso naming constraint (lowercase,
//     alphanumeric + hyphen, ≤ 64 chars after substitution) using the
//     regex `^[a-z0-9-]+$`. An invalid name is rejected before any
//     network I/O so a typo cannot consume quota.
//
//  2. POST /v1/organizations/{org}/databases with a JSON body carrying
//     the resolved name and the caller-supplied region (creds.Region or
//     cfg-default). Auth is `Authorization: Bearer <creds.Token>`. The
//     HTTP client comes from p.httpClient, which (per turso.go) pins
//     TLS 1.3 so the request cannot be downgraded.
//
//  3. Status code dispatch:
//
//     2xx — parse the response body, populate *cloud.DatabaseInfo,
//     return.
//
//     409 — database already exists. Per the idempotency contract
//     (Provider.EnsureDatabase docstring) we GET the existing
//     metadata via /v1/organizations/{org}/databases/{name} and
//     return a *cloud.DatabaseInfo built from the response.
//
//     402 — quota exceeded. Return cloud.ErrQuotaExceeded so callers
//     (cmd/smara/workspace.go --local-only path) can branch on
//     errors.Is and skip cloud provisioning gracefully.
//
//     other 4xx/5xx — wrap the status code + a truncated body slice
//     into the returned error so the operator sees the actual
//     provider response without flooding the terminal.
//
// IMPORTANT — endpoint URLs:
//
//	The path templates below (`/v1/organizations/{org}/databases`,
//	`/v1/organizations/{org}/databases/{name}`) match the public Turso
//	Platform REST API documented at api.turso.tech. They are wrapped in
//	helper functions so a future migration to a different scheme (e.g.
//	region-scoped routing) only needs to change one place.
//
//	TODO(turso-real-endpoints): The exact JSON shape of the Turso
//	create/get database response is provisional. The fields read by
//	databaseInfoFromResponse mirror the spec's design.md contract; once
//	Turso publishes a stable schema we should adjust the struct tags
//	and the test fixtures together.
package turso

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/audit"
	cloud "github.com/gede-cahya/Smara-CLI/internal/memory/cloud"
)

// ----------------------------------------------------------------------------
// Constants
// ----------------------------------------------------------------------------

const (
	// tursoAPIBaseURL is the public base URL of the Turso Platform REST
	// API. It is hard-coded (rather than read from config) because every
	// supported Turso deployment shares the same hostname; routing to a
	// region-specific edge is handled server-side once the request lands.
	//
	// TODO(turso-real-endpoints): if Turso ever ships a region-scoped
	// API endpoint, surface it as a config option and resolve here.
	tursoAPIBaseURL = "https://api.turso.tech"

	// defaultDBNamePattern is the fallback used when cfg.DBNamePattern
	// is empty. It matches the default in DefaultConfig() so an
	// uninitialised TursoProvider (e.g. one constructed for tests via
	// NewTursoProvider() without a wired Config) still produces sane
	// names instead of panicking on the placeholder check.
	defaultDBNamePattern = "smara-{workspace}"

	// dbNameMaxLen is the maximum length of a Turso/libSQL database
	// name after pattern substitution. The limit comes from the libSQL
	// hostname budget (databases live at <name>-<org>.turso.io) and is
	// enforced explicitly so the validation error is descriptive.
	dbNameMaxLen = 64

	// errorBodyMaxLen caps the length of the body slice we embed into
	// non-2xx error messages. 256 chars is enough for a representative
	// snippet without flooding the terminal when the server returns a
	// large HTML error page or stack trace.
	errorBodyMaxLen = 256

	// requestBodyByteCap is the maximum number of bytes we will buffer
	// from a non-error response body. 1 MiB is well above any
	// legitimate Turso JSON response and gives the runtime a hard upper
	// bound so a misbehaving server (or MITM) cannot exhaust memory.
	requestBodyByteCap = 1 << 20
)

// dbNameRegexp is the canonical libSQL/Turso name validator described in
// the task brief and design.md Section "Model 1 / Validation Rules":
// lowercase alphanumeric plus hyphen, at least one character. The length
// budget is enforced separately via dbNameMaxLen so the failure message
// can distinguish "wrong characters" from "too long".
var dbNameRegexp = regexp.MustCompile(`^[a-z0-9-]+$`)

// ----------------------------------------------------------------------------
// Public API — Provider.EnsureDatabase
// ----------------------------------------------------------------------------

// EnsureDatabase provisions (or, when one already exists, fetches) the
// remote Turso database for the given workspace. It is the idempotent
// entrypoint required by Provider.EnsureDatabase and is called from the
// CLI workspace-create and bootstrap paths.
//
// Argument contract:
//
//   - ctx      — caller-driven cancellation. Propagated to every HTTP
//     request so a parent timeout terminates the call cleanly.
//   - creds    — non-nil credentials with a populated Token and OrgID.
//     A nil pointer or empty Token / OrgID is rejected before
//     any network I/O.
//   - workspaceName — the local workspace identifier (no
//     Smara-internal prefix). It is substituted into
//     cfg.DBNamePattern to produce the Turso database
//     name. Empty input is rejected.
//
// Returned *cloud.DatabaseInfo always carries Provider="turso", a Name,
// and a CreatedAt timestamp. URL and AuthToken are populated when the
// upstream response provides them; missing optional fields are left
// zero-valued so the caller can treat them as "unknown" rather than
// "absent".
func (p *TursoProvider) EnsureDatabase(ctx context.Context, creds *cloud.Credentials, workspaceName string) (info *cloud.DatabaseInfo, err error) {
	// Audit hook: log every EnsureDatabase attempt with the resolved
	// db_name and region (when known) so operators can correlate provider
	// state against the local mapping. Per requirement 16.1/16.3 we never
	// record credentials or row content; the success bool reflects the
	// final return value and any error is redacted by audit.LogCloudOp
	// before reaching disk.
	//
	// db_name and region are computed inside this function, so we
	// initialise them as empty strings up front and let the defer pick
	// up whichever value is set by the time the function returns. When a
	// validation error fires before the name is resolved, target is left
	// empty — the audit entry still records the attempt but cannot name
	// a database that was never computed.
	var (
		auditDBName string
		auditRegion string
	)
	defer func() {
		extra := map[string]any{
			"workspace_name": workspaceName,
		}
		if auditRegion != "" {
			extra["region"] = auditRegion
		}
		if info != nil {
			// Prefer the server-side authoritative name/region when the
			// call succeeded, falling back to the locally-computed
			// values if the response omitted them.
			if info.Name != "" {
				auditDBName = info.Name
			}
			if info.Region != "" {
				extra["region"] = info.Region
			}
		}
		if err != nil {
			extra["error"] = err.Error()
		}
		_ = audit.LogCloudOp("ensure_database", err == nil, auditDBName, extra)
	}()

	if creds == nil {
		return nil, errors.New("turso: EnsureDatabase: nil credentials")
	}
	if creds.Token == "" {
		return nil, errors.New("turso: EnsureDatabase: credentials missing token")
	}
	if creds.OrgID == "" {
		return nil, errors.New("turso: EnsureDatabase: credentials missing org_id")
	}
	if workspaceName == "" {
		return nil, errors.New("turso: EnsureDatabase: empty workspace name")
	}

	// Resolve and validate the per-workspace database name. We pull the
	// pattern from p.cfg.DBNamePattern when set so a custom pattern
	// configured via YAML wins over the package default.
	pattern := p.cfg.DBNamePattern
	if pattern == "" {
		pattern = defaultDBNamePattern
	}
	dbName, err := applyDBNamePattern(pattern, workspaceName)
	if err != nil {
		return nil, fmt.Errorf("turso: EnsureDatabase: %w", err)
	}
	auditDBName = dbName

	// Region precedence: prefer the configured creds.Region (set during
	// login or from the SMARA_CLOUD_REGION env), falling back to the
	// empty string so the Turso server-side default is used. We do NOT
	// hard-code a region here — the user's chosen region is the
	// authoritative source of truth.
	region := strings.TrimSpace(creds.Region)
	auditRegion = region

	info, err = p.createDatabase(ctx, creds, dbName, region)
	if err == nil {
		return info, nil
	}

	// 409 Conflict → database already exists. This is the idempotent
	// success path: we fall through to a GET so the caller still gets
	// the full *DatabaseInfo populated.
	var conflict *databaseAlreadyExistsError
	if errors.As(err, &conflict) {
		info, err = p.fetchDatabase(ctx, creds, dbName)
		return info, err
	}
	return nil, err
}

// ----------------------------------------------------------------------------
// Pattern substitution + name validation
// ----------------------------------------------------------------------------

// applyDBNamePattern substitutes the {workspace} placeholder in pattern
// with workspace and validates the result against the libSQL/Turso name
// constraint. It returns the resolved name on success.
//
// The function is exported (lowercase first letter, but referenced from
// EnsureDatabase) so a unit test can exercise the pattern logic without
// the surrounding HTTP machinery; it is also the single source of truth
// for the substitution rules so a future provider that reuses the same
// pattern dialect can call it directly.
//
// Validation rules (mirroring task 8.2 brief and design.md §Validation):
//
//   - The pattern MUST contain the literal "{workspace}" placeholder.
//     This is also enforced by cloud.Config.Validate, but re-checked
//     here so a programmatic caller that bypassed Validate still fails
//     deterministically.
//   - The workspace MUST be non-empty (caller guarantees this; we
//     re-assert defensively).
//   - The substituted name MUST match `^[a-z0-9-]+$` AND be ≤ 64 chars.
//     Failures produce distinct error messages so the user can fix the
//     specific issue (wrong characters vs too long).
func applyDBNamePattern(pattern, workspace string) (string, error) {
	if !strings.Contains(pattern, "{workspace}") {
		return "", fmt.Errorf("invalid DBNamePattern %q: must contain {workspace} placeholder", pattern)
	}
	if workspace == "" {
		return "", errors.New("workspace name must not be empty")
	}

	name := strings.ReplaceAll(pattern, "{workspace}", workspace)

	if len(name) > dbNameMaxLen {
		return "", fmt.Errorf(
			"resolved database name %q exceeds %d characters (got %d)",
			name, dbNameMaxLen, len(name),
		)
	}
	if !dbNameRegexp.MatchString(name) {
		return "", fmt.Errorf(
			"resolved database name %q is invalid: must match %s (lowercase alphanumeric + hyphen)",
			name, dbNameRegexp.String(),
		)
	}
	return name, nil
}

// ----------------------------------------------------------------------------
// HTTP — create
// ----------------------------------------------------------------------------

// createDatabasePayload is the JSON body we send to the Turso create
// endpoint. The fields match the public REST contract (name + region);
// additional fields can be added without breaking existing callers.
type createDatabasePayload struct {
	Name   string `json:"name"`
	Region string `json:"region,omitempty"`
}

// createDatabase issues the POST request that creates a new Turso
// database. On 2xx it returns a populated *cloud.DatabaseInfo; on 409
// it returns a *databaseAlreadyExistsError so EnsureDatabase can fall
// through to the GET path; on 402 it returns cloud.ErrQuotaExceeded;
// other non-2xx statuses surface a wrapped error including the status
// code and a truncated body slice.
func (p *TursoProvider) createDatabase(ctx context.Context, creds *cloud.Credentials, name, region string) (*cloud.DatabaseInfo, error) {
	body, err := json.Marshal(createDatabasePayload{Name: name, Region: region})
	if err != nil {
		return nil, fmt.Errorf("turso: marshal create payload: %w", err)
	}

	endpoint := databasesEndpoint(creds.OrgID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("turso: build create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+creds.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("turso: create database %q: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, requestBodyByteCap))
	if readErr != nil {
		return nil, fmt.Errorf("turso: read create response for %q: %w", name, readErr)
	}

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		info, derr := databaseInfoFromResponse(respBody, name, region)
		if derr != nil {
			return nil, fmt.Errorf("turso: parse create response for %q: %w", name, derr)
		}
		return info, nil

	case resp.StatusCode == http.StatusConflict:
		// 409 means the database already exists for this org. The
		// caller (EnsureDatabase) re-issues a GET to fetch the
		// existing metadata and return it — see the docstring for
		// the idempotency contract.
		return nil, &databaseAlreadyExistsError{name: name}

	case resp.StatusCode == http.StatusPaymentRequired:
		// 402 maps directly to the cloud-package sentinel so the
		// caller can branch via errors.Is(err, cloud.ErrQuotaExceeded)
		// without parsing the message body.
		return nil, fmt.Errorf("%w: turso create %q rejected (HTTP 402): %s",
			cloud.ErrQuotaExceeded, name, truncateForError(string(respBody)))

	default:
		return nil, fmt.Errorf(
			"turso: create database %q failed: HTTP %d: %s",
			name, resp.StatusCode, truncateForError(string(respBody)),
		)
	}
}

// databaseAlreadyExistsError is the typed signal that a POST returned
// HTTP 409. It is package-private; callers branch on it via
// errors.As inside EnsureDatabase to fall through to the GET path.
type databaseAlreadyExistsError struct {
	name string
}

func (e *databaseAlreadyExistsError) Error() string {
	return fmt.Sprintf("turso: database %q already exists", e.name)
}

// ----------------------------------------------------------------------------
// HTTP — fetch (idempotent fall-through)
// ----------------------------------------------------------------------------

// fetchDatabase issues the GET request that retrieves the metadata for
// an existing Turso database. It is invoked from EnsureDatabase after a
// 409 Conflict on POST so the caller still gets a fully populated
// *cloud.DatabaseInfo (mirroring the contract on first creation).
//
// Status code dispatch:
//
//   - 2xx → parse and return.
//   - 404 → wrapped not-found error. This is a defensive branch: a 409
//     on POST followed by a 404 on GET would indicate a race or
//     a permissions issue, and the operator deserves a clear
//     diagnostic rather than a silent fallback.
//   - other → wrapped status + truncated body, mirroring createDatabase.
func (p *TursoProvider) fetchDatabase(ctx context.Context, creds *cloud.Credentials, name string) (*cloud.DatabaseInfo, error) {
	endpoint := databaseEndpoint(creds.OrgID, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("turso: build get request for %q: %w", name, err)
	}
	req.Header.Set("Authorization", "Bearer "+creds.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("turso: fetch database %q: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, requestBodyByteCap))
	if readErr != nil {
		return nil, fmt.Errorf("turso: read get response for %q: %w", name, readErr)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// We know the region only via creds (or via the response); pass
		// the cred-side value as a fallback so an empty server-side
		// "region" field still yields a sensible *DatabaseInfo.
		info, derr := databaseInfoFromResponse(respBody, name, creds.Region)
		if derr != nil {
			return nil, fmt.Errorf("turso: parse get response for %q: %w", name, derr)
		}
		return info, nil
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf(
			"turso: database %q not found after 409 conflict (HTTP 404): %s",
			name, truncateForError(string(respBody)),
		)
	}

	return nil, fmt.Errorf(
		"turso: fetch database %q failed: HTTP %d: %s",
		name, resp.StatusCode, truncateForError(string(respBody)),
	)
}

// ----------------------------------------------------------------------------
// Endpoint helpers
// ----------------------------------------------------------------------------

// databasesEndpoint returns the collection URL for an organisation's
// databases. Centralising the path here keeps the endpoint format in a
// single place so future schema migrations only need to touch one
// function.
func databasesEndpoint(orgID string) string {
	return fmt.Sprintf("%s/v1/organizations/%s/databases", tursoAPIBaseURL, orgID)
}

// databaseEndpoint returns the per-database URL used by the GET (and
// future DELETE, task 8.5) operations.
func databaseEndpoint(orgID, name string) string {
	return fmt.Sprintf("%s/v1/organizations/%s/databases/%s", tursoAPIBaseURL, orgID, name)
}

// ----------------------------------------------------------------------------
// Response parsing
// ----------------------------------------------------------------------------

// tursoDatabaseEnvelope is the JSON shape we expect from the Turso REST
// API for both POST (create) and GET (fetch) responses. The schema
// reflects the publicly documented contract: a top-level "database"
// object plus an optional sibling "auth_token" / "token" emitted on
// creation.
//
// TODO(turso-real-endpoints): Turso has historically tweaked field
// names (Hostname vs URL, AuthToken vs Token); we read both so a minor
// rename does not break the existing wire path. If the schema is ever
// formally locked down we should replace the dual-field decoder with a
// single canonical name.
type tursoDatabaseEnvelope struct {
	// Database is the canonical wrapper used by Turso's public API.
	Database *tursoDatabaseObject `json:"database,omitempty"`

	// AuthToken / Token may be present at the envelope root on
	// create responses. We accept both so a future server tweak does
	// not break the parser.
	AuthToken string `json:"auth_token,omitempty"`
	Token     string `json:"token,omitempty"`
}

// tursoDatabaseObject is the inner database record. We keep the field
// list minimal — anything not consumed by *cloud.DatabaseInfo is
// simply ignored by encoding/json.
type tursoDatabaseObject struct {
	Name          string `json:"name,omitempty"`
	Hostname      string `json:"hostname,omitempty"`
	URL           string `json:"url,omitempty"`
	Region        string `json:"region,omitempty"`
	PrimaryRegion string `json:"primaryRegion,omitempty"`

	// Token-bearing fields. The wire schema is provisional; we accept
	// either name to insulate this code from a future rename.
	AuthToken string `json:"auth_token,omitempty"`
	Token     string `json:"token,omitempty"`

	CreatedAt string `json:"created_at,omitempty"`
}

// databaseInfoFromResponse decodes a Turso REST response body into a
// *cloud.DatabaseInfo. It is shared between createDatabase and
// fetchDatabase so the population rules (URL fallback, region
// precedence, CreatedAt parsing) live in exactly one place.
//
// fallbackName is the database name we expect the server to echo back;
// if the response omits it (legitimate for some Turso schemas) we fill
// in the caller-supplied value so the returned *DatabaseInfo always
// carries a non-empty Name.
//
// fallbackRegion is the region the caller asked for; it is used only
// when the server response omits the "region" field. The server-side
// value, when present, always wins.
func databaseInfoFromResponse(body []byte, fallbackName, fallbackRegion string) (*cloud.DatabaseInfo, error) {
	var env tursoDatabaseEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode response body: %w", err)
	}

	info := &cloud.DatabaseInfo{
		Provider:  "turso",
		Name:      fallbackName,
		Region:    fallbackRegion,
		CreatedAt: time.Now().UTC(),
	}

	if env.Database != nil {
		db := env.Database
		if db.Name != "" {
			info.Name = db.Name
		}
		if db.URL != "" {
			info.URL = db.URL
		} else if db.Hostname != "" {
			// Turso historically returned only Hostname (no scheme);
			// libSQL clients require a libsql:// or https:// prefix,
			// so we synthesise libsql:// to keep downstream OpenStore
			// callers happy.
			info.URL = "libsql://" + db.Hostname
		}
		if db.Region != "" {
			info.Region = db.Region
		} else if db.PrimaryRegion != "" {
			info.Region = db.PrimaryRegion
		}
		if db.AuthToken != "" {
			info.AuthToken = db.AuthToken
		} else if db.Token != "" {
			info.AuthToken = db.Token
		}
		if db.CreatedAt != "" {
			if t, err := time.Parse(time.RFC3339, db.CreatedAt); err == nil {
				info.CreatedAt = t.UTC()
			}
		}
	}

	// Envelope-level token fields override the inner-object value when
	// only one is set. This handles the create-response shape where
	// the freshly minted token is emitted alongside (rather than
	// inside) the database object.
	if info.AuthToken == "" {
		if env.AuthToken != "" {
			info.AuthToken = env.AuthToken
		} else if env.Token != "" {
			info.AuthToken = env.Token
		}
	}

	return info, nil
}

// ----------------------------------------------------------------------------
// ListWorkspaceDatabases — GET all databases for the org (task 8.5)
// ----------------------------------------------------------------------------

// tursoListDatabasesResponse mirrors the JSON shape returned by the Turso
// list-databases endpoint: an array of database objects under the
// "databases" key.
type tursoListDatabasesResponse struct {
	Databases []tursoDatabaseObject `json:"databases"`
}

// ListWorkspaceDatabases returns every remote database belonging to the
// authenticated org. It calls GET /v1/organizations/{org}/databases and
// filters results to those whose name matches the configured
// DBNamePattern prefix (e.g. "smara-"). Unfiltered results are returned
// when the pattern cannot be resolved into a stable prefix.
//
// Requirements: 5.5, 12.3.
func (p *TursoProvider) ListWorkspaceDatabases(ctx context.Context, creds *cloud.Credentials) ([]cloud.DatabaseInfo, error) {
	if creds == nil {
		return nil, errors.New("turso: ListWorkspaceDatabases: nil credentials")
	}
	if creds.Token == "" {
		return nil, errors.New("turso: ListWorkspaceDatabases: empty token")
	}
	if creds.OrgID == "" {
		return nil, errors.New("turso: ListWorkspaceDatabases: empty org_id")
	}

	// Cache org for subsequent calls.
	p.activeOrg = creds.OrgID

	endpoint := databasesEndpoint(creds.OrgID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("turso: ListWorkspaceDatabases: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+creds.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("turso: ListWorkspaceDatabases: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, requestBodyByteCap))
	if err != nil {
		return nil, fmt.Errorf("turso: ListWorkspaceDatabases: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("turso: ListWorkspaceDatabases: HTTP %d: %s",
			resp.StatusCode, truncateForError(string(body)))
	}

	var list tursoListDatabasesResponse
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("turso: ListWorkspaceDatabases: decode response: %w", err)
	}

	// Resolve a prefix filter from the configured DBNamePattern so only
	// Smara-managed databases appear. When the pattern is "smara-{workspace}",
	// the prefix is "smara-". The filter is best-effort: an empty or
	// malformed pattern results in an unfiltered list.
	prefix := dbPatternPrefix(p.cfg.DBNamePattern)

	result := make([]cloud.DatabaseInfo, 0, len(list.Databases))
	for _, db := range list.Databases {
		if prefix != "" && !strings.HasPrefix(db.Name, prefix) {
			continue
		}
		result = append(result, cloud.DatabaseInfo{
			Provider:  "turso",
			Name:      db.Name,
			URL:       coalesce(db.URL, "libsql://"+db.Hostname),
			Region:    coalesce(db.Region, db.PrimaryRegion),
			CreatedAt: parseCreatedAt(db.CreatedAt),
		})
	}

	_ = audit.LogCloudOp("list_workspace_databases", true, creds.Provider, map[string]any{
		"count":  len(result),
		"prefix": prefix,
	})
	return result, nil
}

// ----------------------------------------------------------------------------
// DeleteWorkspaceDatabase — DELETE a single database (task 8.5)
// ----------------------------------------------------------------------------

// DeleteWorkspaceDatabase removes a remote database by name. The
// operation is idempotent: a 404 response is treated as success (the
// desired end state — database does not exist — is already satisfied).
//
// Requirements: 12.3 (nuke command), idempotent delete contract.
func (p *TursoProvider) DeleteWorkspaceDatabase(ctx context.Context, creds *cloud.Credentials, dbName string) error {
	if creds == nil {
		return errors.New("turso: DeleteWorkspaceDatabase: nil credentials")
	}
	if creds.Token == "" {
		return errors.New("turso: DeleteWorkspaceDatabase: empty token")
	}
	if creds.OrgID == "" {
		return errors.New("turso: DeleteWorkspaceDatabase: empty org_id")
	}
	if dbName == "" {
		return errors.New("turso: DeleteWorkspaceDatabase: empty database name")
	}

	endpoint := databaseEndpoint(creds.OrgID, dbName)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("turso: DeleteWorkspaceDatabase %q: build request: %w", dbName, err)
	}
	req.Header.Set("Authorization", "Bearer "+creds.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("turso: DeleteWorkspaceDatabase %q: request: %w", dbName, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Idempotent: 404 means the database is already gone — success.
	if resp.StatusCode == http.StatusNotFound {
		_ = audit.LogCloudOp("delete_database", true, dbName, map[string]any{
			"already_deleted": true,
		})
		return nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyMaxLen))
		return fmt.Errorf("turso: DeleteWorkspaceDatabase %q: HTTP %d: %s",
			dbName, resp.StatusCode, truncateForError(string(body)))
	}

	_ = audit.LogCloudOp("delete_database", true, dbName, nil)
	return nil
}

// ----------------------------------------------------------------------------
// Shared helpers
// ----------------------------------------------------------------------------

// dbPatternPrefix extracts a stable string prefix from the
// DBNamePattern for use in filtering ListWorkspaceDatabases results.
// For "smara-{workspace}" this returns "smara-". For patterns where
// the placeholder is at the start (e.g. "{workspace}-db") this returns
// "" because there is no fixed prefix.
func dbPatternPrefix(pattern string) string {
	if pattern == "" {
		pattern = defaultDBNamePattern
	}
	idx := strings.Index(pattern, "{workspace}")
	if idx <= 0 {
		return ""
	}
	return pattern[:idx]
}

// coalesce returns a if non-empty, otherwise b. Used for picking the
// first available field from the Turso JSON response.
func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// parseCreatedAt attempts to parse a Turso timestamp into a UTC time.
// On failure (or empty input) it returns the current UTC time as a
// fallback so callers always receive a populated CreatedAt.
func parseCreatedAt(raw string) time.Time {
	if raw == "" {
		return time.Now().UTC()
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC()
	}
	return time.Now().UTC()
}
