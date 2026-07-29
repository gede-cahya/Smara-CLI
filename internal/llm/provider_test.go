package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAvailableProviders(t *testing.T) {
	providers := AvailableProviders()
	require.NotNil(t, providers)
	assert.Len(t, providers, 5)
	assert.Contains(t, providers, "ollama")
	assert.Contains(t, providers, "openai")
	assert.Contains(t, providers, "openrouter")
	assert.Contains(t, providers, "anthropic")
	assert.Contains(t, providers, "custom")
}

func TestAvailableProviders_ProviderInfo(t *testing.T) {
	providers := AvailableProviders()

	ollama := providers["ollama"]
	assert.Equal(t, "ollama", ollama.Name)
	assert.False(t, ollama.NeedsAPIKey)
	assert.NotEmpty(t, ollama.Models)

	openai := providers["openai"]
	assert.Equal(t, "openai", openai.Name)
	assert.True(t, openai.NeedsAPIKey)
	assert.NotEmpty(t, openai.Models)

	anthropic := providers["anthropic"]
	assert.Equal(t, "anthropic", anthropic.Name)
	assert.True(t, anthropic.NeedsAPIKey)
}

func TestProviderConfig_Struct(t *testing.T) {
	cfg := ProviderConfig{
		Name:            "openai",
		Model:           "gpt-4o",
		Host:            "https://api.openai.com",
		APIKey:          "sk-test",
		ReasoningEffort: "high",
	}
	assert.Equal(t, "openai", cfg.Name)
	assert.Equal(t, "gpt-4o", cfg.Model)
	assert.Equal(t, "https://api.openai.com", cfg.Host)
	assert.Equal(t, "sk-test", cfg.APIKey)
	assert.Equal(t, "high", cfg.ReasoningEffort)
}

func TestNewProvider_Ollama(t *testing.T) {
	provider, err := NewProvider(ProviderConfig{
		Name:  "ollama",
		Model: "llama3",
		Host:  "http://localhost:11434",
	})
	require.NoError(t, err)
	assert.NotNil(t, provider)
	assert.Equal(t, "ollama", provider.Name())
}

