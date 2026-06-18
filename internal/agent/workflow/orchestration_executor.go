package workflow

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// SubtaskExecutorFunc executes a single orchestration subtask and returns its result.
type SubtaskExecutorFunc func(ctx context.Context, subtask Subtask) ExecutionResult

// ExecutorConfig controls orchestration execution behavior.
type ExecutorConfig struct {
	// MaxConcurrency caps worker count for parallel batches. Batch-level limits still apply.
	MaxConcurrency int
	// ContinueOnLowRiskFailure lets independent low-risk failures be reported without cancelling the whole plan.
	ContinueOnLowRiskFailure bool
	// DryRun returns a plan-shaped result without invoking the subtask handler.
	DryRun bool
	// ParallelOrchestration disables parallel execution when explicitly set to false via pointer.
	// Nil preserves the default enabled behavior for backward compatibility.
	ParallelOrchestration *bool
	// Logger receives lightweight observability events. Nil disables logging.
	Logger func(event string, fields map[string]interface{})
	// ApprovalFunc gates a subtask whose Status is StatusWaitingApproval. It
	// blocks until the user approves (true) or denies/times out (false). When
	// nil, gated subtasks are auto-denied (skipped) to stay safe by default.
	// Only invoked for subtasks the safety guardrail marked as waiting; the
	// non-gated execution path is untouched.
	ApprovalFunc func(ctx context.Context, subtask Subtask) bool
}

// PlanExecutionResult captures the result of executing an orchestration plan.
type PlanExecutionResult struct {
	PlanID    string                     `json:"plan_id"`
	Status    SubtaskStatus              `json:"status"`
	Results   map[string]ExecutionResult `json:"results"`
	StartedAt time.Time                  `json:"started_at"`
	EndedAt   time.Time                  `json:"ended_at"`
	Duration  time.Duration              `json:"duration"`
	Metadata  map[string]interface{}     `json:"metadata,omitempty"`
}

// Executor runs scheduled execution batches with dependency-aware status handling.
type Executor struct {
	config  ExecutorConfig
	handler SubtaskExecutorFunc
}

// NewExecutor creates an execution engine. A nil handler produces successful no-op results.
func NewExecutor(config ExecutorConfig, handler SubtaskExecutorFunc) *Executor {
	if handler == nil {
		handler = func(ctx context.Context, subtask Subtask) ExecutionResult {
			return ExecutionResult{SubtaskID: subtask.ID, Status: StatusSuccess}
		}
	}
	return &Executor{config: config, handler: handler}
}

// Execute runs a scheduled plan and returns stable per-subtask results.
func (e *Executor) Execute(ctx context.Context, plan ExecutionPlan) (PlanExecutionResult, error) {
	if err := ValidateExecutionPlan(plan); err != nil {
		return PlanExecutionResult{}, err
	}
	if len(plan.Batches) == 0 {
		scheduled, err := NewScheduler(SchedulerConfig{MaxConcurrency: e.config.MaxConcurrency}).Schedule(plan)
		if err != nil {
			return PlanExecutionResult{}, err
		}
		plan = scheduled
	}
	if !e.parallelEnabled() {
		plan = forceSerialExecution(plan)
	}

	startEvent := "execution_start"
	if e.config.DryRun {
		startEvent = "dry_run_start"
	}
	e.log(startEvent, map[string]interface{}{"plan_id": plan.ID, "batches": len(plan.Batches)})

	if e.config.DryRun {
		return e.dryRun(plan), nil
	}
	started := time.Now()
	out := PlanExecutionResult{
		PlanID:    plan.ID,
		Status:    StatusRunning,
		Results:   map[string]ExecutionResult{},
		StartedAt: started,
		Metadata:  map[string]interface{}{"batches": len(plan.Batches)},
	}
	subtasks := subtaskMap(plan.Subtasks)
	for _, batch := range plan.Batches {
		if ctx.Err() != nil {
			e.cancelPending(plan, out.Results)
			break
		}

		ready := make([]Subtask, 0, len(batch.SubtaskIDs))
		for _, id := range batch.SubtaskIDs {
			st := subtasks[id]
			if e.hasFailedDependency(st, out.Results) {
				out.Results[id] = ExecutionResult{SubtaskID: id, Status: StatusSkipped, Error: "dependency failed or skipped"}
				continue
			}
			ready = append(ready, st)
		}
		if len(ready) == 0 {
			continue
		}

		results := e.executeBatch(ctx, batch, ready)
		for _, result := range results {
			out.Results[result.SubtaskID] = result
		}

		if e.shouldStopAfterBatch(ready, results) {
			e.skipUnresolvedDependents(plan, out.Results)
			break
		}
	}

	e.cancelPending(plan, out.Results)
	out.EndedAt = time.Now()
	out.Duration = out.EndedAt.Sub(out.StartedAt)
	out.Status = summarizePlanStatus(out.Results)
	return out, nil
}

