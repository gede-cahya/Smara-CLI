package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildExecutionWavesDeterministic(t *testing.T) {
	plan := ExecutionPlan{
		ID: "plan-test",
		Subtasks: []Subtask{
			NewSubtask("verify", "Verify", "Run verification"),
			NewSubtask("scan-b", "Scan B", "Second scan"),
			NewSubtask("scan-a", "Scan A", "First scan"),
			NewSubtask("summarize", "Summarize", "Summarize findings"),
		},
	}
	plan.Subtasks[0].DependsOn = []string{"summarize"}
	plan.Subtasks[3].DependsOn = []string{"scan-a", "scan-b"}

	waves, err := BuildExecutionWaves(plan)
	require.NoError(t, err)
	require.Len(t, waves, 3)
	assert.Equal(t, []string{"scan-a", "scan-b"}, waves[0])
	assert.Equal(t, []string{"summarize"}, waves[1])
	assert.Equal(t, []string{"verify"}, waves[2])
}

func TestSchedulerScheduleParallelSerialAndGatedBatches(t *testing.T) {
	plan := ExecutionPlan{
		ID: "plan-mutate",
		Subtasks: []Subtask{
			NewSubtask("inspect", "Inspect", "Inspect workspace"),
			NewSubtask("search", "Search", "Search code"),
			NewSubtask("summarize", "Summarize", "Summarize findings"),
			NewSubtask("approval", "Approval", "Request approval"),
			NewSubtask("apply", "Apply", "Apply change"),
		},
	}
	plan.Subtasks[2].DependsOn = []string{"inspect", "search"}
	plan.Subtasks[2].CanParallel = false
	plan.Subtasks[3].DependsOn = []string{"summarize"}
	plan.Subtasks[3].CanParallel = false
	plan.Subtasks[3].RiskLevel = RiskHigh
	plan.Subtasks[3].Status = StatusWaitingApproval
	plan.Subtasks[4].DependsOn = []string{"approval"}
	plan.Subtasks[4].CanParallel = false
	plan.Subtasks[4].RiskLevel = RiskHigh

	scheduler := NewScheduler(SchedulerConfig{MaxConcurrency: 2})
	scheduled, err := scheduler.Schedule(plan)
	require.NoError(t, err)
	require.Len(t, scheduled.Batches, 4)

	assert.Equal(t, BatchModeParallel, scheduled.Batches[0].Mode)
	assert.Equal(t, []string{"inspect", "search"}, scheduled.Batches[0].SubtaskIDs)
	assert.Equal(t, 2, scheduled.Batches[0].MaxConcurrency)

	assert.Equal(t, BatchModeSerial, scheduled.Batches[1].Mode)
	assert.Equal(t, []string{"summarize"}, scheduled.Batches[1].SubtaskIDs)
	assert.Equal(t, 1, scheduled.Batches[1].MaxConcurrency)

	assert.Equal(t, BatchModeGated, scheduled.Batches[2].Mode)
	assert.True(t, scheduled.Batches[2].RequiresApproval)
	assert.Equal(t, []string{"approval"}, scheduled.Batches[2].SubtaskIDs)

	assert.Equal(t, BatchModeGated, scheduled.Batches[3].Mode)
	assert.True(t, scheduled.Batches[3].RequiresApproval)
	assert.Equal(t, []string{"apply"}, scheduled.Batches[3].SubtaskIDs)
}

func TestSchedulerRejectsInvalidPlan(t *testing.T) {
	plan := ExecutionPlan{
		ID: "invalid",
		Subtasks: []Subtask{
			NewSubtask("a", "A", "A"),
			NewSubtask("b", "B", "B"),
		},
	}
	plan.Subtasks[0].DependsOn = []string{"b"}
	plan.Subtasks[1].DependsOn = []string{"a"}

	_, err := NewScheduler(SchedulerConfig{}).Schedule(plan)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestSchedulerWorksWithPlannerOutput(t *testing.T) {
	planner := NewPlanner()
	plan, err := planner.Plan(OrchestrationTask{
		ID:          "audit",
		Title:       "Audit repo",
		Description: "Audit repository structure and summarize findings",
	})
	require.NoError(t, err)

	scheduled, err := NewScheduler(SchedulerConfig{MaxConcurrency: 3}).Schedule(plan)
	require.NoError(t, err)
	require.NotEmpty(t, scheduled.Batches)
	assert.Equal(t, BatchModeParallel, scheduled.Batches[0].Mode)
	assert.LessOrEqual(t, scheduled.Batches[0].MaxConcurrency, 3)
}
