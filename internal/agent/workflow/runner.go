package workflow

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
)

// Runner executes blueprint tasks based on dependencies.
type Runner struct {
	Blueprint      Blueprint
	Workers        map[string]*agent.Worker // role → worker
	SharedState    *SharedState
	MaxConcurrency int
	// Serial forces one role per wave. Workflow mode uses this to avoid
	// spawning multiple worker roles at once.
	Serial bool

	// Callbacks for TUI progress
	OnWaveStart    func(wave int, roles []string)
	OnWaveComplete func(wave int, results map[string][]agent.TaskResult)
	OnTaskStart    func(role string, task agent.Task)
	OnTaskStream   func(role, taskID, chunk string, isThinking bool)
	OnTaskComplete func(role, taskID string, result agent.TaskResult)
}

// NewRunner creates a DAG runner for a blueprint.
func NewRunner(bp Blueprint, workers map[string]*agent.Worker, state *SharedState) *Runner {
	return &Runner{
		Blueprint:      bp,
		Workers:        workers,
		SharedState:    state,
		MaxConcurrency: 4,
	}
}

// Run executes the blueprint in dependency-resolved steps/waves.
func (r *Runner) Run(ctx context.Context, supervisor *agent.Supervisor) (map[string][]agent.TaskResult, error) {
	// Build dependency graph
	waves, err := r.BuildWaves()
	if err != nil {
		return nil, err
	}
	completed := make(map[string][]agent.TaskResult)

	for waveIdx, wave := range waves {
		unit := "WAVE"
		if r.Serial {
			unit = "STEP"
		}
		if r.OnWaveStart != nil {
			r.OnWaveStart(waveIdx, wave)
		}

		waveResults := r.runWave(ctx, wave, completed, supervisor)
		for role, results := range waveResults {
			completed[role] = append(completed[role], results...)
		}

		if r.OnWaveComplete != nil {
			r.OnWaveComplete(waveIdx, waveResults)
		}

		// Check for failures
		for _, results := range waveResults {
			for _, res := range results {
				if res.Status == agent.TaskFailed {
					return completed, fmt.Errorf("%s %d failed: task %s error: %s", strings.ToLower(unit), waveIdx, res.TaskID, res.Error)
				}
			}
		}

		// Write outputs to shared state
		for role, results := range waveResults {
			for i, res := range results {
				r.SharedState.WriteArtifact(role, fmt.Sprintf("task_%d", i), res.Output)
			}
		}

		// Mark wave as completed for resume support
		r.SharedState.MarkWaveCompleted(waveIdx, wave)
		_ = r.SharedState.Save()
	}

	return completed, nil
}

// BuildWaves groups agents into deterministic dependency waves based on DependsOn.
func (r *Runner) BuildWaves() ([][]string, error) {
	deps := make(map[string]map[string]bool)
	roleSet := make(map[string]bool)
	for _, spec := range r.Blueprint.Agents {
		if strings.TrimSpace(spec.Role) == "" {
			return nil, fmt.Errorf("agent role cannot be empty")
		}
		if roleSet[spec.Role] {
			return nil, fmt.Errorf("duplicate agent role %q", spec.Role)
		}
		roleSet[spec.Role] = true
		depMap := make(map[string]bool)
		for _, d := range spec.DependsOn {
			if strings.TrimSpace(d) == "" {
				continue
			}
			depMap[d] = true
		}
		deps[spec.Role] = depMap
	}

	for role, depMap := range deps {
		for dep := range depMap {
			if !roleSet[dep] {
				return nil, fmt.Errorf("agent %q depends on unknown role %q", role, dep)
			}
			if dep == role {
				return nil, fmt.Errorf("agent %q cannot depend on itself", role)
			}
		}
	}

	var waves [][]string
	completed := make(map[string]bool)

	for len(completed) < len(roleSet) {
		var wave []string
		roles := sortedKeys(roleSet)
		for _, role := range roles {
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
			remaining := make([]string, 0, len(roleSet)-len(completed))
			for role := range roleSet {
				if !completed[role] {
					remaining = append(remaining, role)
				}
			}
			sort.Strings(remaining)
			return nil, fmt.Errorf("circular dependency detected among roles: %s", strings.Join(remaining, ", "))
		}

		for _, role := range wave {
			completed[role] = true
		}
		waves = append(waves, wave)
	}

	if r.Serial {
		var serialWaves [][]string
		for _, wave := range waves {
			for _, role := range wave {
				serialWaves = append(serialWaves, []string{role})
			}
		}
		return serialWaves, nil
	}

	return waves, nil
}

