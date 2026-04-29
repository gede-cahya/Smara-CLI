package session

import (
	"testing"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/stretchr/testify/assert"
)

func TestState_Constants(t *testing.T) {
	assert.Equal(t, State("active"), StateActive)
	assert.Equal(t, State("paused"), StatePaused)
	assert.Equal(t, State("ended"), StateEnded)
}

func TestSession_Struct(t *testing.T) {
	now := time.Now()
	s := Session{
		ID:          "sess-123",
		WorkspaceID: 42,
		Name:        "test-session",
		State:       StateActive,
		Mode:        "ask",
		MCPServers:  []string{"server1"},
		History:     []llm.Message{{Role: llm.RoleUser, Content: "hello"}},
		Tasks:       []Task{{ID: "task-1", Description: "test task", Status: TaskPending}},
		MemoryIDs:   []int64{1, 2, 3},
		Context:     "project context",
		IsAgentic:   true,
		AutoResume:  false,
		IsArchived:  false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	assert.Equal(t, "sess-123", s.ID)
	assert.Equal(t, int64(42), s.WorkspaceID)
	assert.Equal(t, "test-session", s.Name)
	assert.Equal(t, StateActive, s.State)
	assert.Equal(t, "ask", s.Mode)
	assert.Equal(t, []string{"server1"}, s.MCPServers)
	assert.Len(t, s.History, 1)
	assert.Len(t, s.Tasks, 1)
	assert.Equal(t, []int64{1, 2, 3}, s.MemoryIDs)
	assert.Equal(t, "project context", s.Context)
	assert.True(t, s.IsAgentic)
	assert.False(t, s.AutoResume)
	assert.False(t, s.IsArchived)
	assert.Nil(t, s.ArchivedAt)
}

func TestSession_ArchivedFields(t *testing.T) {
	now := time.Now()
	s := Session{
		IsArchived:  true,
		ArchivedAt:  &now,
	}
	assert.True(t, s.IsArchived)
	assert.NotNil(t, s.ArchivedAt)
	assert.Equal(t, now, *s.ArchivedAt)
}

func TestConfig_Struct(t *testing.T) {
	c := Config{
		Name:        "test",
		WorkspaceID: 1,
		Mode:        "rush",
		MCPServers:  []string{"mcp1"},
		IsAgentic:   true,
		AutoResume:  true,
	}
	assert.Equal(t, "test", c.Name)
	assert.Equal(t, int64(1), c.WorkspaceID)
	assert.Equal(t, "rush", c.Mode)
	assert.Equal(t, []string{"mcp1"}, c.MCPServers)
	assert.True(t, c.IsAgentic)
	assert.True(t, c.AutoResume)
}

func TestStatus_Constants(t *testing.T) {
	assert.Equal(t, Status("pending"), TaskPending)
	assert.Equal(t, Status("running"), TaskRunning)
	assert.Equal(t, Status("completed"), TaskCompleted)
	assert.Equal(t, Status("failed"), TaskFailed)
}

func TestTask_Struct(t *testing.T) {
	now := time.Now()
	task := Task{
		ID:          "task-1",
		Description: "do something",
		Status:      TaskRunning,
		AssignedTo:  "worker-1",
		ParentID:    "parent-1",
		MCPServer:   "builtin",
		ToolName:    "run_command",
		ToolArgs:    map[string]interface{}{"command": "echo hello"},
		CreatedAt:   now,
	}
	assert.Equal(t, "task-1", task.ID)
	assert.Equal(t, "do something", task.Description)
	assert.Equal(t, TaskRunning, task.Status)
	assert.Equal(t, "worker-1", task.AssignedTo)
	assert.Equal(t, "parent-1", task.ParentID)
	assert.Equal(t, "builtin", task.MCPServer)
	assert.Equal(t, "run_command", task.ToolName)
	assert.Equal(t, "echo hello", task.ToolArgs["command"])
	assert.Equal(t, now, task.CreatedAt)
}

func TestResult_Struct(t *testing.T) {
	r := Result{
		TaskID: "task-1",
		Status: TaskCompleted,
		Output: "success",
		Error:  "",
		Files:  []string{"/tmp/test.txt"},
	}
	assert.Equal(t, "task-1", r.TaskID)
	assert.Equal(t, TaskCompleted, r.Status)
	assert.Equal(t, "success", r.Output)
	assert.Empty(t, r.Error)
	assert.Equal(t, []string{"/tmp/test.txt"}, r.Files)
}

func TestResult_Failed(t *testing.T) {
	r := Result{
		TaskID: "task-2",
		Status: TaskFailed,
		Output: "",
		Error:  "something went wrong",
	}
	assert.Equal(t, TaskFailed, r.Status)
	assert.Equal(t, "something went wrong", r.Error)
}
