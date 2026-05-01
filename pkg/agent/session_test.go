package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSessionRegistry(t *testing.T) {
	r := NewSessionRegistry()
	require.NotNil(t, r)
	assert.Nil(t, r.Current())
	assert.Len(t, r.List(), 0)
}

func TestSessionRegistry_CreateAndGet(t *testing.T) {
	r := NewSessionRegistry()
	cfg := SessionConfig{Name: "TestSession", Mode: "ask"}

	sess, err := r.Create(cfg)
	require.NoError(t, err)
	require.NotNil(t, sess)
	assert.NotEmpty(t, sess.ID)
	assert.Equal(t, "TestSession", sess.Name)
	assert.Equal(t, "ask", sess.Mode)

	got, ok := r.Get(sess.ID)
	assert.True(t, ok)
	assert.Equal(t, sess, got)
}

func TestSessionRegistry_Create_SetsCurrent(t *testing.T) {
	r := NewSessionRegistry()
	sess, err := r.Create(SessionConfig{Name: "A", Mode: "ask"})
	require.NoError(t, err)
	assert.Equal(t, sess, r.Current())
}

func TestSessionRegistry_Switch(t *testing.T) {
	r := NewSessionRegistry()
	s1, _ := r.Create(SessionConfig{Name: "First", Mode: "ask"})
	s2, _ := r.Create(SessionConfig{Name: "Second", Mode: "rush"})

	assert.Equal(t, s2, r.Current())

	err := r.Switch(s1.ID)
	require.NoError(t, err)
	assert.Equal(t, s1, r.Current())
}

func TestSessionRegistry_Switch_NotFound(t *testing.T) {
	r := NewSessionRegistry()
	err := r.Switch("nonexistent")
	assert.Error(t, err)
}

func TestSessionRegistry_List(t *testing.T) {
	r := NewSessionRegistry()
	r.Create(SessionConfig{Name: "A", Mode: "ask"})
	r.Create(SessionConfig{Name: "B", Mode: "rush"})

	list := r.List()
	assert.Len(t, list, 2)
}

func TestSessionRegistry_End(t *testing.T) {
	r := NewSessionRegistry()
	sess, _ := r.Create(SessionConfig{Name: "EndTest", Mode: "ask"})

	err := r.End(sess.ID)
	require.NoError(t, err)
	assert.Equal(t, SessionEnded, sess.State)
}

func TestSessionRegistry_End_NotFound(t *testing.T) {
	r := NewSessionRegistry()
	err := r.End("missing")
	assert.Error(t, err)
}

func TestSessionRegistry_EndCurrent(t *testing.T) {
	r := NewSessionRegistry()
	sess, _ := r.Create(SessionConfig{Name: "EndCurr", Mode: "ask"})

	err := r.EndCurrent()
	require.NoError(t, err)
	assert.Equal(t, SessionEnded, sess.State)
}

func TestSessionRegistry_Register(t *testing.T) {
	r := NewSessionRegistry()
	sess := &Session{ID: "manual-123", Name: "Manual", State: SessionActive}

	r.Register(sess)
	assert.Equal(t, sess, r.Current())

	got, ok := r.Get("manual-123")
	assert.True(t, ok)
	assert.Equal(t, sess, got)
}

func TestSessionRegistry_IsCurrent(t *testing.T) {
	r := NewSessionRegistry()
	sess, _ := r.Create(SessionConfig{Name: "Curr", Mode: "ask"})

	assert.True(t, r.IsCurrent(sess.ID))
	assert.False(t, r.IsCurrent("other"))
}

func TestSessionRegistry_UpdateHistory(t *testing.T) {
	r := NewSessionRegistry()
	sess, _ := r.Create(SessionConfig{Name: "Hist", Mode: "ask"})

	r.UpdateHistory(sess.ID, "hi", "hello")
	assert.Len(t, sess.History, 2)
	assert.Equal(t, "hi", sess.History[0].Content)
	assert.Equal(t, "hello", sess.History[1].Content)
}

func TestSessionRegistry_UpdateHistory_NotFound(t *testing.T) {
	r := NewSessionRegistry()
	// Should not panic
	r.UpdateHistory("missing", "a", "b")
}
