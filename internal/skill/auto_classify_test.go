package skill

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyTrace_SSHGoesToRemoteBranch(t *testing.T) {
	tr := ExecutionTrace{
		Steps: []TraceStep{
			{Tool: "ssh_exec"},
			{Tool: "ssh_exec"},
		},
	}
	path, tags := ClassifyTrace(tr)
	assert.Equal(t, []string{"remote", "ssh"}, path)
	assert.Contains(t, tags, "auto-captured")
	assert.Contains(t, tags, "remote")
	assert.Contains(t, tags, "ssh")
}

func TestClassifyTrace_FileEditDominatesShell(t *testing.T) {
	// run_command + edit_file should land in the "file/edit" bucket, not
	// "shell/run", because edit is more specific.
	tr := ExecutionTrace{
		Steps: []TraceStep{
			{Tool: "run_command"},
			{Tool: "edit_file"},
		},
	}
	path, _ := ClassifyTrace(tr)
	assert.Equal(t, []string{"file", "edit"}, path)
}

func TestClassifyTrace_UnknownToolsFallback(t *testing.T) {
	tr := ExecutionTrace{
		Steps: []TraceStep{
			{Tool: "some_unknown_tool"},
			{Tool: "another_unknown"},
		},
	}
	path, tags := ClassifyTrace(tr)
	assert.Equal(t, []string{"general", "multi-step"}, path)
	assert.Contains(t, tags, "auto-captured")
}

func TestPromoteTagsToSubcategory_VPSKeyword(t *testing.T) {
	base := []string{"remote", "ssh"}
	out := PromoteTagsToSubcategory(base, "cek status service di vps saya")
	assert.Equal(t, []string{"remote", "ssh", "vps"}, out)
}

func TestPromoteTagsToSubcategory_NoOpWhenNothingMatches(t *testing.T) {
	base := []string{"file", "read"}
	out := PromoteTagsToSubcategory(base, "tolong lihat isi file ini")
	assert.Equal(t, []string{"file", "read"}, out)
}

func TestFindParentBySubpattern_BasicChain(t *testing.T) {
	setupHome(t)
	db := newMemDB(t)
	require.NoError(t, EnsureAutoDetectTable(db))

	// Parent skill already saved + registered in auto_skill_patterns.
	parent := &Skill{
		Name: "cek-service",
		Steps: []Step{
			{Tool: "ssh_exec", Args: map[string]interface{}{"host": "vps", "command": "systemctl"}},
			{Tool: "ssh_exec", Args: map[string]interface{}{"host": "vps", "command": "pm2"}},
		},
		Description: "cek",
		Version:     1,
	}
	require.NoError(t, Save(parent, nil))

	parentTrace := ExecutionTrace{Steps: []TraceStep{
		{Tool: "ssh_exec", Args: map[string]interface{}{"host": "vps", "command": "systemctl"}},
		{Tool: "ssh_exec", Args: map[string]interface{}{"host": "vps", "command": "pm2"}},
	}}
	for i := 0; i < 3; i++ {
		_, _, _ = RecordTrace(db, parentTrace, 3)
	}
	require.NoError(t, MarkPatternCaptured(db, parentTrace.Fingerprint(), parent.Name))

	// A longer trace that starts with the same two steps → parent found.
	extended := ExecutionTrace{Steps: []TraceStep{
		{Tool: "ssh_exec", Args: map[string]interface{}{"host": "vps", "command": "systemctl"}},
		{Tool: "ssh_exec", Args: map[string]interface{}{"host": "vps", "command": "pm2"}},
		{Tool: "run_command", Args: map[string]interface{}{"command": "echo done"}},
	}}
	name, ok := FindParentBySubpattern(db, extended)
	assert.True(t, ok)
	assert.Equal(t, "cek-service", name)
}

