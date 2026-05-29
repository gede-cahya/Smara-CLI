package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlannerPlanReadOnlyTask(t *testing.T) {
	planner := NewPlanner()

	plan, err := planner.Plan(OrchestrationTask{
		ID:          "audit-repo",
		Title:       "Audit repo",
		Description: "Audit project structure and summarize issues",
	})

	require.NoError(t, err)
	assert.Equal(t, "plan-audit-repo", plan.ID)
	assert.Equal(t, TaskKindReadOnly, plan.Task.Kind)
	assert.Equal(t, RiskLow, plan.Task.RiskLevel)
	require.Len(t, plan.Subtasks, 5)
	assert.Len(t, plan.Dependencies, 4)
	assert.Equal(t, "produce-report", plan.Subtasks[len(plan.Subtasks)-1].ID)
	assert.NoError(t, ValidateExecutionPlan(plan))
}

func TestPlannerPlanMutatingTaskIncludesApprovalAndVerify(t *testing.T) {
	planner := NewPlanner()

	plan, err := planner.Plan(OrchestrationTask{
		ID:          "fix-bug",
		Title:       "Fix bug",
		Description: "Perbaiki bug parser dan jalankan test",
	})

	require.NoError(t, err)
	assert.Equal(t, TaskKindMutating, plan.Task.Kind)
	assert.Equal(t, RiskHigh, plan.Task.RiskLevel)
	ids := subtaskIDs(plan.Subtasks)
	assert.Contains(t, ids, "approval-gate")
	assert.Contains(t, ids, "apply-change")
	assert.Contains(t, ids, "verify-change")

	approval := findSubtask(t, plan, "approval-gate")
	assert.Equal(t, StatusWaitingApproval, approval.Status)
	assert.False(t, approval.CanParallel)
	assert.Equal(t, []string{"summarize-findings"}, approval.DependsOn)
}

func TestPlannerPlanDeployTaskIncludesCriticalRemoteStep(t *testing.T) {
	planner := NewPlanner()

	plan, err := planner.Plan(OrchestrationTask{
		ID:          "deploy-app",
		Title:       "Deploy app",
		Description: "Deploy aplikasi ke VPS production",
	})

	require.NoError(t, err)
	assert.Equal(t, TaskKindProductionImpacting, plan.Task.Kind)
	assert.Equal(t, RiskCritical, plan.Task.RiskLevel)
	deploy := findSubtask(t, plan, "deploy-or-remote-step")
	assert.Equal(t, TaskKindProductionImpacting, deploy.Kind)
	assert.Equal(t, RiskCritical, deploy.RiskLevel)
	assert.False(t, deploy.CanParallel)
	assert.NotEmpty(t, deploy.DependsOn)
}

func TestPlannerRejectsEmptyDescription(t *testing.T) {
	planner := NewPlanner()
	_, err := planner.Plan(OrchestrationTask{ID: "bad"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task description cannot be empty")
}

func TestValidateExecutionPlanRejectsInvalidDependency(t *testing.T) {
	plan := ExecutionPlan{Subtasks: []Subtask{
		{ID: "a", DependsOn: []string{"missing"}},
	}}

	err := ValidateExecutionPlan(plan)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown subtask")
}

func TestValidateExecutionPlanRejectsCircularDependency(t *testing.T) {
	plan := ExecutionPlan{Subtasks: []Subtask{
		{ID: "a", DependsOn: []string{"b"}},
		{ID: "b", DependsOn: []string{"a"}},
	}}

	err := ValidateExecutionPlan(plan)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func subtaskIDs(subtasks []Subtask) []string {
	ids := make([]string, 0, len(subtasks))
	for _, st := range subtasks {
		ids = append(ids, st.ID)
	}
	return ids
}

func findSubtask(t *testing.T, plan ExecutionPlan, id string) Subtask {
	t.Helper()
	for _, st := range plan.Subtasks {
		if st.ID == id {
			return st
		}
	}
	t.Fatalf("subtask %q not found", id)
	return Subtask{}
}
