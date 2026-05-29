package workflow

import (
	"fmt"
	"sort"
	"strings"
)

// Planner converts a top-level orchestration task into a validated execution plan.
type Planner struct{}

// NewPlanner creates a lightweight deterministic orchestration planner.
func NewPlanner() *Planner {
	return &Planner{}
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

	subtasks := p.decompose(task)
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
