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

// NewProvider creates an LLM provider based on the given name.
func NewProvider(name, model, host string) (Provider, error) {
	switch name {
	case "ollama":
		return NewOllamaProvider(model, host), nil
	default:
		return nil, fmt.Errorf("provider tidak dikenali: %s (tersedia: ollama)", name)
	}
}
