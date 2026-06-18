package workflow

import (
	"fmt"

	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// plannerStubProvider returns a fixed content (or error) for Chat.
type plannerStubProvider struct {
	content string
	err     error
}

func (p *plannerStubProvider) Name() string { return "planner-stub" }
func (p *plannerStubProvider) Chat(messages []llm.Message) (*llm.ChatResponse, error) {
	if p.err != nil {
		return nil, p.err
	}
	return &llm.ChatResponse{Content: p.content}, nil
}
func (p *plannerStubProvider) ChatWithTools(messages []llm.Message, tools []llm.ToolFunction) (*llm.ChatResponse, []llm.ToolCall, error) {
	resp, err := p.Chat(messages)
	return resp, nil, err
}
func (p *plannerStubProvider) GenerateEmbedding(text string) ([]float32, error) { return nil, nil }

func TestLLMPlanner_DynamicSubtasks(t *testing.T) {
	provider := &plannerStubProvider{content: `{"subtasks":[
		{"id":"read-spec","title":"Baca spec","description":"Baca dokumen spesifikasi","kind":"read_only","risk_level":"low","depends_on":[],"can_parallel":true},
		{"id":"write-impl","title":"Tulis impl","description":"Implementasi fitur","kind":"mutating","risk_level":"medium","depends_on":["read-spec"],"can_parallel":false}
	]}`}

	plan, err := NewLLMPlanner(provider).Plan(OrchestrationTask{
		ID: "feat", Title: "Tambah fitur", Description: "Tambahkan fitur export CSV",
	})
	require.NoError(t, err)
	require.Len(t, plan.Subtasks, 2)
	assert.Equal(t, "read-spec", plan.Subtasks[0].ID)
	assert.Equal(t, "write-impl", plan.Subtasks[1].ID)
	assert.Equal(t, []string{"read-spec"}, plan.Subtasks[1].DependsOn)
	assert.NoError(t, ValidateExecutionPlan(plan))
}

func TestLLMPlanner_EscalateOnlyRisk(t *testing.T) {
	// Task is mutating (rule baseline = high), but LLM under-reports risk as low.
	provider := &plannerStubProvider{content: `{"subtasks":[
		{"id":"apply","title":"Apply","description":"Ubah file konfigurasi","kind":"read_only","risk_level":"low","depends_on":[],"can_parallel":true}
	]}`}

	plan, err := NewLLMPlanner(provider).Plan(OrchestrationTask{
		ID: "fix", Title: "Fix", Description: "Perbaiki bug dan refactor modul",
	})
	require.NoError(t, err)
	require.Len(t, plan.Subtasks, 1)
	// Rule baseline (mutating→high) must win over LLM's "low".
	assert.Equal(t, RiskHigh, plan.Subtasks[0].RiskLevel)
	// High risk forces serial execution regardless of LLM's can_parallel=true.
	assert.False(t, plan.Subtasks[0].CanParallel)
}

func TestLLMPlanner_LLMCanEscalateAboveBaseline(t *testing.T) {
	// Read-only task (baseline low) but LLM flags a destructive subtask.
	provider := &plannerStubProvider{content: `{"subtasks":[
		{"id":"purge","title":"Purge","description":"Hapus data lama","kind":"destructive","risk_level":"critical","depends_on":[],"can_parallel":true}
	]}`}

	plan, err := NewLLMPlanner(provider).Plan(OrchestrationTask{
		ID: "audit", Title: "Audit", Description: "Tinjau data",
	})
	require.NoError(t, err)
	assert.Equal(t, RiskCritical, plan.Subtasks[0].RiskLevel)
	assert.Equal(t, TaskKindDestructive, plan.Subtasks[0].Kind)
}

func TestLLMPlanner_FallbackOnInvalidJSON(t *testing.T) {
	provider := &plannerStubProvider{content: "ini bukan JSON sama sekali"}
	plan, err := NewLLMPlanner(provider).Plan(OrchestrationTask{
		ID: "x", Title: "X", Description: "Audit project structure and summarize",
	})
	require.NoError(t, err)
	// Fell back to rule-based decompose() → read-only path ends with produce-report.
	assert.Equal(t, "produce-report", plan.Subtasks[len(plan.Subtasks)-1].ID)
}

func TestLLMPlanner_FallbackOnCyclicDAG(t *testing.T) {
	provider := &plannerStubProvider{content: `{"subtasks":[
		{"id":"a","title":"A","description":"step a","depends_on":["b"]},
		{"id":"b","title":"B","description":"step b","depends_on":["a"]}
	]}`}
	plan, err := NewLLMPlanner(provider).Plan(OrchestrationTask{
		ID: "y", Title: "Y", Description: "Audit and report",
	})
	require.NoError(t, err)
	// Cyclic LLM output rejected → rule-based fallback produced a valid DAG.
	assert.NoError(t, ValidateExecutionPlan(plan))
	assert.Contains(t, subtaskIDs(plan.Subtasks), "analyze-context")
}

func TestLLMPlanner_FallbackOnProviderError(t *testing.T) {
	provider := &plannerStubProvider{err: fmt.Errorf("provider down")}
	plan, err := NewLLMPlanner(provider).Plan(OrchestrationTask{
		ID: "z", Title: "Z", Description: "Audit repo",
	})
	require.NoError(t, err)
	assert.NoError(t, ValidateExecutionPlan(plan))
}

func TestPlanner_NilProviderUsesRuleBased(t *testing.T) {
	// NewPlanner() has no provider → must behave exactly as before.
	plan, err := NewPlanner().Plan(OrchestrationTask{
		ID: "audit", Title: "Audit", Description: "Audit project structure and summarize issues",
	})
	require.NoError(t, err)
	require.Len(t, plan.Subtasks, 5)
	assert.Equal(t, "produce-report", plan.Subtasks[len(plan.Subtasks)-1].ID)
}

func TestMaxRisk(t *testing.T) {
	assert.Equal(t, RiskHigh, maxRisk(RiskMedium, RiskHigh))
	assert.Equal(t, RiskHigh, maxRisk(RiskHigh, RiskLow))
	assert.Equal(t, RiskCritical, maxRisk(RiskLow, RiskCritical))
	assert.Equal(t, RiskLow, maxRisk(RiskLow, RiskLow))
}
