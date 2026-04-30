package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gede-cahya/Smara-CLI/pkg/agent"
	"github.com/gede-cahya/Smara-CLI/pkg/llm"
)

// Resumer handles resuming interrupted workflows from disk state.
type Resumer struct {
	ProjectDir string
	Blueprint  Blueprint
	State      *SharedState
}

// CanResume checks if a project directory contains a resumable workflow.
func CanResume(projectDir string) bool {
	bpPath := filepath.Join(projectDir, ".smara", "blueprint.json")
	statePath := filepath.Join(projectDir, ".smara", "state.json")
	_, errBP := os.Stat(bpPath)
	_, errState := os.Stat(statePath)
	return errBP == nil && errState == nil
}

// NewResumer creates a resumer for a project directory.
func NewResumer(projectDir string) (*Resumer, error) {
	state, err := LoadSharedState(projectDir)
	if err != nil {
		return nil, fmt.Errorf("gagal load state: %w", err)
	}

	bpPath := filepath.Join(projectDir, ".smara", "blueprint.json")
	bpData, err := os.ReadFile(bpPath)
	if err != nil {
		return nil, fmt.Errorf("blueprint tidak ditemukan: %w", err)
	}

	var bp Blueprint
	if err := json.Unmarshal(bpData, &bp); err != nil {
		return nil, fmt.Errorf("gagal parse blueprint: %w", err)
	}

	return &Resumer{
		ProjectDir: projectDir,
		Blueprint:  bp,
		State:      state,
	}, nil
}

// Resume continues execution from the last completed wave.
func (r *Resumer) Resume(ctx context.Context, supervisor *agent.Supervisor, provider llm.Provider) (*WorkflowResult, error) {
	completedRoles := r.State.GetCompletedWaveRoles()

	// Build all waves
	allWaves := r.buildWaves()
	if len(allWaves) == 0 {
		return nil, fmt.Errorf("tidak ada waves di blueprint")
	}

	// Determine which waves are done and which need execution
	var resumeFrom int
	for i, wave := range allWaves {
		allDone := true
		for _, role := range wave {
			if !completedRoles[role] {
				allDone = false
				break
			}
		}
		if allDone {
			resumeFrom = i + 1
		}
	}

	// Create workers
	workers, err := CreateWorkersFromBlueprint(r.Blueprint, supervisor)
	if err != nil {
		return nil, fmt.Errorf("worker creation failed: %w", err)
	}

	workerMap := make(map[string]*agent.Worker)
	for i, spec := range r.Blueprint.Agents {
		if i < len(workers) {
			workerMap[spec.Role] = workers[i]
		}
	}

	// Reconstruct completed results from state artifacts
	completed := make(map[string][]agent.TaskResult)
	for role := range completedRoles {
		// Load prior results from artifacts
		var results []agent.TaskResult
		for key, output := range r.State.Artifacts {
			if strings.HasPrefix(key, role+"/") {
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

	// Create runner for remaining waves
	runner := NewRunner(r.Blueprint, workerMap, r.State)
	remainingWaves := allWaves[resumeFrom:]

	if len(remainingWaves) == 0 {
		// All waves done — run QA only
		return r.runQAOnly(ctx, runner, completed, supervisor, provider)
	}

	// Run remaining waves
	for waveIdx, wave := range remainingWaves {
		actualIdx := resumeFrom + waveIdx
		if runner.OnWaveStart != nil {
			runner.OnWaveStart(actualIdx, wave)
		}

		waveResults := runner.runWave(ctx, wave, completed, supervisor)
		for role, results := range waveResults {
			completed[role] = append(completed[role], results...)
		}

		if runner.OnWaveComplete != nil {
			runner.OnWaveComplete(actualIdx, waveResults)
		}

		// Check failures
		for _, results := range waveResults {
			for _, res := range results {
				if res.Status == agent.TaskFailed {
					return nil, fmt.Errorf("wave %d failed: task %s error: %s", actualIdx, res.TaskID, res.Error)
				}
			}
		}

		for role, results := range waveResults {
			for i, res := range results {
				runner.SharedState.WriteArtifact(role, fmt.Sprintf("task_%d", i), res.Output)
			}
		}
		runner.SharedState.MarkWaveCompleted(actualIdx, wave)
		_ = runner.SharedState.Save()
	}

	return r.runQAOnly(ctx, runner, completed, supervisor, provider)
}

func (r *Resumer) runQAOnly(ctx context.Context, runner *Runner, completed map[string][]agent.TaskResult, supervisor *agent.Supervisor, provider llm.Provider) (*WorkflowResult, error) {
	qaResult := runner.RunQA(ctx, r.Blueprint, completed, supervisor)

	result := &WorkflowResult{
		ProjectPath:  r.ProjectDir,
		PRD:          r.Blueprint.PRD,
		Architecture: r.Blueprint.Architecture,
		AgentOutputs: completed,
		QAResult:     qaResult,
	}

	if qaResult.Status == "PASS" {
		result.FinalSummary = fmt.Sprintf("Workflow '%s' resumed and completed. QA: PASS.", r.Blueprint.ProjectName)
	} else {
		result.FinalSummary = fmt.Sprintf("Workflow '%s' resumed and completed. QA: %s. Issues: %d.",
			r.Blueprint.ProjectName, qaResult.Status, len(qaResult.Issues))
	}

	_ = r.State.Save()
	return result, nil
}

// Status returns a human-readable progress string for display.
func (r *Resumer) Status() string {
	completedRoles := r.State.GetCompletedWaveRoles()
	allWaves := r.buildWaves()

	var parts []string
	for i, wave := range allWaves {
		status := "[✓]"
		for _, role := range wave {
			if !completedRoles[role] {
				status = "[⋯]"
				break
			}
		}
		parts = append(parts, fmt.Sprintf("Wave %d %s %v", i, status, wave))
	}

	return strings.Join(parts, " → ")
}

// buildWaves is a standalone wave builder for the resumer.
func (r *Resumer) buildWaves() [][]string {
	deps := make(map[string]map[string]bool)
	roleSet := make(map[string]bool)
	for _, spec := range r.Blueprint.Agents {
		roleSet[spec.Role] = true
		depMap := make(map[string]bool)
		for _, d := range spec.DependsOn {
			depMap[d] = true
		}
		deps[spec.Role] = depMap
	}

	var waves [][]string
	completed := make(map[string]bool)

	for len(completed) < len(roleSet) {
		var wave []string
		for role := range roleSet {
			if completed[role] {
				continue
			}
			ready := true
			for dep := range deps[role] {
				if !completed[dep] {
					ready = false
					break
				}
			}
			if ready {
				wave = append(wave, role)
			}
		}
		if len(wave) == 0 {
			for role := range roleSet {
				if !completed[role] {
					wave = append(wave, role)
				}
			}
		}
		for _, role := range wave {
			completed[role] = true
		}
		waves = append(waves, wave)
	}
	return waves
}

// ResumeWorkflow is the public API for resuming a workflow.
func ResumeWorkflow(projectDir string, supervisor *agent.Supervisor, provider llm.Provider) (*WorkflowResult, error) {
	if !CanResume(projectDir) {
		return nil, fmt.Errorf("tidak ada workflow yang bisa dilanjutkan di %s", projectDir)
	}

	resumer, err := NewResumer(projectDir)
	if err != nil {
		return nil, err
	}

	return resumer.Resume(context.Background(), supervisor, provider)
}

// WorkflowStatus returns the progress status of a workflow.
func WorkflowStatus(projectDir string) (string, error) {
	if !CanResume(projectDir) {
		return "", fmt.Errorf("tidak ada workflow aktif di %s", projectDir)
	}

	resumer, err := NewResumer(projectDir)
	if err != nil {
		return "", err
	}

	return resumer.Status(), nil
}

// ListResumableWorkflows scans a directory for workflow projects that can be resumed.
func ListResumableWorkflows(baseDir string) []string {
	var results []string
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return results
	}

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "smara-workflow-") {
			continue
		}
		projectDir := filepath.Join(baseDir, entry.Name())
		if CanResume(projectDir) {
			results = append(results, projectDir)
		}
	}

	return results
}

