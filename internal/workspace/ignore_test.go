package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewIgnoreMatcher_NoFiles(t *testing.T) {
	tempDir := t.TempDir()
	m := NewIgnoreMatcher(tempDir)
	assert.NotNil(t, m)
}

func TestIsIgnored_GitDir(t *testing.T) {
	tempDir := t.TempDir()
	m := NewIgnoreMatcher(tempDir)

	assert.True(t, m.IsIgnored(filepath.Join(tempDir, ".git"), true))
	assert.True(t, m.IsIgnored(filepath.Join(tempDir, ".git", "config"), false))
	assert.False(t, m.IsIgnored(filepath.Join(tempDir, "main.go"), false))
}

func TestIsIgnored_GitIgnore(t *testing.T) {
	tempDir := t.TempDir()
	os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte("*.log\nbuild/\n"), 0o644)

	m := NewIgnoreMatcher(tempDir)
	assert.True(t, m.IsIgnored(filepath.Join(tempDir, "debug.log"), false))
	assert.True(t, m.IsIgnored(filepath.Join(tempDir, "build", "out.js"), false))
	assert.False(t, m.IsIgnored(filepath.Join(tempDir, "main.go"), false))
}

func TestIsIgnored_SmaraIgnore(t *testing.T) {
	tempDir := t.TempDir()
	os.WriteFile(filepath.Join(tempDir, ".smaraignore"), []byte("*.tmp\n"), 0o644)

	m := NewIgnoreMatcher(tempDir)
	assert.True(t, m.IsIgnored(filepath.Join(tempDir, "temp.tmp"), false))
	assert.False(t, m.IsIgnored(filepath.Join(tempDir, "main.go"), false))
}

func TestGlobalSkipDirs(t *testing.T) {
	assert.True(t, GlobalSkipDirs[".git"])
	assert.True(t, GlobalSkipDirs["node_modules"])
	assert.True(t, GlobalSkipDirs["vendor"])
	assert.False(t, GlobalSkipDirs["src"])
}
