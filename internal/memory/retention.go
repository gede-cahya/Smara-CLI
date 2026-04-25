package memory

import (
	"database/sql"
	"fmt"
	"time"
)

// RetentionPolicy defines how long memories should be kept.
type RetentionPolicy struct {
	WorkspaceID int64
	TTLDays     int           // Time to live in days
	MaxMemories int           // Maximum number of memories to keep (0 = unlimited)
	CreatedAt   time.Time
}

// DeleteExpiredMemories removes all memories that have passed their expiration date.
// Returns the number of memories deleted.
func (s *SQLiteStore) DeleteExpiredMemories() (int, error) {
	result, err := s.db.Exec(
		"DELETE FROM memories WHERE expires_at IS NOT NULL AND expires_at < CURRENT_TIMESTAMP",
	)
	if err != nil {
		return 0, fmt.Errorf("gagal hapus memory kadaluarsa: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("gagal dapatkan jumlah baris terhapus: %w", err)
	}

	return int(rowsAffected), nil
}

// DeleteExpiredMemoriesForWorkspace removes expired memories for a specific workspace.
func (s *SQLiteStore) DeleteExpiredMemoriesForWorkspace(workspaceID int64) (int, error) {
	if workspaceID <= 0 {
		return 0, fmt.Errorf("workspace ID tidak valid")
	}

	result, err := s.db.Exec(
		"DELETE FROM memories WHERE workspace_id = ? AND expires_at IS NOT NULL AND expires_at < CURRENT_TIMESTAMP",
		workspaceID,
	)
	if err != nil {
		return 0, fmt.Errorf("gagal hapus memory kadaluarsa: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	return int(rowsAffected), nil
}

// SetRetentionPolicy sets a retention policy for a workspace.
// This updates the expires_at field for all existing memories in the workspace.
func (s *SQLiteStore) SetRetentionPolicy(workspaceID int64, ttlDays int) error {
	if workspaceID <= 0 {
		return fmt.Errorf("workspace ID tidak valid")
	}

	if ttlDays <= 0 {
		return fmt.Errorf("TTL harus lebih dari 0 hari")
	}

	_, err := s.db.Exec(
		`UPDATE memories 
		 SET expires_at = datetime('now', '+' || ? || ' days')
		 WHERE workspace_id = ?`,
		ttlDays, workspaceID,
	)
	if err != nil {
		return fmt.Errorf("gagal set kebijakan retensi: %w", err)
	}

	return nil
}

// GetRetentionPolicy retrieves the current retention policy for a workspace.
// Returns nil if no policy is set.
func (s *SQLiteStore) GetRetentionPolicy(workspaceID int64) (*RetentionPolicy, error) {
	if workspaceID <= 0 {
		return nil, fmt.Errorf("workspace ID tidak valid")
	}

	// Calculate average TTL from existing memories
	var avgTTL sql.NullFloat64
	err := s.db.QueryRow(
		`SELECT AVG(julianday(expires_at) - julianday(created_at))
		 FROM memories 
		 WHERE workspace_id = ? AND expires_at IS NOT NULL`,
		workspaceID,
	).Scan(&avgTTL)
	if err != nil {
		return nil, fmt.Errorf("gagal baca kebijakan retensi: %w", err)
	}

	policy := &RetentionPolicy{
		WorkspaceID: workspaceID,
		CreatedAt:   time.Now(),
	}

	if avgTTL.Valid {
		policy.TTLDays = int(avgTTL.Float64)
	} else {
		policy.TTLDays = 0 // No policy set
	}

	// Count total memories
	var totalMemories int
	err = s.db.QueryRow(
		"SELECT count(*) FROM memories WHERE workspace_id = ?",
		workspaceID,
	).Scan(&totalMemories)
	if err != nil {
		return nil, fmt.Errorf("gagal hitung total memory: %w", err)
	}
	policy.MaxMemories = totalMemories

	return policy, nil
}

// ApplyRetentionPolicy applies retention rules to a workspace.
// Removes memories older than TTL days and enforces maximum memory count.
func (s *SQLiteStore) ApplyRetentionPolicy(workspaceID int64, ttlDays, maxMemories int) (deletedCount int, err error) {
	if workspaceID <= 0 {
		return 0, fmt.Errorf("workspace ID tidak valid")
	}

	totalDeleted := 0

	// Delete expired memories based on TTL
	if ttlDays > 0 {
		deleted, err := s.deleteMemoriesOlderThan(workspaceID, ttlDays)
		if err != nil {
			return totalDeleted, fmt.Errorf("gagal hapus memory berdasarkan TTL: %w", err)
		}
		totalDeleted += deleted
	}

	// Enforce maximum memory count (keep newest)
	if maxMemories > 0 {
		deleted, err := s.enforceMaxMemories(workspaceID, maxMemories)
		if err != nil {
			return totalDeleted, fmt.Errorf("gagal enforce maksimum memory: %w", err)
		}
		totalDeleted += deleted
	}

	return totalDeleted, nil
}

// deleteMemoriesOlderThan deletes memories older than specified days.
func (s *SQLiteStore) deleteMemoriesOlderThan(workspaceID int64, days int) (int, error) {
	result, err := s.db.Exec(
		`DELETE FROM memories 
		 WHERE workspace_id = ? 
		 AND created_at < datetime('now', '-' || ? || ' days')`,
		workspaceID, days,
	)
	if err != nil {
		return 0, err
	}

	rowsAffected, _ := result.RowsAffected()
	return int(rowsAffected), nil
}

// enforceMaxMemories keeps only the newest N memories, deleting older ones.
func (s *SQLiteStore) enforceMaxMemories(workspaceID int64, maxCount int) (int, error) {
	// First, count total memories
	var total int
	err := s.db.QueryRow(
		"SELECT count(*) FROM memories WHERE workspace_id = ?",
		workspaceID,
	).Scan(&total)
	if err != nil {
		return 0, err
	}

	if total <= maxCount {
		return 0, nil // Nothing to delete
	}

	// Delete oldest memories beyond the limit
	result, err := s.db.Exec(
		`DELETE FROM memories 
		 WHERE id IN (
			 SELECT id FROM memories 
			 WHERE workspace_id = ? 
			 ORDER BY created_at ASC 
			 LIMIT ?
		 )`,
		workspaceID, total-maxCount,
	)
	if err != nil {
		return 0, err
	}

	rowsAffected, _ := result.RowsAffected()
	return int(rowsAffected), nil
}

// CleanOrphanedCategories removes categories that have no associated memories.
func (s *SQLiteStore) CleanOrphanedCategories(workspaceID int64) (int, error) {
	if workspaceID <= 0 {
		return 0, fmt.Errorf("workspace ID tidak valid")
	}

	result, err := s.db.Exec(
		`DELETE FROM categories 
		 WHERE workspace_id = ? 
		 AND id NOT IN (SELECT DISTINCT category_id FROM memories WHERE category_id IS NOT NULL)`,
		workspaceID,
	)
	if err != nil {
		return 0, fmt.Errorf("gagal hapus kategori orphan: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	return int(rowsAffected), nil
}

// GetMemoryStats returns statistics about memories in a workspace.
func (s *SQLiteStore) GetMemoryStats(workspaceID int64) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total memories
	var total int
	err := s.db.QueryRow(
		"SELECT count(*) FROM memories WHERE workspace_id = ?",
		workspaceID,
	).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("gagal hitung total memory: %w", err)
	}
	stats["total"] = total

	// Memories with embeddings
	var withEmbedding int
	err = s.db.QueryRow(
		"SELECT count(*) FROM memories WHERE workspace_id = ? AND embedding IS NOT NULL",
		workspaceID,
	).Scan(&withEmbedding)
	if err != nil {
		return nil, fmt.Errorf("gagal hitung memory dengan embedding: %w", err)
	}
	stats["with_embedding"] = withEmbedding

	// Expired memories
	var expired int
	err = s.db.QueryRow(
		"SELECT count(*) FROM memories WHERE workspace_id = ? AND expires_at IS NOT NULL AND expires_at < CURRENT_TIMESTAMP",
		workspaceID,
	).Scan(&expired)
	if err != nil {
		return nil, fmt.Errorf("gagal hitung memory kadaluarsa: %w", err)
	}
	stats["expired"] = expired

	// Memories by category
	rows, err := s.db.Query(
		`SELECT c.name, count(m.id) 
		 FROM categories c 
		 LEFT JOIN memories m ON c.id = m.category_id 
		 WHERE c.workspace_id = ? 
		 GROUP BY c.id`,
		workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("gagal hitung per kategori: %w", err)
	}
	defer rows.Close()

	categoryStats := make(map[string]int)
	for rows.Next() {
		var name string
		var count int
		if err := rows.Scan(&name, &count); err != nil {
			continue
		}
		categoryStats[name] = count
	}
	stats["by_category"] = categoryStats

	// Oldest and newest memory dates
	var oldest, newest sql.NullTime
	err = s.db.QueryRow(
		`SELECT min(created_at), max(created_at) 
		 FROM memories WHERE workspace_id = ?`,
		workspaceID,
	).Scan(&oldest, &newest)
	if err == nil {
		if oldest.Valid {
			stats["oldest"] = oldest.Time.Format("2006-01-02")
		}
		if newest.Valid {
			stats["newest"] = newest.Time.Format("2006-01-02")
		}
	}

	return stats, nil
}

// RunCleanup performs a full cleanup operation on a workspace.
// Returns statistics about what was cleaned up.
func (s *SQLiteStore) RunCleanup(workspaceID int64) (map[string]interface{}, error) {
	results := make(map[string]interface{})

	// Delete expired memories
	expiredDeleted, err := s.DeleteExpiredMemoriesForWorkspace(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("gagal cleanup memory kadaluarsa: %w", err)
	}
	results["expired_deleted"] = expiredDeleted

	// Clean orphaned categories
	orphanDeleted, err := s.CleanOrphanedCategories(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("gagal cleanup kategori orphan: %w", err)
	}
	results["orphan_categories_deleted"] = orphanDeleted

	// Optimize FTS5 if available
	if s.FTS5Available() {
		if err := s.OptimizeFTS5(); err == nil {
			results["fts_optimized"] = true
		}
	}

	// Get updated stats
	stats, err := s.GetMemoryStats(workspaceID)
	if err == nil {
		results["current_stats"] = stats
	}

	return results, nil
}