func TestNewProvider_OpenAI_NoKey(t *testing.T) {
	_, err := NewProvider(ProviderConfig{Name: "openai", APIKey: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OpenAI")
	assert.Contains(t, err.Error(), "API key")
}

func TestNewProvider_OpenRouter_NoKey(t *testing.T) {
	_, err := NewProvider(ProviderConfig{Name: "openrouter", APIKey: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OpenRouter")
	assert.Contains(t, err.Error(), "API key")
}

func TestNewProvider_Anthropic_NoKey(t *testing.T) {
	_, err := NewProvider(ProviderConfig{Name: "anthropic", APIKey: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Anthropic")
	assert.Contains(t, err.Error(), "API key")
}

func TestNewProvider_Custom_MissingKey(t *testing.T) {
	_, err := NewProvider(ProviderConfig{
		Name:   "custom",
		APIKey: "",
		Host:   "http://localhost",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Custom")
	assert.Contains(t, err.Error(), "API key")
}

func TestNewProvider_Custom_MissingHost(t *testing.T) {
	_, err := NewProvider(ProviderConfig{
		Name:   "custom",
		APIKey: "key",
		Host:   "",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Custom")
	assert.Contains(t, err.Error(), "base URL")
}

func TestNewProvider_Unknown(t *testing.T) {
	_, err := NewProvider(ProviderConfig{Name: "unknown"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tidak dikenali")
	assert.Contains(t, err.Error(), "unknown")
}

func TestNewProvider_Custom_ReasoningEffort(t *testing.T) {
	provider, err := NewProvider(ProviderConfig{
		Name:            "custom",
		Model:           "gpt-5.5",
		Host:            "http://localhost:8317/v1",
		APIKey:          "key",
		ReasoningEffort: "HIGH",
	})
	require.NoError(t, err)

	custom, ok := provider.(*CustomProvider)
	require.True(t, ok)
	assert.Equal(t, "high", custom.reasoningEffort)
}

func TestNormalizeReasoningEffort(t *testing.T) {
	assert.Equal(t, "", normalizeReasoningEffort(""))
	assert.Equal(t, "low", normalizeReasoningEffort(" low "))
	assert.Equal(t, "medium", normalizeReasoningEffort("MEDIUM"))
	assert.Equal(t, "high", normalizeReasoningEffort("high"))
	assert.Equal(t, "xhigh", normalizeReasoningEffort("XHIGH"))
	assert.Equal(t, "", normalizeReasoningEffort("max"))
}

func TestPhaseHint_Constants(t *testing.T) {
	assert.Equal(t, PhaseHint(0), PhaseUnknown)
	assert.Equal(t, PhaseHint(1), PhaseThinking)
	assert.Equal(t, PhaseHint(2), PhaseAnalyzing)
	assert.Equal(t, PhaseHint(3), PhaseExploring)
	assert.Equal(t, PhaseHint(4), PhaseGenerating)
}

func TestStreamCallback_Type(t *testing.T) {
	var cb StreamCallback
	cb = func(chunk string, isThinking bool, phaseHint PhaseHint) {}
	assert.NotNil(t, cb)
}

func TestProviderInfo_Struct(t *testing.T) {
	pi := ProviderInfo{
		Name:        "test-provider",
		Description: "A test provider",
		Models:      []string{"model1", "model2"},
		NeedsAPIKey: true,
	}
	assert.Equal(t, "test-provider", pi.Name)
	assert.Equal(t, "A test provider", pi.Description)
	assert.Equal(t, []string{"model1", "model2"}, pi.Models)
	assert.True(t, pi.NeedsAPIKey)
}

func TestConvertMessagesToOpenAI_ToolCallPairing(t *testing.T) {
	// Normal flow: assistant tool_call then tool response with same ID must stay paired, not renamed to _dup1.
	msgs := []Message{
		{Role: RoleUser, Content: "cek vps"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call_897e1173a376ea9e", Function: "skill_run", Args: map[string]interface{}{"skill_name": "vps-health-check"}}}},
		{Role: RoleTool, ToolCallID: "call_897e1173a376ea9e", Content: "ok"},
	}
	out := convertMessagesToOpenAI(msgs)
	require.Len(t, out, 3)
	assert.Equal(t, "call_897e1173a376ea9e", out[1].ToolCalls[0].ID)
	assert.Equal(t, "call_897e1173a376ea9e", out[2].ToolCallID)
	// Ensure no _dup suffix in happy path
	for _, m := range out {
		assert.NotContains(t, m.ToolCallID, "_dup", "tool_call_id must not be rewritten in normal flow")
		for _, tc := range m.ToolCalls {
			assert.NotContains(t, tc.ID, "_dup")
		}
	}
}

func TestConvertMessagesToOpenAI_DupIDRewritten(t *testing.T) {
	// Same assistant ID reused twice (old DSML bug) must be rewritten and each tool response must match its rewritten ID.
	msgs := []Message{
		{Role: RoleUser, Content: "cek vps"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call_dup", Function: "skill_run"}}},
		{Role: RoleTool, ToolCallID: "call_dup", Content: "first"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call_dup", Function: "skill_run"}}},
		{Role: RoleTool, ToolCallID: "call_dup", Content: "second"},
	}
	out := convertMessagesToOpenAI(msgs)
	// Collect assistant IDs and tool IDs
	var assistantIDs []string
	var toolIDs []string
	for _, m := range out {
		for _, tc := range m.ToolCalls {
			assistantIDs = append(assistantIDs, tc.ID)
		}
		if m.Role == "tool" {
			toolIDs = append(toolIDs, m.ToolCallID)
		}
	}
	require.Len(t, assistantIDs, 2)
	require.Len(t, toolIDs, 2)
	assert.Equal(t, "call_dup", assistantIDs[0])
	assert.NotEqual(t, "call_dup", assistantIDs[1], "second dup must be rewritten")
	assert.Equal(t, assistantIDs[0], toolIDs[0])
	assert.Equal(t, assistantIDs[1], toolIDs[1], "tool response must match its (possibly rewritten) assistant ID")
}

func TestConvertMessagesToOpenAI_OrphanToolDropped(t *testing.T) {
	// Tool response without preceding assistant must be dropped to avoid 400 "'id' does not match any tool_calls[].id"
	msgs := []Message{
		{Role: RoleUser, Content: "hi"},
		{Role: RoleTool, ToolCallID: "call_orphan_dup1", Content: "stale"},
	}
	out := convertMessagesToOpenAI(msgs)
	for _, m := range out {
		assert.NotEqual(t, "call_orphan_dup1", m.ToolCallID, "orphan tool response must be dropped")
	}
}
