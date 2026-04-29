package safety

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEngine_Defaults(t *testing.T) {
	e := NewEngine()
	require.NotNil(t, e)
	assert.Equal(t, ModePlan, e.GetMode())
	assert.Empty(t, e.GetDrafts())
	assert.Empty(t, e.GetBackups())
}

func TestEngine_SetModeAndGetMode(t *testing.T) {
	e := NewEngine()
	e.SetMode(ModeBuild)
	assert.Equal(t, ModeBuild, e.GetMode())
	e.SetMode(ModePlan)
	assert.Equal(t, ModePlan, e.GetMode())
}

func TestEngine_SetMode_Callback(t *testing.T) {
	e := NewEngine()
	var calledWith ExecutionMode
	e.SetModeChangeCallback(func(m ExecutionMode) {
		calledWith = m
	})
	e.SetMode(ModeBuild)
	assert.Equal(t, ModeBuild, calledWith)
}

func TestCanExecute_PlanMode_ReadOnly(t *testing.T) {
	e := NewEngine()
	e.SetMode(ModePlan)

	ok, reason := e.CanExecute("view_file")
	assert.True(t, ok)
	assert.Empty(t, reason)

	ok, reason = e.CanExecute("read_file")
	assert.True(t, ok)
	assert.Empty(t, reason)

	ok, reason = e.CanExecute("search_memories")
	assert.True(t, ok)
	assert.Empty(t, reason)
}

func TestCanExecute_PlanMode_WriteBlocked(t *testing.T) {
	e := NewEngine()
	e.SetMode(ModePlan)

	ok, reason := e.CanExecute("write_file")
	assert.False(t, ok)
	assert.Contains(t, reason, "Plan Mode")
	assert.Contains(t, reason, "write_file")

	ok, reason = e.CanExecute("edit_file")
	assert.False(t, ok)
	assert.Contains(t, reason, "Plan Mode")

	ok, reason = e.CanExecute("delete_file")
	assert.False(t, ok)
	assert.Contains(t, reason, "Plan Mode")
}

func TestCanExecute_PlanMode_ExecuteBlocked(t *testing.T) {
	e := NewEngine()
	e.SetMode(ModePlan)

	ok, reason := e.CanExecute("run_command")
	assert.False(t, ok)
	assert.Contains(t, reason, "Plan Mode")

	ok, reason = e.CanExecute("execute_command")
	assert.False(t, ok)
	assert.Contains(t, reason, "Plan Mode")
}

func TestCanExecute_PlanMode_UnknownToolBlocked(t *testing.T) {
	e := NewEngine()
	e.SetMode(ModePlan)

	ok, reason := e.CanExecute("unknown_tool")
	assert.False(t, ok)
	assert.Contains(t, reason, "unknown_tool")
}

func TestCanExecute_BuildMode_AllAllowed(t *testing.T) {
	e := NewEngine()
	e.SetMode(ModeBuild)

	for _, tool := range []string{"view_file", "read_file", "write_file", "edit_file", "run_command", "delete_file", "unknown_tool"} {
		ok, reason := e.CanExecute(tool)
		assert.True(t, ok, "tool %s should be allowed in build mode", tool)
		assert.Empty(t, reason, "tool %s should have no blocking reason", tool)
	}
}

func TestIsReadOnlyTool(t *testing.T) {
	assert.True(t, IsReadOnlyTool("view_file"))
	assert.True(t, IsReadOnlyTool("read_file"))
	assert.True(t, IsReadOnlyTool("list_directory"))
	assert.True(t, IsReadOnlyTool("search_memories"))
	assert.True(t, IsReadOnlyTool("grep_search"))
	assert.True(t, IsReadOnlyTool("web_search"))
	assert.True(t, IsReadOnlyTool("get_cwd"))
	assert.False(t, IsReadOnlyTool("write_file"))
	assert.False(t, IsReadOnlyTool("edit_file"))
	assert.False(t, IsReadOnlyTool("run_command"))
	assert.False(t, IsReadOnlyTool("delete_file"))
	assert.False(t, IsReadOnlyTool("unknown_tool"))
}

func TestIsWriteTool(t *testing.T) {
	assert.True(t, IsWriteTool("write_file"))
	assert.True(t, IsWriteTool("edit_file"))
	assert.True(t, IsWriteTool("patch_file"))
	assert.True(t, IsWriteTool("delete_file"))
	assert.True(t, IsWriteTool("remember"))
	assert.False(t, IsWriteTool("view_file"))
	assert.False(t, IsWriteTool("run_command"))
	assert.False(t, IsWriteTool("unknown_tool"))
}