// buildWaves preserves the historical test/helper API. It returns nil when the blueprint is invalid.
func (r *Runner) buildWaves() [][]string {
	waves, err := r.BuildWaves()
	if err != nil {
		return nil
	}
	return waves
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// runWave executes roles in the current dependency group.
func (r *Runner) runWave(ctx context.Context, roles []string, completed map[string][]agent.TaskResult, supervisor *agent.Supervisor) map[string][]agent.TaskResult {
	results := make(map[string][]agent.TaskResult)
	if len(roles) == 1 {
		role := roles[0]
		results[role] = r.runRole(ctx, role, completed, supervisor)
		return results
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	// Semaphore for max concurrency
	sem := make(chan struct{}, r.MaxConcurrency)

	for _, role := range roles {
		wg.Add(1)
		go func(role string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			roleResults := r.runRole(ctx, role, completed, supervisor)
			mu.Lock()
			results[role] = roleResults
			mu.Unlock()
		}(role)
	}

	wg.Wait()
	return results
}

func (r *Runner) runRole(ctx context.Context, role string, completed map[string][]agent.TaskResult, supervisor *agent.Supervisor) []agent.TaskResult {
	worker, ok := r.Workers[role]
	if !ok {
		return []agent.TaskResult{{
			TaskID: role + "-missing",
			Status: agent.TaskFailed,
			Error:  fmt.Sprintf("worker for role '%s' not found", role),
		}}
	}

	var spec *AgentSpec
	for i := range r.Blueprint.Agents {
		if r.Blueprint.Agents[i].Role == role {
			spec = &r.Blueprint.Agents[i]
			break
		}
	}
	if spec == nil {
		return []agent.TaskResult{{
			TaskID: role + "-spec",
			Status: agent.TaskFailed,
			Error:  fmt.Sprintf("agent spec for role '%s' not found", role),
		}}
	}

	tasks := BuildRoleTasks(*spec, r.SharedState)
	tasks = injectDependencies(tasks, completed)

	var roleResults []agent.TaskResult
	for _, task := range tasks {
		// Add small delay between tasks for rate limiting.
		time.Sleep(100 * time.Millisecond)

		if r.OnTaskStart != nil {
			r.OnTaskStart(role, task)
		}

		result := worker.ExecuteWithCallback(ctx, task, &agent.WorkerCallback{
			OnStream: func(chunk string, isThinking bool) {
				if r.OnTaskStream != nil {
					r.OnTaskStream(role, task.ID, chunk, isThinking)
				}
			},
		})

		roleResults = append(roleResults, result)

		if r.OnTaskComplete != nil {
			r.OnTaskComplete(role, task.ID, result)
		}

		if result.Status == agent.TaskFailed {
			break
		}
	}

	return roleResults
}

// RunQA spawns the QA agent after all waves complete.
func (r *Runner) RunQA(ctx context.Context, bp Blueprint, allResults map[string][]agent.TaskResult, supervisor *agent.Supervisor) QAResult {
	// Find QA worker
	qaWorker, ok := r.Workers["qa"]
	if !ok {
		// Auto-create QA worker if not in blueprint
		return QAResult{Status: "SKIP", Report: "No QA agent defined in blueprint"}
	}

	// Build QA prompt
	var builder string
	builder += fmt.Sprintf("# PRD\n%s\n\n", bp.PRD)
	builder += fmt.Sprintf("# Architecture\n%s\n\n", bp.Architecture)
	builder += "# Agent Outputs\n"

	for role, results := range allResults {
		builder += fmt.Sprintf("\n## %s\n", strings.ToUpper(role))
		for _, res := range results {
			builder += fmt.Sprintf("- %s: %s\n", res.TaskID, strings.TrimSpace(res.Output))
		}
	}

	contractsJSON, _ := r.SharedState.GetContractsJSON()
	builder += fmt.Sprintf("\n# Shared Contracts\n%s\n", contractsJSON)

	qaTask := agent.Task{
		ID:          "qa-review",
		Description: builder,
	}

	result := qaWorker.Execute(ctx, qaTask)
	return ParseQAResult(result.Output)
}
