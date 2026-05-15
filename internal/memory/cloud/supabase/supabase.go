// Package supabase implements the cloud.Provider interface backed by
// Supabase PostgreSQL + REST API.
//
// Unlike Turso (which uses libSQL embedded replicas), Supabase uses a
// local SQLite store that syncs with the remote PostgreSQL database via
// the Supabase REST API. Push uploads pending local rows; Pull fetches
// remote changes and merges them into the local store.
//
// File layout (mirroring the Turso provider structure):
//
//	supabase.go  — SupabaseProvider struct, constructor, Name, init(), stubs.
//	login.go     — Headless + interactive prompt-based auth flow.
//	database.go  — EnsureDatabase / List / Delete via Supabase REST.
//	openstore.go — OpenStore (returns plain SQLite DSN, no libSQL).
//	sync.go      — Push / Pull / Status via Supabase REST API.
//
// Registration: cloud.Register("supabase", ...) in init().
// The CLI triggers registration via blank-import in cmd/smara/main.go.
package supabase

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"net/http"
	"time"

	cloud "github.com/gede-cahya/Smara-CLI/internal/memory/cloud"
)

// Compile-time: *SupabaseProvider satisfies cloud.Provider.
var _ cloud.Provider = (*SupabaseProvider)(nil)

// SupabaseProvider implements cloud.Provider for Supabase.
//
// Fields:
//   - httpClient: pooled HTTP client for Supabase REST API calls.
//   - cfg: active cloud.Config snapshot (wired via WithConfig).
//   - replicaDB: local SQLite handle for Push/Pull/Status.
//   - restURL: cached Supabase REST endpoint (from login/credentials).
//   - serviceKey: cached service_role key for REST calls (set during
//     ValidateCredentials / EnsureDatabase so Push/Pull/Status can use it).
type SupabaseProvider struct {
	httpClient *http.Client
	cfg        cloud.Config
	replicaDB  *sql.DB
	restURL    string // https://<ref>.supabase.co/rest/v1
	serviceKey string // cached service_role key for Push/Pull/Status
}

// NewSupabaseProvider returns a fresh *SupabaseProvider with TLS 1.3
// enforcement on all outbound HTTP calls (mirroring Turso requirement).
func NewSupabaseProvider() *SupabaseProvider {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS13,
	}
	return &SupabaseProvider{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// Name returns the provider identifier for the cloud registry.
func (p *SupabaseProvider) Name() string {
	return "supabase"
}

// init registers the Supabase provider with the cloud registry.
func init() {
	cloud.Register("supabase", func() cloud.Provider {
		return NewSupabaseProvider()
	})
}

// ---------------------------------------------------------------------------
// Stubs / delegated methods — implemented in their respective files.
// ---------------------------------------------------------------------------

// ValidateCredentials checks token validity by issuing a lightweight GET
// against the Supabase REST API. Implemented in login.go.
func (p *SupabaseProvider) ValidateCredentials(ctx context.Context, creds *cloud.Credentials) error {
	if creds == nil {
		return errors.New("supabase: ValidateCredentials: nil credentials")
	}
	if creds.Token == "" {
		return errors.New("supabase: ValidateCredentials: empty token")
	}

	// Cache the service key for later Push/Pull/Status calls.
	p.serviceKey = creds.Token

	// Derive REST URL from credentials. OrgID = project ref.
	restURL := supabaseRESTURL(creds.OrgID)
	p.restURL = restURL

	// Do a cheap GET on the smara_memories table to verify the key works.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		restURL+"/smara_memories?limit=1", nil)
	if err != nil {
		return err
	}
	req.Header.Set("apikey", creds.Token)
	req.Header.Set("Authorization", "Bearer "+creds.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errors.New("supabase: ValidateCredentials: token is invalid or expired")
	}
	// 2xx or 404 (table not yet created) are both OK.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return nil
}

// Close releases provider-side resources.
func (p *SupabaseProvider) Close() error {
	return nil
}

// SetReplicaDB wires the local SQLite handle for Push/Pull/Status.
func (p *SupabaseProvider) SetReplicaDB(db *sql.DB) {
	if p == nil {
		return
	}
	p.replicaDB = db
}

// WithConfig applies the cloud.Config snapshot.
func (p *SupabaseProvider) WithConfig(cfg cloud.Config) {
	if p == nil {
		return
	}
	p.cfg = cfg
}

// supabaseRESTURL builds the REST API base URL from a project reference.
func supabaseRESTURL(projectRef string) string {
	if projectRef == "" {
		return ""
	}
	return "https://" + projectRef + ".supabase.co/rest/v1"
}
