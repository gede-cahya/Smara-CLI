package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSharedState(t *testing.T) {
	state := NewSharedState("/tmp/test")
	assert.Equal(t, "/tmp/test", state.ProjectDir)
	assert.NotNil(t, state.Artifacts)
	assert.NotNil(t, state.Contracts)
	assert.Empty(t, state.Artifacts)
	assert.Empty(t, state.Contracts)
}

func TestSharedState_Artifacts(t *testing.T) {
	state := NewSharedState("/tmp/test")
	state.WriteArtifact("frontend", "login_page", "/tmp/login.html")

	path, ok := state.ReadArtifact("frontend", "login_page")
	assert.True(t, ok)
	assert.Equal(t, "/tmp/login.html", path)

	_, ok2 := state.ReadArtifact("backend", "api")
	assert.False(t, ok2)
}

func TestSharedState_Contracts(t *testing.T) {
	state := NewSharedState("/tmp/test")
	state.WriteContract("api_endpoints", []string{"GET /users", "POST /login"})

	data, ok := state.ReadContract("api_endpoints")
	assert.True(t, ok)
	endpoints, ok := data.([]string)
	assert.True(t, ok)
	assert.Equal(t, []string{"GET /users", "POST /login"}, endpoints)

	_, ok2 := state.ReadContract("missing")
	assert.False(t, ok2)
}

func TestSharedState_GetContractsJSON(t *testing.T) {
	state := NewSharedState("/tmp/test")
	json, err := state.GetContractsJSON()
	require.NoError(t, err)
	assert.Equal(t, "{}", json)

	state.WriteContract("schema", map[string]string{"users": "id INT PRIMARY KEY"})
	json2, err := state.GetContractsJSON()
	require.NoError(t, err)
	assert.Contains(t, json2, "users")
}

func TestSharedState_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	state := NewSharedState(tmpDir)
	state.WriteArtifact("frontend", "page", "path/to/page")
	state.WriteContract("api", map[string]string{"/users": "GET"})

	err := state.Save()
	require.NoError(t, err)

	// Verify file exists
	statePath := filepath.Join(tmpDir, ".smara", "state.json")
	_, err = os.Stat(statePath)
	require.NoError(t, err)

	// Load and verify
	loaded, err := LoadSharedState(tmpDir)
	require.NoError(t, err)

	path, ok := loaded.ReadArtifact("frontend", "page")
	assert.True(t, ok)
	assert.Equal(t, "path/to/page", path)

	data, ok := loaded.ReadContract("api")
	assert.True(t, ok)
	assert.NotNil(t, data)
}

func TestLoadSharedState_NotExist(t *testing.T) {
	tmpDir := t.TempDir()
	state, err := LoadSharedState(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, tmpDir, state.ProjectDir)
	assert.Empty(t, state.Artifacts)
}
