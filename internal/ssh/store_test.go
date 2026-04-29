package ssh

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStore(t *testing.T) {
	dbPath := t.TempDir() + "/ssh.db"
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	require.NotNil(t, store)
	require.NoError(t, store.Close())
}

func TestStore_SaveAndListLogs(t *testing.T) {
	dbPath := t.TempDir() + "/ssh.db"
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	entry := LogEntry{
		HostName: "web1",
		Address:  "1.1.1.1",
		Command:  "uptime",
		Stdout:   "ok",
		Stderr:   "",
		Status:   "success",
		Duration: 1200,
	}
	require.NoError(t, store.SaveLog(entry))

	logs, err := store.ListLogs(10)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, "web1", logs[0].HostName)
	assert.Equal(t, "uptime", logs[0].Command)
	assert.Equal(t, "success", logs[0].Status)
	assert.Equal(t, int64(1200), logs[0].Duration)
}

func TestStore_ListLogs_DefaultLimit(t *testing.T) {
	dbPath := t.TempDir() + "/ssh.db"
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	for i := 0; i < 3; i++ {
		require.NoError(t, store.SaveLog(LogEntry{HostName: "h", Address: "a", Command: "c"}))
	}

	logs, err := store.ListLogs(0) // should default to 50
	require.NoError(t, err)
	assert.Len(t, logs, 3)
}
