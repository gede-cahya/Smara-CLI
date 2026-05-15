// Package d1 — Push / Pull / Status via Cloudflare D1 REST API.
//
// Unlike Turso (which uses libSQL embedded-replica WAL replication),
// D1 sync is manual via the Cloudflare REST API:
//
//   - Push: reads pending rows from local SQLite sync_log, INSERTs
//     them into the remote D1 database, and marks them synced.
//   - Pull: SELECTs rows from D1 with updated_at > last_sync_at,
//     merges them into the local SQLite store.
//   - Status: queries D1 for row counts and compares with local state.
//
// The smara_memories table schema on D1:
//
//	memory_id    INTEGER PRIMARY KEY
//	workspace_id INTEGER NOT NULL DEFAULT 0
//	cloud_id     TEXT UNIQUE NOT NULL
//	device_id    TEXT NOT NULL DEFAULT ''
//	content      TEXT NOT NULL DEFAULT ''
//	content_hash TEXT NOT NULL DEFAULT ''
//	tags         TEXT NOT NULL DEFAULT '[]'
//	source       TEXT NOT NULL DEFAULT ''
//	version      INTEGER NOT NULL DEFAULT 1
//	created_at   TEXT NOT NULL DEFAULT ''
//	updated_at   TEXT NOT NULL DEFAULT ''
//
// Requirements: 2.5, 7.5, 9.3.
package d1

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/audit"
	cloud "github.com/gede-cahya/Smara-CLI/internal/memory/cloud"
)

// ---------------------------------------------------------------------------
// Push — upload pending rows to D1.
// ---------------------------------------------------------------------------

// Push uploads pending local rows to Cloudflare D1 via the REST API.
//
// Flow:
//  1. Query local sync_log for rows with status='pending'.
//  2. For each pending memory, INSERT OR REPLACE into D1.
//  3. Mark the sync_log rows as status='synced'.
func (p *D1Provider) Push(ctx context.Context) (*cloud.SyncReport, error) {
	report := &cloud.SyncReport{StartedAt: time.Now()}
	defer func() { report.FinishedAt = time.Now() }()

	if p.replicaDB == nil {
		return report, cloud.ErrUnreachable
	}
	if p.databaseID == "" || p.apiToken == "" || p.accountID == "" {
		report.Errors = append(report.Errors, "d1: Push: not authenticated (missing database_id/api_token/account_id)")
		return report, fmt.Errorf("d1: Push: not authenticated")
	}

	// Fetch pending memories from local SQLite.
	pending, err := p.fetchPendingMemories(ctx)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report, err
	}
	if len(pending) == 0 {
		return report, nil
	}

	// Push each pending memory to D1.
	var pushed int
	var pushErrors []string
	for _, mem := range pending {
		if err := p.upsertRemote(ctx, mem); err != nil {
			pushErrors = append(pushErrors, fmt.Sprintf("memory %d: %v", mem.MemoryID, err))
			continue
		}
		pushed++
	}

	// Mark pushed rows as synced in local sync_log.
	if pushed > 0 {
		if err := p.markSynced(ctx, pending); err != nil {
			pushErrors = append(pushErrors, fmt.Sprintf("mark synced: %v", err))
		}
	}

	report.PushedRows = pushed
	report.Errors = pushErrors

	_ = audit.LogCloudOp("push", true, "d1", map[string]any{
		"pushed": pushed,
		"errors": len(pushErrors),
	})
	return report, nil
}

// pendingMemory represents a row ready to be pushed.
type pendingMemory struct {
	SyncLogID   int64
	MemoryID    int64
	WorkspaceID int64
	CloudID     string
	DeviceID    string
	Content     string
	ContentHash string
	Tags        string
	Source      string
	Version     int64
	CreatedAt   string
	UpdatedAt   string
}

