// Package supabase — OpenStore: build a plain SQLite DSN.
//
// Unlike Turso (which returns a libSQL embedded-replica DSN with
// syncURL/authToken), Supabase uses a local SQLite database with
// manual sync via Push/Pull. OpenStore returns a DSN that routes
// to the local `sqlite` driver via modernc.org/sqlite.
//
// DSN shape:
//
//	<localPath>?_journal_mode=WAL&_busy_timeout=5000
//
// This matches the historical NewSQLiteStore DSN format, so
// detectDialect in store.go routes it to the `sqlite` driver
// (not libsql).
//
// Requirements: 6.1, 6.5, 17.5 (local-only store compatibility).
package supabase

import (
	"context"
	"errors"
	"fmt"

	cloud "github.com/gede-cahya/Smara-CLI/internal/memory/cloud"
)

// OpenStore returns a plain SQLite DSN for the local replica.
//
// The returned DSN uses the `sqlite` driver (modernc.org/sqlite),
// NOT libsql. Sync is handled manually via Push/Pull using the
// Supabase REST API — there is no automatic WAL replication.
//
// Arguments:
//   - info: DatabaseInfo from EnsureDatabase (used only for
//     validation; the URL/token are not embedded in the DSN).
//   - localPath: path to the local SQLite file.
func (p *SupabaseProvider) OpenStore(_ context.Context, info *cloud.DatabaseInfo, localPath string) (string, error) {
	if info == nil {
		return "", errors.New("supabase: OpenStore: nil DatabaseInfo")
	}
	if localPath == "" {
		return "", errors.New("supabase: OpenStore: empty localPath")
	}

	// Plain SQLite DSN — no libSQL markers. This routes to the
	// `sqlite` driver (modernc.org/sqlite) via detectDialect.
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000", localPath)
	return dsn, nil
}
