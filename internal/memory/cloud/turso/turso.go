// Package turso implements the cloud.Provider interface backed by
// Turso (libSQL) embedded replicas + Turso Platform REST API.
//
// File layout (as scoped by the cloud-memory spec, section "Tasks 8.x"):
//
//	turso.go      — TursoProvider struct, constructor, Name, Provider stubs.
//	login.go      — PKCE OAuth login flow (task 8.1).
//	database.go   — EnsureDatabase / List / Delete via Turso REST (task 8.2, 8.5).
//	sync.go       — Push / Pull / Status against the embedded replica (task 8.4).
//
// This file is the skeleton landed by task 8.1: it defines the struct,
// the constructor, the Name() identifier, and stub implementations of
// every other Provider method that returns a "not yet implemented"
// error so the package compiles and `cloud.Get("turso")` returns a
// usable handle while subsequent tasks (8.2 — 8.6) fill in the real
// behavior.
//
// Provider registration (cloud.Register("turso", ...)) lives in this
// file's init() (task 8.6). The CLI triggers the registration by
// blank-importing this package from cmd/smara/main.go, so by the time
// rootCmd parses flags the "turso" name is already resolvable via
// cloud.Get.
package turso

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	cloud "github.com/gede-cahya/Smara-CLI/internal/memory/cloud"
)

// errNotYetImplemented is the placeholder returned by every Provider
// method whose body is delivered by a later task in the 8.x series.
// It is package-private so external callers cannot accidentally rely
// on the literal text in errors.Is comparisons; tests inside this
// package can still assert on it via direct reference.
var errNotYetImplemented = errors.New("turso: not yet implemented")

// Compile-time assertion that *TursoProvider satisfies cloud.Provider.
//
// The blank identifier on the LHS yields a zero-cost check that fires
// at build time the moment the cloud.Provider interface gains a method
// not implemented here (or the moment a TursoProvider method drifts
// out of the contract). It complements (rather than replaces) the
// runtime registry test added in task 8.6: the registry can only fire
// once the package's init() runs, while this assertion catches the
// signature mismatch the compiler would otherwise surface only in the
// (much noisier) call site of cloud.Get("turso").
var _ cloud.Provider = (*TursoProvider)(nil)

// TursoProvider implements cloud.Provider for Turso / libSQL.
//
// Fields are populated incrementally across tasks 8.x:
//
//   - httpClient is the HTTP client used to talk to the Turso Platform
//     REST API (`https://api.turso.tech/...`). It is initialised here
//     with a sane default Timeout so login attempts cannot hang
//     indefinitely on a stalled TLS handshake.
//
//   - cfg holds the cloud.Config snapshot that scopes provider
//     behaviour (DB name pattern, sync interval, conflict policy, ...).
//     It is left zero-valued until task 8.2 wires it in via a
//     configuration helper; the login flow does not depend on it.
//
//   - replicaDB is the *sql.DB handle to the libSQL embedded-replica
//     store opened by OpenStoreWithCloud. It is set via SetReplicaDB
//     after the store is constructed so Push/Pull/Status can interact
//     with sync_log and the replica’s SQL-level sync API (task 8.4).
//
// The struct is exported (capital T) so cmd/smara packages can
// blank-import this package and rely on the registry returning a
// *TursoProvider when name == "turso".
type TursoProvider struct {
	// httpClient is reused across REST calls so connections are
	// pooled. The default 30s timeout gives slow free-tier Turso
	// regions (Sydney, Frankfurt) headroom while still surfacing
	// outages quickly.
	httpClient *http.Client

	// cfg is the active cloud configuration. Task 8.1 does not read
	// this field; later tasks (EnsureDatabase, OpenStore, Push, Pull,
	// Status) wire their behaviour against it.
	cfg cloud.Config

	// replicaDB is the libSQL embedded-replica database handle set
	// by SetReplicaDB after OpenStoreWithCloud opens the store. It
	// is consulted by Push (to replay sync_log) and Pull (to force
	// replica catch-up). When nil the provider operates in
	// “connectionless” mode and Push/Pull return ErrUnreachable.
	replicaDB *sql.DB

	// activeOrg is cached from the most recent ValidateCredentials
	// or EnsureDatabase call so Status / List / Delete do not need
	// to re-derive it from an opaque token on every call.
	activeOrg string
}

