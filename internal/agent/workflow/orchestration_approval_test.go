package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gatedPlan() ExecutionPlan {
	read := NewSubtask("read", "Read", "baca konteks")
	gated := NewSubtask("danger", "Danger", "hapus data lama")
	gated.Status = StatusWaitingApproval
	gated.RiskLevel = RiskCritical
	gated.CanParallel = false
	gated.DependsOn = []string{"read"}
	after := NewSubtask("after", "After", "langkah setelah gated")
	after.DependsOn = []string{"danger"}
	plan := ExecutionPlan{ID: "p1", Subtasks: []Subtask{read, gated, after}}
	scheduled, err := NewScheduler(SchedulerConfig{MaxConcurrency: 4}).Schedule(plan)
	if err != nil {
		panic(err)
	}
	return scheduled
}

func okHandler() SubtaskExecutorFunc {
	return func(ctx context.Context, st Subtask) ExecutionResult {
		return ExecutionResult{SubtaskID: st.ID, Status: StatusSuccess, Stdout: "done"}
	}
}

func TestExecutor_ApprovalGranted(t *testing.T) {
	plan := gatedPlan()
	exec := NewExecutor(ExecutorConfig{
		MaxConcurrency: 4,
		ApprovalFunc:   func(ctx context.Context, st Subtask) bool { return true },
	}, okHandler())

	out, err := exec.Execute(context.Background(), plan)
	require.NoError(t, err)
	assert.Equal(t, StatusSuccess, out.Results["danger"].Status)
	assert.Equal(t, StatusSuccess, out.Results["after"].Status)
}

func TestExecutor_ApprovalDeniedSkipsDependents(t *testing.T) {
	plan := gatedPlan()
	exec := NewExecutor(ExecutorConfig{
		MaxConcurrency: 4,
		ApprovalFunc:   func(ctx context.Context, st Subtask) bool { return false },
	}, okHandler())

	out, err := exec.Execute(context.Background(), plan)
	require.NoError(t, err)
	assert.Equal(t, StatusSuccess, out.Results["read"].Status)
	assert.Equal(t, StatusSkipped, out.Results["danger"].Status)
	// Dependent of the denied gated subtask must be skipped too.
	assert.Equal(t, StatusSkipped, out.Results["after"].Status)
}

func TestExecutor_NilApprovalFuncAutoDenies(t *testing.T) {
	plan := gatedPlan()
	exec := NewExecutor(ExecutorConfig{MaxConcurrency: 4}, okHandler()) // no ApprovalFunc

	out, err := exec.Execute(context.Background(), plan)
	require.NoError(t, err)
	assert.Equal(t, StatusSkipped, out.Results["danger"].Status)
	assert.Equal(t, StatusSkipped, out.Results["after"].Status)
}

func TestExecutor_NonGatedUnaffected(t *testing.T) {
	// A plan with no gated subtasks must never call ApprovalFunc.
	read := NewSubtask("a", "A", "x")
	plan, err := NewScheduler(SchedulerConfig{MaxConcurrency: 4}).Schedule(ExecutionPlan{ID: "p", Subtasks: []Subtask{read}})
	require.NoError(t, err)

	called := false
	exec := NewExecutor(ExecutorConfig{
		MaxConcurrency: 4,
		ApprovalFunc:   func(ctx context.Context, st Subtask) bool { called = true; return true },
	}, okHandler())
	out, err := exec.Execute(context.Background(), plan)
	require.NoError(t, err)
	assert.Equal(t, StatusSuccess, out.Results["a"].Status)
	assert.False(t, called, "ApprovalFunc must not run for non-gated subtasks")
}

func TestExecutor_ApprovalContextCancelDenies(t *testing.T) {
	plan := gatedPlan()
	ctx, cancel := context.WithCancel(context.Background())
	exec := NewExecutor(ExecutorConfig{
		MaxConcurrency: 4,
		ApprovalFunc: func(c context.Context, st Subtask) bool {
			<-c.Done() // simulate blocking until cancellation
			return false
		},
	}, okHandler())

	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	out, err := exec.Execute(ctx, plan)
	require.NoError(t, err)
	assert.Equal(t, StatusSkipped, out.Results["danger"].Status)
}
