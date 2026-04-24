package memory

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
)

// searchByEmbedding performs vector similarity search using cosine similarity within a workspace.
func searchByEmbedding(db *sql.DB, queryEmbedding []float32, workspaceID int64, topK int) ([]SearchResult, error) {
	if topK <= 0 {
		topK = 5
	}

	rows, err := db.Query(
		"SELECT id, workspace_id, content, embedding, tags, source, created_at FROM memories WHERE (workspace_id = ? OR workspace_id IS NULL) AND embedding IS NOT NULL",
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
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.Content, &embBlob, &m.Tags, &m.Source, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("gagal scan memory: %w", err)
		}

		if len(embBlob) == 0 {
			continue
		}

		storedEmb := blobToFloat32(embBlob)
		sim := cosineSimilarity(queryEmbedding, storedEmb)

		results = append(results, SearchResult{
			Memory:     m,
			Similarity: sim,
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
