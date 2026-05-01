package skill

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

const skillsDir = ".smara/skills"

// ensureDir creates the skills directory if it doesn't exist.
func ensureDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, skillsDir)
	return dir, os.MkdirAll(dir, 0755)
}

// Save stores a skill to both file and SQLite.
func Save(s *Skill, db *sql.DB) error {
	if err := s.Validate(); err != nil {
		return err
	}
	dir, err := ensureDir()
	if err != nil {
		return err
	}
	data, err := s.ToJSON()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, s.Name+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write skill file: %w", err)
	}
	if db != nil {
		_, _ = db.Exec(`INSERT INTO skills (name, json, version) VALUES (?, ?, ?)
			ON CONFLICT(name) DO UPDATE SET json=excluded.json, version=excluded.version, updated_at=CURRENT_TIMESTAMP`,
			s.Name, string(data), s.Version)
	}
	return nil
}

// Load retrieves a skill from file.
// Tries .json first, then falls back to .md (markdown with YAML frontmatter).
func Load(name string) (*Skill, error) {
	dir, err := ensureDir()
	if err != nil {
		return nil, err
	}

	// Try JSON first
	data, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if err == nil {
		return FromJSON(data)
	}

	// Fallback to markdown
	data, err = os.ReadFile(filepath.Join(dir, name+".md"))
	if err == nil {
		return ParseMarkdownSkill(data)
	}

	return nil, fmt.Errorf("skill '%s' not found (tried .json and .md): %w", name, err)
}

// SaveAsMarkdown stores a skill as a markdown-with-frontmatter file.
func SaveAsMarkdown(s *Skill, db *sql.DB) error {
	if err := s.Validate(); err != nil {
		return err
	}
	dir, err := ensureDir()
	if err != nil {
		return err
	}
	data, err := s.ToMarkdown()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, s.Name+".md")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write skill markdown file: %w", err)
	}
	if db != nil {
		_, _ = db.Exec(`INSERT INTO skills (name, json, version) VALUES (?, ?, ?)
			ON CONFLICT(name) DO UPDATE SET json=excluded.json, version=excluded.version, updated_at=CURRENT_TIMESTAMP`,
			s.Name, string(data), s.Version)
	}
	return nil
}

// Delete removes a skill from file (both .json and .md) and SQLite.
func Delete(name string, db *sql.DB) error {
	dir, err := ensureDir()
	if err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(dir, name+".json"))
	_ = os.Remove(filepath.Join(dir, name+".md"))
	if db != nil {
		_, _ = db.Exec(`DELETE FROM skills WHERE name = ?`, name)
	}
	return nil
}

// List returns all saved skill names (both .json and .md, deduplicated).
func List() ([]string, error) {
	dir, err := ensureDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext != ".json" && ext != ".md" {
			continue
		}
		name := e.Name()[:len(e.Name())-len(ext)]
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names, nil
}