func TestFindParentBySubpattern_NoFalsePositive(t *testing.T) {
	setupHome(t)
	db := newMemDB(t)
	require.NoError(t, EnsureAutoDetectTable(db))

	// Completely unrelated pattern registered
	other := &Skill{
		Name:  "other",
		Steps: []Step{{Tool: "run_command", Args: map[string]interface{}{"command": "ls"}}},
		Version: 1,
		Description: "other",
	}
	require.NoError(t, Save(other, nil))

	otherTrace := ExecutionTrace{Steps: []TraceStep{
		{Tool: "run_command", Args: map[string]interface{}{"command": "ls"}},
		{Tool: "run_command", Args: map[string]interface{}{"command": "pwd"}},
	}}
	for i := 0; i < 3; i++ {
		_, _, _ = RecordTrace(db, otherTrace, 3)
	}
	require.NoError(t, MarkPatternCaptured(db, otherTrace.Fingerprint(), other.Name))

	// Different trace — should NOT match "other"
	different := ExecutionTrace{Steps: []TraceStep{
		{Tool: "ssh_exec", Args: map[string]interface{}{"host": "x"}},
		{Tool: "ssh_exec", Args: map[string]interface{}{"host": "y"}},
	}}
	name, ok := FindParentBySubpattern(db, different)
	assert.False(t, ok)
	assert.Equal(t, "", name)
}

func TestFindParentBySubpattern_LongestPrefixWins(t *testing.T) {
	setupHome(t)
	db := newMemDB(t)
	require.NoError(t, EnsureAutoDetectTable(db))

	// Two captured skills: short is a prefix of long, both prefix of extended
	shortSk := &Skill{
		Name:  "short",
		Steps: []Step{{Tool: "ssh_exec", Args: map[string]interface{}{"h": "1"}}},
		Version: 1,
		Description: "short",
	}
	require.NoError(t, Save(shortSk, nil))
	shortTrace := ExecutionTrace{Steps: []TraceStep{
		{Tool: "ssh_exec", Args: map[string]interface{}{"h": "1"}},
		{Tool: "ssh_exec", Args: map[string]interface{}{"h": "2"}},
	}}
	// Need >=2 steps to be trackable; use short 2-step "short" pattern
	for i := 0; i < 3; i++ {
		_, _, _ = RecordTrace(db, shortTrace, 3)
	}
	require.NoError(t, MarkPatternCaptured(db, shortTrace.Fingerprint(), shortSk.Name))

	longSk := &Skill{
		Name: "long",
		Steps: []Step{
			{Tool: "ssh_exec", Args: map[string]interface{}{"h": "1"}},
			{Tool: "ssh_exec", Args: map[string]interface{}{"h": "2"}},
			{Tool: "run_command", Args: map[string]interface{}{"c": "a"}},
		},
		Version:     1,
		Description: "long",
	}
	require.NoError(t, Save(longSk, nil))
	longTrace := ExecutionTrace{Steps: []TraceStep{
		{Tool: "ssh_exec", Args: map[string]interface{}{"h": "1"}},
		{Tool: "ssh_exec", Args: map[string]interface{}{"h": "2"}},
		{Tool: "run_command", Args: map[string]interface{}{"c": "a"}},
	}}
	for i := 0; i < 3; i++ {
		_, _, _ = RecordTrace(db, longTrace, 3)
	}
	require.NoError(t, MarkPatternCaptured(db, longTrace.Fingerprint(), longSk.Name))

	// Extended trace: contains both prefixes; longest should win.
	extended := ExecutionTrace{Steps: []TraceStep{
		{Tool: "ssh_exec", Args: map[string]interface{}{"h": "1"}},
		{Tool: "ssh_exec", Args: map[string]interface{}{"h": "2"}},
		{Tool: "run_command", Args: map[string]interface{}{"c": "a"}},
		{Tool: "run_command", Args: map[string]interface{}{"c": "b"}},
	}}
	name, ok := FindParentBySubpattern(db, extended)
	assert.True(t, ok)
	assert.Equal(t, "long", name, "longest prefix should win over shorter one")
}

// setupHome for classify tests (shared across this file).
func setupHome(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
}
