package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafetyGuardrail_GatesHighRiskSubtask(t *testing.T) {
	plan := ExecutionPlan{
		ID: "plan-1",
		Subtasks: []Subtask{
			{ID: "edit", Title: "Edit file", Kind: TaskKindMutating, RiskLevel: RiskHigh, CanParallel: true, Status: StatusPending},
		},
	}

	guard := NewSafetyGuardrail(SafetyConfig{})
	updated, report, err := guard.ApplySafetyPolicy(plan)
	require.NoError(t, err)

	require.Len(t, report.RequiresApproval, 1)
	assert.Equal(t, "edit", report.RequiresApproval[0])
	assert.Equal(t, StatusWaitingApproval, updated.Subtasks[0].Status)
	assert.False(t, updated.Subtasks[0].CanParallel)
	assert.NotEmpty(t, updated.Subtasks[0].Metadata["rollback_hint"])
}

func TestSafetyGuardrail_ApprovedHighRiskDoesNotGate(t *testing.T) {
	plan := ExecutionPlan{ID: "plan-1", Subtasks: []Subtask{{ID: "edit", Title: "Edit file", Kind: TaskKindMutating, RiskLevel: RiskHigh, CanParallel: true, Status: StatusPending}}}

	guard := NewSafetyGuardrail(SafetyConfig{ApprovedSubtaskIDs: []string{"edit"}})
	updated, report, err := guard.ApplySafetyPolicy(plan)
	require.NoError(t, err)

	assert.Empty(t, report.RequiresApproval)
	assert.Equal(t, StatusPending, updated.Subtasks[0].Status)
}

func TestSafetyGuardrail_PromotesDestructiveToCritical(t *testing.T) {
	plan := ExecutionPlan{ID: "plan-1", Subtasks: []Subtask{{ID: "drop", Title: "Drop database table", Description: "drop users table", Kind: TaskKindMutating, RiskLevel: RiskMedium, CanParallel: true}}}

	guard := NewSafetyGuardrail(SafetyConfig{})
	updated, report, err := guard.ApplySafetyPolicy(plan)
	require.NoError(t, err)

	st := updated.Subtasks[0]
	assert.Equal(t, TaskKindDestructive, st.Kind)
	assert.Equal(t, RiskCritical, st.RiskLevel)
	assert.Equal(t, StatusWaitingApproval, st.Status)
	assert.Contains(t, report.RequiresApproval, "drop")
}

func TestSafetyGuardrail_DetectsParallelResourceConflictAndSerialFallback(t *testing.T) {
	plan := ExecutionPlan{
		ID: "plan-1",
		Subtasks: []Subtask{
			{ID: "a", Title: "Edit A", Kind: TaskKindMutating, RiskLevel: RiskMedium, CanParallel: true, Metadata: map[string]interface{}{"file": "main.go"}},
			{ID: "b", Title: "Edit B", Kind: TaskKindMutating, RiskLevel: RiskMedium, CanParallel: true, Metadata: map[string]interface{}{"file": "main.go"}},
		},
		Batches: []ExecutionBatch{{ID: "batch-01", Mode: BatchModeParallel, MaxConcurrency: 2, SubtaskIDs: []string{"a", "b"}}},
	}

	guard := NewSafetyGuardrail(SafetyConfig{})
	updated, report, err := guard.ApplySafetyPolicy(plan)
	require.NoError(t, err)

	assert.NotEmpty(t, report.Conflicts)
	assert.Equal(t, BatchModeSerial, updated.Batches[0].Mode)
	assert.Equal(t, 1, updated.Batches[0].MaxConcurrency)
}

func TestSafetyGuardrail_RemoteCommandBecomesHighRisk(t *testing.T) {
	plan := ExecutionPlan{ID: "plan-1", Subtasks: []Subtask{{ID: "remote", Title: "Restart service on VPS", Description: "ssh server systemctl restart app", Kind: TaskKindMutating, RiskLevel: RiskMedium, CanParallel: true}}}

	guard := NewSafetyGuardrail(SafetyConfig{})
	updated, report, err := guard.ApplySafetyPolicy(plan)
	require.NoError(t, err)

	st := updated.Subtasks[0]
	assert.Equal(t, TaskKindRemote, st.Kind)
	assert.Equal(t, RiskHigh, st.RiskLevel)
	assert.Equal(t, StatusWaitingApproval, st.Status)
	assert.Contains(t, report.RequiresApproval, "remote")
}