// NewTursoProvider returns a fresh *TursoProvider with a default HTTP
// client suitable for the Turso Platform REST API.
//
// The returned provider is safe to use immediately for the Login flow
// (task 8.1); operations that depend on cfg or the embedded replica
// (EnsureDatabase, OpenStore, Push, Pull, Status) currently return
// errNotYetImplemented and are filled in by tasks 8.2 — 8.5.
//
// TLS hardening: per requirement 9.1 / threat model section "Encryption /
// In transit", every outbound REST call MUST negotiate TLS 1.3. We clone
// the default transport (so connection-pooling defaults are preserved)
// and pin its TLSClientConfig.MinVersion to tls.VersionTLS13. Any server
// that does not support TLS 1.3 — or any MITM that attempts a downgrade
// — fails the handshake before a single byte of credentials leaves the
// process.
func NewTursoProvider() *TursoProvider {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS13,
	}
	return &TursoProvider{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// Name returns the provider identifier used by the cloud registry.
//
// Per requirement 11.2 the literal value MUST be "turso"; downstream
// code (config validation, audit logging, CLI status output) compares
// against this string verbatim, so changing it is a breaking change
// for users with existing `cfg.CloudMemory.Provider: turso` configs.
func (p *TursoProvider) Name() string {
	return "turso"
}

// init registers the Turso provider with the cloud registry under the
// name "turso" (task 8.6, requirements 11.2 and 11.3).
//
// The factory closure intentionally calls NewTursoProvider lazily —
// cloud.Get invokes it only when a caller actually resolves the
// provider, so importing this package for its side effect (the
// canonical pattern used by cmd/smara/main.go's blank import) does
// not pay the cost of building an *http.Client until the first
// `smara memory cloud …` invocation.
//
// Re-running init across test binaries is safe: cloud.Register
// overwrites any prior factory under the same name, which lets test
// helpers swap in a fake provider via cloud.Register("turso", …)
// without disabling production registration.
func init() {
	cloud.Register("turso", func() cloud.Provider {
		return NewTursoProvider()
	})
}

// ----------------------------------------------------------------------------
// Stubs for Provider methods delivered by later tasks (8.2 — 8.5).
// ----------------------------------------------------------------------------
//
// Each stub returns errNotYetImplemented (wrapped with a descriptive
// prefix so the surfaced error names the missing operation) and the
// appropriate zero value for any non-error return slot. This keeps the
// package compiling and lets the registry hand out *TursoProvider
// instances for tasks that only exercise Login() while remaining safe:
// any caller that exercises an unimplemented method receives an
// explicit error rather than a panic or a silent no-op.

// ----------------------------------------------------------------------------
// ValidateCredentials — cheap online token check (task 8.2).
// ----------------------------------------------------------------------------

// ValidateCredentials checks token validity by issuing a lightweight GET
// against the Turso Platform API. A 2xx response means the token is still
// valid; 401/403 means expired/revoked; any other response (or network
// error) is surfaced as a wrapped error.
//
// On success the provider caches the org ID from credentials so
// subsequent Status / List / Delete calls do not need to re-derive it.
//
// Requirements: 10.1, 10.2, 10.3, 11.4.
func (p *TursoProvider) ValidateCredentials(ctx context.Context, creds *cloud.Credentials) error {
	if creds == nil {
		return errors.New("turso: ValidateCredentials: nil credentials")
	}
	if creds.Token == "" {
		return errors.New("turso: ValidateCredentials: empty token")
	}
	if creds.OrgID == "" {
		return errors.New("turso: ValidateCredentials: empty org_id")
	}

	endpoint := tursoAPIBaseURL + "/v1/organizations/" + creds.OrgID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("turso: ValidateCredentials: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+creds.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("turso: ValidateCredentials: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		// Cache the org for subsequent Status / List / Delete calls.
		p.activeOrg = creds.OrgID
		return nil
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("turso: ValidateCredentials: token is invalid or expired (HTTP %d)", resp.StatusCode)
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyMaxLen))
		return fmt.Errorf("turso: ValidateCredentials: unexpected HTTP %d: %s",
			resp.StatusCode, truncateForError(string(body)))
	}
}

// ----------------------------------------------------------------------------
// Push / Pull / Status — implemented in sync.go (task 8.4)
// ----------------------------------------------------------------------------
// Push, Pull, and Status are implemented in sync.go. The stub bodies
// below have been removed; the real implementations interact with the
// replica DB (via p.replicaDB) for sync_log replay and frame accounting,
// and with the Turso REST API for quota/usage queries.

// ----------------------------------------------------------------------------
// ListWorkspaceDatabases / DeleteWorkspaceDatabase — implemented in
// database.go (task 8.5)
// ----------------------------------------------------------------------------

// Close releases provider-side resources. The current skeleton only
// holds an *http.Client (which has no explicit Close) and a value-type
// config snapshot, so the implementation is trivially nil. Later tasks
// that hold replica handles or background goroutines must extend this
// to drain them; the Provider contract guarantees Close MUST NOT
// delete data, so the default of "release transient resources" is
// always correct.
func (p *TursoProvider) Close() error {
	return nil
}

// SetReplicaDB wires the libSQL embedded-replica database handle to
// the provider so Push, Pull, and Status can interact with sync_log
// and the replica's SQL-level sync primitives. It is called from
// OpenStoreWithCloud after the store is constructed (task 9.1 / 8.4
// integration).
//
// SetReplicaDB is idempotent and safe to call on a nil receiver. It
// is intentionally NOT part of the cloud.Provider interface — only
// providers that need direct DB access (like Turso's sync_log replay)
// expose it.
func (p *TursoProvider) SetReplicaDB(db *sql.DB) {
	if p == nil {
		return
	}
	p.replicaDB = db
}

// WithConfig applies the cloud.Config snapshot that scopes provider
// behaviour (DB name pattern, sync interval, conflict policy, ...).
// It is called from OpenStoreWithCloud before EnsureDatabase so the
// config is available for pattern substitution and sync interval
// wiring. Idempotent; safe to call on nil receiver.
func (p *TursoProvider) WithConfig(cfg cloud.Config) {
	if p == nil {
		return
	}
	p.cfg = cfg
}
