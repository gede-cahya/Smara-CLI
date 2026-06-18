package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
)

// OrchestrationConfig controls the end-to-end subtask orchestration pipeline:
// LLM planning → safety guardrails → scheduling → Worker-backed execution.
type OrchestrationConfig struct {
	MaxConcurrency                 int
	MaxRepairAttempts              int
	RequireApprovalForHighRisk     bool
	RequireApprovalForCriticalRisk bool
	ApprovedSubtaskIDs             []string
	DryRun                         bool

	// ApprovalFunc gates subtasks the guardrail marked as waiting_approval. It
	// blocks until the user approves (true) or denies/times out (false). Nil →
	// gated subtasks are auto-denied (skipped).
	ApprovalFunc func(ctx context.Context, subtask Subtask) bool

	// Live callbacks (all optional).
	OnPlanReady     func(plan ExecutionPlan, report SafetyReport)
	OnSubtaskStart  func(st Subtask)
	OnSubtaskStream func(subtaskID, chunk string, isThinking bool)
	OnSubtaskResult func(result ExecutionResult)
	OnRepairAttempt func(subtaskID string, attempt int, prevError string)
}

// OrchestrationRun captures the full output of a pipeline run.
type OrchestrationRun struct {
	Plan         ExecutionPlan
	SafetyReport SafetyReport
	Execution    PlanExecutionResult
	Report       AggregatedReport
}

// RunOrchestration executes the subtask path end-to-end. This is the production
// engine: an LLM planner drafts context-aware subtasks, deterministic rules
// validate the DAG and clamp risk, the safety guardrail gates risky steps, the
// scheduler builds waves, and a Worker-backed executor runs each subtask with
// self-correction. Returns the run plus a WorkflowResult for UI compatibility.
func RunOrchestration(ctx context.Context, supervisor *agent.Supervisor, projectDir, prompt string, cfg OrchestrationConfig) (*OrchestrationRun, *WorkflowResult, error) {
	if strings.TrimSpace(prompt) == "" {
		return nil, nil, fmt.Errorf("prompt tidak boleh kosong")
	}

	// 1. Plan (LLM proposes, rules validate; falls back to rule-based inside Plan).
	planner := NewLLMPlanner(supervisor.GetProvider())
	plan, err := planner.Plan(OrchestrationTask{
		ID:          "web",
		Title:       extractProjectName(prompt),
		Description: prompt,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("planning gagal: %w", err)
	}

	// 2. Safety guardrails: classify destructive/remote, gate approvals, locks.
	guardrail := NewSafetyGuardrail(SafetyConfig{
		ApprovedSubtaskIDs:             cfg.ApprovedSubtaskIDs,
		RequireApprovalForHighRisk:     cfg.RequireApprovalForHighRisk,
		RequireApprovalForCriticalRisk: cfg.RequireApprovalForCriticalRisk,
	})
	plan, safetyReport, err := guardrail.ApplySafetyPolicy(plan)
	if err != nil {
		return nil, nil, fmt.Errorf("safety policy gagal: %w", err)
	}

	// 3. Schedule into dependency-ordered batches.
	plan, err = NewScheduler(SchedulerConfig{MaxConcurrency: cfg.MaxConcurrency}).Schedule(plan)
	if err != nil {
		return nil, nil, fmt.Errorf("scheduling gagal: %w", err)
	}

	if cfg.OnPlanReady != nil {
		cfg.OnPlanReady(plan, safetyReport)
	}

	// 4. Execute via a Worker-backed handler with self-correction.
	state := NewSharedState(projectDir)
	handler := NewWorkerSubtaskHandler(supervisor, state, WorkerHandlerConfig{
		MaxRepairAttempts: cfg.MaxRepairAttempts,
		OnSubtaskStart:    cfg.OnSubtaskStart,
		OnSubtaskStream:   cfg.OnSubtaskStream,
		OnRepairAttempt:   cfg.OnRepairAttempt,
	})

	// Wrap the handler so results stream to the UI as each subtask finishes.
	baseHandler := handler.Handler()
	wrapped := func(ctx context.Context, st Subtask) ExecutionResult {
		res := baseHandler(ctx, st)
		if cfg.OnSubtaskResult != nil {
			cfg.OnSubtaskResult(res)
		}
		return res
	}

	executor := NewExecutor(ExecutorConfig{
		MaxConcurrency: cfg.MaxConcurrency,
		DryRun:         cfg.DryRun,
		ApprovalFunc:   cfg.ApprovalFunc,
	}, wrapped)

	execResult, err := executor.Execute(ctx, plan)
	if err != nil {
		return nil, nil, fmt.Errorf("eksekusi gagal: %w", err)
	}
	_ = state.Save()

	// 5. Aggregate a final report.
	report := NewReportAggregator().Aggregate(plan, execResult)

	run := &OrchestrationRun{
		Plan:         plan,
		SafetyReport: safetyReport,
		Execution:    execResult,
		Report:       report,
	}
	return run, run.toWorkflowResult(projectDir), nil
}

// toWorkflowResult maps the orchestration run into the legacy WorkflowResult
// shape so existing web/cmd callers keep working unchanged.
func (r *OrchestrationRun) toWorkflowResult(projectDir string) *WorkflowResult {
	agentOutputs := map[string][]agent.TaskResult{}
	for _, st := range r.Plan.Subtasks {
		res := r.Execution.Results[st.ID]
		status := agent.TaskCompleted
		if res.Status == StatusFailed {
			status = agent.TaskFailed
		}
		agentOutputs[st.ID] = []agent.TaskResult{{
			TaskID: st.ID,
			Status: status,
			Output: res.Stdout,
			Error:  res.Error,
		}}
	}

	parallel := false
	for _, b := range r.Plan.Batches {
		if b.Mode == BatchModeParallel && len(b.SubtaskIDs) > 1 {
			parallel = true
			break
		}
	}

	qaStatus := "PASS"
	if r.Execution.Status == StatusFailed {
		qaStatus = "FAIL"
	}

	return &WorkflowResult{
		ProjectPath:       projectDir,
		Domain:            "orchestration",
		PRD:               r.Plan.Task.Description,
		AgentOutputs:      agentOutputs,
		QAResult:          QAResult{Status: qaStatus, Report: r.Report.Markdown()},
		FinalSummary:      r.Report.Markdown(),
		ParallelExecution: parallel,
		MaxConcurrency:    r.maxConcurrency(),
	}
}

func (r *OrchestrationRun) maxConcurrency() int {
	max := 1
	for _, b := range r.Plan.Batches {
		if b.MaxConcurrency > max {
			max = b.MaxConcurrency
		}
	}
	return max
}