func (e *Executor) executeBatch(ctx context.Context, batch ExecutionBatch, subtasks []Subtask) []ExecutionResult {
	if batch.Mode == BatchModeSerial || batch.Mode == BatchModeGated || batch.MaxConcurrency <= 1 {
		results := make([]ExecutionResult, 0, len(subtasks))
		for _, st := range subtasks {
			results = append(results, e.executeOne(ctx, st))
		}
		return results
	}

	limit := batch.MaxConcurrency
	if e.config.MaxConcurrency > 0 && limit > e.config.MaxConcurrency {
		limit = e.config.MaxConcurrency
	}
	if limit < 1 {
		limit = 1
	}

	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make([]ExecutionResult, 0, len(subtasks))
	for _, st := range subtasks {
		st := st
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			result := e.executeOne(ctx, st)
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}()
	}
	wg.Wait()
	sort.Slice(results, func(i, j int) bool { return results[i].SubtaskID < results[j].SubtaskID })
	return results
}

func (e *Executor) executeOne(ctx context.Context, st Subtask) ExecutionResult {
	// Approval gate: a subtask the guardrail marked as waiting must be approved
	// before it runs. Blocks via ApprovalFunc; denied/timeout → skipped so its
	// dependents are also skipped by the normal dependency check.
	if st.Status == StatusWaitingApproval {
		e.log("approval_required", map[string]interface{}{"subtask": st.ID, "risk": string(st.RiskLevel)})
		approved := false
		if e.config.ApprovalFunc != nil {
			approved = e.config.ApprovalFunc(ctx, st)
		}
		if !approved {
			e.log("approval_denied", map[string]interface{}{"subtask": st.ID})
			return ExecutionResult{
				SubtaskID: st.ID,
				Status:    StatusSkipped,
				Error:     "subtask requires approval but was denied or timed out",
				Metadata:  map[string]interface{}{"approval": "denied"},
			}
		}
		e.log("approval_granted", map[string]interface{}{"subtask": st.ID})
	}

	attempts := st.RetryPolicy.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	var last ExecutionResult
	for attempt := 1; attempt <= attempts; attempt++ {
		runCtx := ctx
		cancel := func() {}
		if st.Timeout > 0 {
			runCtx, cancel = context.WithTimeout(ctx, st.Timeout)
		}
		started := time.Now()
		last = e.handler(runCtx, st)
		cancel()
		last.SubtaskID = st.ID
		if last.Duration == 0 {
			last.Duration = time.Since(started)
		}
		if runCtx.Err() != nil && last.Status != StatusSuccess {
			last.Status = StatusFailed
			last.Error = runCtx.Err().Error()
		}
		if last.Status == "" {
			last.Status = StatusSuccess
		}
		if last.Metadata == nil {
			last.Metadata = map[string]interface{}{}
		}
		last.Metadata["attempt"] = attempt
		if last.Status == StatusSuccess || attempt == attempts {
			return last
		}
		if st.RetryPolicy.Backoff > 0 {
			select {
			case <-ctx.Done():
				last.Status = StatusFailed
				last.Error = ctx.Err().Error()
				return last
			case <-time.After(st.RetryPolicy.Backoff):
			}
		}
	}
	return last
}

