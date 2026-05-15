package agent

import (
	"fmt"
	"testing"

	"github.com/gede-cahya/Smara-CLI/pkg/llm"
	"github.com/gede-cahya/Smara-CLI/pkg/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectWorkflowIntent_Triggers(t *testing.T) {
	assert.True(t, DetectWorkflowIntent("buatkan web SaaS restoran"))
	assert.True(t, DetectWorkflowIntent("bikin aplikasi e-commerce"))
	assert.True(t, DetectWorkflowIntent("create a mobile app"))
	assert.True(t, DetectWorkflowIntent("build project dashboard"))
	assert.True(t, DetectWorkflowIntent("generate website portfolio"))
	assert.True(t, DetectWorkflowIntent("buat platform learning"))
	assert.True(t, DetectWorkflowIntent("buatkan system payment"))
}

func TestDetectWorkflowIntent_NoTrigger(t *testing.T) {
	assert.False(t, DetectWorkflowIntent("hello"))
	assert.False(t, DetectWorkflowIntent("what is the weather"))
	assert.False(t, DetectWorkflowIntent("web")) // single keyword not enough
	assert.False(t, DetectWorkflowIntent("app")) // single keyword not enough
	assert.False(t, DetectWorkflowIntent(""))
	assert.False(t, DetectWorkflowIntent("buat makanan")) // only 1 match keyword
}

func TestDetectWorkflowIntent_EdgeCases(t *testing.T) {
	// Case insensitive
	assert.True(t, DetectWorkflowIntent("BUATKAN WEB APPS"))
	// Mixed case
	assert.True(t, DetectWorkflowIntent("Create Saas Platform"))
}

func TestSupervisor_ClearHistory(t *testing.T) {
	s := NewSupervisor(nil, nil)

	// Add some history
	s.AddContext("system instruction")
	s.history = append(s.history, llm.Message{Role: llm.RoleUser, Content: "hello"})
	s.history = append(s.history, llm.Message{Role: llm.RoleAssistant, Content: "hi"})
	require.Len(t, s.history, 3)

	// Create a session so registry has a current session
	sess, err := s.CreateSession(SessionConfig{Name: "Test", Mode: "ask"})
	require.NoError(t, err)
	sess.History = append(sess.History, llm.Message{Role: llm.RoleUser, Content: "session msg"})
	require.Len(t, sess.History, 1)

	// Clear history
	s.ClearHistory()

	// Supervisor history cleared
	assert.Len(t, s.history, 0)
	// Session history cleared
	assert.Len(t, sess.History, 0)
}

func TestSupervisor_ClearHistory_NoSession(t *testing.T) {
	s := NewSupervisor(nil, nil)
	s.history = append(s.history, llm.Message{Role: llm.RoleUser, Content: "hello"})
	require.Len(t, s.history, 1)

	s.ClearHistory()
	assert.Len(t, s.history, 0)
}

// mockSessionStore is a minimal in-memory store for testing.
type mockSessionStore struct {
	sessions map[string]*session.Session
}

func (m *mockSessionStore) CreateSession(s *session.Session) error { m.sessions[s.ID] = s; return nil }
func (m *mockSessionStore) GetSession(id string) (*session.Session, error) {
	s, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return s, nil
}
func (m *mockSessionStore) UpdateSession(s *session.Session) error { m.sessions[s.ID] = s; return nil }
func (m *mockSessionStore) DeleteSession(id string) error          { delete(m.sessions, id); return nil }
func (m *mockSessionStore) ListSessions() ([]session.Session, error) {
	var out []session.Session
	for _, s := range m.sessions {
		out = append(out, *s)
	}
	return out, nil
}
func (m *mockSessionStore) ListSessionsByWorkspace(workspaceID int64) ([]session.Session, error) {
	var out []session.Session
	for _, s := range m.sessions {
		if s.WorkspaceID == workspaceID {
			out = append(out, *s)
		}
	}
	return out, nil
}
func (m *mockSessionStore) ListActiveSessions() ([]session.Session, error)  { return m.ListSessions() }
func (m *mockSessionStore) GetLastActiveSession() (*session.Session, error) { return nil, nil }
func (m *mockSessionStore) GetLastActiveSessionByWorkspace(workspaceID int64) (*session.Session, error) {
	return nil, nil
}
func (m *mockSessionStore) ArchiveSession(id string) error                          { return nil }
func (m *mockSessionStore) UnarchiveSession(id string) error                        { return nil }
func (m *mockSessionStore) ListArchivedSessions(_ int64) ([]session.Session, error) { return nil, nil }
func (m *mockSessionStore) DeleteArchivedSession(id string) error                   { return nil }

func TestSupervisor_SaveSession(t *testing.T) {
	s := NewSupervisor(nil, nil)
	store := &mockSessionStore{sessions: make(map[string]*session.Session)}
	s.SetSessionStore(store)

	sess, err := s.CreateSession(SessionConfig{Name: "Test", Mode: "ask"})
	require.NoError(t, err)
	sess.History = append(sess.History, llm.Message{Role: llm.RoleUser, Content: "msg"})

	err = s.SaveSession()
	require.NoError(t, err)

	stored, err := store.GetSession(sess.ID)
	require.NoError(t, err)
	assert.Len(t, stored.History, 1)
	assert.Equal(t, "msg", stored.History[0].Content)
}

func TestSupervisor_SaveSession_NoStore(t *testing.T) {
	s := NewSupervisor(nil, nil)
	err := s.SaveSession()
	assert.NoError(t, err)
}

func TestSupervisor_SaveSession_NoCurrentSession(t *testing.T) {
	s := NewSupervisor(nil, nil)
	store := &mockSessionStore{sessions: make(map[string]*session.Session)}
	s.SetSessionStore(store)

	err := s.SaveSession()
	assert.NoError(t, err)
}

func TestSessionRegistry_Create_SetsWorkspaceID(t *testing.T) {
	r := NewSessionRegistry()
	cfg := SessionConfig{Name: "Test", WorkspaceID: 42, Mode: "ask"}
	sess, err := r.Create(cfg)
	require.NoError(t, err)
	assert.Equal(t, int64(42), sess.WorkspaceID)
}