func TestIsExecuteTool(t *testing.T) {
	assert.True(t, IsExecuteTool("run_command"))
	assert.True(t, IsExecuteTool("execute_command"))
	assert.True(t, IsExecuteTool("run_terminal"))
	assert.False(t, IsExecuteTool("view_file"))
	assert.False(t, IsExecuteTool("write_file"))
	assert.False(t, IsExecuteTool("unknown_tool"))
}

func TestEngine_RecordDraft(t *testing.T) {
	e := NewEngine()
	e.SetMode(ModePlan)

	args := map[string]interface{}{"path": "/tmp/test.txt", "content": "hello"}
	draft := e.RecordDraft("write_file", args)

	require.NotNil(t, draft)
	assert.Equal(t, "write_file", draft.Tool)
	assert.Equal(t, args, draft.Args)
	assert.Equal(t, ActionWrite, draft.Action)
	assert.False(t, draft.Approved)
	assert.NotEmpty(t, draft.ID)
	assert.False(t, draft.Timestamp.IsZero())
}

func TestEngine_RecordDraft_ReadTool(t *testing.T) {
	e := NewEngine()
	draft := e.RecordDraft("view_file", map[string]interface{}{"path": "/tmp/test.txt"})
	assert.Equal(t, ActionRead, draft.Action)
}

func TestEngine_GetDrafts(t *testing.T) {
	e := NewEngine()
	e.RecordDraft("write_file", map[string]interface{}{})
	e.RecordDraft("edit_file", map[string]interface{}{})

	drafts := e.GetDrafts()
	assert.Len(t, drafts, 2)
	assert.Equal(t, "write_file", drafts[0].Tool)
	assert.Equal(t, "edit_file", drafts[1].Tool)
}

func TestEngine_GetDrafts_Isolation(t *testing.T) {
	e := NewEngine()
	e.RecordDraft("write_file", map[string]interface{}{})

	drafts := e.GetDrafts()
	drafts[0].Tool = "modified"

	// Original should be unaffected
	drafts2 := e.GetDrafts()
	assert.Equal(t, "write_file", drafts2[0].Tool)
}

func TestEngine_ApproveDraft(t *testing.T) {
	e := NewEngine()
	draft := e.RecordDraft("write_file", map[string]interface{}{})
	assert.False(t, draft.Approved)

	ok := e.ApproveDraft(draft.ID)
	assert.True(t, ok)

	drafts := e.GetDrafts()
	assert.True(t, drafts[0].Approved)
}

func TestEngine_ApproveDraft_NotFound(t *testing.T) {
	e := NewEngine()
	ok := e.ApproveDraft("nonexistent")
	assert.False(t, ok)
}

func TestEngine_ClearDrafts(t *testing.T) {
	e := NewEngine()
	e.RecordDraft("write_file", map[string]interface{}{})
	assert.Len(t, e.GetDrafts(), 1)

	e.ClearDrafts()
	assert.Empty(t, e.GetDrafts())
}

func TestEngine_ListBlockedTools_PlanMode(t *testing.T) {
	e := NewEngine()
	e.SetMode(ModePlan)
	blocked := e.ListBlockedTools()
	assert.NotEmpty(t, blocked)
	for _, tool := range blocked {
		assert.False(t, IsReadOnlyTool(tool), "read-only tool %s should not be blocked", tool)
	}
	assert.Contains(t, blocked, "write_file")
	assert.Contains(t, blocked, "run_command")
	assert.Contains(t, blocked, "delete_file")
}

func TestEngine_ListBlockedTools_BuildMode(t *testing.T) {
	e := NewEngine()
	e.SetMode(ModeBuild)
	blocked := e.ListBlockedTools()
	assert.Empty(t, blocked)
}

func TestRegisterToolAction(t *testing.T) {
	RegisterToolAction("custom_delete", ActionDelete)
	assert.True(t, IsWriteTool("custom_delete"))
	assert.False(t, IsReadOnlyTool("custom_delete"))
	assert.False(t, IsExecuteTool("custom_delete"))
}

func TestEngine_BackupFile_NewFile(t *testing.T) {
	e := NewEngine()
	tmpDir := t.TempDir()
	newFile := filepath.Join(tmpDir, "new_file.txt")

	err := e.BackupFile(newFile)
	require.NoError(t, err)

	backups := e.GetBackups()
	require.Len(t, backups, 1)
	assert.Equal(t, newFile, backups[0].OriginalPath)
	assert.Empty(t, backups[0].BackupPath) // No backup needed for new files
	assert.Empty(t, backups[0].OriginalHash)
}

