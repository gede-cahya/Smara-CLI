// Package supabase — Push / Pull / Status via Supabase REST API.
//
// Unlike Turso (which uses libSQL embedded-replica WAL replication),
// Supabase sync is manual:
//
//   - Push: reads pending rows from local SQLite sync_log, POSTs them
//     to the Supabase REST API, and marks them synced.
//   - Pull: GETs changed rows from Supabase (based on updated_at),
//     merges them into the local SQLite store.
//   - Status: queries Supabase REST API for row counts and compares
//     with local state.
//
// The smara_memories table schema on Supabase:
//
//	memory_id    BIGINT PRIMARY KEY
//	workspace_id BIGINT
//	cloud_id     TEXT UNIQUE
//	device_id    TEXT
//	content      TEXT
//	content_hash TEXT
//	tags         TEXT DEFAULT '[]'
//	source       TEXT DEFAULT ''
//	version      INTEGER DEFAULT 1
//	created_at   TIMESTAMPTZ
//	updated_at   TIMESTAMPTZ
//
// Requirements: 2.5, 7.5, 9.3.
package supabase

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/audit"
	cloud "github.com/gede-cahya/Smara-CLI/internal/memory/cloud"
)

// ---------------------------------------------------------------------------
// Push — upload pending rows to Supabase.
// ---------------------------------------------------------------------------

