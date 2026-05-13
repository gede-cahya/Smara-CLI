package skill

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupExportHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, ".smara", "skills"), 0755))
	return tmp
}

func openMemDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestExport_RoundtripPreservesSkill writes a few skills, exports them,
// clears the directory, then imports and verifies everything came back
// intact (including parent_id, tags, lineage).
func TestExport_RoundtripPreservesSkills(t *testing.T) {
	setupExportHome(t)

	// Seed 3 skills with parent relationships and lineage
	parent := &Skill{
		Name:        "parent",
		Description: "root skill",
		Version:     2,
		Tags:        []string{"root"},
		Steps:       []Step{{Tool: "run_command", Args: map[string]interface{}{"command": "p"}}},
		Lineage:     []LineageEntry{{Version: 1, Description: "v1", StepCount: 1, RefinedFrom: "manual"}},
	}
	child := &Skill{
		Name:         "child",
		Description:  "leaf",
		Version:      1,
		Tags:         []string{"leaf"},
		ParentID:     "parent",
		CategoryPath: []string{"demo", "children"},
		Dependencies: []string{"parent"},
		Steps:        []Step{{Tool: "run_command", Args: map[string]interface{}{"command": "c"}}},
	}
	loner := &Skill{
		Name: "loner", Description: "alone", Version: 1,
		Steps: []Step{{Tool: "run_command", Args: map[string]interface{}{"command": "l"}}},
	}
	require.NoError(t, Save(parent, nil))
	require.NoError(t, Save(child, nil))
	require.NoError(t, Save(loner, nil))

	// Export (no DB, patterns will be empty but that's ok)
	env, err := ExportAll(nil, "test-host")
	require.NoError(t, err)
	assert.Equal(t, "smara.skill-tree/v1", env.Schema)
	assert.Equal(t, "test-host", env.Source)
	assert.Len(t, env.Skills, 3)

	// Serialize then deserialize to mimic file IO
	var buf bytes.Buffer
	require.NoError(t, WriteExport(&buf, env))
	parsed, err := ReadExport(&buf)
	require.NoError(t, err)
	assert.Len(t, parsed.Skills, 3)

	// Move to a fresh home and import
	setupExportHome(t)
	result, err := ImportAll(nil, parsed, ConflictOverwrite, false)
	require.NoError(t, err)
	assert.Len(t, result.Created, 3)
	assert.Empty(t, result.Overwritten)
	assert.Empty(t, result.Skipped)

	// Verify the child preserved its parent_id and lineage stayed intact
	restored, err := Load("child")
	require.NoError(t, err)
	assert.Equal(t, "parent", restored.ParentID)
	assert.Equal(t, []string{"demo", "children"}, restored.CategoryPath)
	assert.Equal(t, []string{"parent"}, restored.Dependencies)

	restoredParent, err := Load("parent")
	require.NoError(t, err)
	assert.Equal(t, 2, restoredParent.Version)
	require.Len(t, restoredParent.Lineage, 1)
	assert.Equal(t, "manual", restoredParent.Lineage[0].RefinedFrom)
}

// TestImport_ModeSkipKeepsExisting verifies that when mode=skip and a
// skill with the same name already exists locally, the existing one wins.
func TestImport_ModeSkipKeepsExisting(t *testing.T) {
	setupExportHome(t)

	// Local version
	local := &Skill{
		Name: "dup", Description: "LOCAL", Version: 5,
		Steps: []Step{{Tool: "run_command", Args: map[string]interface{}{"command": "local"}}},
	}
	require.NoError(t, Save(local, nil))

	// Envelope with conflicting name
	envelope := &TreeExport{
		Schema:  "smara.skill-tree/v1",
		Version: 1,
		Skills: []Skill{
			{
				Name: "dup", Description: "IMPORTED", Version: 1,
				Steps: []Step{{Tool: "run_command", Args: map[string]interface{}{"command": "imported"}}},
			},
		},
	}

	result, err := ImportAll(nil, envelope, ConflictSkip, false)
	require.NoError(t, err)
	assert.Contains(t, result.Skipped, "dup")

	current, err := Load("dup")
	require.NoError(t, err)
	assert.Equal(t, "LOCAL", current.Description)
	assert.Equal(t, 5, current.Version)
}

// TestImport_ModeRenameCreatesSuffix verifies that rename preserves both.
func TestImport_ModeRenameCreatesSuffix(t *testing.T) {
	setupExportHome(t)

	local := &Skill{
		Name: "dup", Description: "LOCAL", Version: 1,
		Steps: []Step{{Tool: "run_command", Args: map[string]interface{}{"command": "l"}}},
	}
	require.NoError(t, Save(local, nil))

	envelope := &TreeExport{
		Schema:  "smara.skill-tree/v1",
		Version: 1,
		Skills: []Skill{
			{
				Name: "dup", Description: "IMPORTED", Version: 1,
				Steps: []Step{{Tool: "run_command", Args: map[string]interface{}{"command": "i"}}},
			},
		},
	}

	result, err := ImportAll(nil, envelope, ConflictRename, false)
	require.NoError(t, err)
	require.Len(t, result.Renamed, 1)
	newName := result.Renamed["dup"]
	assert.Equal(t, "dup-2", newName)

	// Both skills exist now
	dup, err := Load("dup")
	require.NoError(t, err)
	assert.Equal(t, "LOCAL", dup.Description)

	dup2, err := Load("dup-2")
	require.NoError(t, err)
	assert.Equal(t, "IMPORTED", dup2.Description)
}

