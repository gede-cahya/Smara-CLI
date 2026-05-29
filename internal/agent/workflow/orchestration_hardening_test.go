package workflow

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutorDryRunDoesNotInvokeHandler(t *testing.T) {
	plan := mustScheduledPlan(t, []Subtask{NewSubtask("a", "A", "dry run")})
	var calls int32
	exec := NewExecutor(ExecutorConfig{DryRun: true}, func(ctx context.Context, st Subtask) ExecutionResult {
		atomic.AddInt32(&calls, 1)
		return ExecutionResult{SubtaskID: st.ID, Status: StatusSuccess}
	})

	result, err := exec.Execute(context.Background(), plan)
	require.NoError(t, err)
	assert.Equal(t, int32(0), atomic.LoadInt32(&calls))
	assert.Equal(t, StatusSkipped, result.Status)
	assert.True(t, result.Metadata["dry_run"].(bool))
	assert.Equal(t, StatusSkipped, result.Results["a"].Status)
}

func TestExecutorParallelCanBeDisabledWithSerialFallback(t *testing.T) {
	plan := mustScheduledPlan(t, []Subtask{
		NewSubtask("a", "A", "first"),
		NewSubtask("b", "B", "second"),
	})
	disabled := false
	var running int32
	var maxRunning int32
	exec := NewExecutor(ExecutorConfig{MaxConcurrency: 2, ParallelOrchestration: &disabled}, func(ctx context.Context, st Subtask) ExecutionResult {
		current := atomic.AddInt32(&running, 1)
		if current > atomic.LoadInt32(&maxRunning) {
			atomic.StoreInt32(&maxRunning, current)
		}
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt32(&running, -1)
		return ExecutionResult{SubtaskID: st.ID, Status: StatusSuccess}
	})

	result, err := exec.Execute(context.Background(), plan)
	require.NoError(t, err)
	assert.Equal(t, StatusSuccess, result.Status)
	assert.Equal(t, int32(1), atomic.LoadInt32(&maxRunning))
}

func TestExecutorEmitsObservabilityEvents(t *testing.T) {
	plan := mustScheduledPlan(t, []Subtask{NewSubtask("a", "A", "logging")})
	events := []string{}
	exec := NewExecutor(ExecutorConfig{Logger: func(event string, fields map[string]interface{}) {
		events = append(events, event)
		assert.Equal(t, plan.ID, fields["plan_id"])
	}}, nil)

	_, err := exec.Execute(context.Background(), plan)
	require.NoError(t, err)
	assert.Contains(t, events, "execution_start")
}

func TestExecutorCancellationMarksPending(t *testing.T) {
	a := NewSubtask("a", "A", "cancel")
	b := NewSubtask("b", "B", "pending")
	b.DependsOn = []string{"a"}
	plan := mustScheduledPlan(t, []Subtask{a, b})
	ctx, cancel := context.WithCancel(context.Background())
	exec := NewExecutor(ExecutorConfig{}, func(ctx context.Context, st Subtask) ExecutionResult {
		cancel()
		return ExecutionResult{SubtaskID: st.ID, Status: StatusFailed, Error: "cancelled"}
	})

	result, err := exec.Execute(ctx, plan)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, result.Results["a"].Status)
	assert.Contains(t, []SubtaskStatus{StatusSkipped, StatusCancelled}, result.Results["b"].Status)
}
