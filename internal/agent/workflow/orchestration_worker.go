package workflow

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
)

// WorkerHandlerConfig configures the Worker-backed subtask executor.
type WorkerHandlerConfig struct {
	// MaxRepairAttempts is how many LLM self-correction retries a failed
	// subtask gets before being reported as failed. 0 disables repair.
	MaxRepairAttempts int
	// OnSubtaskStart fires before a subtask runs (for live UI updates).
	OnSubtaskStart func(st Subtask)
	// OnSubtaskStream relays streamed tokens from the worker.
	OnSubtaskStream func(subtaskID, chunk string, isThinking bool)
	// OnRepairAttempt fires before each self-correction retry.
	OnRepairAttempt func(subtaskID string, attempt int, prevError string)
}

// WorkerSubtaskHandler executes orchestration subtasks using a real agent.Worker
// (LLM + MCP tools), wiring dependency outputs from completed subtasks into the
// prompt and applying LLM self-correction on failure. It is the bridge that
// gives the subtask path the same execution muscle the blueprint Runner has.
type WorkerSubtaskHandler struct {
	worker *agent.Worker
	state  *SharedState
	config WorkerHandlerConfig

	mu      sync.RWMutex
	results map[string]ExecutionResult // subtaskID → result, for dependency injection
}

// NewWorkerSubtaskHandler builds a handler from a supervisor. A single generic
// worker carries the provider, MCP clients, and the full tool set; per-subtask
// specialization is expressed through the prompt rather than separate roles.
func NewWorkerSubtaskHandler(supervisor *agent.Supervisor, state *SharedState, config WorkerHandlerConfig) *WorkerSubtaskHandler {
	worker := agent.NewSpecializedWorker(
		supervisor.GetProvider(),
		supervisor.GetMCPClients(),
		"orchestrator-worker",
		nil, // nil allowedTools → worker may use any available tool
		orchestrationWorkerSystemPrompt,
	)
	return &WorkerSubtaskHandler{
		worker:  worker,
		state:   state,
		config:  config,
		results: map[string]ExecutionResult{},
	}
}

const orchestrationWorkerSystemPrompt = `Kamu adalah worker agent dalam orkestrasi multi-step Smara.
Selesaikan SATU subtask spesifik dengan tepat dan konkret.
- Gunakan output dari subtask sebelumnya (bila disertakan) sebagai konteks, jangan ulangi pekerjaannya.
- Jika subtask mengubah file atau state, lakukan dengan hati-hati dan laporkan hasil akhirnya.
- Jangan mengarang hasil; bila tidak bisa menyelesaikan, jelaskan kenapa.`

// Handler returns the SubtaskExecutorFunc consumed by Executor.
func (h *WorkerSubtaskHandler) Handler() SubtaskExecutorFunc {
	return func(ctx context.Context, st Subtask) ExecutionResult {
		if h.config.OnSubtaskStart != nil {
			h.config.OnSubtaskStart(st)
		}
		result := h.runSubtask(ctx, st)
		h.recordResult(result)
		return result
	}
}

func (h *WorkerSubtaskHandler) runSubtask(ctx context.Context, st Subtask) ExecutionResult {
	task := agent.Task{
		ID:          st.ID,
		Description: h.buildSubtaskPrompt(st, ""),
		AssignedTo:  "orchestrator-worker",
	}

	res := h.execTask(ctx, st.ID, task)
	if res.Status != agent.TaskFailed {
		return toExecutionResult(st.ID, res)
	}

	// Self-correction: retry with the error fed back in, up to the cap.
	for attempt := 1; attempt <= h.config.MaxRepairAttempts; attempt++ {
		if ctx.Err() != nil {
			break
		}
		if h.config.OnRepairAttempt != nil {
			h.config.OnRepairAttempt(st.ID, attempt, res.Error)
		}
		repaired := agent.Task{
			ID:          st.ID,
			Description: h.buildRepairPrompt(st, res, attempt),
			AssignedTo:  "orchestrator-worker",
		}
		res = h.execTask(ctx, st.ID, repaired)
		if res.Status != agent.TaskFailed {
			break
		}
	}

	return toExecutionResult(st.ID, res)
}

