package integrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/gede-cahya/Smara-CLI/internal/cognitive"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
)

func TestNewEngine_CognitiveValidator(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := tempDir + "/test.db"
	store, err := memory.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	engine, err := NewEngine(store)
	require.NoError(t, err)
	require.NotNil(t, engine)
	defer engine.Stop()

	assert.NotNil(t, engine.Cognitive, "Cognitive validator should be initialized")
}

func TestEngine_ValidateToolArgs(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := tempDir + "/test.db"
	store, err := memory.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	engine, err := NewEngine(store)
	require.NoError(t, err)
	defer engine.Stop()

	// Register a test schema
	engine.Cognitive.RegisterTool(cognitive.ToolSchema{
		Name:     "test_tool",
		Type:     cognitive.TypeObject,
		Required: []string{"name"},
		Properties: map[string]cognitive.PropertySchema{
			"name": {Type: cognitive.TypeString},
			"age":  {Type: cognitive.TypeInteger},
		},
	})

	// Valid args
	result := engine.ValidateToolArgs("test_tool", map[string]interface{}{
		"name": "Alice",
		"age":  30,
	})
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)

	// Missing required field
	result = engine.ValidateToolArgs("test_tool", map[string]interface{}{
		"age": 30,
	})
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.Errors)

	// Wrong type
	result = engine.ValidateToolArgs("test_tool", map[string]interface{}{
		"name": "Alice",
		"age":  "thirty",
	})
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.Errors)
}

func TestEngine_ValidateToolArgs_NoValidator(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := tempDir + "/test.db"
	store, err := memory.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	engine, err := NewEngine(store)
	require.NoError(t, err)
	defer engine.Stop()

	// Set cognitive to nil to test the fallback
	engine.Cognitive = nil

	result := engine.ValidateToolArgs("any_tool", map[string]interface{}{})
	assert.True(t, result.Valid, "should return valid when cognitive validator is nil")
}

func TestEngine_SafetyMode(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := tempDir + "/test.db"
	store, err := memory.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	engine, err := NewEngine(store)
	require.NoError(t, err)
	defer engine.Stop()

	// Default mode should be Plan
	assert.Equal(t, "plan", string(engine.GetSafetyMode()))

	// Switch to Build mode
	engine.SetSafetyMode("build")
	assert.Equal(t, "build", string(engine.GetSafetyMode()))
}
