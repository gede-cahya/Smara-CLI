// Package d1 — OpenStore: build a plain SQLite DSN.
//
// Like Supabase, D1 uses a local SQLite database with manual sync
// via Push/Pull against the Cloudflare REST API. There is no automatic
// libSQL WAL replication.
//
// OpenStore returns a DSN that routes to the local `sqlite` driver
// (modernc.org/sqlite), NOT libsql.
//
// DSN shape:
//
//	<localPath>?_journal_mode=WAL&_busy_timeout=5000
//
// This matches the historical NewSQLiteStore DSN format, so
// detectDialect in store.go routes it to the `sqlite` driver.
//
// Requirements: 6.1, 6.5, 17.5.
package d1

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
// Cloudflare D1 REST API — there is no automatic WAL replication.
//
// Arguments:
//   - info: DatabaseInfo from EnsureDatabase (used only for
//     validation; the URL/token are not embedded in the DSN).
//   - localPath: path to the local SQLite file.
func (p *D1Provider) OpenStore(_ context.Context, info *cloud.DatabaseInfo, localPath string) (string, error) {
	if info == nil {
		return "", errors.New("d1: OpenStore: nil DatabaseInfo")
	}
	if localPath == "" {
		return "", errors.New("d1: OpenStore: empty localPath")
	}

	// Cache the database ID and auth from info for later Push/Pull/Status.
	if info.AuthToken != "" {
		p.apiToken = info.AuthToken
	}
	if p.databaseID == "" {
		// Extract database ID from URL.
		// URL format: https://api.cloudflare.com/client/v4/accounts/{id}/d1/database/{dbid}
		if idx := lastSegment(info.URL); idx != "" {
			p.databaseID = idx
		}
	}

	// Plain SQLite DSN — no libSQL markers. This routes to the
	// `sqlite` driver (modernc.org/sqlite) via detectDialect.
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000", localPath)
	return dsn, nil
}

// lastSegment extracts the last path segment from a URL.
func lastSegment(url string) string {
	if url == "" {
		return ""
	}
	// Strip trailing slash.
	if url[len(url)-1] == '/' {
		url = url[:len(url)-1]
	}
	for i := len(url) - 1; i >= 0; i-- {
		if url[i] == '/' {
			return url[i+1:]
		}
	}
	return url
}
