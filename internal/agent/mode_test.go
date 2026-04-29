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
}

func TestAllModes_ReturnsAll(t *testing.T) {
	modes := AllModes()
	assert.Len(t, modes, 4)

	names := make([]Mode, 0, 4)
	for _, m := range modes {
		names = append(names, m.Name)
	}
	assert.Contains(t, names, ModeAsk)
	assert.Contains(t, names, ModeRush)
	assert.Contains(t, names, ModePlan)
	assert.Contains(t, names, ModeTest)
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
	for _, mode := range []Mode{ModeAsk, ModeRush, ModePlan, ModeTest} {
		info := GetModeInfo(mode)
		assert.Equal(t, mode, info.Name, "GetModeInfo should return correct mode for %s", mode)
	}
}

func TestGetModeInfo_UnknownModeDefaultsToAsk(t *testing.T) {
	info := GetModeInfo(Mode("unknown"))
	assert.Equal(t, ModeAsk, info.Name, "unknown mode should default to ask")
	assert.Equal(t, "Ask", info.Label)
}

func TestValidMode(t *testing.T) {
	assert.True(t, ValidMode("ask"))
	assert.True(t, ValidMode("rush"))
	assert.True(t, ValidMode("plan"))
	assert.True(t, ValidMode("test"))
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
