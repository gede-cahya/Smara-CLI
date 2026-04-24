// Package session provides session types used across agent and memory packages.
package session

import (
	"time"

	"github.com/gede-cahya/Smara-CLI/pkg/llm"
)

// State represents the current state of a session.
type State string

const (
	StateActive State = "active"
	StatePaused State = "paused"
	StateEnded  State = "ended"
)

// Session represents an agentic conversation session.
type Session struct {
	ID          string        `json:"id"`
	WorkspaceID int64         `json:"workspace_id"`
	Name        string        `json:"name"`
	State       State         `json:"state"`
	Mode        string        `json:"mode"`
	MCPServers  []string      `json:"mcp_servers"`
	History    []llm.Message `json:"history"`
	Tasks      []Task        `json:"tasks"`
	MemoryIDs  []int64       `json:"memory_ids"`  // References to persistent memories
	Context    string        `json:"context"`     // Session context/summary
	IsAgentic  bool          `json:"is_agentic"`  // Whether session uses agentic AI
	AutoResume bool          `json:"auto_resume"` // Auto-continue from last state
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
}

// Config holds configuration for creating a new session.
type Config struct {
	Name        string
	WorkspaceID int64
	Mode        string
	MCPServers  []string
	IsAgentic   bool // Enable agentic AI mode
	AutoResume  bool // Auto-resume from last state
}

// Store defines the interface for session persistence.
type Store interface {
	CreateSession(session *Session) error
	GetSession(id string) (*Session, error)
	UpdateSession(session *Session) error
	DeleteSession(id string) error
	ListSessions() ([]Session, error)
	ListSessionsByWorkspace(workspaceID int64) ([]Session, error)
	ListActiveSessions() ([]Session, error)
	GetLastActiveSession() (*Session, error)
	GetLastActiveSessionByWorkspace(workspaceID int64) (*Session, error)
}

// TaskStatus represents the current state of a task.
type Status string

const (
	TaskPending   Status = "pending"
	TaskRunning   Status = "running"
	TaskCompleted Status = "completed"
	TaskFailed    Status = "failed"
)

// Task represents a unit of work to be executed by a worker agent.
type Task struct {
	ID          string                 `json:"id"`
	Description string                 `json:"description"`
	Status      Status                 `json:"status"`
	AssignedTo  string                 `json:"assigned_to,omitempty"`
	ParentID    string                 `json:"parent_id,omitempty"`
	MCPServer   string                 `json:"mcp_server,omitempty"`
	ToolName    string                 `json:"tool_name,omitempty"`
	ToolArgs    map[string]interface{} `json:"tool_args,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

// Result represents the output of a completed task.
type Result struct {
	TaskID string   `json:"task_id"`
	Status Status   `json:"status"`
	Output string   `json:"output"`
	Error  string   `json:"error,omitempty"`
	Files  []string `json:"files,omitempty"`
}
