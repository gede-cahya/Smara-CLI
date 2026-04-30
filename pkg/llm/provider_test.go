package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAvailableProviders(t *testing.T) {
	providers := AvailableProviders()
	require.NotNil(t, providers)

	expected := []string{"ollama", "openai", "openrouter", "anthropic", "custom"}
	for _, name := range expected {
		assert.Contains(t, providers, name, "provider %s harus tersedia", name)
	}

	// Verify ollama does not need API key
	assert.False(t, providers["ollama"].NeedsAPIKey)
	assert.NotEmpty(t, providers["ollama"].Models)

	// Verify openai needs API key
	assert.True(t, providers["openai"].NeedsAPIKey)
	assert.NotEmpty(t, providers["openai"].Models)
}

func TestNewProvider_Ollama(t *testing.T) {
	p, err := NewProvider(ProviderConfig{Name: "ollama", Model: "llama3.1", Host: "http://localhost:11434"})
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "ollama", p.Name())
}

func TestNewProvider_OpenAI_MissingKey(t *testing.T) {
	_, err := NewProvider(ProviderConfig{Name: "openai", Model: "gpt-4"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OpenAI memerlukan API key")
}

func TestNewProvider_OpenRouter_MissingKey(t *testing.T) {
	_, err := NewProvider(ProviderConfig{Name: "openrouter", Model: "anthropic/claude"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OpenRouter memerlukan API key")
}

func TestNewProvider_Anthropic_MissingKey(t *testing.T) {
	_, err := NewProvider(ProviderConfig{Name: "anthropic", Model: "claude-sonnet-4"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Anthropic memerlukan API key")
}

func TestNewProvider_Custom_MissingKeyAndHost(t *testing.T) {
	_, err := NewProvider(ProviderConfig{Name: "custom", Model: "custom", APIKey: "key"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Custom provider memerlukan API key dan base URL")

	_, err = NewProvider(ProviderConfig{Name: "custom", Model: "custom", Host: "http://localhost"})
	require.Error(t, err)
}

func TestNewProvider_Custom_Success(t *testing.T) {
	p, err := NewProvider(ProviderConfig{Name: "custom", Model: "custom", APIKey: "key", Host: "http://localhost:8080"})
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestNewProvider_OpenAI_Success(t *testing.T) {
	p, err := NewProvider(ProviderConfig{Name: "openai", Model: "gpt-4", APIKey: "sk-test"})
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "openai", p.Name())
}

func TestNewProvider_Unknown(t *testing.T) {
	_, err := NewProvider(ProviderConfig{Name: "unknown"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider tidak dikenali")
}

func TestPhaseHint_Constants(t *testing.T) {
	assert.Equal(t, PhaseHint(0), PhaseUnknown)
	assert.Equal(t, PhaseHint(1), PhaseThinking)
	assert.Equal(t, PhaseHint(2), PhaseAnalyzing)
	assert.Equal(t, PhaseHint(3), PhaseExploring)
	assert.Equal(t, PhaseHint(4), PhaseGenerating)
}

func TestProviderConfig_Struct(t *testing.T) {
	cfg := ProviderConfig{
		Name:   "ollama",
		Model:  "qwen3",
		Host:   "http://localhost:11434",
		APIKey: "",
	}
	assert.Equal(t, "ollama", cfg.Name)
	assert.Equal(t, "qwen3", cfg.Model)
}

func TestProviderInfo_Struct(t *testing.T) {
	info := ProviderInfo{
		Name:        "test",
		Description: "test provider",
		Models:      []string{"a", "b"},
		NeedsAPIKey: true,
	}
	assert.True(t, info.NeedsAPIKey)
	assert.Len(t, info.Models, 2)
}
