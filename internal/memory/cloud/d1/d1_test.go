// Package d1 — unit tests.
//
// Tests cover:
//   - Provider registration in cloud registry.
//   - Name() returns "d1".
//   - OpenStore DSN construction (no network needed).
//   - DSN pattern does NOT route to libSQL driver.
//   - Login headless mode error paths.
//   - Idempotency: Close, SetReplicaDB, WithConfig.
//   - ValidateCredentials error paths.
//   - Push/Pull without replicaDB.
package d1

import (
	"os"
	"strings"
	"testing"

	cloud "github.com/gede-cahya/Smara-CLI/internal/memory/cloud"
)

// ---------------------------------------------------------------------------
// Registry tests
// ---------------------------------------------------------------------------

func TestProviderRegistered(t *testing.T) {
	p, err := cloud.Get("d1")
	if err != nil {
		t.Fatalf("cloud.Get(\"d1\") returned error: %v", err)
	}
	if p == nil {
		t.Fatal("cloud.Get(\"d1\") returned nil provider")
	}
	if p.Name() != "d1" {
		t.Fatalf("provider.Name() = %q, want \"d1\"", p.Name())
	}
}

func TestD1InProviderList(t *testing.T) {
	names := cloud.List()
	found := false
	for _, n := range names {
		if n == "d1" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("cloud.List() does not contain \"d1\": %v", names)
	}
}

// ---------------------------------------------------------------------------
// OpenStore tests (no network)
// ---------------------------------------------------------------------------

