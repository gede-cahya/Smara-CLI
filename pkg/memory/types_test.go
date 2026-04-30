package memory

import (
	"testing"
	"time"

	"github.com/gede-cahya/Smara-CLI/pkg/llm"
	"github.com/stretchr/testify/assert"
)

func TestSyncStatus_Constants(t *testing.T) {
	assert.Equal(t, SyncStatus("pending"), SyncPending)
	assert.Equal(t, SyncStatus("complete"), SyncComplete)
	assert.Equal(t, SyncStatus("failed"), SyncFailed)
}

func TestMemory_Struct(t *testing.T) {
	now := time.Now()
	m := Memory{
		ID:         1,
		Content:    "test content",
		Tags:       "tag1,tag2",
		Source:     "user",
		IsArchived: false,
		CreatedAt:  now,
	}
	assert.Equal(t, int64(1), m.ID)
	assert.Equal(t, "test content", m.Content)
	assert.Equal(t, "tag1,tag2", m.Tags)
	assert.False(t, m.IsArchived)
	assert.Nil(t, m.ArchivedAt)
}

func TestWorkspace_Struct(t *testing.T) {
	w := Workspace{
		ID:   42,
		Name: "test-workspace",
		Path: "/tmp/test",
	}
	assert.Equal(t, int64(42), w.ID)
	assert.Equal(t, "test-workspace", w.Name)
	assert.False(t, w.IsArchived)
}

func TestSearchResult_Struct(t *testing.T) {
	m := Memory{ID: 1, Content: "hello"}
	sr := SearchResult{
		Memory:     m,
		Similarity: 0.95,
	}
	assert.Equal(t, int64(1), sr.Memory.ID)
	assert.Equal(t, 0.95, sr.Similarity)
}

func TestSyncEntry_Struct(t *testing.T) {
	se := SyncEntry{
		ID:        1,
		MemoryID:  42,
		DeltaHash: "abc123",
		Status:    SyncComplete,
	}
	assert.Equal(t, int64(1), se.ID)
	assert.Equal(t, int64(42), se.MemoryID)
	assert.Equal(t, SyncComplete, se.Status)
}

func TestMessageToJSON(t *testing.T) {
	msg := llm.Message{Role: llm.RoleUser, Content: "hello"}
	jsonStr := MessageToJSON(msg)
	assert.Contains(t, jsonStr, `"role":"user"`)
	assert.Contains(t, jsonStr, `"content":"hello"`)
}

func TestAttachment_Struct(t *testing.T) {
	// Attachment is defined in platform package, not memory.
	// Skip this test.
}
