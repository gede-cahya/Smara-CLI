package safety

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModeConstants(t *testing.T) {
	assert.Equal(t, ExecutionMode("plan"), ModePlan)
	assert.Equal(t, ExecutionMode("build"), ModeBuild)
}

func TestActionConstants(t *testing.T) {
	assert.Equal(t, ActionType("read"), ActionRead)
	assert.Equal(t, ActionType("write"), ActionWrite)
	assert.Equal(t, ActionType("execute"), ActionExecute)
	assert.Equal(t, ActionType("delete"), ActionDelete)
}

func TestNewEngine(t *testing.T) {
	engine := NewEngine()
	assert.NotNil(t, engine)
}

func TestIsReadOnlyTool(t *testing.T) {
	assert.True(t, IsReadOnlyTool("read_file"))
	assert.True(t, IsReadOnlyTool("list_directory"))
	assert.False(t, IsReadOnlyTool("write_file"))
	assert.False(t, IsReadOnlyTool("execute_command"))
	assert.False(t, IsReadOnlyTool("delete_file"))
}

func TestIsWriteTool(t *testing.T) {
	assert.True(t, IsWriteTool("write_file"))
	assert.True(t, IsWriteTool("edit_file"))
	assert.False(t, IsWriteTool("read_file"))
	assert.False(t, IsWriteTool("execute_command"))
}

func TestIsExecuteTool(t *testing.T) {
	assert.True(t, IsExecuteTool("execute_command"))
	assert.True(t, IsExecuteTool("run_command"))
	assert.True(t, IsExecuteTool("run_terminal"))
	assert.False(t, IsExecuteTool("read_file"))
	assert.False(t, IsExecuteTool("write_file"))
	assert.False(t, IsExecuteTool("unknown_tool"))
}

func TestDraftAction_Struct(t *testing.T) {
	da := DraftAction{
		ID:   "act-1",
		Tool: "write_file",
		Args: map[string]interface{}{"path": "/tmp/test.txt", "content": "new content"},
	}
	assert.Equal(t, "act-1", da.ID)
	assert.Equal(t, "write_file", da.Tool)
	assert.Equal(t, "/tmp/test.txt", da.Args["path"])
}

func TestFileBackup_Struct(t *testing.T) {
	fb := FileBackup{
		OriginalPath: "/tmp/test.txt",
		BackupPath:   "/tmp/test.txt.bak",
		OriginalHash: "abc123",
	}
	assert.Equal(t, "/tmp/test.txt", fb.OriginalPath)
	assert.Equal(t, "/tmp/test.txt.bak", fb.BackupPath)
	assert.Equal(t, "abc123", fb.OriginalHash)
}