// TestImport_ModeOverwriteRecordsLineage verifies that overwriting an
// existing skill preserves the old version inside Lineage.
func TestImport_ModeOverwriteRecordsLineage(t *testing.T) {
	setupExportHome(t)

	local := &Skill{
		Name: "dup", Description: "LOCAL v1", Version: 1,
		Steps: []Step{{Tool: "run_command", Args: map[string]interface{}{"command": "l"}}},
	}
	require.NoError(t, Save(local, nil))

	envelope := &TreeExport{
		Schema:  "smara.skill-tree/v1",
		Version: 1,
		Skills: []Skill{
			{
				Name: "dup", Description: "IMPORTED v2", Version: 1,
				Steps: []Step{{Tool: "run_command", Args: map[string]interface{}{"command": "i"}}},
			},
		},
	}

	result, err := ImportAll(nil, envelope, ConflictOverwrite, false)
	require.NoError(t, err)
	assert.Contains(t, result.Overwritten, "dup")

	restored, err := Load("dup")
	require.NoError(t, err)
	assert.Equal(t, "IMPORTED v2", restored.Description)
	assert.Equal(t, 2, restored.Version, "overwrite bumps version")
	require.Len(t, restored.Lineage, 1)
	assert.Equal(t, "LOCAL v1", restored.Lineage[0].Description)
	assert.Equal(t, "import", restored.Lineage[0].RefinedFrom)
}

// TestImport_DryRunDoesNotWrite verifies dry-run actually leaves disk untouched.
func TestImport_DryRunDoesNotWrite(t *testing.T) {
	setupExportHome(t)

	envelope := &TreeExport{
		Schema:  "smara.skill-tree/v1",
		Version: 1,
		Skills: []Skill{
			{
				Name: "dry", Description: "hypothetical", Version: 1,
				Steps: []Step{{Tool: "run_command", Args: map[string]interface{}{"command": "d"}}},
			},
		},
	}
	result, err := ImportAll(nil, envelope, ConflictOverwrite, true)
	require.NoError(t, err)
	assert.Contains(t, result.Created, "dry")

	// Nothing should exist on disk
	_, err = Load("dry")
	assert.Error(t, err, "dry-run must not write to disk")
}

// TestExport_IncludesPatterns verifies that when a DB with auto-skill
// patterns is passed, those patterns survive an export.
func TestExport_IncludesPatterns(t *testing.T) {
	setupExportHome(t)
	db := openMemDB(t)

	// Seed one auto-skill pattern
	tr := ExecutionTrace{
		PromptText: "sample",
		Steps: []TraceStep{
			{Tool: "ssh_exec", Args: map[string]interface{}{"host": "x", "command": "a"}},
			{Tool: "ssh_exec", Args: map[string]interface{}{"host": "x", "command": "b"}},
		},
	}
	for i := 0; i < 3; i++ {
		_, _, _ = RecordTrace(db, tr, 3)
	}
	require.NoError(t, MarkPatternCaptured(db, tr.Fingerprint(), "captured-skill"))

	// Also save the skill itself so Load works on the other side
	require.NoError(t, Save(&Skill{
		Name: "captured-skill", Description: "x", Version: 1,
		Steps: []Step{{Tool: "ssh_exec", Args: map[string]interface{}{"host": "x"}}},
	}, nil))

	env, err := ExportAll(db, "")
	require.NoError(t, err)
	assert.Len(t, env.Patterns, 1)
	assert.Equal(t, 3, env.Patterns[0].Count)
	assert.Equal(t, "captured-skill", env.Patterns[0].CapturedSkill)

	// Round-trip the export and verify patterns restore in a fresh DB.
	var buf bytes.Buffer
	require.NoError(t, WriteExport(&buf, env))

	setupExportHome(t)
	db2 := openMemDB(t)
	parsed, err := ReadExport(&buf)
	require.NoError(t, err)
	result, err := ImportAll(db2, parsed, ConflictOverwrite, false)
	require.NoError(t, err)
	assert.Equal(t, 1, result.PatternsLoaded)

	var count int
	require.NoError(t, db2.QueryRow(`SELECT count FROM auto_skill_patterns WHERE fingerprint = ?`, tr.Fingerprint()).Scan(&count))
	assert.Equal(t, 3, count)
}

func TestImport_RejectsUnknownSchema(t *testing.T) {
	setupExportHome(t)
	envelope := &TreeExport{Schema: "some-other-format/v7", Version: 1}
	_, err := ImportAll(nil, envelope, ConflictOverwrite, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema tidak dikenal")
}

// TestReadExport_ParsesEnvelope sanity test for the decoder.
func TestReadExport_ParsesEnvelope(t *testing.T) {
	body := []byte(`{
		"schema": "smara.skill-tree/v1",
		"version": 1,
		"skills": [{"name": "x", "description": "y", "version": 1, "steps": [{"tool": "run_command", "args": {"command": "z"}}]}]
	}`)
	e, err := ReadExport(bytes.NewReader(body))
	require.NoError(t, err)
	assert.Equal(t, "smara.skill-tree/v1", e.Schema)
	assert.Len(t, e.Skills, 1)
	_ = json.Valid(body)
}

func TestValidateImportModeString(t *testing.T) {
	tests := map[string]ImportConflictMode{
		"":          ConflictOverwrite,
		"overwrite": ConflictOverwrite,
		"OVERWRITE": ConflictOverwrite,
		"  skip  ":  ConflictSkip,
		"rename":    ConflictRename,
	}
	for input, want := range tests {
		got, err := ValidateImportModeString(input)
		require.NoError(t, err)
		assert.Equal(t, want, got, "input=%q", input)
	}
	_, err := ValidateImportModeString("bogus")
	assert.Error(t, err)
}
