package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// SearchFullText searches memories using full-text search with FTS5.
// This is a wrapper around the store method for convenience.
func (s *SQLiteStore) SearchFullText(query string, workspaceID int64, filters MemoryFilters) ([]Memory, error) {
	// Check if FTS5 table exists and has data
	var ftsCount int
	err := s.db.QueryRow("SELECT count(*) FROM memories_fts").Scan(&ftsCount)
	useFTS := err == nil && ftsCount > 0

	var rows *sql.Rows
	var errQuery error

	if useFTS {
		// Use FTS5 for full-text search
		queryStr := `
			SELECT m.id, m.workspace_id, m.content, m.embedding, m.tags, m.source, m.created_at, m.updated_at, m.expires_at, m.category_id, m.metadata, m.version,
				bm25(memories_fts) as rank
			FROM memories m
			JOIN memories_fts ON m.id = memories_fts.rowid
			WHERE memories_fts MATCH ?`
		args := []interface{}{query}

		if workspaceID > 0 {
			queryStr += fmt.Sprintf(" AND (m.workspace_id = $%d OR m.workspace_id IS NULL)", len(args)+1)
			args = append(args, workspaceID)
		}

		// Add filters
		if len(filters.Tags) > 0 {
			tagsJSON, _ := json.Marshal(filters.Tags)
			queryStr += fmt.Sprintf(" AND json_extract(m.tags, '$') LIKE $%d", len(args)+1)
			args = append(args, "%"+string(tagsJSON)+"%")
		}

		if len(filters.Sources) > 0 {
			placeholders := ""
			for i, src := range filters.Sources {
				if i > 0 {
					placeholders += ","
				}
				placeholders += fmt.Sprintf("$%d", len(args)+1)
				args = append(args, src)
			}
			queryStr += fmt.Sprintf(" AND m.source IN (%s)", placeholders)
		}

		if filters.DateFrom != nil {
			queryStr += fmt.Sprintf(" AND m.created_at >= $%d", len(args)+1)
			args = append(args, *filters.DateFrom)
		}

		if filters.DateTo != nil {
			queryStr += fmt.Sprintf(" AND m.created_at <= $%d", len(args)+1)
			args = append(args, *filters.DateTo)
		}

		if filters.CategoryID != nil {
			queryStr += fmt.Sprintf(" AND m.category_id = $%d", len(args)+1)
			args = append(args, *filters.CategoryID)
		}

		queryStr += " ORDER BY rank ASC"

		if filters.MinScore > 0 {
			// FTS5 rank is negative, lower is better
			queryStr += fmt.Sprintf(" AND bm25(memories_fts) < $%d", len(args)+1)
			args = append(args, -filters.MinScore)
		}

		if filters.Limit > 0 {
			queryStr += fmt.Sprintf(" LIMIT $%d", len(args)+1)
			args = append(args, filters.Limit)
		}

		rows, errQuery = s.db.Query(queryStr, args...)
	} else {
		// Fallback to LIKE query
		queryStr := `
			SELECT id, workspace_id, content, embedding, tags, source, created_at, updated_at, expires_at, category_id, metadata, version
			FROM memories
			WHERE (content LIKE ? OR tags LIKE ? OR source LIKE ?)`
		args := []interface{}{"%" + query + "%", "%" + query + "%", "%" + query + "%"}

		if workspaceID > 0 {
			queryStr += fmt.Sprintf(" AND (workspace_id = $%d OR workspace_id IS NULL)", len(args)+1)
			args = append(args, workspaceID)
		}

		if len(filters.Tags) > 0 {
			tagsJSON, _ := json.Marshal(filters.Tags)
			queryStr += fmt.Sprintf(" AND tags LIKE $%d", len(args)+1)
			args = append(args, "%"+string(tagsJSON)+"%")
		}

		if len(filters.Sources) > 0 {
			placeholders := ""
			for i, src := range filters.Sources {
				if i > 0 {
					placeholders += ","
				}
				placeholders += fmt.Sprintf("$%d", len(args)+1)
				args = append(args, src)
			}
			queryStr += fmt.Sprintf(" AND source IN (%s)", placeholders)
		}

		if filters.DateFrom != nil {
			queryStr += fmt.Sprintf(" AND created_at >= $%d", len(args)+1)
			args = append(args, *filters.DateFrom)
		}

		if filters.DateTo != nil {
			queryStr += fmt.Sprintf(" AND created_at <= $%d", len(args)+1)
			args = append(args, *filters.DateTo)
		}

		if filters.CategoryID != nil {
			queryStr += fmt.Sprintf(" AND category_id = $%d", len(args)+1)
			args = append(args, *filters.CategoryID)
		}

		queryStr += " ORDER BY created_at DESC"

		if filters.Limit > 0 {
			queryStr += fmt.Sprintf(" LIMIT $%d", len(args)+1)
			args = append(args, filters.Limit)
		}

		rows, errQuery = s.db.Query(queryStr, args...)
	}

	if errQuery != nil {
		return nil, fmt.Errorf("gagal query full-text search: %w", errQuery)
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var m Memory
		var embBlob []byte
		var tagsJSON, metadataJSON sql.NullString
		var expiresAt sql.NullTime
		var categoryID sql.NullInt64

		if useFTS {
			var rank float64
			if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.Content, &embBlob, &tagsJSON, &m.Source, &m.CreatedAt, &m.UpdatedAt, &expiresAt, &categoryID, &metadataJSON, &m.Version, &rank); err != nil {
				return nil, fmt.Errorf("gagal scan memory FTS: %w", err)
			}
		} else {
			if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.Content, &embBlob, &tagsJSON, &m.Source, &m.CreatedAt, &m.UpdatedAt, &expiresAt, &categoryID, &metadataJSON, &m.Version); err != nil {
				return nil, fmt.Errorf("gagal scan memory LIKE: %w", err)
			}
		}

		if len(embBlob) > 0 {
			m.Embedding = bytesToFloat32(embBlob)
		}
		m.Tags = parseTagsFromJSON(tagsJSON.String)
		if expiresAt.Valid {
			m.ExpiresAt = &expiresAt.Time
		}
		if categoryID.Valid {
			m.CategoryID = &categoryID.Int64
		}
		if metadataJSON.Valid {
			var metadata map[string]interface{}
			if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err == nil {
				m.Metadata = metadata
			}
		}
		memories = append(memories, m)
	}

	return memories, nil
}

