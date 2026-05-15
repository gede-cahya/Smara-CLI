// Package turso — tests for OpenStore (task 8.3).
package turso

import (
	"context"
	"errors"
	"strings"
	"testing"

	cloud "github.com/gede-cahya/Smara-CLI/internal/memory/cloud"
)

// TestOpenStore_BuildsLibSQLEmbeddedReplicaDSN exercises the happy path:
// a well-formed DatabaseInfo with a libsql:// URL produces a DSN that
// (a) starts with the file: prefix the libSQL driver expects,
// (b) carries the syncURL, authToken, and syncInterval params, and
// (c) URL-encodes the remote URL so its scheme separator survives the
// trip through the query string.
func TestOpenStore_BuildsLibSQLEmbeddedReplicaDSN(t *testing.T) {
	p := NewTursoProvider()
	p.cfg = cloud.Config{SyncIntervalSec: 45}

	dsn, err := p.OpenStore(context.Background(), &cloud.DatabaseInfo{
		URL:       "libsql://smara-default-acme.turso.io",
		AuthToken: "tok-abc.def.ghi",
	}, "/tmp/replica.db")
	if err != nil {
		t.Fatalf("OpenStore: unexpected error: %v", err)
	}

	if !strings.HasPrefix(dsn, "file:/tmp/replica.db?") {
		t.Errorf("dsn missing file:<localPath>? prefix: %q", dsn)
	}
	if !strings.Contains(dsn, "syncURL=libsql%3A%2F%2Fsmara-default-acme.turso.io") {
		t.Errorf("dsn missing URL-encoded syncURL: %q", dsn)
	}
	if !strings.Contains(dsn, "authToken=tok-abc.def.ghi") {
		t.Errorf("dsn missing authToken: %q", dsn)
	}
	if !strings.Contains(dsn, "syncInterval=45") {
		t.Errorf("dsn missing syncInterval=45: %q", dsn)
	}
}

// TestOpenStore_DefaultSyncIntervalWhenUnset confirms the fallback to
// 30s when cfg.SyncIntervalSec is zero (the zero value of an unwired
// Config). This pins the spec-mandated default at the boundary.
func TestOpenStore_DefaultSyncIntervalWhenUnset(t *testing.T) {
	p := NewTursoProvider() // cfg left zero-valued
	dsn, err := p.OpenStore(context.Background(), &cloud.DatabaseInfo{
		URL:       "https://example.turso.io",
		AuthToken: "t",
	}, "/tmp/x.db")
	if err != nil {
		t.Fatalf("OpenStore: unexpected error: %v", err)
	}
	if !strings.Contains(dsn, "syncInterval=30") {
		t.Errorf("expected default syncInterval=30 in DSN, got %q", dsn)
	}
}

// TestOpenStore_RejectsNonTLSScheme covers the requirement-7.5 guard:
// http://, tcp://, and friends MUST be rejected so a misconfigured
// provider cannot accidentally downgrade to plaintext.
func TestOpenStore_RejectsNonTLSScheme(t *testing.T) {
	cases := []string{
		"http://insecure.turso.io",
		"tcp://insecure.turso.io",
		"ftp://insecure.turso.io",
		"file:/etc/passwd",
	}
	p := NewTursoProvider()
	for _, u := range cases {
		_, err := p.OpenStore(context.Background(), &cloud.DatabaseInfo{
			URL:       u,
			AuthToken: "t",
		}, "/tmp/x.db")
		if err == nil {
			t.Errorf("OpenStore(%q): expected scheme-rejection error, got nil", u)
			continue
		}
		if !errors.Is(err, errInvalidScheme) {
			t.Errorf("OpenStore(%q): expected errInvalidScheme, got %v", u, err)
		}
	}
}

// TestOpenStore_AcceptsBothTLSSchemes confirms that the two
// requirement-7.5 schemes (libsql://, https://) are both accepted,
// case-insensitively.
func TestOpenStore_AcceptsBothTLSSchemes(t *testing.T) {
	cases := []string{
		"libsql://x.turso.io",
		"LIBSQL://x.turso.io",
		"https://x.turso.io",
		"HTTPS://x.turso.io",
	}
	p := NewTursoProvider()
	for _, u := range cases {
		_, err := p.OpenStore(context.Background(), &cloud.DatabaseInfo{
			URL:       u,
			AuthToken: "t",
		}, "/tmp/x.db")
		if err != nil {
			t.Errorf("OpenStore(%q): unexpected error: %v", u, err)
		}
	}
}

// TestOpenStore_ArgumentValidation pins the explicit error surface for
// the three named failure modes (nil info, empty path, empty URL) so a
// future caller refactor does not accidentally degrade the message.
func TestOpenStore_ArgumentValidation(t *testing.T) {
	p := NewTursoProvider()
	ctx := context.Background()

	if _, err := p.OpenStore(ctx, nil, "/tmp/x.db"); err == nil {
		t.Error("nil info: expected error, got nil")
	}
	if _, err := p.OpenStore(ctx, &cloud.DatabaseInfo{URL: "libsql://x"}, ""); err == nil {
		t.Error("empty localPath: expected error, got nil")
	}
	if _, err := p.OpenStore(ctx, &cloud.DatabaseInfo{URL: ""}, "/tmp/x.db"); err == nil {
		t.Error("empty URL: expected error, got nil")
	}
}
