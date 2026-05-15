package sync

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDaemon(t *testing.T) (*Daemon, *memory.SQLiteStore, string, func()) {
	t.Helper()

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	store, err := memory.NewSQLiteStore(dbPath)
	require.NoError(t, err)

	syncDir := filepath.Join(tempDir, "sync")
	config := SyncConfig{
		SyncDir:     syncDir,
		IntervalMin: 0,
		Enabled:     true,
	}

	daemon := NewDaemon(config, store)
	require.NotNil(t, daemon)

	cleanup := func() {
		daemon.Stop()
		store.Close()
	}

	return daemon, store, syncDir, cleanup
}

func TestDaemonAutonomyLoop_ExportAndImport(t *testing.T) {
	daemon, store, syncDir, cleanup := setupTestDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Save an unsynced memory
	mem, err := store.Save("test memory content", "sync,test", "test", 1, nil)
	require.NoError(t, err)
	require.NotNil(t, mem)

	// Start the autonomy-driven daemon
	daemon.Start(ctx)

	// Wait for the autonomy loop to observe, think, act
	time.Sleep(2 * time.Second)

	// Verify that the delta file was exported
	outboxDir := filepath.Join(syncDir, "outbox")
	entries, err := os.ReadDir(outboxDir)
	require.NoError(t, err)
	require.NotEmpty(t, entries, "expected at least one delta file in outbox")

	// Verify the memory was marked as synced
	unsynced, err := store.GetUnsyncedMemories()
	require.NoError(t, err)
	assert.Empty(t, unsynced, "expected no unsynced memories after export")

	// Verify metrics were recorded
	metrics := daemon.GetMetrics()
	assert.GreaterOrEqual(t, metrics["memory_export"], 1, "expected at least one export cycle")

	// Verify state is idle or holding (not stuck in acting)
	state := daemon.GetState()
	assert.True(t, state == "idle" || state == "holding", "expected idle or holding state, got %s", state)
}

func TestDaemonAutonomyLoop_Import(t *testing.T) {
	daemon, _, syncDir, cleanup := setupTestDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a fake delta file in inbox
	inboxDir := filepath.Join(syncDir, "inbox")
	require.NoError(t, os.MkdirAll(inboxDir, 0o755))

	delta := SyncDelta{
		ID:        "test_import_delta",
		Source:    "peer",
		CreatedAt: time.Now(),
		Memories: []DeltaEntry{
			{MemoryID: 0, Content: "imported memory", Tags: `["imported"]`, Hash: "abc123"},
		},
	}
	data, err := json.MarshalIndent(delta, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(inboxDir, "test.json"), data, 0o644))

	// Start the daemon
	daemon.Start(ctx)

	// Wait for import cycle
	time.Sleep(2 * time.Second)

	// Verify the delta was moved to done
	_, err = os.Stat(filepath.Join(inboxDir, "test.json"))
	assert.True(t, os.IsNotExist(err), "expected inbox file to be moved to done")

	doneDir := filepath.Join(syncDir, "done")
	_, err = os.Stat(filepath.Join(doneDir, "test.json"))
	assert.NoError(t, err, "expected file in done directory")

	// Verify metrics
	metrics := daemon.GetMetrics()
	assert.GreaterOrEqual(t, metrics["memory_import"], 1, "expected at least one import cycle")
}

func TestDaemonAutonomyLoop_HoldWhenNoWork(t *testing.T) {
	daemon, _, _, cleanup := setupTestDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	daemon.Start(ctx)

	// Wait a bit; no memories to export, no inbox to import
	time.Sleep(2 * time.Second)

	// Metrics should show 0 exports and 0 imports (hold state prevented execution)
	metrics := daemon.GetMetrics()
	assert.Equal(t, 0, metrics["memory_export"], "expected zero exports when no unsynced memories")
	assert.Equal(t, 0, metrics["memory_import"], "expected zero imports when no inbox files")
}
