// Package agent implements the multi-agent system for Smara.
package agent

import (
	"github.com/cahya/smara/internal/session"
)

// SessionState is an alias for session.State for backwards compatibility.
type SessionState = session.State

const (
	SessionActive = session.StateActive
	SessionPaused = session.StatePaused
	SessionEnded  = session.StateEnded
)

// Session is an alias for session.Session for backwards compatibility.
type Session = session.Session

// SessionConfig is an alias for session.Config for backwards compatibility.
type SessionConfig = session.Config

// SessionStore is an alias for session.Store for backwards compatibility.
type SessionStore = session.Store

// TaskStatus is an alias for session.Status for backwards compatibility.
type TaskStatus = session.Status

const (
	TaskPending   = session.TaskPending
	TaskRunning   = session.TaskRunning
	TaskCompleted = session.TaskCompleted
	TaskFailed    = session.TaskFailed
)

// Task is an alias for session.Task for backwards compatibility.
type Task = session.Task

// TaskResult is an alias for session.Result for backwards compatibility.
type TaskResult = session.Result

// AgentConfig configures an agent instance.
type AgentConfig struct {
	Name         string `json:"name"`
	SystemPrompt string `json:"system_prompt"`
	MaxRetries   int    `json:"max_retries"`
	TimeoutSec   int    `json:"timeout_sec"`
}