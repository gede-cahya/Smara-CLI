package workflow

import (
	"fmt"
	"sort"
	"strings"
)

// SafetyConfig controls policy decisions before scheduling/execution.
type SafetyConfig struct {
	// ApprovedSubtaskIDs lists high/critical subtasks explicitly approved by the user.
	ApprovedSubtaskIDs []string
	// RequireApprovalForHighRisk gates high-risk subtasks. Defaults to true.
	RequireApprovalForHighRisk bool
	// RequireApprovalForCriticalRisk gates critical subtasks. Defaults to true.
	RequireApprovalForCriticalRisk bool
}

// SafetyFinding describes one guardrail decision applied to a plan.
type SafetyFinding struct {
	SubtaskID string    `json:"subtask_id,omitempty"`
	Severity  RiskLevel `json:"severity"`
	Policy    string    `json:"policy"`
	Message   string    `json:"message"`
}

// SafetyReport summarizes all safety guardrail decisions.
type SafetyReport struct {
	RequiresApproval []string          `json:"requires_approval"`
	Conflicts        []string          `json:"conflicts"`
	ResourceLocks    map[string]string `json:"resource_locks"`
	Findings         []SafetyFinding   `json:"findings"`
}

// SafetyGuardrail applies risk, approval, conflict, and rollback policies to an execution plan.
type SafetyGuardrail struct {
	config SafetyConfig
}

// NewSafetyGuardrail creates a guardrail with safe defaults.
func NewSafetyGuardrail(config SafetyConfig) *SafetyGuardrail {
	return &SafetyGuardrail{config: config}
}

// ApplySafetyPolicy annotates and normalizes a plan before scheduling/execution.
func (g *SafetyGuardrail) ApplySafetyPolicy(plan ExecutionPlan) (ExecutionPlan, SafetyReport, error) {
	if err := ValidateExecutionPlan(plan); err != nil {
		return ExecutionPlan{}, SafetyReport{}, err
	}

	report := SafetyReport{ResourceLocks: map[string]string{}}
	approved := map[string]bool{}
	for _, id := range g.config.ApprovedSubtaskIDs {
		approved[id] = true
	}

	for i := range plan.Subtasks {
		st := &plan.Subtasks[i]
		ensureSubtaskMetadata(st)
		g.classifyDestructiveOrRemote(st, &report)
		g.applyApprovalPolicy(st, approved, &report)
		g.applyRollbackHint(st, &report)
		g.applyResourceLocks(st, &report)
	}

	g.detectBatchConflicts(&plan, &report)
	return plan, report, nil
}

func (g *SafetyGuardrail) requireHighApproval() bool {
	return !g.config.RequireApprovalForHighRisk || g.config.RequireApprovalForHighRisk
}
func (g *SafetyGuardrail) requireCriticalApproval() bool {
	return !g.config.RequireApprovalForCriticalRisk || g.config.RequireApprovalForCriticalRisk
}

func (g *SafetyGuardrail) classifyDestructiveOrRemote(st *Subtask, report *SafetyReport) {
	text := strings.ToLower(st.Title + " " + st.Description)
	if textContainsAny(text, "delete", "remove", "drop", "truncate", "destroy", "rm -rf") {
		st.Kind = TaskKindDestructive
		st.RiskLevel = RiskCritical
		st.CanParallel = false
		report.Findings = append(report.Findings, SafetyFinding{SubtaskID: st.ID, Severity: RiskCritical, Policy: "destructive-command", Message: "destructive operation promoted to critical risk"})
	}
	if textContainsAny(text, "ssh", "vps", "server", "remote", "systemctl", "restart service") && st.RiskLevel != RiskCritical {
		st.Kind = TaskKindRemote
		st.RiskLevel = RiskHigh
		st.CanParallel = false
		report.Findings = append(report.Findings, SafetyFinding{SubtaskID: st.ID, Severity: RiskHigh, Policy: "remote-command", Message: "remote operation requires serialized high-risk handling"})
	}
}

