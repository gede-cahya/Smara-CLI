package agent

import (
	"path/filepath"
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/gede-cahya/Smara-CLI/internal/skill"
	"github.com/stretchr/testify/require"
)

func TestIsMeaningfulChangeTool(t *testing.T) {
	cases := []struct {
		tool string
		want bool
	}{
		{"write_file", true},
		{"edit_file", true},
		{"run_command", true},
		{"remember", true},
		{"generate_image", true},
		{"obsidian_update_note", true},
		{"create_project", true},
		{"read_file", false},
		{"search_memories", false},
		{"obsidian_read_note", false},
		{"list_directory", false},
		{"get_file_nodes", false},
	}
	for _, tc := range cases {
		if got := isMeaningfulChangeTool(tc.tool); got != tc.want {
			t.Fatalf("isMeaningfulChangeTool(%q) = %v, want %v", tc.tool, got, tc.want)
		}
	}
}

func TestBuildChangeJournalEntryRedactsSecrets(t *testing.T) {
	entry := buildChangeJournalEntry(
		"set token=abc123 untuk deploy",
		&PromptResult{Response: "saved password: hunter2", ToolsExecuted: []string{"write_file"}},
		[]skill.TraceStep{{Tool: "write_file", Args: map[string]interface{}{
			"path":     "/tmp/project/.env",
			"api_key":  "abc123",
			"password": "hunter2",
		}}},
		ModeRush,
	)

	require.NotContains(t, entry, "abc123")
	require.NotContains(t, entry, "hunter2")
	require.NotContains(t, entry, "/tmp/project/.env")
	require.Contains(t, entry, "[REDACTED]")
	require.Contains(t, entry, "[REDACTED_PATH]")
}

func TestCaptureChangeJournalSavesMemory(t *testing.T) {
	store, err := memory.NewSQLiteStore(filepath.Join(t.TempDir(), "memory.db"))
	require.NoError(t, err)
	defer store.Close()

	s := NewSupervisor(&mockSSHProvider{}, store)
	s.SetMode(ModeRush)
	s.captureChangeJournal(
		"buat file test",
		&PromptResult{Response: "file dibuat", ToolsExecuted: []string{"write_file"}},
		[]skill.TraceStep{{Tool: "write_file", Args: map[string]interface{}{"path": "notes/test.md"}}},
	)

	items, err := store.List(0, 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "change_journal", items[0].Source)
	require.Contains(t, items[0].Content, "Smara Change")
	require.Contains(t, items[0].Tags, "smara-change")
	require.Contains(t, items[0].Tags, "auto-journal")
}
