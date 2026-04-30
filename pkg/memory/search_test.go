package memory

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCosineSimilarity_Identical(t *testing.T) {
	v := []float32{1, 2, 3}
	result := cosineSimilarity(v, v)
	assert.InDelta(t, 1.0, result, 0.0001)
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{0, 1, 0}
	result := cosineSimilarity(a, b)
	assert.InDelta(t, 0.0, result, 0.0001)
}

func TestCosineSimilarity_Opposite(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{-1, -2, -3}
	result := cosineSimilarity(a, b)
	assert.InDelta(t, -1.0, result, 0.0001)
}

func TestCosineSimilarity_DifferentLengths(t *testing.T) {
	a := []float32{1, 2}
	b := []float32{1, 2, 3}
	result := cosineSimilarity(a, b)
	assert.Equal(t, 0.0, result)
}

func TestCosineSimilarity_Empty(t *testing.T) {
	result := cosineSimilarity([]float32{}, []float32{})
	assert.Equal(t, 0.0, result)
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float32{0, 0, 0}
	b := []float32{1, 2, 3}
	result := cosineSimilarity(a, b)
	assert.Equal(t, 0.0, result)
}

func TestSortBySimDesc(t *testing.T) {
	results := []SearchResult{
		{Similarity: 0.3},
		{Similarity: 0.9},
		{Similarity: 0.5},
		{Similarity: 0.1},
	}
	sortBySimDesc(results)

	assert.InDelta(t, 0.9, results[0].Similarity, 0.0001)
	assert.InDelta(t, 0.5, results[1].Similarity, 0.0001)
	assert.InDelta(t, 0.3, results[2].Similarity, 0.0001)
	assert.InDelta(t, 0.1, results[3].Similarity, 0.0001)
}

func TestSortBySimDesc_Empty(t *testing.T) {
	results := []SearchResult{}
	sortBySimDesc(results)
	assert.Empty(t, results)
}

func TestSortBySimDesc_Single(t *testing.T) {
	results := []SearchResult{{Similarity: 0.5}}
	sortBySimDesc(results)
	assert.Len(t, results, 1)
	assert.InDelta(t, 0.5, results[0].Similarity, 0.0001)
}

func TestBlobToFloat32(t *testing.T) {
	// Create a BLOB from 2 float32 values
	v1 := float32(1.5)
	v2 := float32(-2.5)
	blob := make([]byte, 8)
	// Manually encode little-endian float32
	bits1 := math.Float32bits(v1)
	bits2 := math.Float32bits(v2)
	blob[0] = byte(bits1)
	blob[1] = byte(bits1 >> 8)
	blob[2] = byte(bits1 >> 16)
	blob[3] = byte(bits1 >> 24)
	blob[4] = byte(bits2)
	blob[5] = byte(bits2 >> 8)
	blob[6] = byte(bits2 >> 16)
	blob[7] = byte(bits2 >> 24)

	result := blobToFloat32(blob)
	assert.Len(t, result, 2)
	assert.InDelta(t, 1.5, result[0], 0.0001)
	assert.InDelta(t, -2.5, result[1], 0.0001)
}

func TestBlobToFloat32_Empty(t *testing.T) {
	result := blobToFloat32([]byte{})
	assert.Empty(t, result)
}

func TestBlobToFloat32_Partial(t *testing.T) {
	// 5 bytes -> only 1 float32 (truncated, but should not panic)
	blob := []byte{0, 0, 0, 0, 0}
	result := blobToFloat32(blob)
	assert.Len(t, result, 1)
}
