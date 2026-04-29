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
func Load(name string) (*Skill, error) {
	dir, err := ensureDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if err != nil {
		return nil, fmt.Errorf("skill '%s' not found: %w", name, err)
	}
	return FromJSON(data)
}

// Delete removes a skill from file and SQLite.
func Delete(name string, db *sql.DB) error {
	dir, err := ensureDir()
	if err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(dir, name+".json"))
	if db != nil {
		_, _ = db.Exec(`DELETE FROM skills WHERE name = ?`, name)
	}
	return nil
}

// List returns all saved skill names.
func List() ([]string, error) {
	dir, err := ensureDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			names = append(names, e.Name()[:len(e.Name())-5])
		}
	}
	return names, nil
}
