// Package turso — OpenStore: build the libSQL embedded-replica DSN
// (task 8.3).
//
// This file delivers the real implementation of TursoProvider.OpenStore
// (the stub from turso.go has been removed). Given the *DatabaseInfo
// returned by EnsureDatabase plus a local file path, OpenStore returns
// the dialect-aware DSN that internal/memory.NewSQLiteStoreWithDSN
// routes to the libSQL driver registered by store_libsql.go.
//
// DSN shape (matches libsql-client-go contract):
//
//	file:<localPath>?syncURL=<remoteURL>&authToken=<token>&syncInterval=<sec>
//
// Components:
//
//   - `file:` prefix tells libsql-client-go this is an embedded replica
//     anchored to a local SQLite file.
//   - `<localPath>` is taken verbatim from the caller; any URL-special
//     characters are NOT escaped because the path lives in the URL's
//     opaque/path slot, not the query string. The Smara CLI controls
//     this path (always under `~/.smara/...`) so we do not need to
//     defend against attacker-supplied weird filenames here.
//   - `syncURL` carries the remote primary URL (e.g. libsql://...turso.io).
//     URL-encoded via url.QueryEscape so a future provider that returns a
//     URL with reserved characters cannot break parsing.
//   - `authToken` is the per-database token from EnsureDatabase. URL-
//     encoded for the same reason; tokens are JWT-shaped (`a.b.c`) and
//     do not need escaping in practice, but encoding keeps the contract
//     robust against future token formats.
//   - `syncInterval` is taken from p.cfg.SyncIntervalSec; if that is
//     <= 0 we fall back to the spec-default of 30 seconds. Note that
//     the libSQL driver's syncInterval governs how often it pushes WAL
//     frames upstream; the Smara SyncManager also runs a higher-level
//     ticker on top, so this value is essentially the floor of the
//     sync cadence.
//
// Scheme enforcement (Requirement 7.5): info.URL MUST use either the
// `libsql://` scheme (the canonical Turso primary URL) or the `https://`
// scheme (some Turso deployments expose HTTPS endpoints; the libSQL
// driver promotes them to TLS internally). Anything else — `http://`,
// `tcp://`, an empty string, a path-only URL — is rejected with a
// typed error so callers cannot accidentally downgrade to plaintext.
//
// Requirements covered:
//
//   - 6.1 / 6.5 (Local replica opens against the same Provider URL the
//     remote was provisioned with; schema parity is then guaranteed by
//     NewSQLiteStoreWithDSN running migrate() at the end of construction.)
//   - 7.5 (TLS-only enforcement; reject `http://`, `tcp://`, etc.).
package turso

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	cloud "github.com/gede-cahya/Smara-CLI/internal/memory/cloud"
)

// defaultSyncIntervalSec is the spec-mandated fallback when the active
// cloud.Config does not specify a sync interval (or specifies a non-
// positive one). Matches the default in internal/config.DefaultConfig
// and design.md Section "Model 1 / Validation Rules".
const defaultSyncIntervalSec = 30

// errInvalidScheme is the sentinel returned when info.URL uses a
// non-TLS scheme. It is wrapped (not returned bare) so callers see the
// offending URL in the error message while still being able to test
// with errors.Is for downstream control flow.
var errInvalidScheme = errors.New("turso: OpenStore: remote URL must use libsql:// or https:// scheme (TLS-only)")

