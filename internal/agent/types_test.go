package agent

import (
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/session"
	"github.com/stretchr/testify/assert"
)

func TestSessionState_Aliases(t *testing.T) {
	assert.Equal(t, session.State("active"), SessionActive)
	assert.Equal(t, session.State("paused"), SessionPaused)
	assert.Equal(t, session.State("ended"), SessionEnded)
}

func TestTaskStatus_Aliases(t *testing.T) {
	assert.Equal(t, session.Status("pending"), TaskPending)
	assert.Equal(t, session.Status("running"), TaskRunning)
	assert.Equal(t, session.Status("completed"), TaskCompleted)
	assert.Equal(t, session.Status("failed"), TaskFailed)
}

func TestAgentConfig_Struct(t *testing.T) {
	cfg := AgentConfig{
		Name:         "test-agent",
		SystemPrompt: "You are a helpful assistant",
		MaxRetries:   3,
		TimeoutSec:   30,
	}
	assert.Equal(t, "test-agent", cfg.Name)
	assert.Equal(t, "You are a helpful assistant", cfg.SystemPrompt)
	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, 30, cfg.TimeoutSec)
}
