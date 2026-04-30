package workflow

import (
	"fmt"
	"strings"

	"github.com/gede-cahya/Smara-CLI/pkg/agent"
	"github.com/gede-cahya/Smara-CLI/pkg/llm"
	"github.com/gede-cahya/Smara-CLI/pkg/mcp"
)

// CreateWorkersFromBlueprint spawns specialized workers from a blueprint.
func CreateWorkersFromBlueprint(bp Blueprint, supervisor *agent.Supervisor) ([]*agent.Worker, error) {
	mcpInfo := supervisor.GetMCPInfo()
	var availableServers []string
	for name, info := range mcpInfo {
		if info.Connected {
			availableServers = append(availableServers, name)
		}
	}

	var workers []*agent.Worker
	for _, spec := range bp.Agents {
		roleDef, ok := GetRoleDefinition(spec.Role)
		if !ok {
			// Dynamic role generation
			roleDef = GenerateDynamicRole(spec.Role, spec.Description, availableServers)
		}

		// Determine allowed tools based on role + available MCP servers
		allowedTools := roleDef.DefaultTools
		mappedServers := MapRoleToMCP(spec.Role, availableServers)
		for _, srv := range mappedServers {
			if info, ok := mcpInfo[srv]; ok {
				for _, t := range info.Tools {
					allowedTools = append(allowedTools, t.Name)
				}
			}
		}

		worker := agent.NewSpecializedWorker(
			supervisor.GetProvider(),
			supervisor.GetMCPClients(),
			spec.Role,
			allowedTools,
			roleDef.SystemPrompt,
		)
		workers = append(workers, worker)
	}

	return workers, nil
}

// CreateWorkersFromBlueprintWithProvider spawns workers with an explicit provider.
func CreateWorkersFromBlueprintWithProvider(bp Blueprint, provider llm.Provider, mcpClients map[string]*mcp.Client, mcpInfo map[string]agent.MCPServerInfo) ([]*agent.Worker, error) {
	var availableServers []string
	for name, info := range mcpInfo {
		if info.Connected {
			availableServers = append(availableServers, name)
		}
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

		worker := agent.NewSpecializedWorker(provider, mcpClients, spec.Role, allowedTools, roleDef.SystemPrompt)
		workers = append(workers, worker)
	}

	return workers, nil
}

// BuildRoleTasks creates agent.Task objects from an AgentSpec.
func BuildRoleTasks(spec AgentSpec, state *SharedState) []agent.Task {
	tasks := make([]agent.Task, 0, len(spec.Tasks))

	// Inject shared state context into task descriptions
	contractsJSON, _ := state.GetContractsJSON()
	contextPrefix := ""
	if contractsJSON != "{}" {
		contextPrefix = fmt.Sprintf("## Shared Context (Contracts)\n%s\n\n## Task\n", contractsJSON)
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

// injectDependencies enriches task descriptions with outputs from completed roles.
func injectDependencies(tasks []agent.Task, completed map[string][]agent.TaskResult) []agent.Task {
	for i := range tasks {
		for role, results := range completed {
			if len(results) > 0 {
				summary := fmt.Sprintf("\n\n## Output from %s\n", strings.ToUpper(role))
				for _, r := range results {
					if r.Output != "" {
						summary += fmt.Sprintf("- %s\n", strings.TrimSpace(r.Output))
					}
				}
				tasks[i].Description += summary
			}
		}
	}
	return tasks
}
