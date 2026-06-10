package skill

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTracker(t *testing.T) *ExecutionTracker {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	tracker, err := NewExecutionTracker(db)
	require.NoError(t, err)
	return tracker
}

func TestExecutionTracker_LogRun(t *testing.T) {
	tr := setupTracker(t)

	start := time.Now().Add(-100 * time.Millisecond)
	result := &RunResult{SkillName: "test-skill", Success: true, Summary: "ok"}
	err := tr.LogRun("test-skill", "run-1", "manual", "default", "rush", result, start)
	require.NoError(t, err)

	total, success, avgMs, lastRun, err := tr.GetStats("test-skill")
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, 1, success)
	assert.True(t, avgMs > 0)
	require.NotNil(t, lastRun)
	assert.False(t, lastRun.IsZero())
}

func TestExecutionTracker_GetTimeline(t *testing.T) {
	tr := setupTracker(t)

	for i := 0; i < 5; i++ {
		start := time.Now().Add(-time.Duration(i) * time.Second)
		result := &RunResult{SkillName: "skill-a", Success: i%2 == 0, Summary: "run"}
		err := tr.LogRun("skill-a", "run-id", "auto", "ws1", "rush", result, start)
		require.NoError(t, err)
	}

	items, err := tr.GetTimeline("skill-a", 3)
	require.NoError(t, err)
	assert.Len(t, items, 3)
}

func TestExecutionTracker_GetTopSkills(t *testing.T) {
	tr := setupTracker(t)

	for i := 0; i < 10; i++ {
		start := time.Now().Add(-time.Duration(i) * time.Second)
		result := &RunResult{SkillName: "popular", Success: true, Summary: "ok"}
		_ = tr.LogRun("popular", "r", "web", "ws", "ask", result, start)
	}
	for i := 0; i < 2; i++ {
		start := time.Now().Add(-time.Duration(i) * time.Second)
		result := &RunResult{SkillName: "rare", Success: true, Summary: "ok"}
		_ = tr.LogRun("rare", "r", "web", "ws", "ask", result, start)
	}

	top, err := tr.GetTopSkills(5)
	require.NoError(t, err)
	require.NotEmpty(t, top)
	assert.Equal(t, "popular", top[0].Name)
	assert.Equal(t, 10, top[0].RunCount)
}

func TestExecutionTracker_GetSkillsNeedingAttention(t *testing.T) {
	tr := setupTracker(t)

	// 5 runs, 1 success = 20% success rate
	for i := 0; i < 5; i++ {
		start := time.Now().Add(-time.Duration(i) * time.Second)
		result := &RunResult{SkillName: "failing", Success: i == 0, Summary: "err"}
		_ = tr.LogRun("failing", "r", "web", "ws", "ask", result, start)
	}

	struggling, err := tr.GetSkillsNeedingAttention()
	require.NoError(t, err)
	require.Len(t, struggling, 1)
	assert.Equal(t, "failing", struggling[0].Name)
	assert.InDelta(t, 20.0, struggling[0].SuccessRate, 0.01)
}

func TestExecutionTracker_GlobalAnalytics(t *testing.T) {
	tr := setupTracker(t)

	start := time.Now()
	for i := 0; i < 3; i++ {
		result := &RunResult{SkillName: "s1", Success: true, Summary: "ok"}
		_ = tr.LogRun("s1", "r", "web", "ws", "ask", result, start)
	}
	for i := 0; i < 2; i++ {
		result := &RunResult{SkillName: "s2", Success: false, Summary: "fail"}
		_ = tr.LogRun("s2", "r", "web", "ws", "ask", result, start)
	}

	analytics, err := tr.GlobalAnalytics()
	require.NoError(t, err)
	assert.Equal(t, 5, analytics["total_runs"])
	assert.Equal(t, 3, analytics["successful_runs"])
	assert.Len(t, analytics["top_skills"], 2)
	// struggling may be nil or empty when no skills meet the threshold
	_, hasStruggling := analytics["struggling"]
	assert.True(t, hasStruggling)
}

func TestExecutionTracker_RecordImprovement(t *testing.T) {
	tr := setupTracker(t)

	imp := SkillImprovement{
		SkillName:   "test",
		Version:     2,
		TriggeredAt: time.Now(),
		Trigger:     "auto-refine",
		Applied:     true,
	}
	err := tr.RecordImprovement(imp)
	require.NoError(t, err)

	items, err := tr.GetImprovements("test", 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "auto-refine", items[0].Trigger)
	assert.True(t, items[0].Applied)
	assert.Equal(t, 2, items[0].Version)
}