func (h *WorkerSubtaskHandler) execTask(ctx context.Context, subtaskID string, task agent.Task) agent.TaskResult {
	return h.worker.ExecuteWithCallback(ctx, task, &agent.WorkerCallback{
		OnStream: func(chunk string, isThinking bool) {
			if h.config.OnSubtaskStream != nil {
				h.config.OnSubtaskStream(subtaskID, chunk, isThinking)
			}
		},
	})
}

// buildSubtaskPrompt assembles the worker prompt: shared contracts, completed
// dependency outputs, then the subtask description.
func (h *WorkerSubtaskHandler) buildSubtaskPrompt(st Subtask, extra string) string {
	var sb strings.Builder

	if h.state != nil {
		if contracts, err := h.state.GetContractsJSON(); err == nil && contracts != "{}" {
			sb.WriteString("## Shared Context (Contracts)\n")
			sb.WriteString(contracts)
			sb.WriteString("\n\n")
		}
	}

	if deps := h.dependencyOutputs(st); deps != "" {
		sb.WriteString("## Output dari subtask sebelumnya\n")
		sb.WriteString(deps)
		sb.WriteString("\n")
	}

	sb.WriteString("## Subtask\n")
	sb.WriteString(st.Title)
	sb.WriteString("\n")
	sb.WriteString(st.Description)
	if extra != "" {
		sb.WriteString("\n\n")
		sb.WriteString(extra)
	}
	return sb.String()
}

func (h *WorkerSubtaskHandler) buildRepairPrompt(st Subtask, prev agent.TaskResult, attempt int) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Percobaan sebelumnya GAGAL (self-correction ke-%d).\n\n", attempt))
	sb.WriteString("## Error\n")
	sb.WriteString(strings.TrimSpace(prev.Error))
	sb.WriteString("\n")
	if out := strings.TrimSpace(prev.Output); out != "" {
		if len(out) > 2000 {
			out = out[:2000] + "\n[... output dipotong ...]"
		}
		sb.WriteString("\n## Output parsial sebelumnya\n")
		sb.WriteString(out)
		sb.WriteString("\n")
	}
	sb.WriteString("\nPerbaiki penyebab error lalu selesaikan subtask dengan benar. Jangan ulangi kesalahan yang sama.\n\n")
	return h.buildSubtaskPrompt(st, sb.String())
}

// dependencyOutputs returns concatenated stdout of this subtask's dependencies.
func (h *WorkerSubtaskHandler) dependencyOutputs(st Subtask) string {
	if len(st.DependsOn) == 0 {
		return ""
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	var parts []string
	for _, dep := range st.DependsOn {
		res, ok := h.results[dep]
		if !ok {
			continue
		}
		out := strings.TrimSpace(res.Stdout)
		if out == "" {
			continue
		}
		if len(out) > 4000 {
			out = out[:4000] + "\n[... dipotong ...]"
		}
		parts = append(parts, fmt.Sprintf("### %s\n%s", dep, out))
	}
	return strings.Join(parts, "\n\n")
}

func (h *WorkerSubtaskHandler) recordResult(res ExecutionResult) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.results[res.SubtaskID] = res
	if h.state != nil && res.Status == StatusSuccess && strings.TrimSpace(res.Stdout) != "" {
		h.state.WriteArtifact("orchestrator-worker", res.SubtaskID, res.Stdout)
	}
}

func toExecutionResult(subtaskID string, res agent.TaskResult) ExecutionResult {
	status := StatusSuccess
	if res.Status == agent.TaskFailed {
		status = StatusFailed
	}
	return ExecutionResult{
		SubtaskID: subtaskID,
		Status:    status,
		Stdout:    res.Output,
		Error:     res.Error,
	}
}
