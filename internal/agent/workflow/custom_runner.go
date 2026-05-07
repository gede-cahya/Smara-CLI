package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

type CustomWorkflowResult struct {
	ProjectPath  string
	AgentOutputs map[string][]agent.TaskResult
	QAResult     QAResult
	FinalSummary string
}

func RunCustomWorkflow(supervisor *agent.Supervisor, provider llm.Provider, cw *CustomWorkflow) (*CustomWorkflowResult, error) {
	if err := cw.Validate(); err != nil {
		return nil, fmt.Errorf("invalid custom workflow: %w", err)
	}
	projectDir := cw.ProjectDir
	if projectDir == "" || projectDir == "." {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("gagal get cwd: %w", err)
		}
		projectDir = cwd
	}
	projectDir, _ = filepath.Abs(projectDir)
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		if err := os.MkdirAll(projectDir, 0755); err != nil {
			return nil, fmt.Errorf("gagal buat project dir: %w", err)
		}
	}
	bp := cw.ToBlueprint()
	sharedState := NewSharedState(projectDir)
	workers, err := createCustomWorkers(bp, supervisor, provider, cw)
	if err != nil {
		return nil, fmt.Errorf("worker creation failed: %w", err)
	}
	workerMap := make(map[string]*agent.Worker)
	for i, spec := range bp.Agents {
		if i < len(workers) {
			workerMap[spec.Role] = workers[i]
		}
	}
	injectInputsFrom(cw, sharedState)
	runner := NewRunner(bp, workerMap, sharedState)
	allResults, err := runner.Run(context.Background(), supervisor)
	if err != nil {
		_ = sharedState.Save()
		return nil, fmt.Errorf("workflow execution failed: %w", err)
	}
	qaResult := runner.RunQA(context.Background(), bp, allResults, supervisor)
	result := &CustomWorkflowResult{
		ProjectPath:  projectDir,
		AgentOutputs: allResults,
		QAResult:     qaResult,
	}
	if qaResult.Status == "PASS" {
		result.FinalSummary = fmt.Sprintf("Custom workflow '%s' completed successfully. %d agents executed. QA: PASS.", cw.Name, len(cw.Agents))
	} else {
		result.FinalSummary = fmt.Sprintf("Custom workflow '%s' completed with QA issues. %d agents executed. QA: %s. Issues: %d.", cw.Name, len(cw.Agents), qaResult.Status, len(qaResult.Issues))
	}
	_ = sharedState.Save()
	return result, nil
}

func createCustomWorkers(bp Blueprint, supervisor *agent.Supervisor, provider llm.Provider, cw *CustomWorkflow) ([]*agent.Worker, error) {
	mcpInfo := supervisor.GetMCPInfo()
	var availableServers []string
	for name, info := range mcpInfo {
		if info.Connected {
			availableServers = append(availableServers, name)
		}
	}
	customAgentMap := make(map[string]CustomAgent)
	for _, a := range cw.Agents {
		customAgentMap[a.Role] = a
	}
	var workers []*agent.Worker
	for _, spec := range bp.Agents {
		roleDef, ok := GetRoleDefinition(spec.Role)
		if !ok {
			roleDef = GenerateDynamicRole(spec.Role, spec.Description, availableServers)
		}
		allowedTools := roleDef.DefaultTools
		mappedServers := MapRoleToMCP(spec.Role, availableServers)
		for _, srv := range mappedServers {
			if info, ok := mcpInfo[srv]; ok {
				for _, t := range info.Tools {
					allowedTools = append(allowedTools, t.Name)
				}
			}
		}
		systemPrompt := roleDef.SystemPrompt
		customAgent := customAgentMap[spec.Role]
		if len(customAgent.Skills) > 0 {
			var skillNotes []string
			for _, skName := range customAgent.Skills {
				sk, err := skill.Load(skName)
				if err == nil && sk != nil {
					skillNotes = append(skillNotes, fmt.Sprintf("## Skill: %s\n%s", sk.Name, sk.Description))
					for _, step := range sk.Steps {
						if step.Tool != "" {
							allowedTools = append(allowedTools, step.Tool)
						}
					}
				}
			}
			if len(skillNotes) > 0 {
				systemPrompt += "\n\n# Assigned Skills\n" + strings.Join(skillNotes, "\n\n")
			}
		}
		var p llm.Provider
		if provider != nil {
			p = provider
		} else {
			p = supervisor.GetProvider()
		}
		worker := agent.NewSpecializedWorker(p, supervisor.GetMCPClients(), spec.Role, allowedTools, systemPrompt)
		workers = append(workers, worker)
	}
	return workers, nil
}

func injectInputsFrom(cw *CustomWorkflow, state *SharedState) {
	for _, a := range cw.Agents {
		for srcRole, keys := range a.InputsFrom {
			_ = keys
			contract := fmt.Sprintf("// Agent '%s' expects inputs from '%s': %v", a.Role, srcRole, keys)
			state.WriteContract(fmt.Sprintf("%s->%s", srcRole, a.Role), contract)
		}
	}
}

func BuildCustomRoleTasks(spec AgentSpec, state *SharedState, inputsFrom map[string][]string) []agent.Task {
	tasks := make([]agent.Task, 0, len(spec.Tasks))
	var upstreamContext string
	for srcRole, keys := range inputsFrom {
		_ = keys
		artifacts := state.ListArtifactsByRole(srcRole)
		if len(artifacts) > 0 {
			upstreamContext += fmt.Sprintf("\n## Outputs from '%s'\n", srcRole)
			for _, art := range artifacts {
				upstreamContext += fmt.Sprintf("- %s: %s\n", art.Key, art.Value)
			}
		}
	}
	contextPrefix := ""
	if upstreamContext != "" {
		contextPrefix = fmt.Sprintf("# Upstream Agent Outputs\n%s\n\n# Your Task\n", upstreamContext)
	}
	contractsJSON, _ := state.GetContractsJSON()
	if contractsJSON != "{}" {
		contextPrefix = fmt.Sprintf("## Shared Context (Contracts)\n%s\n\n## Task\n", contractsJSON) + contextPrefix
	}
	for _, t := range spec.Tasks {
		tasks = append(tasks, agent.Task{
			ID:          fmt.Sprintf("%s-%s", spec.Role, t.ID),
			Description: contextPrefix + t.Description,
			AssignedTo:  spec.Role,
			MCPServer:   t.MCPServer,
			ToolName:    t.ToolName,
		})
	}
	return tasks
}
