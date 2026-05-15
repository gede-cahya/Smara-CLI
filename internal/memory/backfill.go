package memory

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
)

// BackfillCloudFields populates cloud_id, device_id, and content_hash for any
// memory row that pre-dates Cloud_Memory enablement (i.e. cloud_id IS NULL).
//
// Per Requirement 8.2 every existing row gets a fresh UUID v7 cloud_id, the
// caller-provided deviceID, and a sha256 hex digest of its content. The
// operation is wrapped in a single BEGIN/COMMIT so a failure mid-flight rolls
// back cleanly and leaves the database safe to retry (Requirement 8.4). On
// success it reports how many rows were filled in (Requirement 8.5).
//
// Re-running this method is a no-op once every row has cloud_id populated,
// which is what makes the bootstrap path in `OpenStoreWithCloud` idempotent.
func (s *SQLiteStore) BackfillCloudFields(deviceID string) (int, error) {
	if deviceID == "" {
		return 0, fmt.Errorf("deviceID kosong: backfill memerlukan device id non-empty")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("gagal memulai transaksi backfill: %w", err)
	}
	// Best-effort rollback on any early return; harmless if the tx already
	// committed because Rollback returns sql.ErrTxDone in that case.
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	rows, err := tx.Query(`SELECT id, content FROM memories WHERE cloud_id IS NULL`)
	if err != nil {
		return 0, fmt.Errorf("gagal query memories tanpa cloud_id: %w", err)
	}

	type pending struct {
		id      int64
		content string
	}
	var todo []pending
	for rows.Next() {
		var p pending
		var content sql.NullString
		if err := rows.Scan(&p.id, &content); err != nil {
			rows.Close()
			return 0, fmt.Errorf("gagal scan baris memory: %w", err)
		}
		p.content = content.String
		todo = append(todo, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("gagal iterasi rows: %w", err)
	}
	rows.Close()

	if len(todo) == 0 {
		if err := tx.Commit(); err != nil {
			return 0, fmt.Errorf("gagal commit transaksi backfill (no-op): %w", err)
		}
		committed = true
		return 0, nil
	}

	stmt, err := tx.Prepare(`UPDATE memories SET cloud_id = ?, device_id = ?, content_hash = ? WHERE id = ? AND cloud_id IS NULL`)
	if err != nil {
		return 0, fmt.Errorf("gagal prepare update backfill: %w", err)
	}
	defer stmt.Close()

	count := 0
	for _, p := range todo {
		cloudID, err := uuid.NewV7()
		if err != nil {
			return 0, fmt.Errorf("gagal generate UUID v7 untuk memory id=%d: %w", p.id, err)
		}

		sum := sha256.Sum256([]byte(p.content))
		hash := hex.EncodeToString(sum[:])

		res, err := stmt.Exec(cloudID.String(), deviceID, hash, p.id)
		if err != nil {
			return 0, fmt.Errorf("gagal update cloud fields untuk memory id=%d: %w", p.id, err)
		}

		// Use RowsAffected so concurrent migrators don't double-count. The WHERE
		// clause `cloud_id IS NULL` makes the UPDATE itself idempotent.
		affected, err := res.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("gagal baca rows affected untuk memory id=%d: %w", p.id, err)
		}
		if affected > 0 {
			count++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("gagal commit transaksi backfill: %w", err)
	}
	committed = true
	return count, nil
}
