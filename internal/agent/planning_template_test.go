package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildPlanningTemplate(t *testing.T) {
	out, err := buildPlanningTemplate("implementation-plan", "Improve plan mode", "Smara CLI")
	require.NoError(t, err)
	assert.Contains(t, out, "# Planning Template: implementation-plan")
	assert.Contains(t, out, "Improve plan mode")
	assert.Contains(t, out, "Recommended approach")
	assert.Contains(t, out, "Verification")
}

func TestBuildPlanningTemplateAgileMinsky(t *testing.T) {
	out, err := buildPlanningTemplate("agile-minsky", "Plan skill pack", "")
	require.NoError(t, err)
	assert.Contains(t, out, "User story")
	assert.Contains(t, out, "Minsky frames")
	assert.Contains(t, out, "Execution backlog")
}

func TestBuildPlanningTemplateValidation(t *testing.T) {
	_, err := buildPlanningTemplate("implementation-plan", "", "")
	assert.Error(t, err)

	_, err = buildPlanningTemplate("unknown", "Goal", "")
	assert.Error(t, err)
}

func TestPlanningTemplateBuiltinTool(t *testing.T) {
	out, err := ExecuteBuiltinTool("planning_template", map[string]interface{}{
		"kind": "test-plan",
		"goal": "Verify planning skills",
	}, nil)
	require.NoError(t, err)
	assert.Contains(t, out, "Golden path")
	assert.Contains(t, out, "Automated tests")
}
