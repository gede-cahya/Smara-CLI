package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCodexSkillMarkdown(t *testing.T) {
	input := `---
name: graphify
description: Build a knowledge graph from project files.
trigger: /graphify
tags: [codebase, graph]
---

# /graphify

Run graphify on the current workspace, then summarize the report.
`

	sk, err := ParseCodexSkillMarkdown([]byte(input), "", "/tmp/graphify")
	require.NoError(t, err)
	assert.Equal(t, "graphify", sk.Name)
	assert.Equal(t, "Build a knowledge graph from project files.", sk.Description)
	assert.Equal(t, "/graphify", sk.Trigger)
	require.Len(t, sk.Steps, 1)
	assert.Equal(t, "skill_instructions", sk.Steps[0].Tool)
	assert.Contains(t, sk.Steps[0].Args["instructions"], "Run graphify")
}

func TestLoadAndList_CodexFolderSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".smara", "skills", "graphify")
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(`---
name: graphify
description: Build a graph.
trigger: /graphify
---

Use graphify for codebase questions.
`), 0644))

	names, err := List()
	require.NoError(t, err)
	assert.Contains(t, names, "graphify")

	sk, err := Load("graphify")
	require.NoError(t, err)
	assert.Equal(t, "graphify", sk.Name)
	require.Len(t, sk.Steps, 1)
	assert.Equal(t, "skill_instructions", sk.Steps[0].Tool)
}