// ProgressReporter provides real-time progress during workflow execution.
type ProgressReporter struct {
	mu         sync.RWMutex
	Completed  map[string]bool   `json:"completed"`
	InProgress map[string]bool   `json:"in_progress"`
	Failed     map[string]string `json:"failed"`
	WaveIndex  int               `json:"wave_index"`
}

// NewProgressReporter creates a new progress tracker.
func NewProgressReporter() *ProgressReporter {
	return &ProgressReporter{
		Completed:  make(map[string]bool),
		InProgress: make(map[string]bool),
		Failed:     make(map[string]string),
	}
}

// MarkInProgress marks a role as currently executing.
func (p *ProgressReporter) MarkInProgress(role string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.InProgress[role] = true
}

// MarkCompleted marks a role as finished successfully.
func (p *ProgressReporter) MarkCompleted(role string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.InProgress, role)
	p.Completed[role] = true
}

// MarkFailed marks a role as failed with an error.
func (p *ProgressReporter) MarkFailed(role, err string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.InProgress, role)
	p.Failed[role] = err
}

// IsDone returns true if all given roles are completed.
func (p *ProgressReporter) IsDone(roles []string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, r := range roles {
		if !p.Completed[r] {
			return false
		}
	}
	return true
}

// StatusBar returns a compact status string like "DB[✓] BE[⋯] FE[ ] QA[ ]"
func (p *ProgressReporter) StatusBar(roles []string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var parts []string
	for _, r := range roles {
		symbol := "[ ]"
		if p.Completed[r] {
			symbol = "[✓]"
		} else if p.InProgress[r] {
			symbol = "[⋯]"
		} else if _, failed := p.Failed[r]; failed {
			symbol = "[✗]"
		}
		parts = append(parts, fmt.Sprintf("%s%s", strings.ToUpper(r[:min(2, len(r))]), symbol))
	}
	return strings.Join(parts, " ")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
