package llm

import (
	"fmt"
)

// Provider is the interface that all LLM backends must implement.
type Provider interface {
	// Name returns the provider name (e.g. "ollama", "openai").
	Name() string

	// Chat sends messages to the LLM and returns the response.
	Chat(messages []Message) (*ChatResponse, error)

	// ChatWithTools sends messages with available tools for function calling.
	ChatWithTools(messages []Message, tools []ToolFunction) (*ChatResponse, []ToolCall, error)

	// GenerateEmbedding creates a vector embedding from the input text.
	GenerateEmbedding(text string) ([]float32, error)
}

// ProviderInfo describes an available provider.
type ProviderInfo struct {
	Name        string
	Description string
	Models      []string
	NeedsAPIKey bool
}

// AvailableProviders returns metadata for all supported providers.
func AvailableProviders() map[string]ProviderInfo {
	return map[string]ProviderInfo{
		"ollama": {
			Name:        "ollama",
			Description: "Local LLM via Ollama (no API key needed)",
			Models:      []string{"minimax-m2.5:cloud", "qwen3.6:latest", "llama3.1:latest", "deepseek-r1:latest", "qwq:latest", "mistral:latest"},
			NeedsAPIKey: false,
		},
		"openai": {
			Name:        "openai",
			Description: "OpenAI API (requires API key)",
			Models:      []string{"gpt-4o", "gpt-4o-mini", "o1", "o3-mini"},
			NeedsAPIKey: true,
		},
		"openrouter": {
			Name:        "openrouter",
			Description: "OpenRouter multi-model gateway (requires API key)",
			Models:      []string{"anthropic/claude-sonnet-4", "openai/gpt-4o", "meta-llama/llama-3.3-70b-instruct", "google/gemini-2.5-pro"},
			NeedsAPIKey: true,
		},
		"anthropic": {
			Name:        "anthropic",
			Description: "Anthropic Claude API (requires API key)",
			Models:      []string{"claude-sonnet-4-20250514", "claude-opus-4-20250514", "claude-haiku-4-20250514"},
			NeedsAPIKey: true,
		},
		"custom": {
			Name:        "custom",
			Description: "Custom OpenAI-compatible API (requires base URL & API key)",
			Models:      []string{"custom"},
			NeedsAPIKey: true,
		},
	}
}

// ProviderConfig holds the parameters to create a provider.
type ProviderConfig struct {
	Name   string
	Model  string
	Host   string
	APIKey string
}

// NewProvider creates an LLM provider based on the given configuration.
func NewProvider(cfg ProviderConfig) (Provider, error) {
	switch cfg.Name {
	case "ollama":
		return NewOllamaProvider(cfg.Model, cfg.Host), nil
	case "openai":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("OpenAI memerlukan API key — jalankan 'smara login --provider openai'")
		}
		return NewOpenAIProvider(cfg.APIKey, cfg.Model, cfg.Host), nil
	case "openrouter":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("OpenRouter memerlukan API key — jalankan 'smara login --provider openrouter'")
		}
		return NewOpenRouterProvider(cfg.APIKey, cfg.Model, cfg.Host), nil
	case "anthropic":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("Anthropic memerlukan API key — jalankan 'smara login --provider anthropic'")
		}
		return NewAnthropicProvider(cfg.APIKey, cfg.Model, cfg.Host), nil
	case "custom":
		if cfg.APIKey == "" || cfg.Host == "" {
			return nil, fmt.Errorf("Custom provider memerlukan API key dan base URL — jalankan 'smara login --custom'")
		}
		return NewCustomProvider("custom", cfg.APIKey, cfg.Model, cfg.Host), nil
	default:
		return nil, fmt.Errorf("provider tidak dikenali: %s (tersedia: ollama, openai, openrouter, anthropic, custom)", cfg.Name)
	}
}
