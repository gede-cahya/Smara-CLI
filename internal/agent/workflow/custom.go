package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/config"
)

// CustomAgent defines a manually-configured agent in a custom workflow.
type CustomAgent struct {
	Role        string            `json:"role"`
	Description string            `json:"description"`
	Skills      []string          `json:"skills,omitempty"`
	Tasks       []Task            `json:"tasks"`
	DependsOn   []string          `json:"depends_on,omitempty"`
	InputsFrom  map[string][]string `json:"inputs_from,omitempty"`
}

// CustomWorkflow is a user-defined workflow with manually-specified agents and connections.
type CustomWorkflow struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	ProjectDir  string        `json:"project_dir,omitempty"`
	Agents      []CustomAgent `json:"agents"`
	CreatedAt   *time.Time    `json:"created_at,omitempty"`
	UpdatedAt   *time.Time    `json:"updated_at,omitempty"`
}

// Validate checks the custom workflow for basic correctness.
func (cw *CustomWorkflow) Validate() error {
	if cw.Name == "" {
		return fmt.Errorf("workflow name is required")
	}
	if len(cw.Agents) == 0 {
		return fmt.Errorf("workflow must have at least one agent")
	}

	roleSet := make(map[string]bool)
	for i, a := range cw.Agents {
		if a.Role == "" {
			return fmt.Errorf("agent %d: role is required", i)
		}
		if roleSet[a.Role] {
			return fmt.Errorf("duplicate agent role: %s", a.Role)
		}
		roleSet[a.Role] = true
		for _, dep := range a.DependsOn {
			if dep == a.Role {
				return fmt.Errorf("agent '%s' depends on itself", a.Role)
			}
		}
		for srcRole, keys := range a.InputsFrom {
			if !roleSet[srcRole] && !cw.hasRole(srcRole) {
				return fmt.Errorf("agent '%s' inputs_from references unknown role '%s'", a.Role, srcRole)
			}
			_ = keys
		}
	}

	// Check dependencies reference known roles
	for _, a := range cw.Agents {
		for _, dep := range a.DependsOn {
			if !cw.hasRole(dep) {
				return fmt.Errorf("agent '%s' depends_on unknown role '%s'", a.Role, dep)
			}
		}
	}

	return nil
}

func (cw *CustomWorkflow) hasRole(role string) bool {
	for _, a := range cw.Agents {
		if a.Role == role {
			return true
		}
	}
	return false
}

// ToBlueprint converts a CustomWorkflow to a Blueprint for execution.
func (cw *CustomWorkflow) ToBlueprint() Blueprint {
	agents := make([]AgentSpec, len(cw.Agents))
	for i, a := range cw.Agents {
		agents[i] = AgentSpec{
			Role:        a.Role,
			Description: a.Description,
			Skills:      a.Skills,
			Tasks:       a.Tasks,
			DependsOn:   a.DependsOn,
		}
	}
	return Blueprint{
		ProjectName:  cw.Name,
		Description:  cw.Description,
		Domain:       "custom",
		PRD:          "",
		Architecture: "",
		Agents:       agents,
	}
}

// ToJSON serializes the custom workflow.
func (cw *CustomWorkflow) ToJSON() ([]byte, error) {
	return json.MarshalIndent(cw, "", "  ")
}

// CustomWorkflowFromJSON deserializes a custom workflow.
func CustomWorkflowFromJSON(data []byte) (*CustomWorkflow, error) {
	var cw CustomWorkflow
	if err := json.Unmarshal(data, &cw); err != nil {
		return nil, fmt.Errorf("invalid custom workflow JSON: %w", err)
	}
	return &cw, nil
}

// customWorkflowDir returns the directory for storing custom workflows.
func customWorkflowDir() string {
	cfg := config.Get()
	dir := filepath.Join(filepath.Dir(cfg.DBPath), "workflows")
	return dir
}

// ensureCustomWorkflowDir ensures the workflows directory exists.
func ensureCustomWorkflowDir() error {
	dir := customWorkflowDir()
	return os.MkdirAll(dir, 0755)
}

// workflowFilePath returns the file path for a named workflow.
func workflowFilePath(name string) string {
	safe := strings.ReplaceAll(name, "/", "_")
	safe = strings.ReplaceAll(safe, "\\", "_")
	return filepath.Join(customWorkflowDir(), safe+".json")
}

// SaveCustomWorkflow persists a custom workflow to disk.
func SaveCustomWorkflow(cw *CustomWorkflow) error {
	if err := cw.Validate(); err != nil {
		return err
	}
	if err := ensureCustomWorkflowDir(); err != nil {
		return err
	}
	now := time.Now()
	cw.UpdatedAt = &now
	if cw.CreatedAt == nil {
		cw.CreatedAt = &now
	}
	data, err := cw.ToJSON()
	if err != nil {
		return err
	}
	return os.WriteFile(workflowFilePath(cw.Name), data, 0644)
}

// LoadCustomWorkflow loads a named custom workflow from disk.
func LoadCustomWorkflow(name string) (*CustomWorkflow, error) {
	data, err := os.ReadFile(workflowFilePath(name))
	if err != nil {
		return nil, fmt.Errorf("workflow '%s' not found: %w", name, err)
	}
	return CustomWorkflowFromJSON(data)
}

// DeleteCustomWorkflow removes a named custom workflow.
func DeleteCustomWorkflow(name string) error {
	return os.Remove(workflowFilePath(name))
}

// ListCustomWorkflows returns all saved custom workflow names.
func ListCustomWorkflows() ([]string, error) {
	if err := ensureCustomWorkflowDir(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(customWorkflowDir())
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".json") {
			names = append(names, strings.TrimSuffix(name, ".json"))
		}
	}
	return names, nil
}

// LoadAllCustomWorkflows loads all saved workflows.
func LoadAllCustomWorkflows() ([]*CustomWorkflow, error) {
	names, err := ListCustomWorkflows()
	if err != nil {
		return nil, err
	}
	var result []*CustomWorkflow
	for _, n := range names {
		cw, err := LoadCustomWorkflow(n)
		if err != nil {
			continue
		}
		result = append(result, cw)
	}
	return result, nil
}