// Push uploads pending local rows to Supabase via the REST API.
//
// Flow:
//  1. Query local sync_log for rows with status='pending'.
//  2. For each pending memory, POST to /rest/v1/smara_memories with
//     Prefer: resolution=merge-duplicates (upsert on memory_id).
//  3. Mark the sync_log rows as status='synced'.
func (p *SupabaseProvider) Push(ctx context.Context) (*cloud.SyncReport, error) {
	report := &cloud.SyncReport{StartedAt: time.Now()}
	defer func() { report.FinishedAt = time.Now() }()

	if p.replicaDB == nil {
		return report, cloud.ErrUnreachable
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

	// Push each pending memory to Supabase.
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

	_ = audit.LogCloudOp("push", true, "supabase", map[string]any{
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
func (p *SupabaseProvider) fetchPendingMemories(ctx context.Context) ([]pendingMemory, error) {
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
		return nil, fmt.Errorf("supabase: fetch pending: %w", err)
	}
	defer rows.Close()

	var pending []pendingMemory
	for rows.Next() {
		var pm pendingMemory
		if err := rows.Scan(&pm.SyncLogID, &pm.MemoryID, &pm.WorkspaceID,
			&pm.CloudID, &pm.DeviceID, &pm.Content, &pm.ContentHash,
			&pm.Tags, &pm.Source, &pm.Version, &pm.CreatedAt, &pm.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("supabase: scan pending: %w", err)
		}
		pending = append(pending, pm)
	}
	return pending, rows.Err()
}

// upsertRemote POSTs a memory to Supabase with upsert semantics.
func (p *SupabaseProvider) upsertRemote(ctx context.Context, pm pendingMemory) error {
	if p.restURL == "" {
		return fmt.Errorf("restURL not set")
	}

	body := map[string]interface{}{
		"memory_id":    pm.MemoryID,
		"workspace_id": pm.WorkspaceID,
		"cloud_id":     pm.CloudID,
		"device_id":    pm.DeviceID,
		"content":      pm.Content,
		"content_hash": pm.ContentHash,
		"tags":         pm.Tags,
		"source":       pm.Source,
		"version":      pm.Version,
		"created_at":   pm.CreatedAt,
		"updated_at":   pm.UpdatedAt,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return err
	}

	endpoint := p.restURL + "/" + smaraMemoriesTable
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("apikey", p.serviceKey)
	req.Header.Set("Authorization", "Bearer "+p.serviceKey)
	req.Header.Set("Content-Type", "application/json")
	// Prefer: resolution=merge-duplicates → upsert on conflict (primary key).
	req.Header.Set("Prefer", "resolution=merge-duplicates")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("POST to supabase: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// markSynced updates sync_log rows to status='synced'.
func (p *SupabaseProvider) markSynced(ctx context.Context, pending []pendingMemory) error {
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
// Pull — fetch changes from Supabase.
// ---------------------------------------------------------------------------

// supabaseMemory is the JSON shape returned by Supabase REST GET.
type supabaseMemory struct {
	MemoryID    int64  `json:"memory_id"`
	WorkspaceID int64  `json:"workspace_id"`
	CloudID     string `json:"cloud_id"`
	DeviceID    string `json:"device_id"`
	Content     string `json:"content"`
	ContentHash string `json:"content_hash"`
	Tags        string `json:"tags"`
	Source      string `json:"source"`
	Version     int64  `json:"version"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// Pull fetches remote changes from Supabase and merges them into the
// local SQLite store.
//
// Flow:
//  1. GET rows from Supabase with updated_at > last_sync_at.
//  2. For each remote row, check if it exists locally:
//     a. If local row has lower version → update local.
//     b. If versions match → skip (already synced).
//     c. If local row has higher version → conflict.
//  3. Update last_sync_at in cloud_databases.
func (p *SupabaseProvider) Pull(ctx context.Context) (*cloud.SyncReport, error) {
	report := &cloud.SyncReport{StartedAt: time.Now()}
	defer func() { report.FinishedAt = time.Now() }()

	if p.replicaDB == nil {
		return report, cloud.ErrUnreachable
	}

	// Get last sync timestamp.
	lastSync := p.lastSyncAt(ctx)

	// Fetch remote changes.
	filter := ""
	if !lastSync.IsZero() {
		filter = fmt.Sprintf("&updated_at=gt.%s", lastSync.UTC().Format(time.RFC3339))
	}
	endpoint := p.restURL + "/" + smaraMemoriesTable + "?order=updated_at.asc&limit=200" + filter
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report, err
	}
	req.Header.Set("apikey", p.serviceKey)
	req.Header.Set("Authorization", "Bearer "+p.serviceKey)
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Table doesn't exist yet — nothing to pull.
		return report, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		err := fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
		report.Errors = append(report.Errors, err.Error())
		return report, err
	}

	var remoteMemories []supabaseMemory
	if err := json.NewDecoder(resp.Body).Decode(&remoteMemories); err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report, err
	}

	if len(remoteMemories) == 0 {
		return report, nil
	}

	// Merge each remote memory into local store.
	var conflicts int
	for _, rm := range remoteMemories {
		action, err := p.mergeRemote(ctx, rm)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("merge memory %d: %v", rm.MemoryID, err))
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

	_ = audit.LogCloudOp("pull", true, "supabase", map[string]any{
		"pulled_frames": len(remoteMemories),
		"conflicts":     conflicts,
	})
	return report, nil
}

// mergeRemote merges a single remote memory into the local store.
// Returns "insert", "update", "skip", or "conflict".
func (p *SupabaseProvider) mergeRemote(ctx context.Context, rm supabaseMemory) (string, error) {
	// Check if local row exists.
	var localVersion int64
	var localCloudID string
	err := p.replicaDB.QueryRowContext(ctx,
		`SELECT COALESCE(version, 0), COALESCE(cloud_id, '') FROM memories WHERE id = ?`,
		rm.MemoryID,
	).Scan(&localVersion, &localCloudID)

	if err == sql.ErrNoRows {
		// Insert new row.
		_, err := p.replicaDB.ExecContext(ctx, `
			INSERT INTO memories (id, workspace_id, content, tags, source,
			                      version, cloud_id, device_id, content_hash,
			                      created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, rm.MemoryID, rm.WorkspaceID, rm.Content, rm.Tags, rm.Source,
			rm.Version, rm.CloudID, rm.DeviceID, rm.ContentHash,
			rm.CreatedAt, rm.UpdatedAt)
		if err != nil && !isConstraintErr(err) {
			return "", err
		}
		return "insert", nil
	}
	if err != nil {
		return "", err
	}

	// Row exists. Compare versions.
	if rm.Version > localVersion {
		// Remote is newer — update local.
		_, err := p.replicaDB.ExecContext(ctx, `
			UPDATE memories SET content=?, tags=?, source=?, version=?,
			       cloud_id=?, device_id=?, content_hash=?,
			       updated_at=?
			WHERE id = ?
		`, rm.Content, rm.Tags, rm.Source, rm.Version,
			rm.CloudID, rm.DeviceID, rm.ContentHash,
			rm.UpdatedAt, rm.MemoryID)
		if err != nil {
			return "", err
		}
		return "update", nil
	} else if rm.Version < localVersion {
		// Local is newer — record conflict.
		var localContent string
		p.replicaDB.QueryRowContext(ctx,
			`SELECT content FROM memories WHERE id=?`, rm.MemoryID,
		).Scan(&localContent)

		p.replicaDB.ExecContext(ctx, `
			INSERT OR IGNORE INTO cloud_conflicts
				(memory_id, local_version, remote_version,
				 local_content, remote_content,
				 local_updated_at, remote_updated_at, detected_at)
			VALUES (?, ?, ?, ?, ?, datetime('now'), ?, datetime('now'))
		`, rm.MemoryID, localVersion, rm.Version,
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
// and the Supabase REST API for remote row counts.
func (p *SupabaseProvider) Status(ctx context.Context) (*cloud.SyncStatus, error) {
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

	// Remote counts via Supabase REST.
	if p.restURL != "" {
		remoteRows, err := p.countRemoteRows(ctx)
		if err == nil {
			st.RemoteFrameNo = remoteRows
		}
	}

	return st, nil
}

// countRemoteRows queries Supabase for row count using
// Prefer: count=exact header.
func (p *SupabaseProvider) countRemoteRows(ctx context.Context) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		p.restURL+"/"+smaraMemoriesTable+"?limit=0", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("apikey", p.serviceKey)
	req.Header.Set("Authorization", "Bearer "+p.serviceKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Prefer", "count=exact")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Content-Range header format: "0-0/*" where * is total.
	contentRange := resp.Header.Get("Content-Range")
	if contentRange == "" {
		return 0, nil
	}
	// Parse: "0-0/123"
	parts := strings.Split(contentRange, "/")
	if len(parts) == 2 {
		var count int64
		fmt.Sscanf(parts[1], "%d", &count)
		return count, nil
	}
	return 0, nil
}

// ---------------------------------------------------------------------------
// Helpers.
// ---------------------------------------------------------------------------

// lastSyncAt reads the last sync timestamp from cloud_databases.
func (p *SupabaseProvider) lastSyncAt(ctx context.Context) time.Time {
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
func (p *SupabaseProvider) updateLastSync(ctx context.Context, t time.Time) {
	if p.replicaDB == nil {
		return
	}
	p.replicaDB.ExecContext(ctx,
		`UPDATE cloud_databases SET last_sync_at=? WHERE id=(SELECT id FROM cloud_databases LIMIT 1)`,
		t.UTC().Format(time.RFC3339),
	)
}

// isConstraintErr reports whether err is a constraint violation.
func isConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") ||
		strings.Contains(msg, "constraint") ||
		strings.Contains(msg, "primary key")
}
