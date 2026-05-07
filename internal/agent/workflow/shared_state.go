package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// WaveStatus tracks which waves have been completed.
type WaveStatus struct {
	Index     int      `json:"index"`
	Roles     []string `json:"roles"`
	Completed bool     `json:"completed"`
}

// SharedState provides a common workspace for inter-agent communication.
type SharedState struct {
	ProjectDir    string                 `json:"project_dir"`
	Artifacts     map[string]string      `json:"artifacts"`     // role → output path mapping
	Contracts     map[string]interface{} `json:"contracts"`     // API contracts, schema defs
	CompletedWaves []WaveStatus          `json:"completed_waves,omitempty"`
	mu            sync.RWMutex
}

// NewSharedState creates a shared state for a workflow project.
func NewSharedState(projectDir string) *SharedState {
	return &SharedState{
		ProjectDir: projectDir,
		Artifacts:  make(map[string]string),
		Contracts:  make(map[string]interface{}),
	}
}

// WriteArtifact records an artifact path for a role.
func (s *SharedState) WriteArtifact(role, key, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Artifacts[role+"/"+key] = path
}

// ReadArtifact retrieves an artifact path.
func (s *SharedState) ReadArtifact(role, key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	path, ok := s.Artifacts[role+"/"+key]
	return path, ok
}

// ArtifactEntry is a key-value artifact record.
type ArtifactEntry struct {
	Key   string
	Value string
}

// ListArtifactsByRole returns all artifacts for a given role.
func (s *SharedState) ListArtifactsByRole(role string) []ArtifactEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var entries []ArtifactEntry
	prefix := role + "/"
	for k, v := range s.Artifacts {
		if strings.HasPrefix(k, prefix) {
			entries = append(entries, ArtifactEntry{Key: strings.TrimPrefix(k, prefix), Value: v})
		}
	}
	return entries
}

// WriteContract stores a structured contract (API schema, DB schema, etc.).
func (s *SharedState) WriteContract(name string, data interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Contracts[name] = data
}

// ReadContract retrieves a structured contract.
func (s *SharedState) ReadContract(name string) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.Contracts[name]
	return data, ok
}

// GetContractsJSON returns all contracts as a JSON string for LLM consumption.
func (s *SharedState) GetContractsJSON() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.Contracts) == 0 {
		return "{}", nil
	}
	b, err := json.MarshalIndent(s.Contracts, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Save persists the shared state to disk.
func (s *SharedState) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stateDir := filepath.Join(s.ProjectDir, ".smara")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("gagal buat state dir: %w", err)
	}

	path := filepath.Join(stateDir, "state.json")
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("gagal marshal state: %w", err)
	}

	if err := os.WriteFile(path, b, 0644); err != nil {
		return fmt.Errorf("gagal write state file: %w", err)
	}

	return nil
}

// MarkWaveCompleted records a completed wave for resume tracking.
func (s *SharedState) MarkWaveCompleted(index int, roles []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, w := range s.CompletedWaves {
		if w.Index == index {
			s.CompletedWaves[i].Completed = true
			s.CompletedWaves[i].Roles = roles
			return
		}
	}
	s.CompletedWaves = append(s.CompletedWaves, WaveStatus{Index: index, Roles: roles, Completed: true})
}

// GetCompletedWaveRoles returns all roles from completed waves.
func (s *SharedState) GetCompletedWaveRoles() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	completed := make(map[string]bool)
	for _, w := range s.CompletedWaves {
		if w.Completed {
			for _, r := range w.Roles {
				completed[r] = true
			}
		}
	}
	return completed
}

// LoadSharedState restores shared state from disk.
func LoadSharedState(projectDir string) (*SharedState, error) {
	path := filepath.Join(projectDir, ".smara", "state.json")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewSharedState(projectDir), nil
		}
		return nil, fmt.Errorf("gagal read state file: %w", err)
	}

	var s SharedState
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("gagal parse state file: %w", err)
	}
	s.ProjectDir = projectDir
	s.Artifacts = make(map[string]string)
	s.Contracts = make(map[string]interface{})

	// Re-populate maps from raw JSON
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err == nil {
		if arts, ok := raw["artifacts"].(map[string]interface{}); ok {
			for k, v := range arts {
				if str, ok := v.(string); ok {
					s.Artifacts[k] = str
				}
			}
		}
		if ctrs, ok := raw["contracts"].(map[string]interface{}); ok {
			for k, v := range ctrs {
				s.Contracts[k] = v
			}
		}
	}

	return &s, nil
}
