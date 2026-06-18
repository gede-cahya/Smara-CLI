package workflow

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gede-cahya/Smara-CLI/internal/llm"
)

// Planner converts a top-level orchestration task into a validated execution plan.
//
// Decomposition follows "LLM proposes, rules validate": when a provider is set,
// an LLM drafts context-aware subtasks; the deterministic rules then validate
// the DAG and clamp risk (escalate-only). Without a provider — or when the LLM
// output is unusable — it falls back to the rule-based decompose().
type Planner struct {
	provider llm.Provider
}

// NewPlanner creates a lightweight deterministic orchestration planner.
func NewPlanner() *Planner {
	return &Planner{}
}

// NewLLMPlanner creates a planner that drafts subtasks via the LLM and validates
// them with the deterministic rules. A nil provider degrades to rule-based.
func NewLLMPlanner(provider llm.Provider) *Planner {
	return &Planner{provider: provider}
}

// riskRank orders risk levels so they can be compared for escalate-only policy.
func riskRank(r RiskLevel) int {
	switch r {
	case RiskCritical:
		return 3
	case RiskHigh:
		return 2
	case RiskMedium:
		return 1
	default:
		return 0
	}
}

// maxRisk returns the higher of two risk levels. Used for escalate-only: an
// LLM suggestion may raise risk above the rule baseline but never lower it.
func maxRisk(a, b RiskLevel) RiskLevel {
	if riskRank(b) > riskRank(a) {
		return b
	}
	return a
}

// Plan decomposes a top-level task into safe, conventional subtasks and validates the resulting DAG.
func (p *Planner) Plan(task OrchestrationTask) (ExecutionPlan, error) {
	if strings.TrimSpace(task.ID) == "" {
		task.ID = "task-1"
	}
	if strings.TrimSpace(task.Title) == "" {
		task.Title = "Orchestration task"
	}
	if strings.TrimSpace(task.Description) == "" {
		return ExecutionPlan{}, fmt.Errorf("task description cannot be empty")
	}
	if task.Kind == "" {
		task.Kind = classifyTaskKind(task.Description)
	}
	if task.RiskLevel == "" {
		task.RiskLevel = riskForKind(task.Kind)
	}

	// LLM proposes, rules validate. Fall back to rule-based decompose() when
	// no provider is set or the LLM output is unusable, so a workflow never
	// blanks out on a bad plan.
	subtasks, ok := p.decomposeLLM(task)
	if !ok {
		subtasks = p.decompose(task)
	}
	deps := dependenciesFromSubtasks(subtasks)
	plan := ExecutionPlan{
		ID:           "plan-" + task.ID,
		Task:         task,
		Subtasks:     subtasks,
		Dependencies: deps,
	}
	if err := ValidateExecutionPlan(plan); err != nil {
		return ExecutionPlan{}, err
	}
	return plan, nil
}

func (p *Planner) decompose(task OrchestrationTask) []Subtask {
	desc := strings.ToLower(task.Description + " " + task.Title)
	includesMutation := textContainsAny(desc, "edit", "ubah", "perbaiki", "fix", "implement", "tambahkan", "hapus", "delete", "refactor") || task.Kind != TaskKindReadOnly
	includesDeploy := textContainsAny(desc, "deploy", "production", "prod", "server", "vps") || task.Kind == TaskKindRemote || task.Kind == TaskKindProductionImpacting

	subtasks := []Subtask{
		NewSubtask("analyze-context", "Analyze context", "Inspect available context and clarify the requested outcome."),
		NewSubtask("inspect-workspace", "Inspect workspace", "Scan project structure and relevant configuration."),
		NewSubtask("search-related-code", "Search related code", "Search for files, symbols, and references related to the task."),
		NewSubtask("summarize-findings", "Summarize findings", "Combine discovery results and identify the safest implementation path."),
	}
	subtasks[3].DependsOn = []string{"analyze-context", "inspect-workspace", "search-related-code"}
	subtasks[3].CanParallel = false

	if includesMutation {
		approval := NewSubtask("approval-gate", "Request approval", "Request approval before mutating files or remote state.")
		approval.DependsOn = []string{"summarize-findings"}
		approval.Kind = TaskKindMutating
		approval.RiskLevel = RiskHigh
		approval.CanParallel = false
		approval.Status = StatusWaitingApproval

		apply := NewSubtask("apply-change", "Apply change", "Apply the approved code or configuration change.")
		apply.DependsOn = []string{"approval-gate"}
		apply.Kind = task.Kind
		if apply.Kind == TaskKindReadOnly {
			apply.Kind = TaskKindMutating
		}
		apply.RiskLevel = riskForKind(apply.Kind)
		apply.CanParallel = false

		verify := NewSubtask("verify-change", "Verify change", "Run targeted validation after the change.")
		verify.DependsOn = []string{"apply-change"}
		verify.CanParallel = false
		verify.RiskLevel = RiskMedium

		subtasks = append(subtasks, approval, apply, verify)
	} else {
		report := NewSubtask("produce-report", "Produce report", "Create a final read-only report from the findings.")
		report.DependsOn = []string{"summarize-findings"}
		report.CanParallel = false
		subtasks = append(subtasks, report)
	}

	if includesDeploy {
		deploy := NewSubtask("deploy-or-remote-step", "Deploy or remote step", "Perform the approved deployment or remote operation.")
		deploy.DependsOn = []string{lastSubtaskID(subtasks)}
		deploy.Kind = TaskKindProductionImpacting
		deploy.RiskLevel = RiskCritical
		deploy.CanParallel = false
		subtasks = append(subtasks, deploy)
	}

	return subtasks
}

