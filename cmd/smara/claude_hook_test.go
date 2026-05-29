package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsMeaningfulClaudeTool(t *testing.T) {
	require.True(t, isMeaningfulClaudeTool("Write", map[string]interface{}{"file_path": "notes/test.md"}))
	require.True(t, isMeaningfulClaudeTool("Edit", map[string]interface{}{"file_path": "notes/test.md"}))
	require.True(t, isMeaningfulClaudeTool("Bash", map[string]interface{}{"command": "gofmt -w main.go"}))
	require.True(t, isMeaningfulClaudeTool("mcp__obsidian__obsidian_update_note", map[string]interface{}{"targetIdentifier": "note.md"}))
	require.False(t, isMeaningfulClaudeTool("Read", map[string]interface{}{"file_path": "notes/test.md"}))
	require.False(t, isMeaningfulClaudeTool("Grep", map[string]interface{}{"pattern": "TODO"}))
	require.False(t, isMeaningfulClaudeTool("Bash", map[string]interface{}{"command": "go test ./..."}))
}

func TestBuildClaudeCodeChangeJournalEntryRedactsSensitiveData(t *testing.T) {
	entry := buildClaudeCodeChangeJournalEntry(claudeHookEvent{
		SessionID:      "sess-1",
		TranscriptPath: "/tmp/project/.env/transcript.jsonl",
		CWD:            "/tmp/project",
		HookEventName:  "PostToolUse",
		ToolName:       "Write",
		ToolInput: map[string]interface{}{
			"file_path": "/tmp/project/.env",
			"content":   "password=hunter2",
		},
		ToolResponse: map[string]interface{}{"success": true},
	})

	require.Contains(t, entry, "Claude Code Change")
	require.Contains(t, entry, "[REDACTED_PATH]")
	require.NotContains(t, entry, "hunter2")
	require.NotContains(t, entry, "/tmp/project/.env")
}

func TestInstallClaudeHookWritesPostToolUseHook(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude", "settings.local.json")
	require.NoError(t, installClaudeHook(path, "smara claude-hook"))
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var settings map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &settings))
	hooks := settings["hooks"].(map[string]interface{})
	postToolUse := hooks["PostToolUse"].([]interface{})
	require.Len(t, postToolUse, 1)
	entry := postToolUse[0].(map[string]interface{})
	require.Contains(t, entry["matcher"], "Write")
	require.Contains(t, entry["hooks"].([]interface{})[0].(map[string]interface{})["command"], "smara claude-hook")
}
