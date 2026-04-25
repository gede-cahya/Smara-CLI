package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// VersionDiff represents the difference between two versions of a memory.
type VersionDiff struct {
	Field     string      `json:"field"`
	OldValue  interface{} `json:"old_value"`
	NewValue  interface{} `json:"new_value"`
	ChangedAt time.Time   `json:"changed_at"`
}

// GetMemoryVersions returns all versions of a memory.
func (s *SQLiteStore) GetMemoryVersions(memoryID int64) ([]MemoryVersion, error) {
	if memoryID <= 0 {
		return nil, fmt.Errorf("ID memory tidak valid")
	}

	rows, err := s.db.Query(
		`SELECT id, memory_id, content, metadata, changed_by, reason, created_at
		 FROM memory_versions 
		 WHERE memory_id = ? 
		 ORDER BY created_at DESC`,
		memoryID,
	)
	if err != nil {
		return nil, fmt.Errorf("gagal query versi memory: %w", err)
	}
	defer rows.Close()

	var versions []MemoryVersion
	for rows.Next() {
		var v MemoryVersion
		if err := rows.Scan(&v.ID, &v.MemoryID, &v.Content, &v.Metadata, &v.ChangedBy, &v.Reason, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("gagal scan versi: %w", err)
		}
		versions = append(versions, v)
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("tidak ada versi untuk memory #%d", memoryID)
	}

	return versions, nil
}

// GetMemoryVersion returns a specific version of a memory.
func (s *SQLiteStore) GetMemoryVersion(memoryID, versionID int64) (*MemoryVersion, error) {
	if memoryID <= 0 || versionID <= 0 {
		return nil, fmt.Errorf("ID tidak valid")
	}

	var v MemoryVersion
	row := s.db.QueryRow(
		`SELECT id, memory_id, content, metadata, changed_by, reason, created_at
		 FROM memory_versions 
		 WHERE memory_id = ? AND id = ?`,
		memoryID, versionID,
	)

	if err := row.Scan(&v.ID, &v.MemoryID, &v.Content, &v.Metadata, &v.ChangedBy, &v.Reason, &v.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("versi tidak ditemukan")
		}
		return nil, fmt.Errorf("gagal membaca versi: %w", err)
	}

	return &v, nil
}

