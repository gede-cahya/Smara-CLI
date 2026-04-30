package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gede-cahya/Smara-CLI/pkg/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanResume_MissingBlueprint(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, ".smara"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, ".smara", "state.json"), []byte(`{}`), 0644)
	assert.False(t, CanResume(tmpDir))
}

func TestCanResume_MissingState(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, ".smara"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, ".smara", "blueprint.json"), []byte(`{}`), 0644)
	assert.False(t, CanResume(tmpDir))
}

func TestCanResume_BothMissing(t *testing.T) {
	tmpDir := t.TempDir()
	assert.False(t, CanResume(tmpDir))
}

func TestNewResumer_Success(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, ".smara"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, ".smara", "blueprint.json"), []byte(`{
		"project_name": "Test",
		"description": "desc",
		"prd": "# PRD",
		"architecture": "# Arch",
		"agents": [{"role": "backend", "description": "API", "tasks": []}]
	}`), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, ".smara", "state.json"), []byte(`{
		"project_dir": "`+tmpDir+`",
		"artifacts": {"backend/task_0": "/tmp/api.go"},
		"contracts": {"api": {"users": "GET /users"}},
		"completed_waves": [{"index": 0, "roles": ["backend"], "completed": true}]
	}`), 0644)

	resumer, err := NewResumer(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, tmpDir, resumer.ProjectDir)
	assert.Equal(t, "Test", resumer.Blueprint.ProjectName)
	assert.Equal(t, "desc", resumer.Blueprint.Description)
	assert.Len(t, resumer.Blueprint.Agents, 1)
	assert.Equal(t, "backend", resumer.Blueprint.Agents[0].Role)
}

func TestNewResumer_InvalidBlueprint(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, ".smara"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, ".smara", "blueprint.json"), []byte(`not json`), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, ".smara", "state.json"), []byte(`{}`), 0644)

	_, err := NewResumer(tmpDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gagal parse blueprint")
}

func TestNewResumer_MissingState(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, ".smara"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, ".smara", "blueprint.json"), []byte(`{}`), 0644)

	// LoadSharedState creates new empty state when file missing, so this succeeds
	resumer, err := NewResumer(tmpDir)
	require.NoError(t, err)
	assert.NotNil(t, resumer.State)
	assert.Equal(t, tmpDir, resumer.State.ProjectDir)
}

func TestResumer_buildWaves(t *testing.T) {
	resumer := &Resumer{
		Blueprint: Blueprint{
			Agents: []AgentSpec{
				{Role: "database", DependsOn: []string{}},
				{Role: "backend", DependsOn: []string{"database"}},
				{Role: "frontend", DependsOn: []string{"backend"}},
			},
		},
	}
	waves := resumer.buildWaves()
	require.Len(t, waves, 3)
	assert.Equal(t, []string{"database"}, waves[0])
	assert.Equal(t, []string{"backend"}, waves[1])
	assert.Equal(t, []string{"frontend"}, waves[2])
}

func TestResumer_Status_AllDone(t *testing.T) {
	resumer := &Resumer{
		Blueprint: Blueprint{
			Agents: []AgentSpec{
				{Role: "database", DependsOn: []string{}},
				{Role: "backend", DependsOn: []string{"database"}},
			},
		},
		State: &SharedState{
			CompletedWaves: []WaveStatus{
				{Index: 0, Roles: []string{"database"}, Completed: true},
				{Index: 1, Roles: []string{"backend"}, Completed: true},
			},
		},
	}
	status := resumer.Status()
	assert.Contains(t, status, "[✓]")
	assert.NotContains(t, status, "[⋯]")
	assert.Contains(t, status, "database")
	assert.Contains(t, status, "backend")
}

func TestResumer_Status_Partial(t *testing.T) {
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
	assert.Contains(t, status, "[✓]")
	assert.Contains(t, status, "[⋯]")
}

func TestResumer_Status_NoneDone(t *testing.T) {
	resumer := &Resumer{
		Blueprint: Blueprint{
			Agents: []AgentSpec{
				{Role: "database", DependsOn: []string{}},
				{Role: "backend", DependsOn: []string{"database"}},
			},
		},
		State: &SharedState{
			CompletedWaves: []WaveStatus{},
		},
	}
	status := resumer.Status()
	assert.Contains(t, status, "[⋯]")
	assert.NotContains(t, status, "[✓]")
}

func TestResumer_runQAOnly_NoQA(t *testing.T) {
	resumer := &Resumer{
		Blueprint: Blueprint{
			ProjectName: "Test",
			Agents: []AgentSpec{
				{Role: "backend", DependsOn: []string{}},
			},
		},
		ProjectDir: t.TempDir(),
		State:      NewSharedState(t.TempDir()),
	}
	completed := map[string][]agent.TaskResult{
		"backend": {{TaskID: "t1", Output: "done"}},
	}

	// Need a non-nil runner for RunQA (even though no QA agent is defined)
	runner := NewRunner(resumer.Blueprint, nil, resumer.State)
	result, err := resumer.runQAOnly(nil, runner, completed, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "SKIP", result.QAResult.Status)
	assert.Contains(t, result.FinalSummary, "Test")
	assert.Contains(t, result.FinalSummary, "completed")
}

