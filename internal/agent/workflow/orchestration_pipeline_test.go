package workflow

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pipelineProvider returns a plan JSON when asked to plan (system prompt is the
// planner prompt), and a per-subtask answer otherwise. Optionally fails a named
// subtask once to exercise self-correction.
type pipelineProvider struct {
	mu          sync.Mutex
	planJSON    string
	failOnce    string // subtask description fragment to fail once
	failedSeen  map[string]bool
	workerCalls int
}

func (p *pipelineProvider) Name() string { return "pipeline-stub" }

func (p *pipelineProvider) Chat(messages []llm.Message) (*llm.ChatResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	isPlanner := false
	var lastUser string
	for _, m := range messages {
		if m.Role == llm.RoleSystem && strings.Contains(m.Content, "perencana eksekusi") {
			isPlanner = true
		}
		if m.Role == llm.RoleUser {
			lastUser = m.Content
		}
	}
	if isPlanner {
		return &llm.ChatResponse{Content: p.planJSON}, nil
	}
	p.workerCalls++
	// Self-correction path: fail the first time we see failOnce, succeed after.
	if p.failOnce != "" && strings.Contains(lastUser, p.failOnce) {
		if p.failedSeen == nil {
			p.failedSeen = map[string]bool{}
		}
		if !p.failedSeen[p.failOnce] && !strings.Contains(lastUser, "self-correction") {
			p.failedSeen[p.failOnce] = true
			return nil, assertErr("simulasi gagal worker")
		}
	}
	return &llm.ChatResponse{Content: "output subtask ok"}, nil
}

func (p *pipelineProvider) ChatWithTools(messages []llm.Message, tools []llm.ToolFunction) (*llm.ChatResponse, []llm.ToolCall, error) {
	resp, err := p.Chat(messages)
	return resp, nil, err
}
func (p *pipelineProvider) GenerateEmbedding(text string) ([]float32, error) { return nil, nil }

type assertErr string

func (e assertErr) Error() string { return string(e) }

const twoStepPlanJSON = `{"subtasks":[
  {"id":"step-read","title":"Baca","description":"Baca konteks awal","kind":"read_only","risk_level":"low","depends_on":[],"can_parallel":true},
  {"id":"step-impl","title":"Implementasi","description":"Implementasi fitur","kind":"read_only","risk_level":"low","depends_on":["step-read"],"can_parallel":false}
]}`

func TestRunOrchestration_HappyPath(t *testing.T) {
	provider := &pipelineProvider{planJSON: twoStepPlanJSON}
	sup := agent.NewSupervisor(provider, nil)

	run, result, err := RunOrchestration(context.Background(), sup, t.TempDir(), "Tambah fitur export", OrchestrationConfig{
		MaxConcurrency: 4, MaxRepairAttempts: 2,
	})
	require.NoError(t, err)
	require.NotNil(t, run)
	require.NotNil(t, result)
	assert.Equal(t, StatusSuccess, run.Execution.Status)
	assert.Len(t, run.Plan.Subtasks, 2)
	assert.Equal(t, "step-read", run.Plan.Subtasks[0].ID)
	// Both subtasks should have succeeded.
	assert.Equal(t, StatusSuccess, run.Execution.Results["step-read"].Status)
	assert.Equal(t, StatusSuccess, run.Execution.Results["step-impl"].Status)
}

func TestRunOrchestration_SelfCorrection(t *testing.T) {
	provider := &pipelineProvider{planJSON: twoStepPlanJSON, failOnce: "Implementasi fitur"}
	sup := agent.NewSupervisor(provider, nil)

	var repairs []string
	run, _, err := RunOrchestration(context.Background(), sup, t.TempDir(), "Tambah fitur", OrchestrationConfig{
		MaxConcurrency: 4, MaxRepairAttempts: 2,
		OnRepairAttempt: func(subtaskID string, attempt int, prevError string) {
			repairs = append(repairs, subtaskID)
		},
	})
	require.NoError(t, err)
	// step-impl failed once then recovered via self-correction.
	assert.Equal(t, StatusSuccess, run.Execution.Results["step-impl"].Status)
	assert.Contains(t, repairs, "step-impl")
}

func TestRunOrchestration_FallbackPlanOnBadJSON(t *testing.T) {
	// Planner returns garbage → planner falls back to rule-based decompose().
	provider := &pipelineProvider{planJSON: "bukan json"}
	sup := agent.NewSupervisor(provider, nil)

	run, _, err := RunOrchestration(context.Background(), sup, t.TempDir(), "Audit project structure", OrchestrationConfig{
		MaxConcurrency: 4,
	})
	require.NoError(t, err)
	// Rule-based read-only plan ends with produce-report.
	ids := subtaskIDs(run.Plan.Subtasks)
	assert.Contains(t, ids, "analyze-context")
	assert.Contains(t, ids, "produce-report")
}

func TestRunOrchestration_LiveCallbacks(t *testing.T) {
	provider := &pipelineProvider{planJSON: twoStepPlanJSON}
	sup := agent.NewSupervisor(provider, nil)

	var planReady bool
	var starts, results []string
	_, _, err := RunOrchestration(context.Background(), sup, t.TempDir(), "Tambah fitur", OrchestrationConfig{
		MaxConcurrency: 4,
		OnPlanReady:    func(plan ExecutionPlan, _ SafetyReport) { planReady = len(plan.Batches) > 0 },
		OnSubtaskStart: func(st Subtask) { starts = append(starts, st.ID) },
		OnSubtaskResult: func(res ExecutionResult) { results = append(results, res.SubtaskID) },
	})
	require.NoError(t, err)
	assert.True(t, planReady)
	assert.ElementsMatch(t, []string{"step-read", "step-impl"}, starts)
	assert.ElementsMatch(t, []string{"step-read", "step-impl"}, results)
}

func TestRunOrchestration_EmptyPrompt(t *testing.T) {
	provider := &pipelineProvider{planJSON: twoStepPlanJSON}
	sup := agent.NewSupervisor(provider, nil)
	_, _, err := RunOrchestration(context.Background(), sup, t.TempDir(), "   ", OrchestrationConfig{})
	require.Error(t, err)
}
