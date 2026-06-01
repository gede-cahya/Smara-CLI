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

type WorkerCallback struct {
	OnStream func(chunk string, isThinking bool)
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
	return w.ExecuteWithCallback(ctx, task, nil)
}

// ExecuteWithCallback runs a task and emits optional live progress for LLM-backed work.
func (w *Worker) ExecuteWithCallback(ctx context.Context, task Task, cb *WorkerCallback) TaskResult {
	// Check if task requires MCP or builtin tool call.
	if task.MCPServer != "" || task.ToolName != "" {
		if task.MCPServer == builtinMCPServerName {
			return w.executeBuiltinTask(ctx, task)
		}
		if task.MCPServer == "" {
			return TaskResult{
				TaskID: task.ID,
				Status: TaskFailed,
				Error:  fmt.Sprintf("tool task '%s' declares tool_name '%s' but no mcp_server; refusing LLM fallback", task.ID, task.ToolName),
			}
		}
		client, ok := w.mcpClients[task.MCPServer]
		if !ok {
			return TaskResult{
				TaskID: task.ID,
				Status: TaskFailed,
				Error:  fmt.Sprintf("MCP server '%s' tidak ditemukan", task.MCPServer),
			}
		}
		if !client.HasTool(task.ToolName) {
			return TaskResult{
				TaskID: task.ID,
				Status: TaskFailed,
				Error:  fmt.Sprintf("tool '%s' tidak tersedia di MCP server '%s'", task.ToolName, task.MCPServer),
			}
		}
		return w.executeMCPTask(ctx, task)
	}
	if strings.EqualFold(task.Type, "tool") || strings.EqualFold(task.Type, "mcp") {
		return TaskResult{
			TaskID: task.ID,
			Status: TaskFailed,
			Error:  fmt.Sprintf("tool task '%s' tidak memiliki mcp_server/tool_name yang executable", task.ID),
		}
	}

	// Otherwise, use LLM to execute the task.
	return w.executeLLMTask(ctx, task, cb)
}

func (w *Worker) executeBuiltinTask(ctx context.Context, task Task) TaskResult {
	output, err := ExecuteBuiltinToolWithContext(ctx, task.ToolName, task.ToolArgs, nil)
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
func (w *Worker) executeLLMTask(ctx context.Context, task Task, cb *WorkerCallback) TaskResult {
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

	streamCb := func(chunk string, isThinking bool, _ llm.PhaseHint) {
		if cb != nil && cb.OnStream != nil {
			cb.OnStream(chunk, isThinking)
		}
	}
	if streamer, ok := w.provider.(llm.ContextStreamer); ok {
		resp, err := streamer.ChatStreamWithContext(ctx, messages, streamCb)
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
	if streamer, ok := w.provider.(llm.Streamer); ok {
		resp, err := streamer.ChatStream(messages, streamCb)
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

	type chatResult struct {
		resp *llm.ChatResponse
		err  error
	}
	done := make(chan chatResult, 1)
	go func() {
		resp, err := w.provider.Chat(messages)
		done <- chatResult{resp: resp, err: err}
	}()

	var resp *llm.ChatResponse
	var err error
	select {
	case out := <-done:
		resp, err = out.resp, out.err
	case <-ctx.Done():
		return TaskResult{
			TaskID: task.ID,
			Status: TaskFailed,
			Error:  fmt.Sprintf("gagal mendapatkan response: %v", ctx.Err()),
		}
	}
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
