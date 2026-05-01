package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/gede-cahya/Smara-CLI/pkg/session"
)

func TestSessionStateAliases(t *testing.T) {
	assert.Equal(t, session.StateActive, SessionActive)
	assert.Equal(t, session.StatePaused, SessionPaused)
	assert.Equal(t, session.StateEnded, SessionEnded)
}

func TestTaskStatusAliases(t *testing.T) {
	assert.Equal(t, session.TaskPending, TaskPending)
	assert.Equal(t, session.TaskRunning, TaskRunning)
	assert.Equal(t, session.TaskCompleted, TaskCompleted)
	assert.Equal(t, session.TaskFailed, TaskFailed)
}

func TestAgentConfig_Default(t *testing.T) {
	cfg := AgentConfig{
		Name:         "test-agent",
		SystemPrompt: "You are a test agent",
		MaxRetries:   3,
		TimeoutSec:   30,
	}
	assert.Equal(t, "test-agent", cfg.Name)
	assert.Equal(t, "You are a test agent", cfg.SystemPrompt)
	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, 30, cfg.TimeoutSec)
}
