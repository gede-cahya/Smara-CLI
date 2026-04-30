package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/stretchr/testify/assert"
)

func TestWorkflowResult_Struct(t *testing.T) {
	result := WorkflowResult{
		ProjectPath:  "/tmp/test",
		PRD:          "# PRD",
		Architecture: "# Arch",
		AgentOutputs: map[string][]agent.TaskResult{
			"frontend": {{TaskID: "t1", Output: "done"}},
		},
		QAResult:     QAResult{Status: "PASS"},
		FinalSummary: "Completed",
	}
	assert.Equal(t, "/tmp/test", result.ProjectPath)
	assert.Equal(t, "# PRD", result.PRD)
	assert.Equal(t, "# Arch", result.Architecture)
	assert.Len(t, result.AgentOutputs["frontend"], 1)
	assert.Equal(t, "PASS", result.QAResult.Status)
	assert.Equal(t, "Completed", result.FinalSummary)
}

func TestOrchestrator_ProgressCallback(t *testing.T) {
	orch := &Orchestrator{ProjectDir: "/tmp/test"}
	var lastStep, lastStatus string
	orch.OnProgress = func(step, status string) {
		lastStep = step
		lastStatus = status
	}

	// Simulate a callback
	if orch.OnProgress != nil {
		orch.OnProgress("test", "running")
	}
	assert.Equal(t, "test", lastStep)
	assert.Equal(t, "running", lastStatus)
}

func TestCanResume_True(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, ".smara"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, ".smara", "blueprint.json"), []byte(`{"project_name":"test"}`), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, ".smara", "state.json"), []byte(`{"project_dir":"`+tmpDir+`"}`), 0644)

	assert.True(t, CanResume(tmpDir))
}

func TestCanResume_False(t *testing.T) {
	tmpDir := t.TempDir()
	assert.False(t, CanResume(tmpDir))
}

func TestResumer_Status(t *testing.T) {
	resumer := &Resumer{
		Blueprint: Blueprint{
			Agents: []AgentSpec{
				{Role: "database", DependsOn: []string{}},
				{Role: "backend", DependsOn: []string{"database"}},
				{Role: "frontend", DependsOn: []string{"backend"}},
			},
		},
		State: &SharedState{
			CompletedWaves: []WaveStatus{
				{Index: 0, Roles: []string{"database"}, Completed: true},
			},
		},
	}

	status := resumer.Status()
	assert.Contains(t, status, "database")
	assert.Contains(t, status, "backend")
	assert.Contains(t, status, "frontend")
	assert.Contains(t, status, "[✓]")
	assert.Contains(t, status, "[⋯]")
}

func TestListResumableWorkflows(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid workflow
	wf1 := filepath.Join(tmpDir, "smara-workflow-123")
	_ = os.MkdirAll(filepath.Join(wf1, ".smara"), 0755)
	_ = os.WriteFile(filepath.Join(wf1, ".smara", "blueprint.json"), []byte(`{}`), 0644)
	_ = os.WriteFile(filepath.Join(wf1, ".smara", "state.json"), []byte(`{}`), 0644)

	// Create invalid (no state)
	wf2 := filepath.Join(tmpDir, "smara-workflow-456")
	_ = os.MkdirAll(filepath.Join(wf2, ".smara"), 0755)
	_ = os.WriteFile(filepath.Join(wf2, ".smara", "blueprint.json"), []byte(`{}`), 0644)

	// Create non-workflow dir
	_ = os.MkdirAll(filepath.Join(tmpDir, "other-dir"), 0755)

	results := ListResumableWorkflows(tmpDir)
	assert.Len(t, results, 1)
	assert.Contains(t, results, wf1)
}

func TestProgressReporter(t *testing.T) {
	pr := NewProgressReporter()
	pr.MarkInProgress("backend")
	assert.True(t, pr.InProgress["backend"])

	pr.MarkCompleted("backend")
	assert.True(t, pr.Completed["backend"])
	assert.False(t, pr.InProgress["backend"])

	pr.MarkFailed("frontend", "timeout")
	assert.Equal(t, "timeout", pr.Failed["frontend"])
	assert.False(t, pr.InProgress["frontend"])
}

func TestProgressReporter_IsDone(t *testing.T) {
	pr := NewProgressReporter()
	pr.MarkCompleted("a")
	pr.MarkCompleted("b")

	assert.True(t, pr.IsDone([]string{"a", "b"}))
	assert.False(t, pr.IsDone([]string{"a", "b", "c"}))
}

func TestProgressReporter_StatusBar(t *testing.T) {
	pr := NewProgressReporter()
	pr.MarkCompleted("database")
	pr.MarkInProgress("backend")
	pr.MarkFailed("frontend", "err")

	bar := pr.StatusBar([]string{"database", "backend", "frontend", "qa"})
	assert.Contains(t, bar, "DA[✓]")
	assert.Contains(t, bar, "BA[⋯]")
	assert.Contains(t, bar, "FR[✗]")
	assert.Contains(t, bar, "QA[ ]")
}

func TestSharedState_MarkWaveCompleted(t *testing.T) {
	state := NewSharedState("/tmp/test")
	state.MarkWaveCompleted(0, []string{"database"})
	state.MarkWaveCompleted(1, []string{"backend"})

	completed := state.GetCompletedWaveRoles()
	assert.True(t, completed["database"])
	assert.True(t, completed["backend"])
	assert.False(t, completed["frontend"])
}