func TestEngine_BackupFile_ExistingFile(t *testing.T) {
	e := NewEngine()
	tmpDir := t.TempDir()
	existingFile := filepath.Join(tmpDir, "existing.txt")
	require.NoError(t, os.WriteFile(existingFile, []byte("original content"), 0o644))

	err := e.BackupFile(existingFile)
	require.NoError(t, err)

	backups := e.GetBackups()
	require.Len(t, backups, 1)
	assert.Equal(t, existingFile, backups[0].OriginalPath)
	assert.NotEmpty(t, backups[0].BackupPath)
	assert.NotEmpty(t, backups[0].OriginalHash)

	// Verify backup content matches
	backupData, err := os.ReadFile(backups[0].BackupPath)
	require.NoError(t, err)
	assert.Equal(t, "original content", string(backupData))
}

func TestEngine_GetBackups_Isolation(t *testing.T) {
	e := NewEngine()
	tmpDir := t.TempDir()
	existingFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(existingFile, []byte("data"), 0o644))

	e.BackupFile(existingFile)
	backups := e.GetBackups()
	require.Len(t, backups, 1)

	// Modifying returned slice should not affect internal state
	backups[0].OriginalPath = "modified"
	backups2 := e.GetBackups()
	assert.Equal(t, existingFile, backups2[0].OriginalPath)
}

func TestEngine_RevertFile_NewFile(t *testing.T) {
	e := NewEngine()
	tmpDir := t.TempDir()
	newFile := filepath.Join(tmpDir, "new.txt")

	// Backup file BEFORE it exists (no backup will be created)
	err := e.BackupFile(newFile)
	require.NoError(t, err)

	// Now create the file
	require.NoError(t, os.WriteFile(newFile, []byte("created"), 0o644))

	// Revert should delete the file (it didn't exist originally)
	err = e.RevertFile(newFile)
	require.NoError(t, err)
	_, statErr := os.Stat(newFile)
	assert.True(t, os.IsNotExist(statErr))
}

func TestEngine_RevertFile_ExistingFile(t *testing.T) {
	e := NewEngine()
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(file, []byte("original"), 0o644))

	err := e.BackupFile(file)
	require.NoError(t, err)

	// Modify the file
	require.NoError(t, os.WriteFile(file, []byte("modified"), 0o644))

	// Revert should restore original content
	err = e.RevertFile(file)
	require.NoError(t, err)

	content, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "original", string(content))
}

func TestEngine_RevertFile_NoBackup(t *testing.T) {
	e := NewEngine()
	err := e.RevertFile("/tmp/nonexistent.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tidak ada backup")
}

func TestToolAction_MapCompleteness(t *testing.T) {
	// Ensure all tools in the map are categorized
	for tool, action := range toolActions {
		switch action {
		case ActionRead:
			assert.True(t, IsReadOnlyTool(tool), "%s should be read-only", tool)
			assert.False(t, IsWriteTool(tool), "%s should not be write", tool)
			assert.False(t, IsExecuteTool(tool), "%s should not be execute", tool)
		case ActionWrite:
			assert.True(t, IsWriteTool(tool), "%s should be write", tool)
			assert.False(t, IsReadOnlyTool(tool), "%s should not be read-only", tool)
			assert.False(t, IsExecuteTool(tool), "%s should not be execute", tool)
		case ActionExecute:
			assert.True(t, IsExecuteTool(tool), "%s should be execute", tool)
			assert.False(t, IsReadOnlyTool(tool), "%s should not be read-only", tool)
			assert.False(t, IsWriteTool(tool), "%s should not be write", tool)
		case ActionDelete:
			assert.True(t, IsWriteTool(tool), "%s should be write (delete is write)", tool)
			assert.False(t, IsReadOnlyTool(tool), "%s should not be read-only", tool)
		}
	}
}

func TestActionType_String(t *testing.T) {
	assert.Equal(t, ActionRead, ActionType("read"))
	assert.Equal(t, ActionWrite, ActionType("write"))
	assert.Equal(t, ActionExecute, ActionType("execute"))
	assert.Equal(t, ActionDelete, ActionType("delete"))
}

func TestExecutionMode_String(t *testing.T) {
	assert.Equal(t, ModePlan, ExecutionMode("plan"))
	assert.Equal(t, ModeBuild, ExecutionMode("build"))
}
