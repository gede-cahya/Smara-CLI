package memory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCompactor_Defaults(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	c := NewCompactor(store, DefaultCompactionConfig)
	require.NotNil(t, c)
	assert.Equal(t, DefaultCompactionConfig.MaxTotalMemories, c.config.MaxTotalMemories)
	assert.Equal(t, 0, c.stats.TotalCompactions)
}

func TestCompactor_Compact_Disabled(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	config := DefaultCompactionConfig
	config.Enabled = false
	c := NewCompactor(store, config)
	err := c.Compact()
	require.NoError(t, err)
	assert.Equal(t, 0, c.stats.TotalCompactions)
}

func TestCompactor_Compact_OverLimit(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	ws, err := store.CreateWorkspace("test-ws", "/tmp/test-ws")
	require.NoError(t, err)
	config := DefaultCompactionConfig
	config.MaxTotalMemories = 2
	config.MaxAgeDays = 0
	c := NewCompactor(store, config)
	for i := 0; i < 3; i++ {
		_, err := store.Save("memory", "tag", "test", ws.ID, nil)
		require.NoError(t, err)
	}
	err = c.Compact()
	require.NoError(t, err)
	assert.Equal(t, 1, c.stats.TotalCompactions)
	assert.False(t, c.stats.LastCompaction.IsZero())
}

func TestCompactor_CompactWorkspace(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	ws, err := store.CreateWorkspace("test-ws", "/tmp/test-ws")
	require.NoError(t, err)
	config := DefaultCompactionConfig
	config.MaxAgeDays = 0
	c := NewCompactor(store, config)
	for i := 0; i < 3; i++ {
		_, err := store.Save("memory", "tag", "test", ws.ID, nil)
		require.NoError(t, err)
	}
	err = c.CompactWorkspace(ws.ID)
	require.NoError(t, err)
}

func TestCompactor_GetStats(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	c := NewCompactor(store, DefaultCompactionConfig)
	stats := c.GetStats()
	assert.Equal(t, 0, stats.TotalCompactions)
	assert.True(t, stats.LastCompaction.IsZero())
}

func TestCompactor_UpdateConfig(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	c := NewCompactor(store, DefaultCompactionConfig)
	newConfig := CompactionConfig{Enabled: false, MaxTotalMemories: 100}
	c.UpdateConfig(newConfig)
	assert.Equal(t, newConfig, c.config)
}

func TestSummarizeMemories_Empty(t *testing.T) {
	result := SummarizeMemories([]Memory{})
	assert.Equal(t, "", result)
}

func TestSummarizeMemories_Grouped(t *testing.T) {
	mems := []Memory{
		{ID: 1, Source: "user", Content: "First memory"},
		{ID: 2, Source: "user", Content: "Second memory"},
		{ID: 3, Source: "agent", Content: "Agent note"},
	}
	result := SummarizeMemories(mems)
	assert.Contains(t, result, "[Summary of 3 memories]")
	assert.Contains(t, result, "From user")
	assert.Contains(t, result, "From agent")
}

func TestCompactor_ShouldCompact_Disabled(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	config := DefaultCompactionConfig
	config.Enabled = false
	c := NewCompactor(store, config)
	assert.False(t, c.ShouldCompact())
}

func TestCompactor_ShouldCompact_IntervalNotMet(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	config := DefaultCompactionConfig
	config.CompactInterval = 1 * time.Hour
	c := NewCompactor(store, config)
	c.stats.LastCompaction = time.Now()
	assert.False(t, c.ShouldCompact())
}

func TestCompactor_ShouldCompact_UnderCount(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	config := DefaultCompactionConfig
	config.MaxTotalMemories = 100
	config.CompactInterval = 0
	c := NewCompactor(store, config)
	assert.False(t, c.ShouldCompact())
}

func TestCompactor_ShouldCompact_OverCount(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	ws, err := store.CreateWorkspace("test-ws", "/tmp/test-ws")
	require.NoError(t, err)
	config := DefaultCompactionConfig
	config.MaxTotalMemories = 2
	config.CompactInterval = 0
	c := NewCompactor(store, config)
	for i := 0; i < 3; i++ {
		_, err := store.Save("memory", "tag", "test", ws.ID, nil)
		require.NoError(t, err)
	}
	assert.True(t, c.ShouldCompact())
}

func TestCompactor_AutoCompact(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	ws, err := store.CreateWorkspace("test-ws", "/tmp/test-ws")
	require.NoError(t, err)
	config := DefaultCompactionConfig
	config.MaxTotalMemories = 2
	config.CompactInterval = 0
	c := NewCompactor(store, config)
	for i := 0; i < 3; i++ {
		_, err := store.Save("memory", "tag", "test", ws.ID, nil)
		require.NoError(t, err)
	}
	err = c.AutoCompact()
	require.NoError(t, err)
	assert.Equal(t, 1, c.stats.TotalCompactions)
}

func TestCompactor_AutoCompact_Skip(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	config := DefaultCompactionConfig
	config.Enabled = false
	c := NewCompactor(store, config)
	err := c.AutoCompact()
	require.NoError(t, err)
	assert.Equal(t, 0, c.stats.TotalCompactions)
}
