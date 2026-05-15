package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Registry tracks all workflow projects with friendly names.
type Registry struct {
	mu       sync.RWMutex
	Entries  []RegistryEntry `json:"entries"`
	filePath string
}

// RegistryEntry stores metadata for a single workflow project.
type RegistryEntry struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ProjectDir  string    `json:"project_dir"`
	Description string    `json:"description"`
	Status      string    `json:"status"` // "running", "completed", "failed", "paused"
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// registryFilePath returns the default registry file path.
func registryFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, ".smara", "workflows.json")
}

// LoadRegistry loads the workflow registry from disk.
func LoadRegistry() (*Registry, error) {
	path := registryFilePath()
	reg := &Registry{filePath: path}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return reg, nil
		}
		return nil, fmt.Errorf("gagal read registry: %w", err)
	}

	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("gagal parse registry: %w", err)
	}

	return reg, nil
}

// Save persists the registry to disk.
func (r *Registry) Save() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(r.filePath), 0755); err != nil {
		return fmt.Errorf("gagal buat registry dir: %w", err)
	}

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("gagal marshal registry: %w", err)
	}

	return os.WriteFile(r.filePath, data, 0644)
}

// Add registers a new workflow project.
func (r *Registry) Add(name, projectDir, description string) *RegistryEntry {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	entry := RegistryEntry{
		ID:          fmt.Sprintf("wf-%d", now.Unix()),
		Name:        name,
		ProjectDir:  projectDir,
		Description: description,
		Status:      "running",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	r.Entries = append(r.Entries, entry)
	return &entry
}

// UpdateStatus changes the status of a workflow by directory.
func (r *Registry) UpdateStatus(projectDir, status string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.Entries {
		if r.Entries[i].ProjectDir == projectDir {
			r.Entries[i].Status = status
			r.Entries[i].UpdatedAt = time.Now()
			break
		}
	}
}

// GetByIndex returns the nth entry (1-based for UI).
func (r *Registry) GetByIndex(n int) (*RegistryEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if n < 1 || n > len(r.Entries) {
		return nil, false
	}
	return &r.Entries[n-1], true
}

// List returns all entries sorted by updated time (newest first).
func (r *Registry) List() []RegistryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]RegistryEntry, len(r.Entries))
	copy(result, r.Entries)
	sort.Slice(result, func(i, j int) bool {
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return result
}

// Remove deletes an entry by project directory.
func (r *Registry) Remove(projectDir string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var filtered []RegistryEntry
	for _, e := range r.Entries {
		if e.ProjectDir != projectDir {
			filtered = append(filtered, e)
		}
	}
	r.Entries = filtered
}

// FormatList returns a human-readable table of workflows.
func (r *Registry) FormatList() string {
	entries := r.List()
	if len(entries) == 0 {
		return "Tidak ada workflow yang terdaftar."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s  %-12s  %-20s  %-10s  %s\n", "#", "ID", "Nama", "Status", "Updated"))
	sb.WriteString(strings.Repeat("-", 70) + "\n")

	for i, e := range entries {
		statusEmoji := "🟡"
		switch e.Status {
		case "completed":
			statusEmoji = "✅"
		case "failed":
			statusEmoji = "❌"
		case "paused":
			statusEmoji = "⏸️"
		}
		sb.WriteString(fmt.Sprintf("%d  %-12s  %-20s  %s %-8s  %s\n",
			i+1, e.ID, truncate(e.Name, 20), statusEmoji, e.Status, e.UpdatedAt.Format("2006-01-02 15:04")))
	}

	return sb.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
