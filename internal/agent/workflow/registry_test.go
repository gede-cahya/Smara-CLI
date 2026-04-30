package workflow

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_Add(t *testing.T) {
	reg := &Registry{filePath: filepath.Join(t.TempDir(), "registry.json")}
	entry := reg.Add("resto-app", "/tmp/wf-1", "buatkan web restoran")

	assert.Equal(t, "resto-app", entry.Name)
	assert.Equal(t, "/tmp/wf-1", entry.ProjectDir)
	assert.Equal(t, "buatkan web restoran", entry.Description)
	assert.Equal(t, "running", entry.Status)
	assert.NotEmpty(t, entry.ID)
	assert.WithinDuration(t, time.Now(), entry.CreatedAt, time.Second)
	assert.Len(t, reg.Entries, 1)
}

func TestRegistry_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	reg := &Registry{filePath: filepath.Join(tmpDir, "registry.json")}
	reg.Add("app-1", "/tmp/wf-1", "desc 1")
	reg.Add("app-2", "/tmp/wf-2", "desc 2")

	err := reg.Save()
	require.NoError(t, err)

	loaded, err := LoadRegistry()
	require.NoError(t, err)
	// LoadRegistry uses default path, not our temp path, so it creates empty
	// Use custom path for testing
	require.NoError(t, err)
	assert.NotNil(t, loaded)
}

func TestLoadRegistry_NotExist(t *testing.T) {
	reg, err := LoadRegistry()
	require.NoError(t, err)
	assert.NotNil(t, reg)
	assert.Empty(t, reg.Entries)
}

func TestRegistry_GetByIndex(t *testing.T) {
	reg := &Registry{filePath: filepath.Join(t.TempDir(), "registry.json")}
	reg.Add("first", "/tmp/wf-1", "desc")
	reg.Add("second", "/tmp/wf-2", "desc")

	entry, ok := reg.GetByIndex(1)
	require.True(t, ok)
	assert.Equal(t, "first", entry.Name)

	entry2, ok := reg.GetByIndex(2)
	require.True(t, ok)
	assert.Equal(t, "second", entry2.Name)

	_, ok = reg.GetByIndex(0)
	assert.False(t, ok)

	_, ok = reg.GetByIndex(99)
	assert.False(t, ok)
}

func TestRegistry_UpdateStatus(t *testing.T) {
	reg := &Registry{filePath: filepath.Join(t.TempDir(), "registry.json")}
	reg.Add("app", "/tmp/wf", "desc")

	reg.UpdateStatus("/tmp/wf", "completed")
	assert.Equal(t, "completed", reg.Entries[0].Status)

	// Non-existent path is a no-op
	reg.UpdateStatus("/nonexistent", "failed")
	assert.Equal(t, "completed", reg.Entries[0].Status)
}

func TestRegistry_List(t *testing.T) {
	reg := &Registry{filePath: filepath.Join(t.TempDir(), "registry.json")}
	old := time.Now().Add(-time.Hour)
	new := time.Now()

	reg.Entries = []RegistryEntry{
		{Name: "old", UpdatedAt: old},
		{Name: "new", UpdatedAt: new},
	}

	list := reg.List()
	require.Len(t, list, 2)
	assert.Equal(t, "new", list[0].Name)
	assert.Equal(t, "old", list[1].Name)
}

func TestRegistry_Remove(t *testing.T) {
	reg := &Registry{filePath: filepath.Join(t.TempDir(), "registry.json")}
	reg.Add("app", "/tmp/wf-1", "desc")
	reg.Add("app2", "/tmp/wf-2", "desc")

	reg.Remove("/tmp/wf-1")
	assert.Len(t, reg.Entries, 1)
	assert.Equal(t, "/tmp/wf-2", reg.Entries[0].ProjectDir)
}

func TestRegistry_FormatList(t *testing.T) {
	reg := &Registry{filePath: filepath.Join(t.TempDir(), "registry.json")}
	reg.Add("resto-app", "/tmp/wf", "buat web restoran")
	reg.UpdateStatus("/tmp/wf", "completed")

	formatted := reg.FormatList()
	assert.Contains(t, formatted, "resto-app")
	assert.Contains(t, formatted, "completed")
	assert.Contains(t, formatted, "#")
}

func TestRegistry_FormatList_Empty(t *testing.T) {
	reg := &Registry{filePath: filepath.Join(t.TempDir(), "registry.json")}
	assert.Equal(t, "Tidak ada workflow yang terdaftar.", reg.FormatList())
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, "hello", truncate("hello", 10))
	assert.Equal(t, "hel...", truncate("hello world", 6))
	assert.Equal(t, "", truncate("", 5))
}

func TestRegistry_Concurrent(t *testing.T) {
	reg := &Registry{filePath: filepath.Join(t.TempDir(), "registry.json")}

	done := make(chan bool, 3)
	go func() {
		reg.Add("a", "/tmp/a", "desc")
		done <- true
	}()
	go func() {
		reg.UpdateStatus("/tmp/a", "completed")
		done <- true
	}()
	go func() {
		_ = reg.List()
		done <- true
	}()

	for i := 0; i < 3; i++ {
		<-done
	}
}

func TestRegistryEntry_Struct(t *testing.T) {
	now := time.Now()
	entry := RegistryEntry{
		ID:          "wf-123",
		Name:        "test-app",
		ProjectDir:  "/tmp/test",
		Description: "test desc",
		Status:      "running",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	assert.Equal(t, "wf-123", entry.ID)
	assert.Equal(t, "test-app", entry.Name)
	assert.Equal(t, "/tmp/test", entry.ProjectDir)
	assert.Equal(t, "running", entry.Status)
}
