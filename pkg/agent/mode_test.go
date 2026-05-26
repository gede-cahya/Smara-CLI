package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModeImage_Constant(t *testing.T) {
	assert.Equal(t, Mode("image"), ModeImage)
}

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
	assert.Len(t, names, 6)
}

func TestGetModeInfo_Image(t *testing.T) {
	info := GetModeInfo(ModeImage)
	assert.Equal(t, ModeImage, info.Name)
	assert.Equal(t, "Image", info.Label)
	assert.Equal(t, "🎨", info.Emoji)
	assert.Contains(t, info.SystemPrompt, "generate_image")
	assert.Contains(t, info.SystemPrompt, "edit_image")
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
	assert.True(t, ValidMode("image"))
	assert.False(t, ValidMode("WORKFLOW")) // case sensitive
	assert.False(t, ValidMode("unknown"))
}
