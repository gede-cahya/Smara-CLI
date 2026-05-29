package workflow

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSubtaskDefaults(t *testing.T) {
	subtask := NewSubtask("scan", "Scan workspace", "Inspect project structure")

	assert.Equal(t, "scan", subtask.ID)
	assert.Equal(t, "Scan workspace", subtask.Title)
	assert.Equal(t, "Inspect project structure", subtask.Description)
	assert.Equal(t, TaskKindReadOnly, subtask.Kind)
	assert.Empty(t, subtask.DependsOn)
	assert.True(t, subtask.CanParallel)
	assert.Equal(t, RiskLow, subtask.RiskLevel)
	assert.Equal(t, StatusPending, subtask.Status)
}

func TestExecutionPlanJSONRoundTrip(t *testing.T) {
	plan := ExecutionPlan{
		ID: "plan-1",
		Task: OrchestrationTask{
			ID:          "task-1",
			Title:       "Audit repo",
			Description: "Audit repository in parallel",
			Kind:        TaskKindReadOnly,
			RiskLevel:   RiskLow,
		},
		Subtasks: []Subtask{
			NewSubtask("scan", "Scan workspace", "Inspect project structure"),
			{
				ID:          "grep",
				Title:       "Search TODO",
				Description: "Search TODO markers",
				Kind:        TaskKindReadOnly,
				DependsOn:   []string{},
				CanParallel: true,
				RiskLevel:   RiskLow,
				Status:      StatusPending,
				Timeout:     30 * time.Second,
				RetryPolicy: RetryPolicy{MaxAttempts: 2, Backoff: time.Second},
			},
		},
		Dependencies: []Dependency{
			{FromID: "scan", ToID: "grep", Type: DependencyAfter, Reason: "example edge"},
		},
		Batches: []ExecutionBatch{
			{
				ID:             "batch-1",
				Name:           "Read-only discovery",
				Mode:           BatchModeParallel,
				SubtaskIDs:     []string{"scan", "grep"},
				MaxConcurrency: 2,
			},
		},
	}

	data, err := json.Marshal(plan)
	require.NoError(t, err)
	assert.Contains(t, string(data), "risk_level")
	assert.Contains(t, string(data), "depends_on")
	assert.Contains(t, string(data), "max_concurrency")

	var decoded ExecutionPlan
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, plan.ID, decoded.ID)
	assert.Equal(t, plan.Task.Title, decoded.Task.Title)
	require.Len(t, decoded.Subtasks, 2)
	assert.Equal(t, StatusPending, decoded.Subtasks[0].Status)
	assert.Equal(t, 30*time.Second, decoded.Subtasks[1].Timeout)
	require.Len(t, decoded.Dependencies, 1)
	assert.Equal(t, DependencyAfter, decoded.Dependencies[0].Type)
	require.Len(t, decoded.Batches, 1)
	assert.Equal(t, BatchModeParallel, decoded.Batches[0].Mode)
}

func TestExecutionResultJSON(t *testing.T) {
	result := ExecutionResult{
		SubtaskID: "test",
		Status:    StatusFailed,
		Stdout:    "partial output",
		Stderr:    "failure details",
		Error:     "exit status 1",
		Duration:  1500 * time.Millisecond,
		Metadata: map[string]interface{}{
			"command": "go test ./...",
		},
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(data), "stdout")
	assert.Contains(t, string(data), "stderr")
	assert.Contains(t, string(data), "duration")

	var decoded ExecutionResult
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, result.SubtaskID, decoded.SubtaskID)
	assert.Equal(t, result.Status, decoded.Status)
	assert.Equal(t, result.Error, decoded.Error)
	assert.Equal(t, result.Duration, decoded.Duration)
}
