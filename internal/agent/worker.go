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
	provider   llm.Provider
	mcpClients map[string]*mcp.Client
}

// NewWorker creates a new worker agent.
func NewWorker(provider llm.Provider, mcpClients map[string]*mcp.Client) *Worker {
	return &Worker{
		provider:   provider,
		mcpClients: mcpClients,
	}
}

// Execute runs a task and returns the result.
func (w *Worker) Execute(ctx context.Context, task Task) TaskResult {
	// Check if task requires MCP tool call
	if task.MCPServer != "" && task.ToolName != "" {
		return w.executeMCPTask(ctx, task)
	}

	// Otherwise, use LLM to execute the task
	return w.executeLLMTask(ctx, task)
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
	messages := []llm.Message{
		{
			Role:    llm.RoleSystem,
			Content: "Kamu adalah worker agent yang bertugas menyelesaikan satu tugas spesifik dengan tepat.",
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
