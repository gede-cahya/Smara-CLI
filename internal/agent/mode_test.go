package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMode_Constants(t *testing.T) {
	assert.Equal(t, Mode("ask"), ModeAsk)
	assert.Equal(t, Mode("rush"), ModeRush)
	assert.Equal(t, Mode("plan"), ModePlan)
	assert.Equal(t, Mode("test"), ModeTest)
	assert.Equal(t, Mode("image"), ModeImage)
	assert.Equal(t, Mode("workflow"), ModeWorkflow)
	assert.Equal(t, Mode("parallel"), ModeParallel)
}

func TestAllModes_ReturnsAll(t *testing.T) {
	modes := AllModes()
	assert.Len(t, modes, 7)

	names := make([]Mode, 0, 7)
	for _, m := range modes {
		names = append(names, m.Name)
	}
	assert.Contains(t, names, ModeAsk)
	assert.Contains(t, names, ModeRush)
	assert.Contains(t, names, ModePlan)
	assert.Contains(t, names, ModeTest)
	assert.Contains(t, names, ModeImage)
	assert.Contains(t, names, ModeWorkflow)
	assert.Contains(t, names, ModeParallel)
}

func TestAllModes_ModeInfoFields(t *testing.T) {
	modes := AllModes()
	for _, m := range modes {
		assert.NotEmpty(t, m.Name, "mode should have a name")
		assert.NotEmpty(t, m.Label, "mode %s should have a label", m.Name)
		assert.NotEmpty(t, m.Emoji, "mode %s should have an emoji", m.Name)
		assert.NotEmpty(t, m.Description, "mode %s should have a description", m.Name)
		assert.NotEmpty(t, m.SystemPrompt, "mode %s should have a system prompt", m.Name)
	}
}

func TestGetModeInfo_ExistingModes(t *testing.T) {
	for _, mode := range []Mode{ModeAsk, ModeRush, ModePlan, ModeTest, ModeImage, ModeWorkflow, ModeParallel} {
		info := GetModeInfo(mode)
		assert.Equal(t, mode, info.Name, "GetModeInfo should return correct mode for %s", mode)
	}
}

func TestGetModeInfo_UnknownModeDefaultsToAsk(t *testing.T) {
	info := GetModeInfo(Mode("unknown"))
	assert.Equal(t, ModeAsk, info.Name, "unknown mode should default to ask")
	assert.Equal(t, "Ask", info.Label)
}

func TestEveryModeHasExpectedRoutingPrompt(t *testing.T) {
	tests := []struct {
		mode     Mode
		label    string
		contains []string
	}{
		{ModeAsk, "Ask", []string{"MENJAWAB PERTANYAAN", "tools"}},
		{ModeRush, "Rush", []string{"LANGSUNG EKSEKUSI", "maksimal aksi"}},
		{ModePlan, "Plan", []string{"Recommended approach", "SMARA_PLAN_QUEST"}},
		{ModeTest, "Test", []string{"TESTING", "Jangan menyatakan tugas selesai"}},
		{ModeImage, "Image", []string{"generate_image", "edit_image"}},
		{ModeWorkflow, "Workflow", []string{"WORKFLOW biasa", "Jangan memulai generic parallel task orchestration otomatis"}},
		{ModeParallel, "Parallel", []string{"PARALLEL TASK", "Agent Swarm Workflow"}},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			info := GetModeInfo(tt.mode)
			assert.Equal(t, tt.label, info.Label)
			assert.True(t, ValidMode(string(tt.mode)))
			for _, want := range tt.contains {
				assert.Contains(t, info.SystemPrompt, want)
			}
		})
	}
}

func TestParallelPolicyPromptsOnlyAllowGenericParallelInParallelMode(t *testing.T) {
	workflowPrompt := GetModeInfo(ModeWorkflow).SystemPrompt
	parallelPrompt := GetModeInfo(ModeParallel).SystemPrompt

	assert.Contains(t, workflowPrompt, "Jangan memulai generic parallel task orchestration otomatis")
	assert.Contains(t, workflowPrompt, "mengganti mode ke Parallel")
	assert.Contains(t, parallelPrompt, "Mode ini khusus untuk menjalankan generic parallel task orchestration")
}

func TestModePlan_SystemPromptStructure(t *testing.T) {
	info := GetModeInfo(ModePlan)
	assert.Contains(t, info.SystemPrompt, "Recommended approach")
	assert.Contains(t, info.SystemPrompt, "Verification")
	assert.Contains(t, info.SystemPrompt, "Risks / rollback")
	assert.Contains(t, info.SystemPrompt, "planning-agile-minsky")
	assert.Contains(t, info.SystemPrompt, "Lanjutkan eksekusi? (ya/tidak)")
	assert.Contains(t, info.SystemPrompt, "SMARA_PLAN_QUEST")
	assert.Contains(t, info.SystemPrompt, "allow_custom")
}

func TestModeImage_SystemPromptStructure(t *testing.T) {
	info := GetModeInfo(ModeImage)
	assert.Equal(t, "Image", info.Label)
	assert.Contains(t, info.SystemPrompt, "generate_image")
	assert.Contains(t, info.SystemPrompt, "analyze_image")
	assert.Contains(t, info.SystemPrompt, "edit_image")
}

func TestValidMode(t *testing.T) {
	assert.True(t, ValidMode("ask"))
	assert.True(t, ValidMode("rush"))
	assert.True(t, ValidMode("plan"))
	assert.True(t, ValidMode("test"))
	assert.True(t, ValidMode("image"))
	assert.True(t, ValidMode("workflow"))
	assert.True(t, ValidMode("parallel"))
	assert.False(t, ValidMode("unknown"))
	assert.False(t, ValidMode(""))
	assert.False(t, ValidMode("ASK")) // case sensitive
}

func TestModeInfo_Struct(t *testing.T) {
	info := ModeInfo{
		Name:         ModeAsk,
		Label:        "Ask",
		Emoji:        "💬",
		Description:  "Test description",
		SystemPrompt: "Test prompt",
	}
	assert.Equal(t, ModeAsk, info.Name)
	assert.Equal(t, "Ask", info.Label)
	assert.Equal(t, "💬", info.Emoji)
	assert.Equal(t, "Test description", info.Description)
	assert.Equal(t, "Test prompt", info.SystemPrompt)
}
