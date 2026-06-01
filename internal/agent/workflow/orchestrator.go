package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
)

// Orchestrator runs the full workflow: blueprint → workers → QA → result.
type Orchestrator struct {
	Supervisor       *agent.Supervisor
	Provider         llm.Provider
	MCPInfo          map[string]agent.MCPServerInfo
	ProjectDir       string
	SharedState      *SharedState
	Blueprint        Blueprint
	Result           *WorkflowResult
	OnProgress       func(step, status string) // Callback for TUI
	OnBlueprintReady func(Blueprint, [][]string)
	OnRoleStart      func(role string)
	OnTaskComplete   func(role, taskID string, result agent.TaskResult)
}

// NewOrchestrator creates a workflow orchestrator.
func NewOrchestrator(supervisor *agent.Supervisor, provider llm.Provider, projectDir string) *Orchestrator {
	return &Orchestrator{
		Supervisor:  supervisor,
		Provider:    provider,
		MCPInfo:     supervisor.GetMCPInfo(),
		ProjectDir:  projectDir,
		SharedState: NewSharedState(projectDir),
	}
}

// Run executes the full workflow pipeline.
func (o *Orchestrator) Run(ctx context.Context, prompt string) (*WorkflowResult, error) {
	// 1. Generate Blueprint
	if o.OnProgress != nil {
		o.OnProgress("orchestrator", "generating blueprint")
	}

	bp, err := o.generateBlueprint(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("blueprint generation failed: %w", err)
	}
	o.Blueprint = bp

	// Save blueprint to project dir
	if err := o.saveBlueprint(); err != nil {
		// Non-fatal
		_ = err
	}

	// 2. Create workers from blueprint
	if o.OnProgress != nil {
		o.OnProgress("factory", "spawning workers")
	}

	workers, err := o.createWorkers(bp)
	if err != nil {
		return nil, fmt.Errorf("worker creation failed: %w", err)
	}

	// 3. Run DAG execution
	if o.OnProgress != nil {
		o.OnProgress("runner", "executing steps")
	}

	workerMap := make(map[string]*agent.Worker)
	for i, spec := range bp.Agents {
		if i < len(workers) {
			workerMap[spec.Role] = workers[i]
		}
	}

	runner := NewRunner(bp, workerMap, o.SharedState)
	if o.Supervisor == nil || o.Supervisor.GetMode() != agent.ModeParallel {
		// Only ModeParallel may execute dependency-safe roles in parallel.
		// Workflow, Ask, Plan, Rush, Test, and Image stay serial.
		runner.Serial = true
	}
	executionWaves := [][]string(nil)
	if waves, err := runner.BuildWaves(); err == nil {
		executionWaves = waves
		if o.OnBlueprintReady != nil {
			o.OnBlueprintReady(bp, waves)
		}
	} else {
		return nil, fmt.Errorf("dependency wave build failed: %w", err)
	}
	runner.OnWaveStart = func(wave int, roles []string) {
		if o.OnProgress != nil {
			unit := "wave"
			if runner.Serial {
				unit = "step"
			}
			o.OnProgress("runner", fmt.Sprintf("%s %d: %s", unit, wave, roles))
		}
		if o.OnRoleStart != nil {
			for _, role := range roles {
				o.OnRoleStart(role)
			}
		}
	}
	runner.OnWaveComplete = func(wave int, results map[string][]agent.TaskResult) {
		if o.OnProgress != nil {
			unit := "wave"
			if runner.Serial {
				unit = "step"
			}
			o.OnProgress(fmt.Sprintf("%s-%d", unit, wave), "complete")
		}
	}
	runner.OnTaskComplete = func(role, taskID string, result agent.TaskResult) {
		if o.OnTaskComplete != nil {
			o.OnTaskComplete(role, taskID, result)
		}
	}

	allResults, err := runner.Run(ctx, o.Supervisor)
	if err != nil {
		// Save state even on partial failure
		_ = o.SharedState.Save()
		return nil, fmt.Errorf("workflow execution failed: %w", err)
	}

	// 4. QA Review
	if o.OnProgress != nil {
		o.OnProgress("qa", "reviewing")
	}

	qaResult := runner.RunQA(ctx, bp, allResults, o.Supervisor)

	// 5. Build result
	parallelExecution := false
	for _, wave := range executionWaves {
		if len(wave) > 1 {
			parallelExecution = true
			break
		}
	}
	result := &WorkflowResult{
		ProjectPath:       o.ProjectDir,
		Domain:            bp.Domain,
		PRD:               bp.PRD,
		Architecture:      bp.Architecture,
		AgentOutputs:      allResults,
		QAResult:          qaResult,
		ExecutionWaves:    executionWaves,
		MaxConcurrency:    runner.MaxConcurrency,
		ParallelExecution: parallelExecution,
	}

	if qaResult.Status == "PASS" {
		result.FinalSummary = fmt.Sprintf("Workflow '%s' completed successfully. %d agents executed. QA: PASS.",
			bp.ProjectName, len(bp.Agents))
	} else {
		result.FinalSummary = fmt.Sprintf("Workflow '%s' completed with QA issues. %d agents executed. QA: %s. Issues: %d.",
			bp.ProjectName, len(bp.Agents), qaResult.Status, len(qaResult.Issues))
	}

	o.Result = result

	// Save final state
	_ = o.SharedState.Save()

	if o.OnProgress != nil {
		o.OnProgress("done", result.FinalSummary)
	}

	return result, nil
}

func (o *Orchestrator) generateBlueprint(ctx context.Context, prompt string) (Blueprint, error) {
	return GenerateBlueprintWithProvider(o.Provider, o.MCPInfo, prompt)
}

func (o *Orchestrator) createWorkers(bp Blueprint) ([]*agent.Worker, error) {
	return CreateWorkersFromBlueprint(bp, o.Supervisor)
}

func (o *Orchestrator) saveBlueprint() error {
	bpPath := filepath.Join(o.ProjectDir, ".smara", "blueprint.json")
	if err := os.MkdirAll(filepath.Dir(bpPath), 0755); err != nil {
		return err
	}
	b, err := o.Blueprint.ToJSON()
	if err != nil {
		return err
	}
	return os.WriteFile(bpPath, b, 0644)
}

// WorkflowResult aggregates all workflow outputs.
type WorkflowResult struct {
	ProjectPath       string                        `json:"project_path"`
	Domain            string                        `json:"domain"`
	PRD               string                        `json:"prd"`
	Architecture      string                        `json:"architecture"`
	AgentOutputs      map[string][]agent.TaskResult `json:"agent_outputs"`
	QAResult          QAResult                      `json:"qa_result"`
	FinalSummary      string                        `json:"final_summary"`
	CompletedAt       time.Time                     `json:"completed_at"`
	ExecutionWaves    [][]string                    `json:"execution_waves,omitempty"`
	MaxConcurrency    int                           `json:"max_concurrency,omitempty"`
	ParallelExecution bool                          `json:"parallel_execution,omitempty"`
}
