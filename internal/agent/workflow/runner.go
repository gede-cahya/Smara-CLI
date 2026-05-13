package workflow

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
)

// Runner executes blueprint tasks in parallel waves based on dependencies.
type Runner struct {
	Blueprint      Blueprint
	Workers        map[string]*agent.Worker // role → worker
	SharedState    *SharedState
	MaxConcurrency int

	// Callbacks for TUI progress
	OnWaveStart  func(wave int, roles []string)
	OnWaveComplete func(wave int, results map[string][]agent.TaskResult)
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

// Run executes the blueprint in dependency-resolved waves.
func (r *Runner) Run(ctx context.Context, supervisor *agent.Supervisor) (map[string][]agent.TaskResult, error) {
	// Build dependency graph
	waves := r.buildWaves()
	completed := make(map[string][]agent.TaskResult)

	totalWaves := len(waves)
	for waveIdx, wave := range waves {
		log.Printf("[workflow] === WAVE %d/%d START (%d roles: %s) ===", waveIdx+1, totalWaves, len(wave), strings.Join(wave, ", "))
		if r.OnWaveStart != nil {
			r.OnWaveStart(waveIdx, wave)
		}

		waveResults := r.runWave(ctx, wave, completed, supervisor)
		for role, results := range waveResults {
			completed[role] = append(completed[role], results...)
		}

		log.Printf("[workflow] === WAVE %d/%d COMPLETE ===", waveIdx+1, totalWaves)
		if r.OnWaveComplete != nil {
			r.OnWaveComplete(waveIdx, waveResults)
		}

		// Check for failures
		for _, results := range waveResults {
			for _, res := range results {
				if res.Status == agent.TaskFailed {
					return completed, fmt.Errorf("wave %d failed: task %s error: %s", waveIdx, res.TaskID, res.Error)
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

// buildWaves groups agents into parallel waves based on DependsOn.
func (r *Runner) buildWaves() [][]string {
	// Map role → remaining dependencies
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
			// Check if all dependencies are completed
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
			// Circular dependency or missing dependency — force remaining
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

// runWave executes all roles in a wave concurrently.
func (r *Runner) runWave(ctx context.Context, roles []string, completed map[string][]agent.TaskResult, supervisor *agent.Supervisor) map[string][]agent.TaskResult {
	var mu sync.Mutex
	results := make(map[string][]agent.TaskResult)
	var wg sync.WaitGroup

	// Semaphore for max concurrency
	sem := make(chan struct{}, r.MaxConcurrency)

	for _, role := range roles {
		wg.Add(1)
		go func(role string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			log.Printf("[workflow] Role '%s' starting execution", role)
			startTime := time.Now()

			worker, ok := r.Workers[role]
			if !ok {
				mu.Lock()
				results[role] = append(results[role], agent.TaskResult{
					TaskID: role + "-missing",
					Status: agent.TaskFailed,
					Error:  fmt.Sprintf("worker for role '%s' not found", role),
				})
				mu.Unlock()
				log.Printf("[workflow] Role '%s' FAILED: worker not found", role)
				return
			}

			// Find agent spec for this role
			var spec *AgentSpec
			for i := range r.Blueprint.Agents {
				if r.Blueprint.Agents[i].Role == role {
					spec = &r.Blueprint.Agents[i]
					break
				}
			}
			if spec == nil {
				mu.Lock()
				results[role] = append(results[role], agent.TaskResult{
					TaskID: role + "-spec",
					Status: agent.TaskFailed,
					Error:  fmt.Sprintf("agent spec for role '%s' not found", role),
				})
				mu.Unlock()
				log.Printf("[workflow] Role '%s' FAILED: spec not found", role)
				return
			}

			tasks := BuildRoleTasks(*spec, r.SharedState)
			tasks = injectDependencies(tasks, completed)

			log.Printf("[workflow] Role '%s' executing %d task(s)", role, len(tasks))
			var roleResults []agent.TaskResult
			for taskIdx, task := range tasks {
				// Add small delay between tasks for rate limiting
				time.Sleep(100 * time.Millisecond)

				log.Printf("[workflow] Role '%s' task %d/%d (%s) starting...", role, taskIdx+1, len(tasks), task.ID)
				taskStart := time.Now()
				result := worker.Execute(ctx, task)
				duration := time.Since(taskStart)

				if result.Status == agent.TaskCompleted {
					log.Printf("[workflow] Role '%s' task %d/%d (%s) COMPLETE (%v)", role, taskIdx+1, len(tasks), task.ID, duration)
				} else {
					log.Printf("[workflow] Role '%s' task %d/%d (%s) FAILED: %s (%v)", role, taskIdx+1, len(tasks), task.ID, result.Error, duration)
				}

				roleResults = append(roleResults, result)

				if r.OnTaskComplete != nil {
					r.OnTaskComplete(role, task.ID, result)
				}

				if result.Status == agent.TaskFailed {
					// Fail-fast: stop this role's tasks
					break
				}
			}

			mu.Lock()
			results[role] = roleResults
			mu.Unlock()
			log.Printf("[workflow] Role '%s' FINISHED (%d/%d tasks, %v)", role, len(roleResults), len(tasks), time.Since(startTime))
		}(role)
	}

	wg.Wait()
	return results
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