// RollbackMemory reverts a memory to a previous version.
func (s *SQLiteStore) RollbackMemory(memoryID int64, versionID int64) error {
	if memoryID <= 0 || versionID <= 0 {
		return fmt.Errorf("ID tidak valid")
	}

	// Get the target version
	targetVersion, err := s.GetMemoryVersion(memoryID, versionID)
	if err != nil {
		return fmt.Errorf("gagal mendapatkan versi target: %w", err)
	}

	// Get current memory
	currentMemory, err := s.GetMemoryByID(memoryID)
	if err != nil {
		return fmt.Errorf("gagal membaca memory saat ini: %w", err)
	}
	if currentMemory == nil {
		return fmt.Errorf("memory tidak ditemukan")
	}

	// Create a version record for the current state before rollback
	_, err = s.db.Exec(
		`INSERT INTO memory_versions (memory_id, content, metadata, changed_by, reason, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		memoryID,
		currentMemory.Content,
		formatMetadata(currentMemory.Metadata),
		"system",
		"pre-rollback backup",
		time.Now(),
	)
	if err != nil {
		// Non-critical error, log but don't fail
		fmt.Printf("Warning: gagal menyimpan versi pre-rollback: %v\n", err)
	}

	// Update memory to target version
	_, err = s.db.Exec(
		`UPDATE memories 
		 SET content = ?, metadata = ?, updated_at = CURRENT_TIMESTAMP, version = version + 1 
		 WHERE id = ?`,
		targetVersion.Content,
		targetVersion.Metadata,
		memoryID,
	)
	if err != nil {
		return fmt.Errorf("gagal update memory: %w", err)
	}

	// Create version record for the rollback
	_, err = s.db.Exec(
		`INSERT INTO memory_versions (memory_id, content, metadata, changed_by, reason, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		memoryID,
		targetVersion.Content,
		targetVersion.Metadata,
		"system",
		fmt.Sprintf("rollback ke versi %d", versionID),
		time.Now(),
	)
	if err != nil {
		// Non-critical error
		fmt.Printf("Warning: gagal menyimpan versi rollback: %v\n", err)
	}

	return nil
}

// CompareVersions compares two versions of a memory and returns the differences.
func (s *SQLiteStore) CompareVersions(memoryID, versionID1, versionID2 int64) ([]VersionDiff, error) {
	if memoryID <= 0 || versionID1 <= 0 || versionID2 <= 0 {
		return nil, fmt.Errorf("ID tidak valid")
	}

	v1, err := s.GetMemoryVersion(memoryID, versionID1)
	if err != nil {
		return nil, fmt.Errorf("gagal mendapatkan versi 1: %w", err)
	}

	v2, err := s.GetMemoryVersion(memoryID, versionID2)
	if err != nil {
		return nil, fmt.Errorf("gagal mendapatkan versi 2: %w", err)
	}

	var diffs []VersionDiff

	// Compare content
	if v1.Content != v2.Content {
		diffs = append(diffs, VersionDiff{
			Field:     "content",
			OldValue:  v1.Content,
			NewValue:  v2.Content,
			ChangedAt: v2.CreatedAt,
		})
	}

	// Compare metadata
	meta1 := parseMetadata(v1.Metadata)
	meta2 := parseMetadata(v2.Metadata)

	// Check for added/changed keys
	for k, v := range meta2 {
		if oldVal, exists := meta1[k]; !exists {
			diffs = append(diffs, VersionDiff{
				Field:     fmt.Sprintf("metadata.%s", k),
				OldValue:  nil,
				NewValue:  v,
				ChangedAt: v2.CreatedAt,
			})
		} else if oldVal != v {
			diffs = append(diffs, VersionDiff{
				Field:     fmt.Sprintf("metadata.%s", k),
				OldValue:  oldVal,
				NewValue:  v,
				ChangedAt: v2.CreatedAt,
			})
		}
	}

	// Check for removed keys
	for k, v := range meta1 {
		if _, exists := meta2[k]; !exists {
			diffs = append(diffs, VersionDiff{
				Field:     fmt.Sprintf("metadata.%s", k),
				OldValue:  v,
				NewValue:  nil,
				ChangedAt: v2.CreatedAt,
			})
		}
	}

	return diffs, nil
}

// GetVersionHistory returns a formatted version history for a memory.
func (s *SQLiteStore) GetVersionHistory(memoryID int64) (string, error) {
	versions, err := s.GetMemoryVersions(memoryID)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Riwayat Versi Memory #%d ===\n\n", memoryID))

	for i, v := range versions {
		sb.WriteString(fmt.Sprintf("Versi %d (ID: %d)\n", len(versions)-i, v.ID))
		sb.WriteString(fmt.Sprintf("  Diubah oleh: %s\n", v.ChangedBy))
		sb.WriteString(fmt.Sprintf("  Alasan: %s\n", v.Reason))
		sb.WriteString(fmt.Sprintf("  Tanggal: %s\n", v.CreatedAt.Format("2006-01-02 15:04:05")))
		
		// Show content preview
		contentPreview := v.Content
		if len(contentPreview) > 100 {
			contentPreview = contentPreview[:100] + "..."
		}
		sb.WriteString(fmt.Sprintf("  Konten: %s\n", contentPreview))
		
		// Show metadata if present
		if v.Metadata != "{}" && v.Metadata != "" {
			sb.WriteString(fmt.Sprintf("  Metadata: %s\n", v.Metadata))
		}
		
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// AutoVersion creates a new version before updating a memory.
func (s *SQLiteStore) AutoVersion(memoryID int64, reason string) error {
	if memoryID <= 0 {
		return fmt.Errorf("ID memory tidak valid")
	}

	// Get current memory
	current, err := s.GetMemoryByID(memoryID)
	if err != nil {
		return fmt.Errorf("gagal membaca memory: %w", err)
	}
	if current == nil {
		return fmt.Errorf("memory tidak ditemukan")
	}

	// Create version record
	_, err = s.db.Exec(
		`INSERT INTO memory_versions (memory_id, content, metadata, changed_by, reason, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		memoryID,
		current.Content,
		formatMetadata(current.Metadata),
		"system",
		reason,
		time.Now(),
	)
	if err != nil {
		return fmt.Errorf("gagal membuat versi: %w", err)
	}

	return nil
}

// GetLatestVersion returns the most recent version of a memory.
func (s *SQLiteStore) GetLatestVersion(memoryID int64) (*MemoryVersion, error) {
	if memoryID <= 0 {
		return nil, fmt.Errorf("ID memory tidak valid")
	}

	var v MemoryVersion
	row := s.db.QueryRow(
		`SELECT id, memory_id, content, metadata, changed_by, reason, created_at
		 FROM memory_versions 
		 WHERE memory_id = ? 
		 ORDER BY created_at DESC 
		 LIMIT 1`,
		memoryID,
	)

	if err := row.Scan(&v.ID, &v.MemoryID, &v.Content, &v.Metadata, &v.ChangedBy, &v.Reason, &v.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("tidak ada versi untuk memory ini")
		}
		return nil, fmt.Errorf("gagal membaca versi: %w", err)
	}

	return &v, nil
}

// formatMetadata converts metadata map to JSON string
func formatMetadata(metadata map[string]interface{}) string {
	if metadata == nil {
		return "{}"
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// parseMetadata converts JSON string to metadata map
func parseMetadata(metadataStr string) map[string]interface{} {
	if metadataStr == "" || metadataStr == "{}" {
		return make(map[string]interface{})
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(metadataStr), &metadata); err != nil {
		return make(map[string]interface{})
	}
	return metadata
}

// PruneOldVersions removes old versions of memories, keeping only the most recent N versions.
func (s *SQLiteStore) PruneOldVersions(memoryID int64, keepVersions int) (int, error) {
	if memoryID <= 0 {
		return 0, fmt.Errorf("ID memory tidak valid")
	}

	if keepVersions <= 0 {
		keepVersions = 10 // Default to keeping 10 versions
	}

	// Get all version IDs for this memory, ordered by creation date
	rows, err := s.db.Query(
		`SELECT id FROM memory_versions WHERE memory_id = ? ORDER BY created_at DESC`,
		memoryID,
	)
	if err != nil {
		return 0, fmt.Errorf("gagal query versi: %w", err)
	}
	defer rows.Close()

	var versionIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			continue
		}
		versionIDs = append(versionIDs, id)
	}

	if len(versionIDs) <= keepVersions {
		return 0, nil // Nothing to prune
	}

	// Delete old versions (keeping only the most recent N)
	versionsToDelete := versionIDs[keepVersions:]
	placeholders := ""
	args := []interface{}{}
	for i, id := range versionsToDelete {
		if i > 0 {
			placeholders += ","
		}
		placeholders += fmt.Sprintf("$%d", i+1)
		args = append(args, id)
	}

	query := fmt.Sprintf("DELETE FROM memory_versions WHERE id IN (%s)", placeholders)
	result, err := s.db.Exec(query, args...)
	if err != nil {
		return 0, fmt.Errorf("gagal hapus versi lama: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	return int(rowsAffected), nil
}

// RestoreFromVersion restores a memory to the state of a specific version.
// This is similar to RollbackMemory but creates a new version instead of updating the existing one.
func (s *SQLiteStore) RestoreFromVersion(memoryID, versionID int64) error {
	if memoryID <= 0 || versionID <= 0 {
		return fmt.Errorf("ID tidak valid")
	}

	// Get the version to restore
	version, err := s.GetMemoryVersion(memoryID, versionID)
	if err != nil {
		return fmt.Errorf("gagal mendapatkan versi: %w", err)
	}

	// Get current memory
	current, err := s.GetMemoryByID(memoryID)
	if err != nil {
		return fmt.Errorf("gagal membaca memory: %w", err)
	}
	if current == nil {
		return fmt.Errorf("memory tidak ditemukan")
	}

	// Create a new version from current state
	_, err = s.db.Exec(
		`INSERT INTO memory_versions (memory_id, content, metadata, changed_by, reason, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		memoryID,
		current.Content,
		formatMetadata(current.Metadata),
		"system",
		"pre-restore backup",
		time.Now(),
	)
	if err != nil {
		fmt.Printf("Warning: gagal menyimpan versi pre-restore: %v\n", err)
	}

	// Update memory with version content
	_, err = s.db.Exec(
		`UPDATE memories 
		 SET content = ?, metadata = ?, updated_at = CURRENT_TIMESTAMP, version = version + 1 
		 WHERE id = ?`,
		version.Content,
		version.Metadata,
		memoryID,
	)
	if err != nil {
		return fmt.Errorf("gagal restore memory: %w", err)
	}

	// Create version record for the restore
	_, err = s.db.Exec(
		`INSERT INTO memory_versions (memory_id, content, metadata, changed_by, reason, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		memoryID,
		version.Content,
		version.Metadata,
		"system",
		fmt.Sprintf("restore dari versi %d", versionID),
		time.Now(),
	)
	if err != nil {
		fmt.Printf("Warning: gagal menyimpan versi restore: %v\n", err)
	}

	return nil
}

// GenerateVersionReport generates a detailed report of version changes.
func (s *SQLiteStore) GenerateVersionReport(memoryID int64) (string, error) {
	versions, err := s.GetMemoryVersions(memoryID)
	if err != nil {
		return "", err
	}

	if len(versions) < 2 {
		return "Tidak cukup versi untuk membuat laporan perbandingan", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Laporan Perubahan Memory #%d ===\n\n", memoryID))

	for i := 0; i < len(versions)-1; i++ {
		current := versions[i]
		next := versions[i+1]

		diffs, err := s.CompareVersions(memoryID, current.ID, next.ID)
		if err != nil {
			continue
		}

		sb.WriteString(fmt.Sprintf("Perubahan dari Versi %d ke Versi %d:\n", 
			len(versions)-i, len(versions)-i-1))
		sb.WriteString(fmt.Sprintf("  Tanggal: %s\n", next.CreatedAt.Format("2006-01-02 15:04")))
		sb.WriteString(fmt.Sprintf("  Alasan: %s\n", next.Reason))

		if len(diffs) == 0 {
			sb.WriteString("  Tidak ada perubahan\n")
		} else {
			for _, diff := range diffs {
				sb.WriteString(fmt.Sprintf("  - %s:\n", diff.Field))
				sb.WriteString(fmt.Sprintf("    Dari: %v\n", diff.OldValue))
				sb.WriteString(fmt.Sprintf("    Ke:   %v\n", diff.NewValue))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}