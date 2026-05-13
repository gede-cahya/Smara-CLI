package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

// setupSkillTestHome points HOME to a temp dir so skills are isolated.
func setupSkillTestHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	// Ensure directory exists
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, ".smara", "skills"), 0755))
	return tmp
}

func TestSkillCreate_Happy(t *testing.T) {
	setupSkillTestHome(t)

	args := map[string]interface{}{
		"name":        "cek-service-vps",
		"description": "Cek status service penting di VPS.",
		"steps": []interface{}{
			map[string]interface{}{
				"tool": "ssh_exec",
				"args": map[string]interface{}{
					"host":    "vps-cahya",
					"command": "systemctl status smara.service --no-pager | head -10",
				},
			},
		},
		"tags": []interface{}{"vps", "monitoring"},
	}

	out, err := createSkillFromArgs(args)
	require.NoError(t, err)
	assert.Contains(t, out, "cek-service-vps")
	assert.Contains(t, out, "v1")

	// Verify persisted
	sk, err := skill.Load("cek-service-vps")
	require.NoError(t, err)
	assert.Equal(t, "cek-service-vps", sk.Name)
	assert.Equal(t, 1, sk.Version)
	assert.Len(t, sk.Steps, 1)
	assert.Equal(t, "ssh_exec", sk.Steps[0].Tool)
	assert.Equal(t, []string{"vps", "monitoring"}, sk.Tags)
}

func TestSkillCreate_RejectsDuplicate(t *testing.T) {
	setupSkillTestHome(t)

	base := map[string]interface{}{
		"name":        "dup",
		"description": "first",
		"steps": []interface{}{
			map[string]interface{}{
				"tool": "run_command",
				"args": map[string]interface{}{"command": "echo hi"},
			},
		},
	}

	_, err := createSkillFromArgs(base)
	require.NoError(t, err)

	// Second create without overwrite should fail.
	_, err = createSkillFromArgs(base)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sudah ada")
}

func TestSkillCreate_Overwrite(t *testing.T) {
	setupSkillTestHome(t)

	args := map[string]interface{}{
		"name":        "ow",
		"description": "v1",
		"steps": []interface{}{
			map[string]interface{}{"tool": "run_command", "args": map[string]interface{}{"command": "echo 1"}},
		},
	}
	_, err := createSkillFromArgs(args)
	require.NoError(t, err)

	// Overwrite with v2 body
	args["description"] = "v2 description"
	args["overwrite"] = true
	_, err = createSkillFromArgs(args)
	require.NoError(t, err)

	sk, err := skill.Load("ow")
	require.NoError(t, err)
	assert.Equal(t, 2, sk.Version)
	assert.Equal(t, "v2 description", sk.Description)
}

