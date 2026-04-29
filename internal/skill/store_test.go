package skill

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func TestSaveAndLoad(t *testing.T) {
	setupTestHome(t)

	s := &Skill{Name: "test-save", Steps: []Step{{Tool: "echo"}}}
	require.NoError(t, Save(s, nil))

	loaded, err := Load("test-save")
	require.NoError(t, err)
	assert.Equal(t, "test-save", loaded.Name)
	assert.Len(t, loaded.Steps, 1)
	assert.Equal(t, "echo", loaded.Steps[0].Tool)
}

func TestLoad_NotFound(t *testing.T) {
	setupTestHome(t)

	_, err := Load("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestSave_Invalid(t *testing.T) {
	setupTestHome(t)

	s := &Skill{Name: ""} // invalid: no name
	err := Save(s, nil)
	assert.Error(t, err)
}

func TestDelete(t *testing.T) {
	setupTestHome(t)

	s := &Skill{Name: "to-delete", Steps: []Step{{Tool: "echo"}}}
	require.NoError(t, Save(s, nil))

	require.NoError(t, Delete("to-delete", nil))

	_, err := Load("to-delete")
	assert.Error(t, err)
}

func TestList(t *testing.T) {
	setupTestHome(t)

	names, err := List()
	require.NoError(t, err)
	assert.Empty(t, names)

	require.NoError(t, Save(&Skill{Name: "a", Steps: []Step{{Tool: "echo"}}}, nil))
	require.NoError(t, Save(&Skill{Name: "b", Steps: []Step{{Tool: "echo"}}}, nil))

	names, err = List()
	require.NoError(t, err)
	assert.Len(t, names, 2)
	assert.Contains(t, names, "a")
	assert.Contains(t, names, "b")
}
