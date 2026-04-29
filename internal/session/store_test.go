package session

import (
	"fmt"
	"testing"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *SQLiteStore {
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	return store
}

func TestNewSQLiteStore_Init(t *testing.T) {
	store := newTestStore(t)
	assert.NotNil(t, store)
}

func TestSQLiteStore_CreateAndGet(t *testing.T) {
	store := newTestStore(t)

	now := time.Now()
	original := &Session{
		ID:          "sess-1",
		WorkspaceID: 42,
		Name:        "test-session",
		State:       StateActive,
		Mode:        "ask",
		MCPServers:  []string{"server1"},
		History:     []llm.Message{{Role: llm.RoleUser, Content: "hello"}},
		Tasks:       []Task{{ID: "task-1", Description: "test", Status: TaskPending}},
		MemoryIDs:   []int64{1, 2},
		Context:     "project context",
		IsAgentic:   true,
		AutoResume:  true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	err := store.CreateSession(original)
	require.NoError(t, err)

	retrieved, err := store.GetSession("sess-1")
	require.NoError(t, err)
	assert.Equal(t, "sess-1", retrieved.ID)
	assert.Equal(t, int64(42), retrieved.WorkspaceID)
	assert.Equal(t, "test-session", retrieved.Name)
	assert.Equal(t, StateActive, retrieved.State)
	assert.Equal(t, "ask", retrieved.Mode)
	assert.Equal(t, []string{"server1"}, retrieved.MCPServers)
	assert.Len(t, retrieved.History, 1)
	assert.Equal(t, "hello", retrieved.History[0].Content)
	assert.Len(t, retrieved.Tasks, 1)
	assert.Equal(t, "task-1", retrieved.Tasks[0].ID)
	assert.Equal(t, []int64{1, 2}, retrieved.MemoryIDs)
	assert.Equal(t, "project context", retrieved.Context)
	assert.True(t, retrieved.IsAgentic)
	assert.True(t, retrieved.AutoResume)
}

func TestSQLiteStore_Get_NotFound(t *testing.T) {
	store := newTestStore(t)
	_, err := store.GetSession("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tidak ditemukan")
}

func TestSQLiteStore_Update(t *testing.T) {
	store := newTestStore(t)

	session := &Session{
		ID:        "sess-1",
		Name:      "original",
		State:     StateActive,
		Mode:      "ask",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := store.CreateSession(session)
	require.NoError(t, err)

	session.Name = "updated"
	session.State = StatePaused
	err = store.UpdateSession(session)
	require.NoError(t, err)

	retrieved, err := store.GetSession("sess-1")
	require.NoError(t, err)
	assert.Equal(t, "updated", retrieved.Name)
	assert.Equal(t, StatePaused, retrieved.State)
}

func TestSQLiteStore_Delete(t *testing.T) {
	store := newTestStore(t)

	session := &Session{
		ID:        "sess-1",
		Name:      "to-delete",
		State:     StateActive,
		Mode:      "ask",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := store.CreateSession(session)
	require.NoError(t, err)

	err = store.DeleteSession("sess-1")
	require.NoError(t, err)

	_, err = store.GetSession("sess-1")
	assert.Error(t, err)
}

func TestSQLiteStore_Delete_NotFound(t *testing.T) {
	store := newTestStore(t)
	err := store.DeleteSession("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tidak ditemukan")
}

func TestSQLiteStore_List(t *testing.T) {
	store := newTestStore(t)

	for i := 1; i <= 3; i++ {
		session := &Session{
			ID:          fmt.Sprintf("sess-%d", i),
			WorkspaceID: int64(i),
			Name:        fmt.Sprintf("session-%d", i),
			State:       StateActive,
			Mode:        "ask",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		err := store.CreateSession(session)
		require.NoError(t, err)
	}

	sessions, err := store.ListSessions()
	require.NoError(t, err)
	assert.Len(t, sessions, 3)
}

func TestSQLiteStore_ListByWorkspace(t *testing.T) {
	store := newTestStore(t)

	store.CreateSession(&Session{
		ID:          "sess-1",
		WorkspaceID: 1,
		Name:        "ws1-a",
		State:       StateActive,
		Mode:        "ask",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	})
	store.CreateSession(&Session{
		ID:          "sess-2",
		WorkspaceID: 1,
		Name:        "ws1-b",
		State:       StateActive,
		Mode:        "ask",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	})
	store.CreateSession(&Session{
		ID:          "sess-3",
		WorkspaceID: 2,
		Name:        "ws2-a",
		State:       StateActive,
		Mode:        "ask",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	})

	sessions, err := store.ListSessionsByWorkspace(1)
	require.NoError(t, err)
	assert.Len(t, sessions, 2)

	sessions, err = store.ListSessionsByWorkspace(2)
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "ws2-a", sessions[0].Name)
}
