package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModeWorkflow_Constant(t *testing.T) {
	assert.Equal(t, Mode("workflow"), ModeWorkflow)
}

func TestAllModes_IncludesWorkflow(t *testing.T) {
	modes := AllModes()
	names := make([]Mode, 0, len(modes))
	for _, m := range modes {
		names = append(names, m.Name)
	}
	assert.Contains(t, names, ModeWorkflow)
	assert.Len(t, names, 5)
}

func TestGetModeInfo_Workflow(t *testing.T) {
	info := GetModeInfo(ModeWorkflow)
	assert.Equal(t, ModeWorkflow, info.Name)
	assert.Equal(t, "Workflow", info.Label)
	assert.Equal(t, "🔄", info.Emoji)
	assert.NotEmpty(t, info.Description)
	assert.NotEmpty(t, info.SystemPrompt)
	assert.Contains(t, info.SystemPrompt, "WORKFLOW")
}

func TestModePlan_SystemPromptStructure(t *testing.T) {
	info := GetModeInfo(ModePlan)
	assert.Contains(t, info.SystemPrompt, "Recommended approach")
	assert.Contains(t, info.SystemPrompt, "Verification")
	assert.Contains(t, info.SystemPrompt, "Risks / rollback")
	assert.Contains(t, info.SystemPrompt, "planning-agile-minsky")
	assert.Contains(t, info.SystemPrompt, "Lanjutkan eksekusi? (ya/tidak)")
}

func TestValidMode_Workflow(t *testing.T) {
	assert.True(t, ValidMode("workflow"))
	assert.False(t, ValidMode("WORKFLOW")) // case sensitive
	assert.False(t, ValidMode("unknown"))
}
