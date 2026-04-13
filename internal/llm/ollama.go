package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaProvider implements the Provider interface for the Ollama local LLM.
type OllamaProvider struct {
	model  string
	host   string
	client *http.Client
}

// Ollama API request/response structures
type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatResponse struct {
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done           bool   `json:"done"`
	Model          string `json:"model"`
	TotalDuration  int64  `json:"total_duration"`
	EvalCount      int    `json:"eval_count"`
}

type ollamaEmbedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type ollamaEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// NewOllamaProvider creates a new Ollama provider instance.
func NewOllamaProvider(model, host string) *OllamaProvider {
	if model == "" {
		model = "minimax-m2.5:cloud"
	}
	if host == "" {
		host = "http://localhost:11434"
	}
	return &OllamaProvider{
		model: model,
		host:  host,
		client: &http.Client{
			Timeout: 5 * time.Minute, // LLM generation can be slow
		},
	}
}

func (o *OllamaProvider) Name() string {
	return "ollama"
}

// Chat sends a conversation to Ollama and returns the response.
func (o *OllamaProvider) Chat(messages []Message) (*ChatResponse, error) {
	// Convert to Ollama format
	ollamaMessages := make([]ollamaMessage, len(messages))
	for i, m := range messages {
		ollamaMessages[i] = ollamaMessage{
			Role:    string(m.Role),
			Content: m.Content,
		}
	}

	reqBody := ollamaChatRequest{
		Model:    o.model,
		Messages: ollamaMessages,
		Stream:   false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("gagal marshal request: %w", err)
	}

	resp, err := o.client.Post(
		o.host+"/api/chat",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("gagal menghubungi Ollama di %s: %w", o.host, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama error (status %d): %s", resp.StatusCode, string(body))
	}

	var ollamaResp ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("gagal decode response: %w", err)
	}

	return &ChatResponse{
		Content:     ollamaResp.Message.Content,
		Model:       ollamaResp.Model,
		TotalTokens: ollamaResp.EvalCount,
	}, nil
}

// ChatWithTools is not fully supported by Ollama in MVP.
// Falls back to regular chat with tool descriptions in the system prompt.
func (o *OllamaProvider) ChatWithTools(messages []Message, tools []ToolFunction) (*ChatResponse, []ToolCall, error) {
	// For MVP: inject tool descriptions into system prompt
	if len(tools) > 0 {
		toolDesc := "Kamu memiliki akses ke tools berikut:\n"
		for _, t := range tools {
			toolDesc += fmt.Sprintf("- %s: %s\n", t.Name, t.Description)
		}
		toolDesc += "\nUntuk menggunakan tool, jawab dengan format JSON: {\"tool\": \"nama_tool\", \"args\": {...}}"

		// Prepend tool description to messages
		enhanced := make([]Message, 0, len(messages)+1)
		enhanced = append(enhanced, Message{
			Role:    RoleSystem,
			Content: toolDesc,
		})
		enhanced = append(enhanced, messages...)
		messages = enhanced
	}

	resp, err := o.Chat(messages)
	if err != nil {
		return nil, nil, err
	}

	// Try to parse tool calls from the response
	var toolCalls []ToolCall
	var rawCall struct {
		Tool string                 `json:"tool"`
		Args map[string]interface{} `json:"args"`
	}
	if err := json.Unmarshal([]byte(resp.Content), &rawCall); err == nil && rawCall.Tool != "" {
		toolCalls = append(toolCalls, ToolCall{
			ID:       fmt.Sprintf("call_%d", time.Now().UnixNano()),
			Function: rawCall.Tool,
			Args:     rawCall.Args,
		})
	}

	return resp, toolCalls, nil
}

// GenerateEmbedding creates a vector embedding using Ollama's embed API.
func (o *OllamaProvider) GenerateEmbedding(text string) ([]float32, error) {
	reqBody := ollamaEmbedRequest{
		Model: o.model,
		Input: text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("gagal marshal embed request: %w", err)
	}

	resp, err := o.client.Post(
		o.host+"/api/embed",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("gagal menghubungi Ollama embed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama embed error (status %d): %s", resp.StatusCode, string(body))
	}

	var embedResp ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, fmt.Errorf("gagal decode embed response: %w", err)
	}

	if len(embedResp.Embeddings) == 0 {
		return nil, fmt.Errorf("tidak ada embedding yang dikembalikan")
	}

	return embedResp.Embeddings[0], nil
}