func TestResumeWorkflow_CannotResume(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := ResumeWorkflow(tmpDir, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tidak ada workflow yang bisa dilanjutkan")
}

func TestWorkflowStatus_CannotResume(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := WorkflowStatus(tmpDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tidak ada workflow aktif")
}

func TestWorkflowStatus_Success(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, ".smara"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, ".smara", "blueprint.json"), []byte(`{
		"project_name": "Test",
		"agents": [{"role": "backend", "description": "API", "tasks": []}]
	}`), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, ".smara", "state.json"), []byte(`{
		"project_dir": "`+tmpDir+`",
		"completed_waves": [{"index": 0, "roles": ["backend"], "completed": true}]
	}`), 0644)

	status, err := WorkflowStatus(tmpDir)
	require.NoError(t, err)
	assert.Contains(t, status, "backend")
	assert.Contains(t, status, "[✓]")
}

func TestListResumableWorkflows_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	results := ListResumableWorkflows(tmpDir)
	assert.Empty(t, results)
}

func TestListResumableWorkflows_InvalidDir(t *testing.T) {
	results := ListResumableWorkflows("/nonexistent/path/12345")
	assert.Empty(t, results)
}

func TestListResumableWorkflows_Mixed(t *testing.T) {
	tmpDir := t.TempDir()

	// Valid workflow
	wf1 := filepath.Join(tmpDir, "smara-workflow-123")
	_ = os.MkdirAll(filepath.Join(wf1, ".smara"), 0755)
	_ = os.WriteFile(filepath.Join(wf1, ".smara", "blueprint.json"), []byte(`{}`), 0644)
	_ = os.WriteFile(filepath.Join(wf1, ".smara", "state.json"), []byte(`{}`), 0644)

	// Missing state
	wf2 := filepath.Join(tmpDir, "smara-workflow-456")
	_ = os.MkdirAll(filepath.Join(wf2, ".smara"), 0755)
	_ = os.WriteFile(filepath.Join(wf2, ".smara", "blueprint.json"), []byte(`{}`), 0644)

	// Non-workflow dir
	_ = os.MkdirAll(filepath.Join(tmpDir, "other-project"), 0755)

	results := ListResumableWorkflows(tmpDir)
	assert.Len(t, results, 1)
	assert.Contains(t, results, wf1)
}

func TestListResumableWorkflows_NonWorkflowPrefix(t *testing.T) {
	tmpDir := t.TempDir()

	// Has both files but wrong prefix
	wf := filepath.Join(tmpDir, "my-project")
	_ = os.MkdirAll(filepath.Join(wf, ".smara"), 0755)
	_ = os.WriteFile(filepath.Join(wf, ".smara", "blueprint.json"), []byte(`{}`), 0644)
	_ = os.WriteFile(filepath.Join(wf, ".smara", "state.json"), []byte(`{}`), 0644)

	results := ListResumableWorkflows(tmpDir)
	assert.Empty(t, results)
}

func TestProgressReporter_Concurrent(t *testing.T) {
	pr := NewProgressReporter()

	// Simulate concurrent operations
	pr.MarkInProgress("backend")
	pr.MarkInProgress("frontend")
	pr.MarkCompleted("backend")
	pr.MarkFailed("frontend", "timeout")

	assert.True(t, pr.Completed["backend"])
	assert.False(t, pr.InProgress["backend"])
	assert.Equal(t, "timeout", pr.Failed["frontend"])
	assert.False(t, pr.InProgress["frontend"])
}

func TestProgressReporter_StatusBar_ShortRole(t *testing.T) {
	pr := NewProgressReporter()
	pr.MarkCompleted("a")

	bar := pr.StatusBar([]string{"a"})
	assert.Equal(t, "A[✓]", bar)
}

func TestProgressReporter_StatusBar_AllStates(t *testing.T) {
	pr := NewProgressReporter()
	pr.MarkCompleted("done")
	pr.MarkInProgress("running")
	pr.MarkFailed("broken", "err")
	// "pending" tidak dimark = pending

	bar := pr.StatusBar([]string{"done", "running", "broken", "pending"})
	assert.Contains(t, bar, "DO[✓]")
	assert.Contains(t, bar, "RU[⋯]")
	assert.Contains(t, bar, "BR[✗]")
	assert.Contains(t, bar, "PE[ ]")
}

func TestMin_Helper(t *testing.T) {
	assert.Equal(t, 1, min(1, 2))
	assert.Equal(t, 1, min(1, 1))
	assert.Equal(t, 0, min(0, 5))
	assert.Equal(t, -1, min(-1, 0))
}

func TestResumer_StateArtifactsReconstruction(t *testing.T) {
	state := NewSharedState(t.TempDir())
	state.WriteArtifact("backend", "task_0", "API output")
	state.WriteArtifact("backend", "task_1", "More output")
	state.WriteArtifact("frontend", "task_0", "UI output")

	completed := make(map[string][]agent.TaskResult)
	completedRoles := map[string]bool{"backend": true, "frontend": true}

	for role := range completedRoles {
		var results []agent.TaskResult
		for key, output := range state.Artifacts {
			if key == role+"/task_0" || key == role+"/task_1" {
				results = append(results, agent.TaskResult{
					TaskID: key,
					Status: agent.TaskCompleted,
					Output: output,
				})
			}
		}
		if len(results) > 0 {
			completed[role] = results
		}
	}

	assert.Len(t, completed["backend"], 2)
	assert.Len(t, completed["frontend"], 1)
	assert.Equal(t, "API output", completed["backend"][0].Output)
}
