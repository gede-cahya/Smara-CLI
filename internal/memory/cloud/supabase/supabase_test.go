// Package supabase — unit tests.
//
// Tests cover:
//   - Provider registration in cloud registry.
//   - Name() returns "supabase".
//   - OpenStore DSN construction (no network needed).
//   - DSN pattern does NOT route to libSQL driver.
//   - Login headless mode error paths.
package supabase

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
	p, err := cloud.Get("supabase")
	if err != nil {
		t.Fatalf("cloud.Get(\"supabase\") returned error: %v", err)
	}
	if p == nil {
		t.Fatal("cloud.Get(\"supabase\") returned nil provider")
	}
	if p.Name() != "supabase" {
		t.Fatalf("provider.Name() = %q, want \"supabase\"", p.Name())
	}
}

func TestSupabaseInProviderList(t *testing.T) {
	names := cloud.List()
	found := false
	for _, n := range names {
		if n == "supabase" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("cloud.List() does not contain \"supabase\": %v", names)
	}
}

// ---------------------------------------------------------------------------
// OpenStore tests (no network)
// ---------------------------------------------------------------------------

func TestOpenStoreNilInfo(t *testing.T) {
	p := NewSupabaseProvider()
	_, err := p.OpenStore(nil, nil, "/tmp/test.db")
	if err == nil {
		t.Fatal("expected error for nil DatabaseInfo")
	}
	if !strings.Contains(err.Error(), "nil DatabaseInfo") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOpenStoreEmptyPath(t *testing.T) {
	p := NewSupabaseProvider()
	info := &cloud.DatabaseInfo{Provider: "supabase", Name: "test"}
	_, err := p.OpenStore(nil, info, "")
	if err == nil {
		t.Fatal("expected error for empty localPath")
	}
	if !strings.Contains(err.Error(), "empty localPath") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOpenStoreReturnsPlainSQLiteDSN(t *testing.T) {
	p := NewSupabaseProvider()
	info := &cloud.DatabaseInfo{Provider: "supabase", Name: "test-db", URL: "https://abc.supabase.co"}
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
}

// ---------------------------------------------------------------------------
// Headless login tests
// ---------------------------------------------------------------------------

func TestLoginHeadlessMissingToken(t *testing.T) {
	// Ensure env vars are not set.
	os.Unsetenv("SMARA_CLOUD_TOKEN")
	os.Unsetenv("SMARA_CLOUD_ORG")

	p := NewSupabaseProvider()
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

	p := NewSupabaseProvider()
	_, err := p.Login(nil, cloud.LoginOptions{Headless: true})
	if err == nil {
		t.Fatal("expected error for missing SMARA_CLOUD_ORG")
	}
	if !strings.Contains(err.Error(), "SMARA_CLOUD_ORG") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoginHeadlessSuccess(t *testing.T) {
	os.Setenv("SMARA_CLOUD_TOKEN", "test-token-123")
	os.Setenv("SMARA_CLOUD_ORG", "testprojectref")
	defer os.Unsetenv("SMARA_CLOUD_TOKEN")
	defer os.Unsetenv("SMARA_CLOUD_ORG")

	p := NewSupabaseProvider()
	creds, err := p.Login(nil, cloud.LoginOptions{Headless: true})
	if err != nil {
		t.Fatalf("Login headless: %v", err)
	}
	if creds.Provider != "supabase" {
		t.Errorf("creds.Provider = %q, want \"supabase\"", creds.Provider)
	}
	if creds.Token != "test-token-123" {
		t.Errorf("creds.Token = %q, want \"test-token-123\"", creds.Token)
	}
	if creds.OrgID != "testprojectref" {
		t.Errorf("creds.OrgID = %q, want \"testprojectref\"", creds.OrgID)
	}
}

// ---------------------------------------------------------------------------
// Close / SetReplicaDB / WithConfig — idempotency tests
// ---------------------------------------------------------------------------

func TestCloseIdempotent(t *testing.T) {
	p := NewSupabaseProvider()
	if err := p.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestSetReplicaDBNilReceiver(t *testing.T) {
	var p *SupabaseProvider
	p.SetReplicaDB(nil) // must not panic
}

func TestWithConfigNilReceiver(t *testing.T) {
	var p *SupabaseProvider
	p.WithConfig(cloud.Config{Provider: "supabase"}) // must not panic
}

// ---------------------------------------------------------------------------
// ValidateCredentials — error paths
// ---------------------------------------------------------------------------

func TestValidateCredentialsNilCreds(t *testing.T) {
	p := NewSupabaseProvider()
	err := p.ValidateCredentials(nil, nil)
	if err == nil {
		t.Fatal("expected error for nil credentials")
	}
	if !strings.Contains(err.Error(), "nil credentials") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateCredentialsEmptyToken(t *testing.T) {
	p := NewSupabaseProvider()
	err := p.ValidateCredentials(nil, &cloud.Credentials{Provider: "supabase"})
	if err == nil {
		t.Fatal("expected error for empty token")
	}
	if !strings.Contains(err.Error(), "empty token") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Push/Pull without replicaDB
// ---------------------------------------------------------------------------

func TestPushWithoutReplica(t *testing.T) {
	p := NewSupabaseProvider()
	report, err := p.Push(nil)
	if err != cloud.ErrUnreachable {
		t.Errorf("Push without replicaDB: got %v, want cloud.ErrUnreachable", err)
	}
	if report == nil {
		t.Fatal("Push returned nil report")
	}
}

func TestPullWithoutReplica(t *testing.T) {
	p := NewSupabaseProvider()
	report, err := p.Pull(nil)
	if err != cloud.ErrUnreachable {
		t.Errorf("Pull without replicaDB: got %v, want cloud.ErrUnreachable", err)
	}
	if report == nil {
		t.Fatal("Pull returned nil report")
	}
}