// fetchPendingMemories reads sync_log JOIN memories for pending rows.
func (p *D1Provider) fetchPendingMemories(ctx context.Context) ([]pendingMemory, error) {
	rows, err := p.replicaDB.QueryContext(ctx, `
		SELECT sl.id, sl.memory_id,
		       COALESCE(m.workspace_id, 0),
		       COALESCE(m.cloud_id, ''),
		       COALESCE(m.device_id, ''),
		       m.content,
		       COALESCE(m.content_hash, ''),
		       COALESCE(m.tags, '[]'),
		       COALESCE(m.source, ''),
		       COALESCE(m.version, 1),
		       COALESCE(m.created_at, datetime('now')),
		       COALESCE(m.updated_at, datetime('now'))
		FROM sync_log sl
		JOIN memories m ON m.id = sl.memory_id
		WHERE sl.status = 'pending'
		ORDER BY sl.id ASC
		LIMIT 100
	`)
	if err != nil {
		return nil, fmt.Errorf("d1: fetch pending: %w", err)
	}
	defer rows.Close()

	var pending []pendingMemory
	for rows.Next() {
		var pm pendingMemory
		if err := rows.Scan(&pm.SyncLogID, &pm.MemoryID, &pm.WorkspaceID,
			&pm.CloudID, &pm.DeviceID, &pm.Content, &pm.ContentHash,
			&pm.Tags, &pm.Source, &pm.Version, &pm.CreatedAt, &pm.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("d1: scan pending: %w", err)
		}
		pending = append(pending, pm)
	}
	return pending, rows.Err()
}

// upsertRemote INSERTs or REPLACEs a memory in the remote D1 database.
func (p *D1Provider) upsertRemote(ctx context.Context, pm pendingMemory) error {
	// Use INSERT OR REPLACE for upsert semantics on primary key.
	sql := fmt.Sprintf(`
		INSERT OR REPLACE INTO %s
			(memory_id, workspace_id, cloud_id, device_id,
			 content, content_hash, tags, source, version,
			 created_at, updated_at)
		VALUES (%d, %d, '%s', '%s', '%s', '%s', '%s', '%s', %d, '%s', '%s')
	`, smaraMemoriesTable,
		pm.MemoryID, pm.WorkspaceID,
		escapeSQL(pm.CloudID), escapeSQL(pm.DeviceID),
		escapeSQL(pm.Content), escapeSQL(pm.ContentHash),
		escapeSQL(pm.Tags), escapeSQL(pm.Source),
		pm.Version,
		escapeSQL(pm.CreatedAt), escapeSQL(pm.UpdatedAt),
	)

	return p.execD1Query(ctx, sql)
}

