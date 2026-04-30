package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorker_DefaultFields(t *testing.T) {
	w := NewWorker(nil, nil)
	assert.Empty(t, w.Role)
	assert.Empty(t, w.AllowedTools)
	assert.Empty(t, w.SystemPrompt)
}

func TestNewSpecializedWorker(t *testing.T) {
	w := NewSpecializedWorker(nil, nil, "frontend", []string{"stitch", "figma"}, "You are FE")
	assert.Equal(t, "frontend", w.Role)
	assert.Equal(t, []string{"stitch", "figma"}, w.AllowedTools)
	assert.Equal(t, "You are FE", w.SystemPrompt)
}

func TestContains(t *testing.T) {
	assert.True(t, contains([]string{"a", "b", "c"}, "b"))
	assert.False(t, contains([]string{"a", "b"}, "c"))
	assert.False(t, contains([]string{}, "a"))
	assert.True(t, contains([]string{"single"}, "single"))
}
