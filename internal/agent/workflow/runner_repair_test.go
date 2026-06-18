package workflow

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repairStubProvider succeeds only when the latest user message contains the
// marker; otherwise it errors. This lets a test drive "fail then succeed after
// repair" deterministically without a real LLM.
type repairStubProvider struct {
	mu     sync.Mutex
	marker string
	calls  int
}

func (p *repairStubProvider) Name() string { return "repair-stub" }

func (p *repairStubProvider) Chat(messages []llm.Message) (*llm.ChatResponse, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	var last string
	for _, m := range messages {
		if m.Role == llm.RoleUser {
			last = m.Content
		}
	}
	if p.marker != "" && strings.Contains(last, p.marker) {
		return &llm.ChatResponse{Content: "berhasil setelah perbaikan"}, nil
	}
	return nil, fmt.Errorf("simulasi gagal eksekusi task")
}

func (p *repairStubProvider) ChatWithTools(messages []llm.Message, tools []llm.ToolFunction) (*llm.ChatResponse, []llm.ToolCall, error) {
	resp, err := p.Chat(messages)
	return resp, nil, err
}

func (p *repairStubProvider) GenerateEmbedding(text string) ([]float32, error) { return nil, nil }

func newRepairRunner(t *testing.T, provider llm.Provider, maxRepair int) (*Runner, *agent.Worker) {
	t.Helper()
	bp := Blueprint{Agents: []AgentSpec{{
		Role:  "backend",
		Tasks: []Task{{ID: "t1", Description: "Kerjakan task awal"}},
	}}}
	worker := agent.NewSpecializedWorker(provider, nil, "backend", nil, "")
	r := NewRunner(bp, map[string]*agent.Worker{"backend": worker}, NewSharedState(t.TempDir()))
	r.Serial = true
	r.MaxRepairAttempts = maxRepair
	return r, worker
}

func TestRunner_RepairSucceedsAfterRetry(t *testing.T) {
	provider := &repairStubProvider{marker: "FIXED"}
	r, _ := newRepairRunner(t, provider, 2)

	var attempts []int
	r.OnRepairAttempt = func(role, taskID string, attempt int, prevError string) {
		attempts = append(attempts, attempt)
		assert.Equal(t, "backend", role)
		assert.Contains(t, prevError, "simulasi gagal")
	}
	// Mock repair: inject the success marker so the retry passes.
	r.RepairFunc = func(ctx context.Context, w *agent.Worker, role string, failed agent.Task, result agent.TaskResult, attempt int) (agent.Task, bool) {
		fixed := failed
		fixed.Description = "FIXED: " + failed.Description
		return fixed, true
	}

	results := r.runRole(context.Background(), "backend", map[string][]agent.TaskResult{}, nil)
	require.Len(t, results, 1)
	assert.Equal(t, agent.TaskCompleted, results[0].Status)
	assert.Equal(t, []int{1}, attempts) // succeeded on first repair attempt
}

func TestRunner_RepairExhaustsAttempts(t *testing.T) {
	provider := &repairStubProvider{marker: "NEVER_MATCHES"}
	r, _ := newRepairRunner(t, provider, 2)

	var attempts []int
	r.OnRepairAttempt = func(role, taskID string, attempt int, prevError string) {
		attempts = append(attempts, attempt)
	}
	r.RepairFunc = func(ctx context.Context, w *agent.Worker, role string, failed agent.Task, result agent.TaskResult, attempt int) (agent.Task, bool) {
		return failed, true // keep trying but provider still fails
	}

	results := r.runRole(context.Background(), "backend", map[string][]agent.TaskResult{}, nil)
	require.Len(t, results, 1)
	assert.Equal(t, agent.TaskFailed, results[0].Status)
	assert.Equal(t, []int{1, 2}, attempts) // both attempts used
}

func TestRunner_RepairDisabled(t *testing.T) {
	provider := &repairStubProvider{marker: "FIXED"}
	r, _ := newRepairRunner(t, provider, 0)

	repairCalled := false
	r.RepairFunc = func(ctx context.Context, w *agent.Worker, role string, failed agent.Task, result agent.TaskResult, attempt int) (agent.Task, bool) {
		repairCalled = true
		return failed, true
	}

	results := r.runRole(context.Background(), "backend", map[string][]agent.TaskResult{}, nil)
	require.Len(t, results, 1)
	assert.Equal(t, agent.TaskFailed, results[0].Status)
	assert.False(t, repairCalled, "repair must not run when MaxRepairAttempts=0")
}

func TestRunner_RepairFuncCannotRepair(t *testing.T) {
	provider := &repairStubProvider{marker: "FIXED"}
	r, _ := newRepairRunner(t, provider, 3)

	r.RepairFunc = func(ctx context.Context, w *agent.Worker, role string, failed agent.Task, result agent.TaskResult, attempt int) (agent.Task, bool) {
		return agent.Task{}, false // signal "cannot repair" → stop immediately
	}

	results := r.runRole(context.Background(), "backend", map[string][]agent.TaskResult{}, nil)
	require.Len(t, results, 1)
	assert.Equal(t, agent.TaskFailed, results[0].Status)
}

func TestDefaultRepair_SkipsToolTasks(t *testing.T) {
	failed := agent.Task{ID: "x", MCPServer: "stitch", ToolName: "create_project"}
	_, ok := defaultRepair(context.Background(), nil, "designer", failed, agent.TaskResult{Error: "boom"}, 1)
	assert.False(t, ok, "tool/MCP tasks are not LLM-repairable")
}

func TestBuildRepairPrompt_IncludesErrorAndOriginal(t *testing.T) {
	failed := agent.Task{Description: "Buat endpoint /users"}
	res := agent.TaskResult{Error: "panic: nil map", Output: "partial work"}
	prompt := buildRepairPrompt(failed, res, 2)
	assert.Contains(t, prompt, "panic: nil map")
	assert.Contains(t, prompt, "Buat endpoint /users")
	assert.Contains(t, prompt, "partial work")
	assert.Contains(t, prompt, "ke-2")
}
