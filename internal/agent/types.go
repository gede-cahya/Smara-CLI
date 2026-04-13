// Package agent implements the multi-agent system for Smara.
package agent

import "time"

// TaskStatus represents the current state of a task.
type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
)

// Task represents a unit of work to be executed by a worker agent.
type Task struct {
	ID          string                 `json:"id"`
	Description string                 `json:"description"`
	Status      TaskStatus             `json:"status"`
	AssignedTo  string                 `json:"assigned_to,omitempty"`
	ParentID    string                 `json:"parent_id,omitempty"`
	MCPServer   string                 `json:"mcp_server,omitempty"` // which MCP server to use
	ToolName    string                 `json:"tool_name,omitempty"`
	ToolArgs    map[string]interface{} `json:"tool_args,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

// TaskResult represents the output of a completed task.
type TaskResult struct {
	TaskID  string     `json:"task_id"`
	Status  TaskStatus `json:"status"`
	Output  string     `json:"output"`
	Error   string     `json:"error,omitempty"`
	Files   []string   `json:"files,omitempty"` // output file paths
}

// AgentConfig configures an agent instance.
type AgentConfig struct {
	Name       string `json:"name"`
	SystemPrompt string `json:"system_prompt"`
	MaxRetries int    `json:"max_retries"`
	TimeoutSec int    `json:"timeout_sec"`
}
