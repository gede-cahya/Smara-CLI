package llm

import (
	"context"
	"fmt"
	"strings"
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

// PhaseHint is a hint about what stage the LLM is currently in.
type PhaseHint int

const (
	PhaseUnknown    PhaseHint = iota
	PhaseThinking             // reasoning / <think> content
	PhaseAnalyzing            // processing / interpreting context
	PhaseExploring            // tool calls / external data gathering
	PhaseGenerating           // final response generation
)

// StreamCallback is a function called for each streamed chunk.
// If isThinking is true, the chunk is from the reasoning process (e.g., inside <think> tags).
// phaseHint indicates the current pipeline stage (when detectable).
type StreamCallback func(chunk string, isThinking bool, phaseHint PhaseHint)

// Streamer is an optional interface for providers that support real-time streaming.
type Streamer interface {
	// ChatStream sends messages and streams the response via callback.
	ChatStream(messages []Message, callback StreamCallback) (*ChatResponse, error)

	// ChatStreamWithTools sends messages with tools and streams the text response.
	// Tool calls are generally collected and returned at the end.
	ChatStreamWithTools(messages []Message, tools []ToolFunction, callback StreamCallback) (*ChatResponse, []ToolCall, error)
}

// ContextStreamer is implemented by streaming providers that can cancel
// in-flight HTTP/SSE requests when the agent turn times out.
type ContextStreamer interface {
	ChatStreamWithContext(ctx context.Context, messages []Message, callback StreamCallback) (*ChatResponse, error)
	ChatStreamWithToolsWithContext(ctx context.Context, messages []Message, tools []ToolFunction, callback StreamCallback) (*ChatResponse, []ToolCall, error)
}

type ImageGenerator interface {
	GenerateImage(prompt string, opts ImageGenerationOptions) (*ImageGenerationResult, error)
}

// ImageGeneratorWithContext is optionally implemented by image providers that can
// cancel in-flight HTTP requests via context cancellation.
type ImageGeneratorWithContext interface {
	GenerateImageWithContext(ctx context.Context, prompt string, opts ImageGenerationOptions) (*ImageGenerationResult, error)
}

// ImageEditor is implemented by providers that support image-to-image edits.
type ImageEditor interface {
	EditImage(imagePath, prompt string, opts ImageEditOptions) (*ImageGenerationResult, error)
}

// ImageEditorWithContext is optionally implemented by image providers that can
// cancel in-flight HTTP requests via context cancellation.
type ImageEditorWithContext interface {
	EditImageWithContext(ctx context.Context, imagePath, prompt string, opts ImageEditOptions) (*ImageGenerationResult, error)
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
			Models:      []string{"gpt-4o", "gpt-4o-mini", "o1", "o3-mini", "gpt-image-2"},
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
			Description: "9router (OpenAI-compatible) via local proxy — semua model tersedia",
			Models: []string{
				// Combo (direct)
				"mimo", "glm5", "dsv4", "minimaxm3",
				// cx (premium)
				"cx/gpt-5.5", "cx/gpt-5.4", "cx/gpt-5.3-codex", "cx/gpt-5.2", "cx/gpt-5.1", "cx/gpt-5-codex",
				// sr (shared)
				"sr/gpt-5.5", "sr/gpt-5.4", "sr/claude-opus-4-7", "sr/claude-sonnet-4-6", "sr/deepseek-v4-pro", "sr/gemini-2.5-pro", "sr/glm-5", "sr/kimi-k2.7", "sr/minimax-m3", "sr/mimo-v2.5-pro", "sr/qwen3.7-plus",
				// bai
				"bai/gpt-5.5", "bai/claude-opus-4.8", "bai/gemini-3.1-pro", "bai/deepseek-v4-pro", "bai/glm-5.1", "bai/minimax-m3",
				// tr (together)
				"tr/openai/gpt-5.5", "tr/anthropic/claude-opus-4.8", "tr/google/gemini-3.1-pro-preview", "tr/deepseek/deepseek-v4-pro", "tr/z-ai/glm-5.1", "tr/x-ai/grok-4.3",
				// tk
				"tk/gpt-5.5", "tk/claude-opus-4-8", "tk/gemini-3.1-pro", "tk/deepseek-v4-pro", "tk/glm-5.2", "tk/minimax-m3",
				// ae (anthropic endpoints)
				"ae/claude-opus-4-8", "ae/claude-fable-5", "ae/claude-sonnet-4-6",
				// gemini
				"gemini/gemini-3.1-pro-preview", "gemini/gemini-3-flash-preview",
				// glm
				"glm/glm-5.1", "glm/glm-5", "glm/glm-4.7",
				// mimo
				"mimo/mimo-v2.5-pro", "mimo/mimo-v2.5",
				// kr
				"kr/claude-sonnet-4.5", "kr/deepseek-3.2", "kr/qwen3-coder-next",
				// nara (free)
				"nara/mimo-v2.5-free", "nara/mistral-large",
				// Image models
				"gpt-image-2", "gemini-2.5-flash-image", "gemini-3.1-flash-image-preview",
			},
			NeedsAPIKey: true,
		},
	}
}

