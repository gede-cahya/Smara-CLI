package audit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLogger_CreatesDirAndPath(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := NewLogger(tempDir)
	require.NoError(t, err)
	require.NotNil(t, logger)
	assert.DirExists(t, tempDir)
	assert.Contains(t, logger.GetLogPath(), "audit_")
	require.NoError(t, logger.Close())
}

func TestLogger_LogRoundTrip(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	err = logger.Log(Entry{Type: EntryToolCall, SessionID: "s1", Action: "write_file", Success: true})
	require.NoError(t, err)
	require.NoError(t, logger.Flush())

	entries, err := logger.ReadEntries()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, EntryToolCall, entries[0].Type)
	assert.Equal(t, "s1", entries[0].SessionID)
	assert.NotEmpty(t, entries[0].ID)
}

func TestLogger_LogPrompt(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	logger.LogPrompt("s1", "ws1", "hello world")
	require.NoError(t, logger.Flush())

	entries, err := logger.ReadEntries()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, EntryPrompt, entries[0].Type)
	assert.Equal(t, "user_prompt", entries[0].Action)
}

func TestLogger_LogToolCall(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	logger.LogToolCall("s1", "ws1", "search", map[string]interface{}{"q": "foo"}, true, 500*time.Millisecond)
	require.NoError(t, logger.Flush())

	entries, err := logger.ReadEntries()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, int64(500), entries[0].Duration)
}

func TestLogger_LogFileWriteAndDelete(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	logger.LogFileWrite("s1", "ws1", "/tmp/a.go", true)
	logger.LogFileDelete("s1", "ws1", "/tmp/a.go", true)
	require.NoError(t, logger.Flush())

	entries, err := logger.ReadEntries()
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, EntryFileWrite, entries[0].Type)
	assert.Equal(t, EntryFileDelete, entries[1].Type)
}

func TestLogger_LogModeChange(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	logger.LogModeChange("s1", "ws1", "plan", "build")
	require.NoError(t, logger.Flush())

	entries, err := logger.ReadEntries()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "plan", entries[0].Details["from"])
}

func TestLogger_LogError(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	logger.LogError("s1", "ws1", "exec", "not found")
	require.NoError(t, logger.Flush())

	entries, err := logger.ReadEntries()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, EntryError, entries[0].Type)
	assert.False(t, entries[0].Success)
	assert.Equal(t, "not found", entries[0].Error)
}

func TestLogger_LogDecision(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	logger.LogDecision("s1", "ws1", "approve", map[string]interface{}{"r": "safe"})
	require.NoError(t, logger.Flush())

	entries, err := logger.ReadEntries()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, EntryDecision, entries[0].Type)
}

func TestLogger_LogSafetyCheck(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	logger.LogSafetyCheck("s1", "ws1", "write_file", "blocked", false)
	require.NoError(t, logger.Flush())

	entries, err := logger.ReadEntries()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, EntrySafetyCheck, entries[0].Type)
	assert.False(t, entries[0].Success)
}

func TestLogger_AutoFlush(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	for i := 0; i < 100; i++ {
		require.NoError(t, logger.Log(Entry{Type: EntryPrompt, Action: "auto"}))
	}

	entries, err := logger.ReadEntries()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 100)
}

func TestFilterEntries_ByType(t *testing.T) {
	entries := []Entry{
		{Type: EntryPrompt, Action: "a"},
		{Type: EntryError, Action: "b"},
		{Type: EntryPrompt, Action: "c"},
	}
	filtered := FilterEntries(entries, EntryPrompt, time.Time{}, time.Time{})
	assert.Len(t, filtered, 2)
	assert.Equal(t, "a", filtered[0].Action)
	assert.Equal(t, "c", filtered[1].Action)
}

func TestFilterEntries_ByTimeRange(t *testing.T) {
	now := time.Now()
	entries := []Entry{
		{Type: EntryPrompt, Action: "old", Timestamp: now.Add(-2 * time.Hour)},
		{Type: EntryPrompt, Action: "new", Timestamp: now},
	}
	from := now.Add(-30 * time.Minute)
	filtered := FilterEntries(entries, "", from, time.Time{})
	assert.Len(t, filtered, 1)
	assert.Equal(t, "new", filtered[0].Action)
}

func TestFilterEntries_Empty(t *testing.T) {
	assert.Empty(t, FilterEntries([]Entry{}, "", time.Time{}, time.Time{}))
}
