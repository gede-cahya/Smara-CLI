package lsp

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectLanguage(t *testing.T) {
	assert.Equal(t, "go", DetectLanguage("main.go"))
	assert.Equal(t, "typescript", DetectLanguage("app.ts"))
	assert.Equal(t, "typescript", DetectLanguage("app.tsx"))
	assert.Equal(t, "javascript", DetectLanguage("script.js"))
	assert.Equal(t, "python", DetectLanguage("script.py"))
	assert.Equal(t, "rust", DetectLanguage("lib.rs"))
	assert.Equal(t, "java", DetectLanguage("Main.java"))
	assert.Equal(t, "cpp", DetectLanguage("main.cpp"))
	assert.Equal(t, "cpp", DetectLanguage("header.hpp"))
	assert.Equal(t, "", DetectLanguage("file.unknown"))
}

func TestFileToURI(t *testing.T) {
	uri := FileToURI("/tmp/main.go")
	assert.Contains(t, uri, "file://")
	assert.Contains(t, uri, "/tmp/main.go")
}

func TestNewManager(t *testing.T) {
	m := NewManager()
	require.NotNil(t, m)
}

func TestManager_GetClient_NotFound(t *testing.T) {
	m := NewManager()
	_, ok := m.GetClient("go")
	assert.False(t, ok)
}

func TestManager_CloseClient_NotFound(t *testing.T) {
	m := NewManager()
	// Should not panic
	m.CloseClient("go")
}

func TestManager_CloseAll_Empty(t *testing.T) {
	m := NewManager()
	// Should not panic
	m.CloseAll()
}

func TestParseLocation(t *testing.T) {
	m := map[string]interface{}{
		"uri": "file:///tmp/a.go",
		"range": map[string]interface{}{
			"start": map[string]interface{}{"line": 1.0, "character": 2.0},
			"end":   map[string]interface{}{"line": 3.0, "character": 4.0},
		},
	}
	loc := parseLocation(m)
	assert.Equal(t, "file:///tmp/a.go", loc.URI)
	assert.Equal(t, 1, loc.Range.Start.Line)
	assert.Equal(t, 2, loc.Range.Start.Character)
	assert.Equal(t, 3, loc.Range.End.Line)
	assert.Equal(t, 4, loc.Range.End.Character)
}

func TestParseHover_String(t *testing.T) {
	resp := map[string]interface{}{
		"result": map[string]interface{}{
			"contents": "hover text",
		},
	}
	h := parseHover(resp)
	require.NotNil(t, h)
	assert.Equal(t, "hover text", h.Contents)
}

func TestParseHover_MarkedString(t *testing.T) {
	resp := map[string]interface{}{
		"result": map[string]interface{}{
			"contents": map[string]interface{}{"value": "marked text"},
		},
	}
	h := parseHover(resp)
	require.NotNil(t, h)
	assert.Equal(t, "marked text", h.Contents)
}

func TestParseDocumentSymbol(t *testing.T) {
	m := map[string]interface{}{
		"name":   "MyFunc",
		"detail": "func()",
		"kind":   12.0,
		"range": map[string]interface{}{
			"start": map[string]interface{}{"line": 5.0, "character": 0.0},
			"end":   map[string]interface{}{"line": 10.0, "character": 1.0},
		},
	}
	s := parseDocumentSymbol(m)
	assert.Equal(t, "MyFunc", s.Name)
	assert.Equal(t, "func()", s.Detail)
	assert.Equal(t, 12, s.Kind)
	assert.Equal(t, 5, s.Range.Start.Line)
}

func TestParseLocations_Array(t *testing.T) {
	resp := map[string]interface{}{
		"result": []interface{}{
			map[string]interface{}{
				"uri": "file:///tmp/a.go",
				"range": map[string]interface{}{
					"start": map[string]interface{}{"line": 1.0},
				},
			},
		},
	}
	locs := parseLocations(resp)
	require.Len(t, locs, 1)
	assert.Equal(t, "file:///tmp/a.go", locs[0].URI)
}

func TestParseLocations_Single(t *testing.T) {
	resp := map[string]interface{}{
		"result": map[string]interface{}{
			"uri": "file:///tmp/b.go",
		},
	}
	locs := parseLocations(resp)
	require.Len(t, locs, 1)
	assert.Equal(t, "file:///tmp/b.go", locs[0].URI)
}

func TestNewClient_Conditional(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed, skipping integration test")
	}
	client, err := NewClient("go", t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.True(t, client.IsReady())
	require.NoError(t, client.Close())
}

func TestManager_GetOrCreateClient_Conditional(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed, skipping integration test")
	}
	m := NewManager()
	client, err := m.GetOrCreateClient("go", t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, client)

	// Second call should reuse
	client2, err := m.GetOrCreateClient("go", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, client, client2)

	m.CloseAll()
}
