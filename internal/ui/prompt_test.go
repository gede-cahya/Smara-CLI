package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewPrompt_DefaultModeAsk(t *testing.T) {
	p := NewPrompt()
	assert.Equal(t, "ask", p.currentMode)
	assert.Empty(t, p.history)
	assert.Nil(t, p.onModeChange)
}

func TestPrompt_SetMode(t *testing.T) {
	p := NewPrompt()
	p.SetMode("rush")
	assert.Equal(t, "rush", p.currentMode)
	p.SetMode("plan")
	assert.Equal(t, "plan", p.currentMode)
	p.SetMode("test")
	assert.Equal(t, "test", p.currentMode)
}

func TestPrompt_NextMode_CyclesCorrectly(t *testing.T) {
	p := NewPrompt()
	assert.Equal(t, "rush", p.nextMode())

	p.currentMode = "rush"
	assert.Equal(t, "plan", p.nextMode())

	p.currentMode = "plan"
	assert.Equal(t, "test", p.nextMode())

	p.currentMode = "test"
	assert.Equal(t, "workflow", p.nextMode())

	p.currentMode = "workflow"
	assert.Equal(t, "ask", p.nextMode()) // wrap around
}

func TestPrompt_NextMode_UnknownModeReturnsFirst(t *testing.T) {
	p := NewPrompt()
	p.currentMode = "unknown"
	assert.Equal(t, "ask", p.nextMode())
}

func TestPrompt_OnModeChange(t *testing.T) {
	p := NewPrompt()
	var calledMode string
	p.OnModeChange(func(newMode string) {
		calledMode = newMode
	})

	p.currentMode = "ask"
	newMode := p.nextMode()
	p.currentMode = newMode
	if p.onModeChange != nil {
		p.onModeChange(newMode)
	}
	assert.Equal(t, "rush", calledMode)
}

func TestIsExitCommand(t *testing.T) {
	assert.True(t, IsExitCommand("exit"))
	assert.True(t, IsExitCommand("EXIT"))
	assert.True(t, IsExitCommand("quit"))
	assert.True(t, IsExitCommand("QUIT"))
	assert.True(t, IsExitCommand(":q"))
	assert.True(t, IsExitCommand("keluar"))
	assert.True(t, IsExitCommand("KELUAR"))
	assert.False(t, IsExitCommand("hello"))
	assert.False(t, IsExitCommand(""))
	assert.False(t, IsExitCommand("exit now"))
}

func TestIsCommand(t *testing.T) {
	assert.True(t, IsCommand("/help"))
	assert.True(t, IsCommand("/mode ask"))
	assert.True(t, IsCommand("/memory"))
	assert.False(t, IsCommand("hello"))
	assert.False(t, IsCommand(""))
	assert.False(t, IsCommand("not/command"))
}

func TestParseCommand(t *testing.T) {
	cmd, args := ParseCommand("/help")
	assert.Equal(t, "help", cmd)
	assert.Empty(t, args)

	cmd, args = ParseCommand("/mode ask")
	assert.Equal(t, "mode", cmd)
	assert.Equal(t, []string{"ask"}, args)

	cmd, args = ParseCommand("/memory list --all")
	assert.Equal(t, "memory", cmd)
	assert.Equal(t, []string{"list", "--all"}, args)

	cmd, args = ParseCommand("/model openai gpt-4")
	assert.Equal(t, "model", cmd)
	assert.Equal(t, []string{"openai", "gpt-4"}, args)

	cmd, args = ParseCommand("")
	assert.Equal(t, "", cmd)
	assert.Nil(t, args)
}

func TestModeColors(t *testing.T) {
	assert.Equal(t, Cyan, ModeColors["ask"])
	assert.Equal(t, Yellow, ModeColors["rush"])
	assert.Equal(t, Magenta, ModeColors["plan"])
	assert.Equal(t, Green, ModeColors["test"])
	assert.Empty(t, ModeColors["unknown"])
}

func TestModeEmojis(t *testing.T) {
	assert.Equal(t, "💬", ModeEmojis["ask"])
	assert.Equal(t, "⚡", ModeEmojis["rush"])
	assert.Equal(t, "📋", ModeEmojis["plan"])
	assert.Equal(t, "🧪", ModeEmojis["test"])
	assert.Empty(t, ModeEmojis["unknown"])
}

func TestModeOrder(t *testing.T) {
	assert.Equal(t, []string{"ask", "rush", "plan", "test", "workflow"}, ModeOrder)
}

func TestErrInterrupted(t *testing.T) {
	assert.EqualError(t, ErrInterrupted, "interrupted")
}
