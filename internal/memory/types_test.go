package memory

import (
	"testing"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/stretchr/testify/assert"
)

func TestMemory_Struct(t *testing.T) {
	now := time.Now()
	catID := int64(5)
	m := Memory{
		ID:          1,
		WorkspaceID: 42,
		CategoryID:  &catID,
		Content:     "test memory",
		Embedding:   []float32{0.1, 0.2, 0.3},
		Tags:        []string{"tag1", "tag2"},
		Source:      "user",
		Metadata:    map[string]interface{}{"key": "value"},
		IsArchived:  false,
		CreatedAt:   now,
		UpdatedAt:   now,
		Version:     1,
	}
	assert.Equal(t, int64(1), m.ID)
	assert.Equal(t, int64(42), m.WorkspaceID)
	assert.Equal(t, int64(5), *m.CategoryID)
	assert.Equal(t, "test memory", m.Content)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, m.Embedding)
	assert.Equal(t, []string{"tag1", "tag2"}, m.Tags)
	assert.Equal(t, "user", m.Source)
	assert.Equal(t, "value", m.Metadata["key"])
	assert.False(t, m.IsArchived)
	assert.Equal(t, 1, m.Version)
	assert.Nil(t, m.ExpiresAt)
	assert.Nil(t, m.ArchivedAt)
}

func TestMemory_NilCategoryID(t *testing.T) {
	m := Memory{CategoryID: nil}
	assert.Nil(t, m.CategoryID)
}

func TestWorkspace_Struct(t *testing.T) {
	now := time.Now()
	w := Workspace{
		ID:         1,
		Name:       "test-workspace",
		Path:       "/tmp/test",
		IsArchived: false,
		CreatedAt:  now,
	}
	assert.Equal(t, int64(1), w.ID)
	assert.Equal(t, "test-workspace", w.Name)
	assert.Equal(t, "/tmp/test", w.Path)
	assert.False(t, w.IsArchived)
	assert.Nil(t, w.ArchivedAt)
}

func TestCategory_Struct(t *testing.T) {
	now := time.Now()
	parentID := int64(0)
	c := Category{
		ID:          1,
		WorkspaceID: 42,
		Name:        "test-category",
		Description: "A test category",
		ParentID:    &parentID,
		CreatedAt:   now,
	}
	assert.Equal(t, int64(1), c.ID)
	assert.Equal(t, int64(42), c.WorkspaceID)
	assert.Equal(t, "test-category", c.Name)
	assert.Equal(t, "A test category", c.Description)
	assert.Equal(t, int64(0), *c.ParentID)
}

func TestMemoryVersion_Struct(t *testing.T) {
	now := time.Now()
	v := MemoryVersion{
		ID:        1,
		MemoryID:  42,
		Content:   "old content",
		Metadata:  `{"key":"old"}`,
		ChangedBy: "user",
		Reason:    "manual update",
		CreatedAt: now,
	}
	assert.Equal(t, int64(1), v.ID)
	assert.Equal(t, int64(42), v.MemoryID)
	assert.Equal(t, "old content", v.Content)
	assert.Equal(t, `{"key":"old"}`, v.Metadata)
	assert.Equal(t, "user", v.ChangedBy)
	assert.Equal(t, "manual update", v.Reason)
}

func TestSyncEntry_Struct(t *testing.T) {
	now := time.Now()
	s := SyncEntry{
		ID:        1,
		MemoryID:  42,
		DeltaHash: "abc123",
		Status:    SyncPending,
		SyncedAt:  now,
	}
	assert.Equal(t, int64(1), s.ID)
	assert.Equal(t, int64(42), s.MemoryID)
	assert.Equal(t, "abc123", s.DeltaHash)
	assert.Equal(t, SyncPending, s.Status)
	assert.Equal(t, now, s.SyncedAt)
}

func TestSyncStatus_Constants(t *testing.T) {
	assert.Equal(t, SyncStatus("pending"), SyncPending)
	assert.Equal(t, SyncStatus("complete"), SyncComplete)
	assert.Equal(t, SyncStatus("failed"), SyncFailed)
}

func TestSearchResult_Struct(t *testing.T) {
	m := Memory{ID: 1, Content: "hello"}
	sr := SearchResult{
		Memory:     m,
		Similarity: 0.95,
		Score:      0.88,
	}
	assert.Equal(t, int64(1), sr.Memory.ID)
	assert.Equal(t, 0.95, sr.Similarity)
	assert.Equal(t, 0.88, sr.Score)
}

func TestSearchFilters_Struct(t *testing.T) {
	now := time.Now()
	catID := int64(1)
	f := SearchFilters{
		Tags:       []string{"tag1"},
		Sources:    []string{"user"},
		DateFrom:   &now,
		DateTo:     &now,
		CategoryID: &catID,
		MinScore:   0.5,
	}
	assert.Equal(t, []string{"tag1"}, f.Tags)
	assert.Equal(t, []string{"user"}, f.Sources)
	assert.Equal(t, &now, f.DateFrom)
	assert.Equal(t, &catID, f.CategoryID)
	assert.Equal(t, 0.5, f.MinScore)
}

func TestMemoryFilters_Struct(t *testing.T) {
	f := MemoryFilters{
		Limit:   10,
		Offset:  5,
		SortBy:  "created_at",
		SortDir: "DESC",
	}
	assert.Equal(t, 10, f.Limit)
	assert.Equal(t, 5, f.Offset)
	assert.Equal(t, "created_at", f.SortBy)
	assert.Equal(t, "DESC", f.SortDir)
}

func TestMessageToJSON(t *testing.T) {
	m := llm.Message{Role: llm.RoleUser, Content: "hello"}
	jsonStr := MessageToJSON(m)
	assert.Contains(t, jsonStr, "user")
	assert.Contains(t, jsonStr, "hello")
}

func TestMessageFromJSON(t *testing.T) {
	jsonStr := `{"role":"assistant","content":"hi there"}`
	m, err := MessageFromJSON(jsonStr)
	assert.NoError(t, err)
	assert.Equal(t, llm.RoleAssistant, m.Role)
	assert.Equal(t, "hi there", m.Content)
}

func TestMessageFromJSON_Invalid(t *testing.T) {
	_, err := MessageFromJSON("not json")
	assert.Error(t, err)
}

func TestMessageRoundTrip(t *testing.T) {
	original := llm.Message{
		Role:       llm.RoleTool,
		Content:    "tool result",
		ToolCallID: "call_1",
	}
	jsonStr := MessageToJSON(original)
	decoded, err := MessageFromJSON(jsonStr)
	assert.NoError(t, err)
	assert.Equal(t, original.Role, decoded.Role)
	assert.Equal(t, original.Content, decoded.Content)
	assert.Equal(t, original.ToolCallID, decoded.ToolCallID)
}
