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
	Tools    []ollamaTool    `json:"tools,omitempty"`
}

type ollamaMessage struct {
	Role       string                    `json:"role"`
	Content    string                    `json:"content,omitempty"`
	ToolCalls  []ollamaToolCall          `json:"tool_calls,omitempty"`
	ToolCallID string                    `json:"tool_call_id,omitempty"`
}

// ollamaTool represents a tool definition sent to Ollama.
type ollamaTool struct {
	Type     string          `json:"type"`
	Function ollamaToolFunc  `json:"function"`
}

type ollamaToolFunc struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ollamaToolCall represents a tool call requested by the assistant.
type ollamaToolCall struct {
	Function ollamaToolCallFunc `json:"function"`
}

type ollamaToolCallFunc struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type ollamaChatResponse struct {
	Message struct {
		Role      string             `json:"role"`
		Content   string             `json:"content"`
		ToolCalls []ollamaToolCall   `json:"tool_calls,omitempty"`
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
		om := ollamaMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		// Convert tool calls to Ollama format
		for _, tc := range m.ToolCalls {
			om.ToolCalls = append(om.ToolCalls, ollamaToolCall{
				Function: ollamaToolCallFunc{
					Name:      tc.Function,
					Arguments: tc.Args,
				},
			})
		}
		ollamaMessages[i] = om
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

// ChatWithTools sends messages with tool definitions for function calling.
func (o *OllamaProvider) ChatWithTools(messages []Message, tools []ToolFunction) (*ChatResponse, []ToolCall, error) {
	// Convert to Ollama format
	ollamaMessages := make([]ollamaMessage, len(messages))
	for i, m := range messages {
		om := ollamaMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			om.ToolCalls = append(om.ToolCalls, ollamaToolCall{
				Function: ollamaToolCallFunc{
					Name:      tc.Function,
					Arguments: tc.Args,
				},
			})
		}
		ollamaMessages[i] = om
	}

	// Convert tools to Ollama format
	ollamaTools := make([]ollamaTool, len(tools))
	for i, t := range tools {
		ollamaTools[i] = ollamaTool{
			Type: "function",
			Function: ollamaToolFunc{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		}
	}

	reqBody := ollamaChatRequest{
		Model:    o.model,
		Messages: ollamaMessages,
		Stream:   false,
		Tools:    ollamaTools,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("gagal marshal request: %w", err)
	}

	resp, err := o.client.Post(
		o.host+"/api/chat",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("gagal menghubungi Ollama di %s: %w", o.host, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("ollama error (status %d): %s", resp.StatusCode, string(body))
	}

	var ollamaResp ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, nil, fmt.Errorf("gagal decode response: %w", err)
	}

	// Parse tool calls from response
	var toolCalls []ToolCall
	for i, tc := range ollamaResp.Message.ToolCalls {
		toolCalls = append(toolCalls, ToolCall{
			ID:       fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), i),
			Function: tc.Function.Name,
			Args:     tc.Function.Arguments,
		})
	}

	return &ChatResponse{
		Content:     ollamaResp.Message.Content,
		Model:       ollamaResp.Model,
		TotalTokens: ollamaResp.EvalCount,
	}, toolCalls, nil
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
