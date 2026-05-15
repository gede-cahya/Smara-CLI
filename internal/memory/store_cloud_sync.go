package memory

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/memory/cloud"
)

// GetMemoryByCloudID returns a cloud resolver row by cloud_id.
func (s *SQLiteStore) GetMemoryByCloudID(cloudID string) (*cloud.MemoryRow, error) {
	var r cloud.MemoryRow
	err := s.db.QueryRow(`
		SELECT id, COALESCE(cloud_id,''), content, COALESCE(device_id,''), version, updated_at
		FROM memories
		WHERE cloud_id = ?
		LIMIT 1`, cloudID).Scan(&r.ID, &r.CloudID, &r.Content, &r.DeviceID, &r.Version, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// InsertCloudConflict records a divergent local/remote row pair.
func (s *SQLiteStore) InsertCloudConflict(c cloud.CloudConflict) error {
	// Avoid piling up duplicate unresolved conflicts for the same memory pair.
	var existing int64
	_ = s.db.QueryRow(`SELECT id FROM cloud_conflicts WHERE memory_id = ? AND resolved_at IS NULL LIMIT 1`, c.MemoryID).Scan(&existing)
	if existing != 0 {
		_, err := s.db.Exec(`UPDATE cloud_conflicts SET local_version = ?, remote_version = ?, local_content = ?, remote_content = ?, local_updated_at = ?, remote_updated_at = ?, detected_at = CURRENT_TIMESTAMP WHERE id = ?`,
			c.LocalVersion, c.RemoteVersion, c.LocalContent, c.RemoteContent, c.LocalUpdatedAt, c.RemoteUpdatedAt, existing)
		return err
	}
	_, err := s.db.Exec(`INSERT INTO cloud_conflicts (memory_id, local_version, remote_version, local_content, remote_content, local_updated_at, remote_updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.MemoryID, c.LocalVersion, c.RemoteVersion, c.LocalContent, c.RemoteContent, c.LocalUpdatedAt, c.RemoteUpdatedAt)
	return err
}

// ListUnresolvedConflicts lists conflicts awaiting resolution.
func (s *SQLiteStore) ListUnresolvedConflicts() ([]cloud.CloudConflict, error) {
	rows, err := s.db.Query(`SELECT cc.id, cc.memory_id, COALESCE(m.cloud_id,''), cc.local_version, cc.remote_version, cc.local_content, cc.remote_content, COALESCE(m.device_id,''), '', cc.local_updated_at, cc.remote_updated_at, cc.detected_at FROM cloud_conflicts cc LEFT JOIN memories m ON m.id = cc.memory_id WHERE cc.resolved_at IS NULL ORDER BY cc.detected_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []cloud.CloudConflict
	for rows.Next() {
		var c cloud.CloudConflict
		if err := rows.Scan(&c.ID, &c.MemoryID, &c.CloudID, &c.LocalVersion, &c.RemoteVersion, &c.LocalContent, &c.RemoteContent, &c.LocalDeviceID, &c.RemoteDeviceID, &c.LocalUpdatedAt, &c.RemoteUpdatedAt, &c.DetectedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateMemoryFromConflict applies the winner and optionally archives the loser atomically.
func (s *SQLiteStore) UpdateMemoryFromConflict(memID int64, winner cloud.MemoryRow, loser *cloud.MemoryVersionRow) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if winner.UpdatedAt.IsZero() {
		winner.UpdatedAt = time.Now()
	}
	if _, err := tx.Exec(`UPDATE memories SET content = ?, device_id = ?, version = ?, updated_at = ?, content_hash = ? WHERE id = ?`, winner.Content, winner.DeviceID, winner.Version, winner.UpdatedAt, contentHashHex(winner.Content), memID); err != nil {
		return fmt.Errorf("update memory conflict winner: %w", err)
	}
	if loser != nil {
		if loser.MemoryID == 0 {
			loser.MemoryID = memID
		}
		if _, err := tx.Exec(`INSERT INTO memory_versions (memory_id, content, metadata, changed_by, reason, created_at) VALUES (?, ?, '{}', ?, ?, CURRENT_TIMESTAMP)`, loser.MemoryID, loser.Content, loser.ChangedBy, loser.Reason); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// MarkConflictResolved marks a conflict resolved.
func (s *SQLiteStore) MarkConflictResolved(id int64, resolution string) error {
	_, err := s.db.Exec(`UPDATE cloud_conflicts SET resolved_at = CURRENT_TIMESTAMP, resolution = ? WHERE id = ?`, resolution, id)
	return err
}

func contentHashHex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