// FTS5Available checks if FTS5 is available in the current SQLite build.
func (s *SQLiteStore) FTS5Available() bool {
	var count int
	err := s.db.QueryRow("SELECT count(*) FROM pragma_compile_options WHERE compile_options = 'ENABLE_FTS5'").Scan(&count)
	if err != nil {
		return false
	}
	return count > 0
}

// RebuildFTS5 rebuilds the FTS5 index from the memories table.
func (s *SQLiteStore) RebuildFTS5() error {
	// Check if FTS5 table exists
	var count int
	err := s.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='memories_fts'").Scan(&count)
	if err != nil || count == 0 {
		return fmt.Errorf("tabel FTS5 tidak ditemukan")
	}

	// Rebuild the FTS5 index
	_, err = s.db.Exec("INSERT INTO memories_fts(memories_fts, rowid, content, tags, source) SELECT 'rebuild', id, content, tags, source FROM memories")
	if err != nil {
		return fmt.Errorf("gagal rebuild FTS5: %w", err)
	}

	return nil
}

// OptimizeFTS5 optimizes the FTS5 index for better search performance.
func (s *SQLiteStore) OptimizeFTS5() error {
	_, err := s.db.Exec("INSERT INTO memories_fts(memories_fts) VALUES('optimize')")
	if err != nil {
		return fmt.Errorf("gagal optimize FTS5: %w", err)
	}
	return nil
}

// FTSSearchStats returns statistics about the FTS5 index.
func (s *SQLiteStore) FTSSearchStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	var totalDocs int
	err := s.db.QueryRow("SELECT count(*) FROM memories_fts").Scan(&totalDocs)
	if err != nil {
		return nil, fmt.Errorf("gagal query statistik FTS: %w", err)
	}
	stats["total_documents"] = totalDocs

	var totalSize int64
	err = s.db.QueryRow("SELECT sum(length(content)) + sum(length(tags)) + sum(length(source)) FROM memories_fts").Scan(&totalSize)
	if err != nil {
		return nil, fmt.Errorf("gagal query ukuran FTS: %w", err)
	}
	stats["total_size_bytes"] = totalSize

	return stats, nil
}

// parseQueryTerms breaks down a search query into individual terms for FTS5.
// FTS5 supports quoted phrases, AND/OR operators, and prefix matching.
func parseQueryTerms(query string) string {
	// Trim whitespace
	query = strings.TrimSpace(query)

	// If query contains quotes, keep as is (FTS5 handles phrase search)
	if strings.Contains(query, "\"") {
		return query
	}

	// Split by spaces and join with AND operator
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return ""
	}

	// For single term, use prefix matching
	if len(terms) == 1 {
		return terms[0] + "*"
	}

	// For multiple terms, join with AND
	return strings.Join(terms, " AND ")
}