// llmSubtask is the JSON shape the planner LLM emits per subtask. Risk/kind are
// advisory: the rules clamp them (escalate-only) after parsing.
type llmSubtask struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Kind        string   `json:"kind"`
	RiskLevel   string   `json:"risk_level"`
	DependsOn   []string `json:"depends_on"`
	CanParallel bool     `json:"can_parallel"`
}

type llmPlanResponse struct {
	Subtasks []llmSubtask `json:"subtasks"`
}

const plannerSubtaskSchema = `{
  "subtasks": [
    {
      "id": "string (kebab-case, unik)",
      "title": "string (singkat)",
      "description": "string (apa yang dikerjakan, konkret)",
      "kind": "read_only|mutating|destructive|remote|production_impacting",
      "risk_level": "low|medium|high|critical",
      "depends_on": ["id subtask lain yang harus selesai dulu"],
      "can_parallel": true
    }
  ]
}`

const plannerSystemPrompt = `Kamu adalah perencana eksekusi (execution planner) untuk agen Smara.
Pecah permintaan user menjadi subtask yang spesifik dan kontekstual — JUMLAH dan ISI subtask harus menyesuaikan tugas nyata, BUKAN template tetap.

ATURAN:
- Setiap subtask punya id unik (kebab-case) dan deskripsi konkret.
- depends_on hanya boleh merujuk id subtask lain yang ADA di output. Jangan buat siklus.
- Subtask read-only yang independen boleh can_parallel=true; subtask yang mengubah file/remote state harus can_parallel=false.
- Jika tugas mengubah kode/file/remote atau berisiko, sertakan langkah verifikasi setelahnya.
- Tandai kind & risk_level sejujurnya. Sistem akan menaikkan risk bila perlu (kamu tak bisa menurunkannya), jadi jangan meremehkan.

Output HANYA JSON valid sesuai schema. Tidak ada teks di luar JSON.`

// decomposeLLM asks the LLM to draft subtasks, then maps them into validated
// Subtasks with escalate-only risk. Returns ok=false when the provider is unset
// or the output cannot be used, so the caller can fall back to rule-based.
func (p *Planner) decomposeLLM(task OrchestrationTask) ([]Subtask, bool) {
	if p.provider == nil {
		return nil, false
	}

	baselineKind := task.Kind
	if baselineKind == "" {
		baselineKind = classifyTaskKind(task.Description + " " + task.Title)
	}
	baselineRisk := riskForKind(baselineKind)

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: plannerSystemPrompt},
		{Role: llm.RoleSystem, Content: "Schema JSON wajib:\n" + plannerSubtaskSchema},
		{Role: llm.RoleUser, Content: fmt.Sprintf("Judul: %s\n\nDeskripsi tugas:\n%s", task.Title, task.Description)},
	}

	resp, err := p.provider.Chat(messages)
	if err != nil || resp == nil {
		return nil, false
	}

	var parsed llmPlanResponse
	if err := json.Unmarshal([]byte(extractJSON(resp.Content)), &parsed); err != nil {
		return nil, false
	}
	if len(parsed.Subtasks) == 0 {
		return nil, false
	}

	subtasks := make([]Subtask, 0, len(parsed.Subtasks))
	seen := map[string]bool{}
	for _, ls := range parsed.Subtasks {
		id := strings.TrimSpace(ls.ID)
		if id == "" || seen[id] {
			return nil, false // malformed: missing/duplicate id → fall back
		}
		seen[id] = true

		st := NewSubtask(id, strings.TrimSpace(ls.Title), strings.TrimSpace(ls.Description))
		if st.Title == "" {
			st.Title = id
		}
		if st.Description == "" {
			return nil, false
		}
		st.DependsOn = sanitizeDeps(ls.DependsOn, id)
		st.CanParallel = ls.CanParallel

		// Escalate-only: rule baseline is the floor; the LLM may raise it.
		st.Kind = escalateKind(baselineKind, parseTaskKind(ls.Kind))
		st.RiskLevel = maxRisk(baselineRisk, parseRiskLevel(ls.RiskLevel))
		st.RiskLevel = maxRisk(st.RiskLevel, riskForKind(st.Kind))
		if st.RiskLevel == RiskHigh || st.RiskLevel == RiskCritical {
			st.CanParallel = false
		}
		subtasks = append(subtasks, st)
	}

	// Validate dependency references now so we can fall back cleanly instead of
	// surfacing a hard error from Plan().
	if err := ValidateExecutionPlan(ExecutionPlan{Subtasks: subtasks}); err != nil {
		return nil, false
	}
	return subtasks, true
}