// ProviderConfig holds the parameters to create a provider.
type ProviderConfig struct {
	Name            string
	Model           string
	Host            string
	APIKey          string
	ReasoningEffort string
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
		return NewOpenAIProvider(cfg.APIKey, cfg.Model, cfg.Host, cfg.ReasoningEffort), nil
	case "openrouter":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("OpenRouter memerlukan API key — jalankan 'smara login --provider openrouter'")
		}
		return NewOpenRouterProvider(cfg.APIKey, cfg.Model, cfg.Host, cfg.ReasoningEffort), nil
	case "anthropic":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("Anthropic memerlukan API key — jalankan 'smara login --provider anthropic'")
		}
		return NewAnthropicProvider(cfg.APIKey, cfg.Model, cfg.Host), nil
	case "custom":
		if cfg.APIKey == "" || cfg.Host == "" {
			return nil, fmt.Errorf("Custom provider memerlukan API key dan base URL — jalankan 'smara login --custom'")
		}
		return NewCustomProvider("custom", cfg.APIKey, cfg.Model, cfg.Host, cfg.ReasoningEffort), nil
	default:
		return nil, fmt.Errorf("provider tidak dikenali: %s (tersedia: ollama, openai, openrouter, anthropic, custom)", cfg.Name)
	}
}

// ModelSupportsNativeToolCall returns true if the model supports native
// function calling (tool_calls in OpenAI format). Models that don't support
// native tool calls will use DSML text-based tool calling as fallback.
func ModelSupportsNativeToolCall(model string) bool {
	model = strings.ToLower(model)

	// Models that support native function calling
	nativePrefixes := []string{
		// OpenAI GPT family
		"cx/gpt-", "sr/gpt-", "bai/gpt-", "tr/openai/gpt-", "tk/gpt-", "gpt-",
		// Claude family
		"sr/claude-", "bai/claude-", "tr/anthropic/claude-", "ae/claude-", "tk/claude-", "kr/claude-", "cl/anthropic/claude-", "claude-",
		// Gemini family
		"gemini/", "sr/gemini-", "bai/gemini-", "tr/google/gemini-", "tk/gemini-", "cl/google/gemini-", "gemini-",
		// GLM-5+ (supports function calling)
		"glm/glm-5", "glm/glm-4.7", "sr/glm-5", "bai/glm-5", "tr/z-ai/glm-5", "tk/glm-5", "glm5",
		// Grok
		"tr/x-ai/grok-",
		// MiniMax M3 (newer models support it)
		"minimaxm3", "sr/minimax-m3", "tk/minimax-m3",
		// mimo (supports function calling)
		"mimo/mimo-v2.5", "mimo",
		// qwen3.7+ (supports function calling)
		"sr/qwen3.7", "tk/qwen3.7", "tr/qwen/qwen3.7",
	}

	for _, prefix := range nativePrefixes {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}

	// Models that DON'T support native function calling (use DSML fallback)
	// DeepSeek, older Qwen, MiniMax M2.x, Kimi, etc.
	return false
}
