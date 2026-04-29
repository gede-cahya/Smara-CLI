package memory

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCosineSimilarity_IdenticalVectors(t *testing.T) {
	a := []float32{1.0, 2.0, 3.0}
	b := []float32{1.0, 2.0, 3.0}
	result := cosineSimilarity(a, b)
	assert.InDelta(t, 1.0, result, 0.0001)
}

func TestCosineSimilarity_OppositeVectors(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{-1.0, 0.0}
	result := cosineSimilarity(a, b)
	assert.InDelta(t, -1.0, result, 0.0001)
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{0.0, 1.0}
	result := cosineSimilarity(a, b)
	assert.InDelta(t, 0.0, result, 0.0001)
}

func TestCosineSimilarity_MismatchedLengths(t *testing.T) {
	a := []float32{1.0, 2.0}
	b := []float32{1.0, 2.0, 3.0}
	result := cosineSimilarity(a, b)
	assert.Equal(t, 0.0, result)
}

func TestCosineSimilarity_EmptyVectors(t *testing.T) {
	a := []float32{}
	b := []float32{}
	result := cosineSimilarity(a, b)
	assert.Equal(t, 0.0, result)
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float32{0.0, 0.0}
	b := []float32{1.0, 1.0}
	result := cosineSimilarity(a, b)
	assert.Equal(t, 0.0, result)
}

func TestBlobToFloat32_RoundTrip(t *testing.T) {
	original := []float32{1.5, 2.5, 3.5}
	// Convert to bytes manually
	bytes := make([]byte, len(original)*4)
	for i, f := range original {
		binary.LittleEndian.PutUint32(bytes[i*4:], math.Float32bits(f))
	}
	result := blobToFloat32(bytes)
	assert.Equal(t, len(original), len(result))
	for i := range original {
		assert.InDelta(t, original[i], result[i], 0.0001)
	}
}

func TestBlobToFloat32_Empty(t *testing.T) {
	result := blobToFloat32([]byte{})
	assert.Empty(t, result)
}
