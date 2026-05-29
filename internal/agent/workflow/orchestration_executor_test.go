package workflow

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutorRunsParallelBatch(t *testing.T) {
	plan := mustScheduledPlan(t, []Subtask{
		NewSubtask("a", "A", "first"),
		NewSubtask("b", "B", "second"),
	})

	var running int32
	var maxRunning int32
	exec := NewExecutor(ExecutorConfig{MaxConcurrency: 2, ContinueOnLowRiskFailure: true}, func(ctx context.Context, st Subtask) ExecutionResult {
		current := atomic.AddInt32(&running, 1)
		for {
			old := atomic.LoadInt32(&maxRunning)
			if current <= old || atomic.CompareAndSwapInt32(&maxRunning, old, current) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&running, -1)
		return ExecutionResult{SubtaskID: st.ID, Status: StatusSuccess}
	})

	result, err := exec.Execute(context.Background(), plan)
	require.NoError(t, err)
	assert.Equal(t, StatusSuccess, result.Status)
	assert.Len(t, result.Results, 2)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&maxRunning), int32(2))
}

func TestExecutorSkipsDependentAfterFailure(t *testing.T) {
	a := NewSubtask("a", "A", "fails")
	b := NewSubtask("b", "B", "depends")
	b.DependsOn = []string{"a"}
	plan := mustScheduledPlan(t, []Subtask{a, b})

	exec := NewExecutor(ExecutorConfig{ContinueOnLowRiskFailure: true}, func(ctx context.Context, st Subtask) ExecutionResult {
		if st.ID == "a" {
			return ExecutionResult{SubtaskID: st.ID, Status: StatusFailed, Error: "boom"}
		}
		return ExecutionResult{SubtaskID: st.ID, Status: StatusSuccess}
	})

	result, err := exec.Execute(context.Background(), plan)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, result.Status)
	assert.Equal(t, StatusFailed, result.Results["a"].Status)
	assert.Equal(t, StatusSkipped, result.Results["b"].Status)
}

func TestExecutorRetriesUntilSuccess(t *testing.T) {
	st := NewSubtask("retry", "Retry", "retry test")
	st.RetryPolicy = RetryPolicy{MaxAttempts: 3}
	plan := mustScheduledPlan(t, []Subtask{st})

	var attempts int32
	exec := NewExecutor(ExecutorConfig{}, func(ctx context.Context, st Subtask) ExecutionResult {
		attempt := atomic.AddInt32(&attempts, 1)
		if attempt < 3 {
			return ExecutionResult{SubtaskID: st.ID, Status: StatusFailed, Error: "temporary"}
		}
		return ExecutionResult{SubtaskID: st.ID, Status: StatusSuccess}
	})

	result, err := exec.Execute(context.Background(), plan)
	require.NoError(t, err)
	assert.Equal(t, StatusSuccess, result.Status)
	assert.Equal(t, int32(3), atomic.LoadInt32(&attempts))
	assert.Equal(t, 3, result.Results["retry"].Metadata["attempt"])
}

func TestExecutorTimeout(t *testing.T) {
	st := NewSubtask("slow", "Slow", "timeout test")
	st.Timeout = 10 * time.Millisecond
	plan := mustScheduledPlan(t, []Subtask{st})

	exec := NewExecutor(ExecutorConfig{}, func(ctx context.Context, st Subtask) ExecutionResult {
		<-ctx.Done()
		return ExecutionResult{SubtaskID: st.ID, Status: StatusFailed}
	})

	result, err := exec.Execute(context.Background(), plan)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, result.Status)
	assert.Contains(t, result.Results["slow"].Error, "deadline")
}

func TestExecutorRespectsSerialBatchOrder(t *testing.T) {
	a := NewSubtask("a", "A", "first")
	a.CanParallel = false
	b := NewSubtask("b", "B", "second")
	b.CanParallel = false
	plan := mustScheduledPlan(t, []Subtask{a, b})
	plan.Batches[0].Mode = BatchModeSerial
	plan.Batches[0].MaxConcurrency = 1

	order := []string{}
	exec := NewExecutor(ExecutorConfig{}, func(ctx context.Context, st Subtask) ExecutionResult {
		order = append(order, st.ID)
		return ExecutionResult{SubtaskID: st.ID, Status: StatusSuccess}
	})

	_, err := exec.Execute(context.Background(), plan)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, order)
}

func mustScheduledPlan(t *testing.T, subtasks []Subtask) ExecutionPlan {
	t.Helper()
	plan := ExecutionPlan{
		ID:       "plan-test",
		Task:     OrchestrationTask{ID: "task-test", Title: "Task", Description: "test", Kind: TaskKindReadOnly, RiskLevel: RiskLow},
		Subtasks: subtasks,
	}
	for _, st := range subtasks {
		for _, dep := range st.DependsOn {
			plan.Dependencies = append(plan.Dependencies, Dependency{FromID: dep, ToID: st.ID, Type: DependencyRequires})
		}
	}
	scheduled, err := NewScheduler(SchedulerConfig{MaxConcurrency: 2}).Schedule(plan)
	require.NoError(t, err)
	return scheduled
}