func sanitizeDeps(deps []string, selfID string) []string {
	out := make([]string, 0, len(deps))
	for _, d := range deps {
		d = strings.TrimSpace(d)
		if d == "" || d == selfID {
			continue
		}
		out = append(out, d)
	}
	return out
}

func parseTaskKind(s string) TaskKind {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "mutating":
		return TaskKindMutating
	case "destructive":
		return TaskKindDestructive
	case "remote":
		return TaskKindRemote
	case "production_impacting":
		return TaskKindProductionImpacting
	default:
		return TaskKindReadOnly
	}
}

func parseRiskLevel(s string) RiskLevel {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return RiskCritical
	case "high":
		return RiskHigh
	case "medium":
		return RiskMedium
	default:
		return RiskLow
	}
}

// escalateKind returns the higher-impact of two task kinds using risk ordering.
func escalateKind(a, b TaskKind) TaskKind {
	if riskRank(riskForKind(b)) > riskRank(riskForKind(a)) {
		return b
	}
	return a
}

// ValidateExecutionPlan validates subtask IDs, dependency references, and DAG acyclicity.
func ValidateExecutionPlan(plan ExecutionPlan) error {
	ids := map[string]bool{}
	depMap := map[string][]string{}
	for _, st := range plan.Subtasks {
		if strings.TrimSpace(st.ID) == "" {
			return fmt.Errorf("subtask id cannot be empty")
		}
		if ids[st.ID] {
			return fmt.Errorf("duplicate subtask id %q", st.ID)
		}
		ids[st.ID] = true
		depMap[st.ID] = append([]string{}, st.DependsOn...)
	}
	for _, st := range plan.Subtasks {
		for _, dep := range st.DependsOn {
			if !ids[dep] {
				return fmt.Errorf("subtask %q depends on unknown subtask %q", st.ID, dep)
			}
			if dep == st.ID {
				return fmt.Errorf("subtask %q cannot depend on itself", st.ID)
			}
		}
	}
	return validateAcyclic(depMap)
}

func validateAcyclic(deps map[string][]string) error {
	visited := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if visited[id] == 1 {
			return fmt.Errorf("circular dependency detected at subtask %q", id)
		}
		if visited[id] == 2 {
			return nil
		}
		visited[id] = 1
		children := append([]string{}, deps[id]...)
		sort.Strings(children)
		for _, dep := range children {
			if err := visit(dep); err != nil {
				return err
			}
		}
		visited[id] = 2
		return nil
	}
	keys := make([]string, 0, len(deps))
	for id := range deps {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func dependenciesFromSubtasks(subtasks []Subtask) []Dependency {
	var deps []Dependency
	for _, st := range subtasks {
		for _, parent := range st.DependsOn {
			deps = append(deps, Dependency{FromID: parent, ToID: st.ID, Type: DependencyRequires, Reason: "declared by planner"})
		}
	}
	return deps
}

func classifyTaskKind(text string) TaskKind {
	lower := strings.ToLower(text)
	switch {
	case textContainsAny(lower, "production", "prod", "deploy"):
		return TaskKindProductionImpacting
	case textContainsAny(lower, "vps", "server", "remote", "ssh"):
		return TaskKindRemote
	case textContainsAny(lower, "delete", "hapus", "remove", "drop", "destroy"):
		return TaskKindDestructive
	case textContainsAny(lower, "edit", "ubah", "perbaiki", "fix", "implement", "tambahkan", "refactor"):
		return TaskKindMutating
	default:
		return TaskKindReadOnly
	}
}

func riskForKind(kind TaskKind) RiskLevel {
	switch kind {
	case TaskKindProductionImpacting, TaskKindDestructive:
		return RiskCritical
	case TaskKindRemote, TaskKindMutating:
		return RiskHigh
	default:
		return RiskLow
	}
}

func textContainsAny(text string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func lastSubtaskID(subtasks []Subtask) string {
	if len(subtasks) == 0 {
		return ""
	}
	return subtasks[len(subtasks)-1].ID
}
