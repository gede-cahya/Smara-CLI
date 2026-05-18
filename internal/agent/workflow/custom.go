package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/config"
)

type MemoryNodeConfig struct {
	Action  string `json:"action,omitempty"` // shared, read, search, write, read_write
	Query   string `json:"query,omitempty"`
	Content string `json:"content,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

type LoopRetryConfig struct {
	MaxAttempts    int     `json:"max_attempts,omitempty"`
	InitialDelayMs int     `json:"initial_delay_ms,omitempty"`
	Multiplier     float64 `json:"multiplier,omitempty"`
	MaxDelayMs     int     `json:"max_delay_ms,omitempty"`
}

type LoopNodeConfig struct {
	Mode          string           `json:"mode,omitempty"` // count, until_success, until_condition, while_condition, for_each, interval, retry_backoff, infinite_guarded
	MaxIterations int              `json:"max_iterations,omitempty"`
	DelayMs       int              `json:"delay_ms,omitempty"`
	TimeoutMs     int              `json:"timeout_ms,omitempty"`
	Condition     string           `json:"condition,omitempty"`
	ItemsSource   string           `json:"items_source,omitempty"`
	Retry         *LoopRetryConfig `json:"retry,omitempty"`
	OnError       string           `json:"on_error,omitempty"` // stop, continue, retry, skip
}

// CustomAgent defines a manually-configured agent in a custom workflow.
type CustomAgent struct {
	Role        string              `json:"role"`
	Description string              `json:"description"`
	Skills      []string            `json:"skills,omitempty"`
	Tasks       []Task              `json:"tasks"`
	DependsOn   []string            `json:"depends_on,omitempty"`
	InputsFrom  map[string][]string `json:"inputs_from,omitempty"`
	Memory      *MemoryNodeConfig   `json:"memory,omitempty"`
	Loop        *LoopNodeConfig     `json:"loop,omitempty"`
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
		if a.Loop != nil {
			if err := validateLoopNodeConfig(a.Loop); err != nil {
				return fmt.Errorf("agent '%s' loop: %w", a.Role, err)
			}
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

// MergeCustomWorkflow appends imported agents into an existing workflow safely.
func MergeCustomWorkflow(base, imported *CustomWorkflow) *CustomWorkflow {
	if base == nil {
		return imported
	}
	if imported == nil || len(imported.Agents) == 0 {
		return base
	}
	merged := *base
	merged.Agents = append([]CustomAgent{}, base.Agents...)
	if strings.TrimSpace(merged.Description) == "" {
		merged.Description = imported.Description
	} else if strings.TrimSpace(imported.Description) != "" && !strings.Contains(merged.Description, imported.Description) {
		merged.Description += "\n\nMerged import: " + imported.Description
	}
	roleMap := map[string]string{}
	used := map[string]bool{}
	for _, agent := range merged.Agents {
		used[agent.Role] = true
	}
	for _, agent := range imported.Agents {
		oldRole := agent.Role
		newRole := nextAvailableRole(oldRole, used)
		used[newRole] = true
		roleMap[oldRole] = newRole
	}
	for _, agent := range imported.Agents {
		agent.Role = roleMap[agent.Role]
		agent.DependsOn = remapWorkflowRoles(agent.DependsOn, roleMap)
		agent.InputsFrom = remapWorkflowInputs(agent.InputsFrom, roleMap)
		if agent.Role != "master" && !containsString(agent.DependsOn, "master") && used["master"] {
			agent.DependsOn = append([]string{"master"}, agent.DependsOn...)
		}
		if agent.InputsFrom == nil {
			agent.InputsFrom = map[string][]string{}
		}
		merged.Agents = append(merged.Agents, agent)
	}
	return &merged
}

func nextAvailableRole(role string, used map[string]bool) string {
	role = strings.TrimSpace(role)
	if role == "" {
		role = "agent"
	}
	if !used[role] {
		return role
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", role, i)
		if !used[candidate] {
			return candidate
		}
	}
}

func remapWorkflowRoles(values []string, roleMap map[string]string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		if mapped, ok := roleMap[value]; ok {
			value = mapped
		}
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func remapWorkflowInputs(inputs map[string][]string, roleMap map[string]string) map[string][]string {
	if len(inputs) == 0 {
		return map[string][]string{}
	}
	out := map[string][]string{}
	for role, keys := range inputs {
		if mapped, ok := roleMap[role]; ok {
			role = mapped
		}
		out[role] = append(out[role], keys...)
	}
	return out
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
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
	if len(cw.Agents) > 0 {
		return &cw, nil
	}
	if imported, ok := customWorkflowFromAgentSpec(data); ok {
		return imported, nil
	}
	return &cw, nil
}

type externalAgentSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type externalAgentStage struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	Skill            string            `json:"skill"`
	Actions          []json.RawMessage `json:"actions"`
	Conditions       []json.RawMessage `json:"conditions"`
	SuccessCondition string            `json:"success_condition"`
	FailureAction    string            `json:"failure_action"`
}

type externalAgentSpec struct {
	Agent struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Type        string `json:"type"`
	} `json:"agent"`
	Purpose struct {
		PrimaryGoal string   `json:"primary_goal"`
		Outcomes    []string `json:"outcomes"`
	} `json:"purpose"`
	Skills struct {
		Required []externalAgentSkill `json:"required"`
		Optional []externalAgentSkill `json:"optional"`
	} `json:"skills"`
	Workflow struct {
		Stages []externalAgentStage `json:"stages"`
	} `json:"workflow"`
}

func customWorkflowFromAgentSpec(data []byte) (*CustomWorkflow, bool) {
	var spec externalAgentSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, false
	}
	if spec.Agent.ID == "" && spec.Agent.Name == "" && len(spec.Workflow.Stages) == 0 {
		return nil, false
	}

	workflowName := firstNonEmpty(spec.Agent.ID, spec.Agent.Name, "imported-workflow")
	mainRole := safeWorkflowID(firstNonEmpty(spec.Agent.ID, spec.Agent.Name, "imported-agent"))
	if mainRole == "master" {
		mainRole = "imported-agent"
	}
	memoryRole := uniqueWorkflowRole("memory-context", mainRole)
	toolRole := uniqueWorkflowRole("tool-runner", mainRole, memoryRole)
	description := firstNonEmpty(spec.Agent.Description, spec.Purpose.PrimaryGoal, spec.Agent.Name, "Imported custom agent workflow")

	master := CustomAgent{
		Role:        "master",
		Description: "Node master dari import workflow. Baca tujuan workflow, koordinasikan agent utama, memory, dan tool runner.",
		Skills:      []string{"orchestrator"},
		Tasks: []Task{{
			ID:          "coordinate",
			Description: externalWorkflowDescription(spec),
		}},
		DependsOn:  []string{},
		InputsFrom: map[string][]string{},
	}
	memory := CustomAgent{
		Role:        memoryRole,
		Description: "Memory node untuk menyimpan konteks import, tujuan, output, guardrail, dan kontrak antar node workflow.",
		Skills:      []string{"memory"},
		Tasks: []Task{{
			ID:          "workflow-context",
			Description: externalMemoryTaskDescription(spec),
		}},
		DependsOn: []string{"master"},
		InputsFrom: map[string][]string{
			"master": {"workflow_goal", "outcomes"},
		},
		Memory: &MemoryNodeConfig{Action: "shared", Limit: 5},
	}
	tools := CustomAgent{
		Role:        toolRole,
		Description: "Tool node hasil import. Jalankan atau terjemahkan action eksternal seperti shell, GitHub Actions, validasi, build, publish, dan verifikasi.",
		Skills:      []string{"tool"},
		Tasks:       externalToolTasks(spec),
		DependsOn:   []string{"master"},
		InputsFrom: map[string][]string{
			"master": {"workflow_goal"},
		},
	}
	main := CustomAgent{
		Role:        mainRole,
		Description: description,
		Skills:      externalWorkflowSkills(spec),
		Tasks:       externalAgentTasks(spec, description),
		DependsOn:   []string{"master", memoryRole, toolRole},
		InputsFrom: map[string][]string{
			"master":   {"workflow_goal", "outcomes"},
			memoryRole: {"workflow_context", "guardrails"},
			toolRole:   {"tool_actions", "execution_plan"},
		},
	}

	return &CustomWorkflow{
		Name:        workflowName,
		Description: externalWorkflowDescription(spec),
		Agents:      []CustomAgent{master, main, memory, tools},
	}, true
}

func externalAgentTasks(spec externalAgentSpec, fallback string) []Task {
	tasks := make([]Task, 0, len(spec.Workflow.Stages))
	for i, stage := range spec.Workflow.Stages {
		taskID := safeWorkflowID(firstNonEmpty(stage.ID, stage.Name, fmt.Sprintf("stage-%d", i+1)))
		tasks = append(tasks, Task{ID: taskID, Description: externalStageTaskDescription(stage)})
	}
	if len(tasks) == 0 {
		tasks = append(tasks, Task{ID: "main", Description: fallback})
	}
	return tasks
}

func externalToolTasks(spec externalAgentSpec) []Task {
	tasks := []Task{}
	for i, stage := range spec.Workflow.Stages {
		if len(stage.Actions) == 0 && len(stage.Conditions) == 0 {
			continue
		}
		id := safeWorkflowID(firstNonEmpty(stage.ID, stage.Name, fmt.Sprintf("stage-%d", i+1)))
		tasks = append(tasks, Task{
			ID:          id + "-tools",
			Description: externalToolTaskDescription(stage),
			Type:        "tool",
			ToolName:    externalStageToolName(stage),
		})
	}
	if len(tasks) == 0 {
		tasks = append(tasks, Task{ID: "tool-plan", Description: "Tidak ada action eksplisit di JSON import. Terjemahkan instruksi workflow menjadi rencana tool yang aman.", Type: "tool"})
	}
	return tasks
}

func externalMemoryTaskDescription(spec externalAgentSpec) string {
	parts := []string{}
	if spec.Agent.Name != "" {
		parts = append(parts, "Agent: "+spec.Agent.Name)
	}
	if spec.Agent.Description != "" {
		parts = append(parts, "Description: "+spec.Agent.Description)
	}
	if spec.Purpose.PrimaryGoal != "" {
		parts = append(parts, "Goal: "+spec.Purpose.PrimaryGoal)
	}
	if len(spec.Purpose.Outcomes) > 0 {
		parts = append(parts, "Outcomes: "+strings.Join(spec.Purpose.Outcomes, "; "))
	}
	if len(spec.Workflow.Stages) > 0 {
		stageNames := make([]string, 0, len(spec.Workflow.Stages))
		for _, stage := range spec.Workflow.Stages {
			stageNames = append(stageNames, firstNonEmpty(stage.Name, stage.ID, "stage"))
		}
		parts = append(parts, "Stages: "+strings.Join(stageNames, " -> "))
	}
	return strings.Join(parts, "\n")
}

func externalToolTaskDescription(stage externalAgentStage) string {
	parts := []string{firstNonEmpty(stage.Name, stage.ID, "Imported tool stage")}
	if stage.Description != "" {
		parts = append(parts, stage.Description)
	}
	if len(stage.Actions) > 0 {
		parts = append(parts, "Tool actions: "+summarizeExternalActions(stage.Actions))
		if raw := compactExternalActions(stage.Actions); raw != "" {
			parts = append(parts, "Action JSON: "+raw)
		}
	}
	if len(stage.Conditions) > 0 {
		parts = append(parts, fmt.Sprintf("Condition blocks: %d", len(stage.Conditions)))
	}
	return strings.Join(parts, "\n")
}

func externalStageToolName(stage externalAgentStage) string {
	for _, raw := range stage.Actions {
		var action map[string]interface{}
		if err := json.Unmarshal(raw, &action); err != nil {
			continue
		}
		if tool := firstString(action, "tool_name", "tool", "type", "uses"); tool != "" {
			return safeWorkflowID(tool)
		}
	}
	if len(stage.Conditions) > 0 {
		return "conditional"
	}
	return "tool"
}

func compactExternalActions(actions []json.RawMessage) string {
	items := make([]string, 0, len(actions))
	for _, raw := range actions {
		var out bytes.Buffer
		if err := json.Compact(&out, raw); err != nil {
			continue
		}
		items = append(items, out.String())
	}
	if len(items) == 0 {
		return ""
	}
	joined := strings.Join(items, "; ")
	if len(joined) > 1200 {
		return joined[:1200] + "..."
	}
	return joined
}

func uniqueWorkflowRole(base string, existing ...string) string {
	used := map[string]bool{}
	for _, role := range existing {
		used[role] = true
	}
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !used[candidate] {
			return candidate
		}
	}
}

func externalWorkflowDescription(spec externalAgentSpec) string {
	parts := []string{}
	if spec.Purpose.PrimaryGoal != "" {
		parts = append(parts, spec.Purpose.PrimaryGoal)
	} else if spec.Agent.Description != "" {
		parts = append(parts, spec.Agent.Description)
	}
	if len(spec.Purpose.Outcomes) > 0 {
		parts = append(parts, "Outcomes: "+strings.Join(spec.Purpose.Outcomes, "; "))
	}
	return strings.Join(parts, "\n")
}

func externalWorkflowSkills(spec externalAgentSpec) []string {
	seen := map[string]bool{}
	skills := []string{}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		skills = append(skills, name)
	}
	for _, item := range spec.Skills.Required {
		add(item.Name)
	}
	for _, item := range spec.Skills.Optional {
		add(item.Name)
	}
	for _, stage := range spec.Workflow.Stages {
		add(stage.Skill)
	}
	if len(skills) == 0 {
		add(spec.Agent.Type)
	}
	return skills
}

func externalStageTaskDescription(stage externalAgentStage) string {
	parts := []string{}
	if stage.Name != "" {
		parts = append(parts, stage.Name)
	}
	if stage.Description != "" {
		parts = append(parts, stage.Description)
	}
	if stage.Skill != "" {
		parts = append(parts, "Skill: "+stage.Skill)
	}
	if len(stage.Actions) > 0 {
		parts = append(parts, "Actions: "+summarizeExternalActions(stage.Actions))
	}
	if len(stage.Conditions) > 0 {
		parts = append(parts, fmt.Sprintf("Conditions: %d condition block(s)", len(stage.Conditions)))
	}
	if stage.SuccessCondition != "" {
		parts = append(parts, "Success: "+stage.SuccessCondition)
	}
	if stage.FailureAction != "" {
		parts = append(parts, "Failure action: "+stage.FailureAction)
	}
	return strings.Join(parts, "\n")
}

func summarizeExternalActions(actions []json.RawMessage) string {
	summary := make([]string, 0, len(actions))
	for _, raw := range actions {
		var action map[string]interface{}
		if err := json.Unmarshal(raw, &action); err != nil {
			continue
		}
		label := firstString(action, "name", "type", "uses", "run")
		if label == "" {
			label = "action"
		}
		summary = append(summary, label)
	}
	if len(summary) == 0 {
		return fmt.Sprintf("%d action(s)", len(actions))
	}
	return strings.Join(summary, "; ")
}

func firstString(values map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func safeWorkflowID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if valid {
			b.WriteRune(r)
			lastDash = r == '-'
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(b.String(), "-_")
	if result == "" {
		return "imported"
	}
	return result
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
