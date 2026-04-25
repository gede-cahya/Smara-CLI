package memory

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// searchByEmbedding performs vector similarity search using cosine similarity within a workspace.
func searchByEmbedding(db *sql.DB, queryEmbedding []float32, workspaceID int64, topK int) ([]SearchResult, error) {
	if topK <= 0 {
		topK = 5
	}

	rows, err := db.Query(
		"SELECT id, workspace_id, content, embedding, tags, source, created_at, updated_at, expires_at, category_id, metadata, version FROM memories WHERE (workspace_id = ? OR workspace_id IS NULL) AND embedding IS NOT NULL",
		workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("gagal query untuk search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var m Memory
		var embBlob []byte
		var tagsJSON, metadataJSON sql.NullString
		var expiresAt sql.NullTime
		var categoryID sql.NullInt64
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.Content, &embBlob, &tagsJSON, &m.Source, &m.CreatedAt, &m.UpdatedAt, &expiresAt, &categoryID, &metadataJSON, &m.Version); err != nil {
			return nil, fmt.Errorf("gagal scan memory: %w", err)
		}

		if len(embBlob) == 0 {
			continue
		}

		storedEmb := blobToFloat32(embBlob)
		sim := cosineSimilarity(queryEmbedding, storedEmb)

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

		results = append(results, SearchResult{
			Memory:     m,
			Similarity: sim,
			Score:      sim,
		})
	}

	// Sort by similarity descending and return top K
	sortBySimDesc(results)
	if len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}

// cosineSimilarity computes cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// SearchHybrid combines vector and full-text search.
func (s *SQLiteStore) SearchHybrid(query string, embedding []float32, workspaceID int64, topK int) ([]SearchResult, error) {
	// Get vector search results
	vectorResults, err := s.Search(embedding, workspaceID, topK*2)
	if err != nil {
		return nil, fmt.Errorf("gagal search vektor: %w", err)
	}

	// Get full-text search results
	ftsResults, err := s.SearchFullText(query, workspaceID, MemoryFilters{
		SearchFilters: SearchFilters{},
		Limit:         topK * 2,
	})
	if err != nil {
		return nil, fmt.Errorf("gagal search teks: %w", err)
	}

	// Create a map to combine results
	resultMap := make(map[int64]*SearchResult)

	// Add vector results with weight 0.6
	for _, r := range vectorResults {
		score := r.Similarity * 0.6
		resultMap[r.Memory.ID] = &SearchResult{
			Memory:     r.Memory,
			Similarity: r.Similarity,
			Score:      score,
		}
	}

	// Add FTS results with weight 0.4
	// For FTS, we need to calculate a relevance score
	for i, m := range ftsResults {
		// Calculate a simple relevance score based on position (earlier = better)
		ftsScore := 1.0 - (float64(i) / float64(len(ftsResults)))
		if existing, ok := resultMap[m.ID]; ok {
			// Combine scores
			existing.Score += ftsScore * 0.4
		} else {
			resultMap[m.ID] = &SearchResult{
				Memory:     m,
				Similarity: 0,
				Score:      ftsScore * 0.4,
			}
		}
	}

	// Convert map to slice
	var combined []SearchResult
	for _, r := range resultMap {
		combined = append(combined, *r)
	}

	// Sort by combined score
	sort.Slice(combined, func(i, j int) bool {
		return combined[i].Score > combined[j].Score
	})

	// Limit to topK
	if len(combined) > topK {
		combined = combined[:topK]
	}

	return combined, nil
}

// mergeResults combines vector and full-text search results with weighted scoring.
func mergeResults(vectorResults []SearchResult, ftsResults []Memory, vectorWeight, ftsWeight float64) []SearchResult {
	resultMap := make(map[int64]*SearchResult)

	// Add vector results with weight
	for _, r := range vectorResults {
		score := r.Similarity * vectorWeight
		resultMap[r.Memory.ID] = &SearchResult{
			Memory:     r.Memory,
			Similarity: r.Similarity,
			Score:      score,
		}
	}

	// Add FTS results with weight
	for i, m := range ftsResults {
		// Calculate relevance score based on position (earlier = better)
		ftsScore := 1.0 - (float64(i) / float64(len(ftsResults)))
		if existing, ok := resultMap[m.ID]; ok {
			// Combine scores
			existing.Score += ftsScore * ftsWeight
		} else {
			resultMap[m.ID] = &SearchResult{
				Memory:     m,
				Similarity: 0,
				Score:      ftsScore * ftsWeight,
			}
		}
	}

	// Convert map to slice
	var combined []SearchResult
	for _, r := range resultMap {
		combined = append(combined, *r)
	}

	// Sort by combined score
	sort.Slice(combined, func(i, j int) bool {
		return combined[i].Score > combined[j].Score
	})

	return combined
}

// sortBySimDesc sorts search results by similarity score in descending order.
func sortBySimDesc(results []SearchResult) {
	// Simple insertion sort (fine for small result sets)
	for i := 1; i < len(results); i++ {
		key := results[i]
		j := i - 1
		for j >= 0 && results[j].Similarity < key.Similarity {
			results[j+1] = results[j]
			j--
		}
		results[j+1] = key
	}
}

func blobToFloat32(data []byte) []float32 {
	floats := make([]float32, len(data)/4)
	for i := range floats {
		floats[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return floats
}