func (e *Executor) hasFailedDependency(st Subtask, results map[string]ExecutionResult) bool {
	for _, dep := range st.DependsOn {
		result, ok := results[dep]
		if !ok || result.Status != StatusSuccess {
			return true
		}
	}
	return false
}

func (e *Executor) shouldStopAfterBatch(subtasks []Subtask, results []ExecutionResult) bool {
	byID := map[string]Subtask{}
	for _, st := range subtasks {
		byID[st.ID] = st
	}
	for _, result := range results {
		if result.Status != StatusFailed {
			continue
		}
		st := byID[result.SubtaskID]
		if st.RiskLevel == RiskCritical || st.RiskLevel == RiskHigh || !e.config.ContinueOnLowRiskFailure {
			return true
		}
	}
	return false
}

func (e *Executor) skipUnresolvedDependents(plan ExecutionPlan, results map[string]ExecutionResult) {
	changed := true
	for changed {
		changed = false
		for _, st := range plan.Subtasks {
			if _, ok := results[st.ID]; ok {
				continue
			}
			if e.hasFailedDependency(st, results) {
				results[st.ID] = ExecutionResult{SubtaskID: st.ID, Status: StatusSkipped, Error: "dependency failed or skipped"}
				changed = true
			}
		}
	}
}

func (e *Executor) cancelPending(plan ExecutionPlan, results map[string]ExecutionResult) {
	for _, st := range plan.Subtasks {
		if _, ok := results[st.ID]; !ok {
			results[st.ID] = ExecutionResult{SubtaskID: st.ID, Status: StatusCancelled, Error: "not executed"}
		}
	}
}

func subtaskMap(subtasks []Subtask) map[string]Subtask {
	m := map[string]Subtask{}
	for _, st := range subtasks {
		m[st.ID] = st
	}
	return m
}

func summarizePlanStatus(results map[string]ExecutionResult) SubtaskStatus {
	hasFailure := false
	hasCancelled := false
	for _, result := range results {
		switch result.Status {
		case StatusFailed:
			hasFailure = true
		case StatusCancelled:
			hasCancelled = true
		}
	}
	if hasFailure {
		return StatusFailed
	}
	if hasCancelled {
		return StatusCancelled
	}
	return StatusSuccess
}

func validateBatchSubtasks(batch ExecutionBatch, subtasks map[string]Subtask) error {
	for _, id := range batch.SubtaskIDs {
		if _, ok := subtasks[id]; !ok {
			return fmt.Errorf("batch %s references unknown subtask %s", batch.ID, id)
		}
	}
	return nil
}

func (e *Executor) parallelEnabled() bool {
	return e.config.ParallelOrchestration == nil || *e.config.ParallelOrchestration
}

func (e *Executor) log(event string, fields map[string]interface{}) {
	if e.config.Logger != nil {
		e.config.Logger(event, fields)
	}
}

func (e *Executor) dryRun(plan ExecutionPlan) PlanExecutionResult {
	started := time.Now()
	results := make(map[string]ExecutionResult, len(plan.Subtasks))
	for _, st := range plan.Subtasks {
		results[st.ID] = ExecutionResult{
			SubtaskID: st.ID,
			Status:    StatusSkipped,
			Metadata:  map[string]interface{}{"dry_run": true},
		}
	}
	ended := time.Now()
	return PlanExecutionResult{
		PlanID:    plan.ID,
		Status:    StatusSkipped,
		Results:   results,
		StartedAt: started,
		EndedAt:   ended,
		Duration:  ended.Sub(started),
		Metadata:  map[string]interface{}{"dry_run": true, "batches": len(plan.Batches)},
	}
}

func forceSerialExecution(plan ExecutionPlan) ExecutionPlan {
	plan.Batches = append([]ExecutionBatch(nil), plan.Batches...)
	for i := range plan.Batches {
		plan.Batches[i].Mode = BatchModeSerial
		plan.Batches[i].MaxConcurrency = 1
		if plan.Batches[i].Metadata == nil {
			plan.Batches[i].Metadata = map[string]interface{}{}
		}
		plan.Batches[i].Metadata["forced_serial"] = true
	}
	return plan
}