func TestSkillCreate_InvalidName(t *testing.T) {
	setupSkillTestHome(t)

	_, err := createSkillFromArgs(map[string]interface{}{
		"name":        "bad name with spaces",
		"description": "x",
		"steps":       []interface{}{map[string]interface{}{"tool": "run_command", "args": map[string]interface{}{}}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "huruf, angka")
}

func TestSkillCreate_MissingRequired(t *testing.T) {
	setupSkillTestHome(t)

	tests := []struct {
		name string
		args map[string]interface{}
		msg  string
	}{
		{
			name: "no name",
			args: map[string]interface{}{"description": "x", "steps": []interface{}{map[string]interface{}{"tool": "run_command", "args": map[string]interface{}{}}}},
			msg:  "'name' wajib",
		},
		{
			name: "no description",
			args: map[string]interface{}{"name": "foo", "steps": []interface{}{map[string]interface{}{"tool": "run_command", "args": map[string]interface{}{}}}},
			msg:  "'description' wajib",
		},
		{
			name: "no steps",
			args: map[string]interface{}{"name": "foo", "description": "x"},
			msg:  "'steps' wajib",
		},
		{
			name: "empty steps",
			args: map[string]interface{}{"name": "foo", "description": "x", "steps": []interface{}{}},
			msg:  "'steps' wajib",
		},
		{
			name: "step missing tool",
			args: map[string]interface{}{"name": "foo", "description": "x", "steps": []interface{}{map[string]interface{}{"args": map[string]interface{}{}}}},
			msg:  ".tool wajib",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := createSkillFromArgs(tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.msg)
		})
	}
}

func TestSkillCreate_WithParams(t *testing.T) {
	setupSkillTestHome(t)

	args := map[string]interface{}{
		"name":        "greet",
		"description": "Greeting skill",
		"steps": []interface{}{
			map[string]interface{}{
				"tool": "run_command",
				"args": map[string]interface{}{"command": "echo hello __PARAM__who"},
			},
		},
		"params": []interface{}{
			map[string]interface{}{
				"name":        "who",
				"type":        "string",
				"description": "who to greet",
				"required":    true,
				"default":     "world",
			},
		},
	}
	_, err := createSkillFromArgs(args)
	require.NoError(t, err)

	sk, err := skill.Load("greet")
	require.NoError(t, err)
	require.Len(t, sk.Params, 1)
	assert.Equal(t, "who", sk.Params[0].Name)
	assert.Equal(t, "string", sk.Params[0].Type)
	assert.True(t, sk.Params[0].Required)
	assert.Equal(t, "world", sk.Params[0].Default)
}

func TestBuiltinTools_IncludeSkillCreate(t *testing.T) {
	tools := GetBuiltinTools()
	names := map[string]bool{}
	for _, t := range tools {
		names[t.Name] = true
	}
	assert.True(t, names["skill_create"], "skill_create should be registered")
	assert.True(t, names["skill_list"], "skill_list should be registered")
	assert.True(t, names["skill_delete"], "skill_delete should be registered")
	assert.True(t, names["skill_run"], "skill_run should still be registered")
}


// TestSkillCreate_EndToEnd verifies LLM can create a skill via ExecuteBuiltinTool
// and then run it via skill_run — simulating the full bot flow.
func TestSkillCreate_EndToEnd(t *testing.T) {
	setupSkillTestHome(t)

	// 1. LLM calls skill_create to save a skill
	out, err := ExecuteBuiltinTool("skill_create", map[string]interface{}{
		"name":        "say-hi",
		"description": "Echo hello message",
		"steps": []interface{}{
			map[string]interface{}{
				"tool": "run_command",
				"args": map[string]interface{}{"command": "echo hi-from-skill"},
			},
		},
	}, nil)
	require.NoError(t, err)
	assert.Contains(t, out, "say-hi")

	// 2. LLM calls skill_list to confirm
	listOut, err := ExecuteBuiltinTool("skill_list", map[string]interface{}{}, nil)
	require.NoError(t, err)
	assert.Contains(t, listOut, "say-hi")
	assert.Contains(t, listOut, "Echo hello message")

	// 3. LLM calls skill_delete to clean up
	delOut, err := ExecuteBuiltinTool("skill_delete", map[string]interface{}{
		"skill_name": "say-hi",
	}, nil)
	require.NoError(t, err)
	assert.Contains(t, delOut, "dihapus")

	// 4. Verify deletion
	_, err = skill.Load("say-hi")
	require.Error(t, err)
}


// TestSkillCreate_OverwritePreservesLineage verifies that when a skill is
// overwritten via skill_create, the previous version's description/tags/step
// count are captured in the new skill's Lineage array. This powers the
// Hierarchy view's refine chain display.
func TestSkillCreate_OverwritePreservesLineage(t *testing.T) {
	setupSkillTestHome(t)

	v1 := map[string]interface{}{
		"name":        "chain",
		"description": "first revision",
		"tags":        []interface{}{"foo"},
		"steps": []interface{}{
			map[string]interface{}{"tool": "run_command", "args": map[string]interface{}{"command": "ls"}},
		},
	}
	_, err := createSkillFromArgs(v1)
	require.NoError(t, err)

	v2 := map[string]interface{}{
		"name":        "chain",
		"description": "improved revision",
		"tags":        []interface{}{"foo", "bar"},
		"overwrite":   true,
		"steps": []interface{}{
			map[string]interface{}{"tool": "run_command", "args": map[string]interface{}{"command": "ls -la"}},
			map[string]interface{}{"tool": "run_command", "args": map[string]interface{}{"command": "pwd"}},
		},
	}
	_, err = createSkillFromArgs(v2)
	require.NoError(t, err)

	sk, err := skill.Load("chain")
	require.NoError(t, err)
	assert.Equal(t, 2, sk.Version)
	assert.Equal(t, "improved revision", sk.Description)
	require.Len(t, sk.Lineage, 1, "v1 should be captured in lineage")
	assert.Equal(t, 1, sk.Lineage[0].Version)
	assert.Equal(t, "first revision", sk.Lineage[0].Description)
	assert.Equal(t, 1, sk.Lineage[0].StepCount)
	assert.Equal(t, "manual", sk.Lineage[0].RefinedFrom)

	// A third overwrite keeps the chain intact.
	v3 := map[string]interface{}{
		"name":        "chain",
		"description": "third revision",
		"overwrite":   true,
		"steps":       v2["steps"],
	}
	_, err = createSkillFromArgs(v3)
	require.NoError(t, err)

	sk, err = skill.Load("chain")
	require.NoError(t, err)
	assert.Equal(t, 3, sk.Version)
	require.Len(t, sk.Lineage, 2, "lineage should grow to 2 ancestors")
	assert.Equal(t, 1, sk.Lineage[0].Version)
	assert.Equal(t, 2, sk.Lineage[1].Version)
}

// TestSkillCreate_ExplicitParent verifies the parent relationship is stored
// so hierarchy view can render parent → child edges.
func TestSkillCreate_ExplicitParent(t *testing.T) {
	setupSkillTestHome(t)

	// Create parent first
	_, err := createSkillFromArgs(map[string]interface{}{
		"name":        "parent-skill",
		"description": "root",
		"steps": []interface{}{
			map[string]interface{}{"tool": "run_command", "args": map[string]interface{}{"command": "echo p"}},
		},
	})
	require.NoError(t, err)

	// Child points to parent
	out, err := createSkillFromArgs(map[string]interface{}{
		"name":          "child-skill",
		"description":   "leaf",
		"parent":        "parent-skill",
		"category_path": []interface{}{"examples", "children"},
		"dependencies":  []interface{}{"parent-skill"},
		"steps": []interface{}{
			map[string]interface{}{"tool": "run_command", "args": map[string]interface{}{"command": "echo c"}},
		},
	})
	require.NoError(t, err)
	assert.Contains(t, out, "parent=parent-skill")

	sk, err := skill.Load("child-skill")
	require.NoError(t, err)
	assert.Equal(t, "parent-skill", sk.ParentID)
	assert.Equal(t, []string{"examples", "children"}, sk.CategoryPath)
	assert.Equal(t, []string{"parent-skill"}, sk.Dependencies)
}

// TestSkillCreate_UnknownParentRejected ensures we reject a parent reference
// that does not actually exist, to avoid creating orphaned hierarchy nodes.
func TestSkillCreate_UnknownParentRejected(t *testing.T) {
	setupSkillTestHome(t)

	_, err := createSkillFromArgs(map[string]interface{}{
		"name":        "lonely",
		"description": "x",
		"parent":      "nonexistent-parent",
		"steps": []interface{}{
			map[string]interface{}{"tool": "run_command", "args": map[string]interface{}{"command": "ls"}},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tidak ditemukan")
}

// TestSkillCreate_SelfParentRejected prevents A pointing at itself.
func TestSkillCreate_SelfParentRejected(t *testing.T) {
	setupSkillTestHome(t)

	// First create the skill
	_, err := createSkillFromArgs(map[string]interface{}{
		"name":        "loop",
		"description": "x",
		"steps": []interface{}{
			map[string]interface{}{"tool": "run_command", "args": map[string]interface{}{"command": "ls"}},
		},
	})
	require.NoError(t, err)

	// Now try to overwrite with self as parent
	_, err = createSkillFromArgs(map[string]interface{}{
		"name":        "loop",
		"description": "x",
		"parent":      "loop",
		"overwrite":   true,
		"steps": []interface{}{
			map[string]interface{}{"tool": "run_command", "args": map[string]interface{}{"command": "ls"}},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tidak bisa menjadi induk dirinya sendiri")
}
