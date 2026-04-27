package workspace

import (
	"os"
	"path/filepath"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"
)

// IgnoreMatcher provides logic for checking if files/dirs should be ignored.
type IgnoreMatcher struct {
	gitIgnore   *ignore.GitIgnore
	smaraIgnore *ignore.GitIgnore
	baseDir     string
}

// NewIgnoreMatcher initializes a new matcher from a base directory.
// It looks for .gitignore and .smaraignore.
func NewIgnoreMatcher(baseDir string) *IgnoreMatcher {
	m := &IgnoreMatcher{
		baseDir: baseDir,
	}

	gitIgnorePath := filepath.Join(baseDir, ".gitignore")
	if _, err := os.Stat(gitIgnorePath); err == nil {
		m.gitIgnore, _ = ignore.CompileIgnoreFile(gitIgnorePath)
	}

	smaraIgnorePath := filepath.Join(baseDir, ".smaraignore")
	if _, err := os.Stat(smaraIgnorePath); err == nil {
		m.smaraIgnore, _ = ignore.CompileIgnoreFile(smaraIgnorePath)
	}

	return m
}

// IsIgnored checks if the target path is ignored by .gitignore, .smaraignore,
// or some standard hardcoded rules (e.g. .git/).
func (m *IgnoreMatcher) IsIgnored(targetPath string, isDir bool) bool {
	rel, err := filepath.Rel(m.baseDir, targetPath)
	if err != nil {
		rel = targetPath
	}

	// Always ignore .git directory
	if rel == ".git" || strings.HasPrefix(rel, ".git"+string(os.PathSeparator)) {
		return true
	}
	
	// Normalize path for matching (some matchers prefer / separators)
	relPath := filepath.ToSlash(rel)

	if m.smaraIgnore != nil && m.smaraIgnore.MatchesPath(relPath) {
		return true
	}

	if m.gitIgnore != nil && m.gitIgnore.MatchesPath(relPath) {
		return true
	}

	return false
}

// GlobalSkipDirs is a fallback for agents if no matcher is used
var GlobalSkipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"__pycache__":  true,
	".next":        true,
	".kilo":        true,
}