// markSynced updates sync_log rows to status='synced'.
func (p *D1Provider) markSynced(ctx context.Context, pending []pendingMemory) error {
	tx, err := p.replicaDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, pm := range pending {
		if _, err := tx.ExecContext(ctx,
			`UPDATE sync_log SET status='synced', synced_at=? WHERE id=?`,
			time.Now().UTC().Format(time.RFC3339), pm.SyncLogID,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Pull — fetch changes from D1.
// ---------------------------------------------------------------------------

// d1Memory is the JSON shape returned by D1 query results.
type d1Memory struct {
	MemoryID    float64 `json:"memory_id"`
	WorkspaceID float64 `json:"workspace_id"`
	CloudID     string  `json:"cloud_id"`
	DeviceID    string  `json:"device_id"`
	Content     string  `json:"content"`
	ContentHash string  `json:"content_hash"`
	Tags        string  `json:"tags"`
	Source      string  `json:"source"`
	Version     float64 `json:"version"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// Pull fetches remote changes from D1 and merges them into the local
// SQLite store.
//
// Flow:
//  1. SELECT rows from D1 with updated_at > last_sync_at.
//  2. For each remote row, check if it exists locally:
//     a. If local row has lower version → update local.
//     b. If versions match → skip (already synced).
//     c. If local row has higher version → conflict.
//  3. Update last_sync_at in cloud_databases.
func (p *D1Provider) Pull(ctx context.Context) (*cloud.SyncReport, error) {
	report := &cloud.SyncReport{StartedAt: time.Now()}
	defer func() { report.FinishedAt = time.Now() }()

	if p.replicaDB == nil {
		return report, cloud.ErrUnreachable
	}
	if p.databaseID == "" || p.apiToken == "" || p.accountID == "" {
		report.Errors = append(report.Errors, "d1: Pull: not authenticated")
		return report, fmt.Errorf("d1: Pull: not authenticated")
	}

	// Get last sync timestamp.
	lastSync := p.lastSyncAt(ctx)

	// Fetch remote changes.
	query := fmt.Sprintf(
		"SELECT * FROM %s ORDER BY updated_at ASC LIMIT 200",
		smaraMemoriesTable,
	)
	if !lastSync.IsZero() {
		query = fmt.Sprintf(
			"SELECT * FROM %s WHERE updated_at > '%s' ORDER BY updated_at ASC LIMIT 200",
			smaraMemoriesTable,
			lastSync.UTC().Format(time.RFC3339),
		)
	}

	rawRows, err := p.queryD1Rows(ctx, query)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report, err
	}
	if len(rawRows) == 0 {
		return report, nil
	}

	// Convert raw rows to typed structs.
	var remoteMemories []d1Memory
	for _, row := range rawRows {
		rm := d1Memory{}
		if v, ok := row["memory_id"]; ok {
			rm.MemoryID, _ = v.(float64)
		}
		if v, ok := row["workspace_id"]; ok {
			rm.WorkspaceID, _ = v.(float64)
		}
		if v, ok := row["cloud_id"]; ok {
			rm.CloudID, _ = v.(string)
		}
		if v, ok := row["device_id"]; ok {
			rm.DeviceID, _ = v.(string)
		}
		if v, ok := row["content"]; ok {
			rm.Content, _ = v.(string)
		}
		if v, ok := row["content_hash"]; ok {
			rm.ContentHash, _ = v.(string)
		}
		if v, ok := row["tags"]; ok {
			rm.Tags, _ = v.(string)
		}
		if v, ok := row["source"]; ok {
			rm.Source, _ = v.(string)
		}
		if v, ok := row["version"]; ok {
			rm.Version, _ = v.(float64)
		}
		if v, ok := row["created_at"]; ok {
			rm.CreatedAt, _ = v.(string)
		}
		if v, ok := row["updated_at"]; ok {
			rm.UpdatedAt, _ = v.(string)
		}
		remoteMemories = append(remoteMemories, rm)
	}

	// Merge each remote memory into local store.
	var conflicts int
	for _, rm := range remoteMemories {
		action, err := p.mergeRemote(ctx, rm)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("merge memory %d: %v", int64(rm.MemoryID), err))
			continue
		}
		if action == "conflict" {
			conflicts++
		}
	}

	report.PulledFrames = len(remoteMemories)
	report.Conflicts = conflicts

	// Update last_sync_at.
	p.updateLastSync(ctx, report.FinishedAt)

	_ = audit.LogCloudOp("pull", true, "d1", map[string]any{
		"pulled_frames": len(remoteMemories),
		"conflicts":     conflicts,
	})
	return report, nil
}

// mergeRemote merges a single remote memory into the local store.
// Returns "insert", "update", "skip", or "conflict".
func (p *D1Provider) mergeRemote(ctx context.Context, rm d1Memory) (string, error) {
	memoryID := int64(rm.MemoryID)

	// Check if local row exists.
	var localVersion int64
	err := p.replicaDB.QueryRowContext(ctx,
		`SELECT COALESCE(version, 0) FROM memories WHERE id = ?`,
		memoryID,
	).Scan(&localVersion)

	remoteVersion := int64(rm.Version)

	if err == sql.ErrNoRows {
		// Insert new row.
		_, err := p.replicaDB.ExecContext(ctx, `
			INSERT INTO memories (id, workspace_id, content, tags, source,
			                      version, cloud_id, device_id, content_hash,
			                      created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, memoryID, int64(rm.WorkspaceID), rm.Content, rm.Tags, rm.Source,
			remoteVersion, rm.CloudID, rm.DeviceID, rm.ContentHash,
			rm.CreatedAt, rm.UpdatedAt)
		if err != nil && !isConstraintErrD1(err) {
			return "", err
		}
		return "insert", nil
	}
	if err != nil {
		return "", err
	}

	// Row exists. Compare versions.
	if remoteVersion > localVersion {
		// Remote is newer — update local.
		_, err := p.replicaDB.ExecContext(ctx, `
			UPDATE memories SET content=?, tags=?, source=?, version=?,
			       cloud_id=?, device_id=?, content_hash=?,
			       updated_at=?
			WHERE id = ?
		`, rm.Content, rm.Tags, rm.Source, remoteVersion,
			rm.CloudID, rm.DeviceID, rm.ContentHash,
			rm.UpdatedAt, memoryID)
		if err != nil {
			return "", err
		}
		return "update", nil
	} else if remoteVersion < localVersion {
		// Local is newer — record conflict.
		var localContent string
		p.replicaDB.QueryRowContext(ctx,
			`SELECT content FROM memories WHERE id=?`, memoryID,
		).Scan(&localContent)

		p.replicaDB.ExecContext(ctx, `
			INSERT OR IGNORE INTO cloud_conflicts
				(memory_id, local_version, remote_version,
				 local_content, remote_content,
				 local_updated_at, remote_updated_at, detected_at)
			VALUES (?, ?, ?, ?, ?, datetime('now'), ?, datetime('now'))
		`, memoryID, localVersion, remoteVersion,
			localContent, rm.Content, rm.UpdatedAt)
		return "conflict", nil
	}

	// Versions equal — skip.
	return "skip", nil
}

// ---------------------------------------------------------------------------
// Status — local + remote snapshot.
// ---------------------------------------------------------------------------

// Status returns a SyncStatus snapshot by querying both local state
// and the D1 REST API for remote row counts.
func (p *D1Provider) Status(ctx context.Context) (*cloud.SyncStatus, error) {
	st := &cloud.SyncStatus{
		LastSyncAt: time.Now().UTC(),
	}

	// Local counters.
	if p.replicaDB != nil {
		// Pending push count.
		var pending int
		if err := p.replicaDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sync_log WHERE status='pending'`,
		).Scan(&pending); err == nil {
			st.PendingPush = pending
		}

		// Unresolved conflicts.
		var conflicts int
		if err := p.replicaDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM cloud_conflicts WHERE resolved_at IS NULL`,
		).Scan(&conflicts); err == nil {
			st.UnresolvedConflicts = conflicts
		}

		// Local row count.
		var localRows int64
		if err := p.replicaDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM memories`,
		).Scan(&localRows); err == nil {
			st.LocalFrameNo = localRows
		}
	}

	// Remote counts via D1 REST.
	if p.databaseID != "" {
		remoteRows, err := p.countD1Rows(ctx, smaraMemoriesTable)
		if err == nil {
			st.RemoteFrameNo = remoteRows
		}
	}

	return st, nil
}

// ---------------------------------------------------------------------------
// Helpers.
// ---------------------------------------------------------------------------

// lastSyncAt reads the last sync timestamp from cloud_databases.
func (p *D1Provider) lastSyncAt(ctx context.Context) time.Time {
	if p.replicaDB == nil {
		return time.Time{}
	}
	var last sql.NullString
	err := p.replicaDB.QueryRowContext(ctx,
		`SELECT last_sync_at FROM cloud_databases LIMIT 1`,
	).Scan(&last)
	if err != nil || !last.Valid {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, last.String)
	if err != nil {
		return time.Time{}
	}
	return t
}

// updateLastSync updates the last_sync_at timestamp in cloud_databases.
func (p *D1Provider) updateLastSync(ctx context.Context, t time.Time) {
	if p.replicaDB == nil {
		return
	}
	p.replicaDB.ExecContext(ctx,
		`UPDATE cloud_databases SET last_sync_at=? WHERE id=(SELECT id FROM cloud_databases LIMIT 1)`,
		t.UTC().Format(time.RFC3339),
	)
}

// isConstraintErrD1 reports whether err is a constraint violation.
func isConstraintErrD1(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") ||
		strings.Contains(msg, "constraint") ||
		strings.Contains(msg, "primary key")
}

// escapeSQL escapes single quotes for safe SQL string literal embedding.
// This is used for building SQL statements sent to the D1 REST API.
func escapeSQL(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
