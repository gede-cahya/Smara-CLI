package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/mcp"
)

// Worker executes individual tasks as delegated by the Supervisor.
type Worker struct {
	provider     llm.Provider
	mcpClients   map[string]*mcp.Client
	Role         string
	AllowedTools []string
	SystemPrompt string
}

// NewWorker creates a new worker agent.
func NewWorker(provider llm.Provider, mcpClients map[string]*mcp.Client) *Worker {
	return &Worker{
		provider:   provider,
		mcpClients: mcpClients,
	}
}

// NewSpecializedWorker creates a role-specialized worker with custom system prompt and tool filtering.
func NewSpecializedWorker(provider llm.Provider, mcpClients map[string]*mcp.Client, role string, allowedTools []string, systemPrompt string) *Worker {
	return &Worker{
		provider:     provider,
		mcpClients:   mcpClients,
		Role:         role,
		AllowedTools: allowedTools,
		SystemPrompt: systemPrompt,
	}
}

// Execute runs a task and returns the result.
func (w *Worker) Execute(ctx context.Context, task Task) TaskResult {
	// Check if task requires MCP or builtin tool call
	if task.MCPServer != "" && task.ToolName != "" {
		if task.MCPServer == builtinMCPServerName {
			return w.executeBuiltinTask(task)
		}
		// Validate: if MCP client exists but tool doesn't, fallback to LLM
		if client, ok := w.mcpClients[task.MCPServer]; ok && !client.HasTool(task.ToolName) {
			return w.executeLLMTask(ctx, Task{
				ID:          task.ID,
				Description: task.Description + "\n\n[NOTE: Tool '" + task.ToolName + "' tidak tersedia di MCP server '" + task.MCPServer + "'. Menyelesaikan dengan LLM fallback.]",
			})
		}
		return w.executeMCPTask(ctx, task)
	}

	// Otherwise, use LLM to execute the task
	return w.executeLLMTask(ctx, task)
}

func (w *Worker) executeBuiltinTask(task Task) TaskResult {
	output, err := ExecuteBuiltinTool(task.ToolName, task.ToolArgs, nil)
	if err != nil {
		return TaskResult{TaskID: task.ID, Status: TaskFailed, Error: fmt.Sprintf("gagal memanggil builtin tool '%s': %v", task.ToolName, err)}
	}
	return TaskResult{TaskID: task.ID, Status: TaskCompleted, Output: output}
}

// executeMCPTask runs a task that involves an MCP server tool call.
func (w *Worker) executeMCPTask(ctx context.Context, task Task) TaskResult {
	client, ok := w.mcpClients[task.MCPServer]
	if !ok {
		return TaskResult{
			TaskID: task.ID,
			Status: TaskFailed,
			Error:  fmt.Sprintf("MCP server '%s' tidak ditemukan", task.MCPServer),
		}
	}

	result, err := client.CallTool(task.ToolName, task.ToolArgs)
	if err != nil {
		return TaskResult{
			TaskID: task.ID,
			Status: TaskFailed,
			Error:  fmt.Sprintf("gagal memanggil tool '%s': %v", task.ToolName, err),
		}
	}

	if result.IsError {
		var errText string
		for _, c := range result.Content {
			errText += c.Text
		}
		return TaskResult{
			TaskID: task.ID,
			Status: TaskFailed,
			Error:  errText,
		}
	}

	var output strings.Builder
	for _, c := range result.Content {
		if c.Text != "" {
			output.WriteString(c.Text)
			output.WriteString("\n")
		}
	}

	return TaskResult{
		TaskID: task.ID,
		Status: TaskCompleted,
		Output: output.String(),
	}
}

// executeLLMTask runs a task using only the LLM.
func (w *Worker) executeLLMTask(ctx context.Context, task Task) TaskResult {
	systemPrompt := "Kamu adalah worker agent yang bertugas menyelesaikan satu tugas spesifik dengan tepat."
	if w.SystemPrompt != "" {
		systemPrompt = w.SystemPrompt
	}

	messages := []llm.Message{
		{
			Role:    llm.RoleSystem,
			Content: systemPrompt,
		},
		{
			Role:    llm.RoleUser,
			Content: task.Description,
		},
	}

	resp, err := w.provider.Chat(messages)
	if err != nil {
		return TaskResult{
			TaskID: task.ID,
			Status: TaskFailed,
			Error:  fmt.Sprintf("gagal mendapatkan response: %v", err),
		}
	}

	return TaskResult{
		TaskID: task.ID,
		Status: TaskCompleted,
		Output: resp.Content,
	}
}