func TestOpenStoreNilInfo(t *testing.T) {
	p := NewD1Provider()
	_, err := p.OpenStore(nil, nil, "/tmp/test.db")
	if err == nil {
		t.Fatal("expected error for nil DatabaseInfo")
	}
	if !strings.Contains(err.Error(), "nil DatabaseInfo") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOpenStoreEmptyPath(t *testing.T) {
	p := NewD1Provider()
	info := &cloud.DatabaseInfo{Provider: "d1", Name: "test"}
	_, err := p.OpenStore(nil, info, "")
	if err == nil {
		t.Fatal("expected error for empty localPath")
	}
	if !strings.Contains(err.Error(), "empty localPath") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOpenStoreReturnsPlainSQLiteDSN(t *testing.T) {
	p := NewD1Provider()
	info := &cloud.DatabaseInfo{
		Provider:  "d1",
		Name:      "test-db",
		URL:       "https://api.cloudflare.com/client/v4/accounts/abc123/d1/database/xyz789",
		AuthToken: "test-token",
	}
	dsn, err := p.OpenStore(nil, info, "/tmp/test.db")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}

	// Must NOT contain libSQL markers.
	if strings.Contains(dsn, "libsql://") {
		t.Error("DSN contains libsql:// marker — should be plain SQLite")
	}
	if strings.Contains(dsn, "authToken=") {
		t.Error("DSN contains authToken= — should be plain SQLite")
	}
	if strings.Contains(dsn, "syncUrl=") || strings.Contains(dsn, "syncURL=") {
		t.Error("DSN contains syncUrl= — should be plain SQLite")
	}

	// Must contain SQLite WAL markers.
	if !strings.Contains(dsn, "_journal_mode=WAL") {
		t.Error("DSN missing _journal_mode=WAL")
	}
	if !strings.Contains(dsn, "_busy_timeout=5000") {
		t.Error("DSN missing _busy_timeout=5000")
	}
	if !strings.HasPrefix(dsn, "/tmp/test.db?") {
		t.Errorf("DSN does not start with localPath: %s", dsn)
	}

	// Verify auth cache was populated.
	if p.apiToken != "test-token" {
		t.Errorf("apiToken not cached: got %q, want \"test-token\"", p.apiToken)
	}
	if p.databaseID != "xyz789" {
		t.Errorf("databaseID not cached from URL: got %q, want \"xyz789\"", p.databaseID)
	}
}

// ---------------------------------------------------------------------------
// Headless login tests
// ---------------------------------------------------------------------------

func TestLoginHeadlessMissingToken(t *testing.T) {
	os.Unsetenv("SMARA_CLOUD_TOKEN")
	os.Unsetenv("SMARA_CLOUD_ORG")

	p := NewD1Provider()
	_, err := p.Login(nil, cloud.LoginOptions{Headless: true})
	if err == nil {
		t.Fatal("expected error for missing SMARA_CLOUD_TOKEN")
	}
	if !strings.Contains(err.Error(), "SMARA_CLOUD_TOKEN") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoginHeadlessMissingOrg(t *testing.T) {
	os.Setenv("SMARA_CLOUD_TOKEN", "test-token")
	os.Unsetenv("SMARA_CLOUD_ORG")
	defer os.Unsetenv("SMARA_CLOUD_TOKEN")

	p := NewD1Provider()
	_, err := p.Login(nil, cloud.LoginOptions{Headless: true})
	if err == nil {
		t.Fatal("expected error for missing SMARA_CLOUD_ORG")
	}
	if !strings.Contains(err.Error(), "SMARA_CLOUD_ORG") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoginHeadlessSuccess(t *testing.T) {
	os.Setenv("SMARA_CLOUD_TOKEN", "test-api-token-abc")
	os.Setenv("SMARA_CLOUD_ORG", "abc123def456")
	defer os.Unsetenv("SMARA_CLOUD_TOKEN")
	defer os.Unsetenv("SMARA_CLOUD_ORG")

	p := NewD1Provider()
	creds, err := p.Login(nil, cloud.LoginOptions{Headless: true})
	if err != nil {
		t.Fatalf("Login headless: %v", err)
	}
	if creds.Provider != "d1" {
		t.Errorf("creds.Provider = %q, want \"d1\"", creds.Provider)
	}
	if creds.Token != "test-api-token-abc" {
		t.Errorf("creds.Token = %q, want \"test-api-token-abc\"", creds.Token)
	}
	if creds.OrgID != "abc123def456" {
		t.Errorf("creds.OrgID = %q, want \"abc123def456\"", creds.OrgID)
	}
}

// ---------------------------------------------------------------------------
// Close / SetReplicaDB / WithConfig — idempotency tests
// ---------------------------------------------------------------------------

func TestCloseIdempotent(t *testing.T) {
	p := NewD1Provider()
	if err := p.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestSetReplicaDBNilReceiver(t *testing.T) {
	var p *D1Provider
	p.SetReplicaDB(nil) // must not panic
}

func TestWithConfigNilReceiver(t *testing.T) {
	var p *D1Provider
	p.WithConfig(cloud.Config{Provider: "d1"}) // must not panic
}

// ---------------------------------------------------------------------------
// ValidateCredentials — error paths
// ---------------------------------------------------------------------------

func TestValidateCredentialsNilCreds(t *testing.T) {
	p := NewD1Provider()
	err := p.ValidateCredentials(nil, nil)
	if err == nil {
		t.Fatal("expected error for nil credentials")
	}
	if !strings.Contains(err.Error(), "nil credentials") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateCredentialsEmptyToken(t *testing.T) {
	p := NewD1Provider()
	err := p.ValidateCredentials(nil, &cloud.Credentials{Provider: "d1"})
	if err == nil {
		t.Fatal("expected error for empty token")
	}
	if !strings.Contains(err.Error(), "empty token") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateCredentialsEmptyAccountID(t *testing.T) {
	p := NewD1Provider()
	err := p.ValidateCredentials(nil, &cloud.Credentials{Provider: "d1", Token: "token"})
	if err == nil {
		t.Fatal("expected error for empty account ID")
	}
	if !strings.Contains(err.Error(), "account ID") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Push/Pull without replicaDB
// ---------------------------------------------------------------------------

func TestPushWithoutReplica(t *testing.T) {
	p := NewD1Provider()
	report, err := p.Push(nil)
	if err != cloud.ErrUnreachable {
		t.Errorf("Push without replicaDB: got %v, want cloud.ErrUnreachable", err)
	}
	if report == nil {
		t.Fatal("Push returned nil report")
	}
}

func TestPullWithoutReplica(t *testing.T) {
	p := NewD1Provider()
	report, err := p.Pull(nil)
	if err != cloud.ErrUnreachable {
		t.Errorf("Pull without replicaDB: got %v, want cloud.ErrUnreachable", err)
	}
	if report == nil {
		t.Fatal("Pull returned nil report")
	}
}

// ---------------------------------------------------------------------------
// lastSegment tests
// ---------------------------------------------------------------------------

func TestLastSegment(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://api.cloudflare.com/client/v4/accounts/abc/d1/database/xyz", "xyz"},
		{"https://api.cloudflare.com/client/v4/accounts/abc/d1/database/xyz/", "xyz"},
		{"xyz", "xyz"},
		{"", ""},
	}
	for _, tt := range tests {
		got := lastSegment(tt.url)
		if got != tt.want {
			t.Errorf("lastSegment(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// escapeSQL tests
// ---------------------------------------------------------------------------

func TestEscapeSQL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"it's", "it''s"},
		{"a'b'c", "a''b''c"},
		{"", ""},
	}
	for _, tt := range tests {
		got := escapeSQL(tt.input)
		if got != tt.want {
			t.Errorf("escapeSQL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
