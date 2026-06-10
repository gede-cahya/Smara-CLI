package agent

import (
	"path/filepath"
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/gede-cahya/Smara-CLI/internal/skill"
	"github.com/stretchr/testify/require"
)

func TestCaptureSelfImprovementStoresDetectedCorrection(t *testing.T) {
	store, err := memory.NewSQLiteStore(filepath.Join(t.TempDir(), "memory.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	previousDB := BuiltinDB
	BuiltinDB = store.DB()
	t.Cleanup(func() { BuiltinDB = previousDB })

	supervisor := NewSupervisor(nil, store)
	supervisor.captureSelfImprovement("Mulai sekarang jangan hapus file konfigurasi.")

	var count int
	err = store.DB().QueryRow(`SELECT COUNT(*) FROM memories WHERE source = ?`, selfImprovementSource).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestBuiltinSkillRunRecordsExecutionHistory(t *testing.T) {
	setupSkillTestHome(t)
	store, err := memory.NewSQLiteStore(filepath.Join(t.TempDir(), "memory.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	previousDB := BuiltinDB
	BuiltinDB = store.DB()
	t.Cleanup(func() { BuiltinDB = previousDB })

	require.NoError(t, skill.Save(&skill.Skill{
		Name:        "tracked-runtime-skill",
		Description: "Tracked runtime skill",
		Version:     1,
		Steps: []skill.Step{{
			Tool: "run_command",
			Args: map[string]interface{}{"command": "printf tracked"},
		}},
	}, BuiltinDB))

	_, err = ExecuteBuiltinTool("skill_run", map[string]interface{}{
		"skill_name": "tracked-runtime-skill",
	}, nil)
	require.NoError(t, err)

	tracker, err := skill.NewExecutionTracker(BuiltinDB)
	require.NoError(t, err)
	total, success, _, _, err := tracker.GetStats("tracked-runtime-skill")
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, 1, success)
}