// OpenStore builds the libSQL embedded-replica DSN for the remote
// database described by info, with the local replica file at localPath.
//
// The returned DSN is suitable for direct use with
// memory.NewSQLiteStoreWithDSN; that constructor's detectDialect helper
// recognises the `syncURL=`/`authToken=` markers and routes to the
// libSQL driver registered by store_libsql.go.
//
// OpenStore does NOT open the database itself — it only constructs the
// DSN. The caller (typically OpenStoreWithCloud in store_cloud.go) is
// responsible for invoking NewSQLiteStoreWithDSN and running migrate()
// against the resulting handle. This separation keeps the Provider
// interface free of database/sql dependencies and lets the SyncManager
// reuse the same store handle the CLI commands talk to.
//
// Errors:
//
//   - info==nil → "turso: OpenStore: nil DatabaseInfo"
//   - localPath=="" → "turso: OpenStore: empty localPath"
//   - info.URL=="" → "turso: OpenStore: empty remote URL"
//   - info.URL is malformed → wraps url.Parse's error
//   - info.URL.Scheme is neither libsql nor https → wraps errInvalidScheme
//
// AuthToken is allowed to be empty — some Turso deployments accept
// anonymous read-only access — and we simply emit `authToken=` with no
// value rather than rejecting the call.
func (p *TursoProvider) OpenStore(_ context.Context, info *cloud.DatabaseInfo, localPath string) (string, error) {
	// ------------------------------------------------------------------
	// Argument validation. We surface concrete, named errors here so a
	// future caller wiring this against a fake provider receives a
	// descriptive message instead of a generic "invalid argument".
	// ------------------------------------------------------------------
	if info == nil {
		return "", errors.New("turso: OpenStore: nil DatabaseInfo")
	}
	if localPath == "" {
		return "", errors.New("turso: OpenStore: empty localPath")
	}
	if info.URL == "" {
		return "", errors.New("turso: OpenStore: empty remote URL")
	}

	// ------------------------------------------------------------------
	// TLS-only enforcement (Requirement 7.5).
	//
	// We use net/url to parse the scheme rather than a HasPrefix string
	// check so a URL like "LIBSQL://..." (legal per RFC 3986 §3.1, scheme
	// is case-insensitive) is correctly normalized before comparison.
	// ------------------------------------------------------------------
	parsed, err := url.Parse(info.URL)
	if err != nil {
		return "", fmt.Errorf("turso: OpenStore: parse remote URL %q: %w", info.URL, err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "libsql" && scheme != "https" {
		return "", fmt.Errorf("%w (got scheme %q in URL %q)", errInvalidScheme, parsed.Scheme, info.URL)
	}

	// ------------------------------------------------------------------
	// Sync interval resolution.
	//
	// p.cfg.SyncIntervalSec may be zero (the default zero value) when
	// OpenStore is invoked before WithConfig wiring lands; fall back to
	// the spec default (30s) so the libSQL driver still has a sensible
	// cadence. Negative values are not legal per Config.Validate, but we
	// defend against them here too — Validate may not have run yet on
	// this codepath.
	// ------------------------------------------------------------------
	syncIntervalSec := p.cfg.SyncIntervalSec
	if syncIntervalSec <= 0 {
		syncIntervalSec = defaultSyncIntervalSec
	}

	// ------------------------------------------------------------------
	// DSN assembly.
	//
	// We build the query string by hand (rather than via url.Values +
	// Encode()) so the parameter ORDER is deterministic. Determinism is
	// important for two reasons:
	//
	//   1. NewSQLiteStoreWithDSN's detectDialect helper does substring
	//      matches on the DSN; while it is order-insensitive today,
	//      pinning the order keeps the surface predictable for future
	//      tests that assert on the literal DSN.
	//
	//   2. The DSN is embedded into the SQLiteStore.dbPath field and
	//      consequently appears in error messages. A stable shape makes
	//      log diffing across runs straightforward.
	//
	// url.QueryEscape handles the only risky characters (the colons and
	// slashes in the syncURL value, plus any reserved chars in a future
	// auth token format).
	// ------------------------------------------------------------------
	dsn := fmt.Sprintf(
		"file:%s?syncURL=%s&authToken=%s&syncInterval=%d",
		localPath,
		url.QueryEscape(info.URL),
		url.QueryEscape(info.AuthToken),
		syncIntervalSec,
	)
	return dsn, nil
}
