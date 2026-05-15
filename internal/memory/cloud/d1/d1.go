// Package d1 implements the cloud.Provider interface backed by
// Cloudflare D1 (serverless SQLite) + REST API.
//
// D1 is Cloudflare's serverless SQLite offering. Unlike Turso (which
// uses libSQL embedded replicas with automatic WAL replication), D1
// sync is manual via the Cloudflare REST API:
//
//   - Push: INSERT/UPDATE rows on the remote D1 database via HTTP.
//   - Pull: SELECT rows from D1 and merge into local SQLite.
//   - Status: query D1 for row counts and compare with local state.
//
// The local store is a plain SQLite database (modernc.org/sqlite),
// NOT libSQL. Sync is performed explicitly via the D1 REST API.
//
// File layout:
//
//	d1.go       — D1Provider struct, constructor, Name, init(), Close.
//	login.go    — Headless + interactive prompt-based auth flow.
//	database.go — EnsureDatabase / List / Delete via Cloudflare REST.
//	openstore.go— OpenStore (returns plain SQLite DSN).
//	sync.go     — Push / Pull / Status via Cloudflare D1 REST API.
//
// Registration: cloud.Register("d1", ...) in init().
// The CLI triggers registration via blank-import in cmd/smara/main.go.
package d1

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"net/http"
	"time"

	cloud "github.com/gede-cahya/Smara-CLI/internal/memory/cloud"
)

// Compile-time: *D1Provider satisfies cloud.Provider.
var _ cloud.Provider = (*D1Provider)(nil)

// Cloudflare API base URL.
const cfAPIBase = "https://api.cloudflare.com/client/v4"

// D1Provider implements cloud.Provider for Cloudflare D1.
//
// Fields:
//   - httpClient: pooled HTTP client for Cloudflare REST API calls
//     with TLS 1.3 enforcement.
//   - cfg: active cloud.Config snapshot (wired via WithConfig).
//   - replicaDB: local SQLite handle for Push/Pull/Status.
//   - accountID: cached Cloudflare account ID (from login/credentials).
//   - databaseID: cached D1 database UUID (from EnsureDatabase).
//   - apiToken: cached API token for REST calls.
type D1Provider struct {
	httpClient *http.Client
	cfg        cloud.Config
	replicaDB  *sql.DB
	accountID  string // Cloudflare account ID
	databaseID string // D1 database UUID
	apiToken   string // cached API token for Push/Pull/Status
}

// NewD1Provider returns a fresh *D1Provider with TLS 1.3 enforcement
// on all outbound HTTP calls (mirroring Turso/Supabase requirement).
func NewD1Provider() *D1Provider {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS13,
	}
	return &D1Provider{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// Name returns the provider identifier for the cloud registry.
func (p *D1Provider) Name() string {
	return "d1"
}

// init registers the D1 provider with the cloud registry.
func init() {
	cloud.Register("d1", func() cloud.Provider {
		return NewD1Provider()
	})
}

// ---------------------------------------------------------------------------
// Stubs / delegated methods — implemented in their respective files.
// ---------------------------------------------------------------------------

// ValidateCredentials checks token validity by issuing a lightweight GET
// against the Cloudflare API. Implemented in login.go.
func (p *D1Provider) ValidateCredentials(ctx context.Context, creds *cloud.Credentials) error {
	if creds == nil {
		return errors.New("d1: ValidateCredentials: nil credentials")
	}
	if creds.Token == "" {
		return errors.New("d1: ValidateCredentials: empty token")
	}
	if creds.OrgID == "" {
		return errors.New("d1: ValidateCredentials: empty account ID")
	}

	// Cache for later calls.
	p.apiToken = creds.Token
	p.accountID = creds.OrgID

	// Verify the token works by listing D1 databases.
	// A 2xx response confirms validity; 401/403 means invalid/expired.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		cfAPIBase+"/accounts/"+p.accountID+"/d1/database?per_page=1", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errors.New("d1: ValidateCredentials: token is invalid or expired")
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return nil
}

// Close releases provider-side resources.
func (p *D1Provider) Close() error {
	return nil
}

// SetReplicaDB wires the local SQLite handle for Push/Pull/Status.
func (p *D1Provider) SetReplicaDB(db *sql.DB) {
	if p == nil {
		return
	}
	p.replicaDB = db
}

// WithConfig applies the cloud.Config snapshot.
func (p *D1Provider) WithConfig(cfg cloud.Config) {
	if p == nil {
		return
	}
	p.cfg = cfg
}