func (g *SafetyGuardrail) applyApprovalPolicy(st *Subtask, approved map[string]bool, report *SafetyReport) {
	needsApproval := (st.RiskLevel == RiskHigh && g.requireHighApproval()) || (st.RiskLevel == RiskCritical && g.requireCriticalApproval()) || st.Status == StatusWaitingApproval
	if !needsApproval || approved[st.ID] {
		return
	}
	st.Status = StatusWaitingApproval
	st.CanParallel = false
	report.RequiresApproval = append(report.RequiresApproval, st.ID)
	report.Findings = append(report.Findings, SafetyFinding{SubtaskID: st.ID, Severity: st.RiskLevel, Policy: "approval-gate", Message: "subtask is gated until explicit approval is provided"})
}

func (g *SafetyGuardrail) applyRollbackHint(st *Subtask, report *SafetyReport) {
	if st.RiskLevel != RiskHigh && st.RiskLevel != RiskCritical {
		return
	}
	if _, ok := st.Metadata["rollback_hint"]; !ok {
		st.Metadata["rollback_hint"] = defaultRollbackHint(*st)
		report.Findings = append(report.Findings, SafetyFinding{SubtaskID: st.ID, Severity: st.RiskLevel, Policy: "rollback-hint", Message: "rollback hint attached for high/critical action"})
	}
}

func (g *SafetyGuardrail) applyResourceLocks(st *Subtask, report *SafetyReport) {
	if st.Kind == TaskKindReadOnly && st.RiskLevel == RiskLow {
		return
	}
	for _, res := range metadataResources(st.Metadata) {
		if owner, exists := report.ResourceLocks[res]; exists && owner != st.ID {
			report.Conflicts = append(report.Conflicts, fmt.Sprintf("resource %s touched by %s and %s", res, owner, st.ID))
		} else {
			report.ResourceLocks[res] = st.ID
		}
	}
}

func (g *SafetyGuardrail) detectBatchConflicts(plan *ExecutionPlan, report *SafetyReport) {
	if len(plan.Batches) == 0 {
		return
	}
	byID := subtaskMap(plan.Subtasks)
	for i := range plan.Batches {
		seen := map[string]string{}
		for _, id := range plan.Batches[i].SubtaskIDs {
			st := byID[id]
			if st.Kind == TaskKindReadOnly && st.RiskLevel == RiskLow {
				continue
			}
			for _, res := range metadataResources(st.Metadata) {
				if other, ok := seen[res]; ok && other != id {
					plan.Batches[i].Mode = BatchModeSerial
					plan.Batches[i].MaxConcurrency = 1
					report.Conflicts = append(report.Conflicts, fmt.Sprintf("batch %s serial fallback for resource %s (%s, %s)", plan.Batches[i].ID, res, other, id))
				}
				seen[res] = id
			}
		}
	}
	sort.Strings(report.Conflicts)
}

func ensureSubtaskMetadata(st *Subtask) {
	if st.Metadata == nil {
		st.Metadata = map[string]interface{}{}
	}
}

func metadataResources(metadata map[string]interface{}) []string {
	if metadata == nil {
		return nil
	}
	var out []string
	add := func(v string) {
		if strings.TrimSpace(v) != "" {
			out = append(out, strings.TrimSpace(v))
		}
	}
	if v, ok := metadata["resource"].(string); ok {
		add(v)
	}
	if v, ok := metadata["file"].(string); ok {
		add("file:" + v)
	}
	if values, ok := metadata["resources"].([]string); ok {
		for _, v := range values {
			add(v)
		}
	}
	if values, ok := metadata["files"].([]string); ok {
		for _, v := range values {
			add("file:" + v)
		}
	}
	sort.Strings(out)
	return out
}

func defaultRollbackHint(st Subtask) string {
	switch st.Kind {
	case TaskKindDestructive:
		return "verify backup/snapshot, document affected resources, and prepare restore steps before execution"
	case TaskKindRemote, TaskKindProductionImpacting:
		return "capture current service state, keep previous artifact/config, and prepare restart/revert command"
	default:
		return "record changed files and keep a revert plan before execution"
	}
}
