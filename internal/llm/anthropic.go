package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AnthropicProvider implements the Provider interface for Anthropic API.
type AnthropicProvider struct {
	apiKey     string
	model      string
	host       string // default: https://api.anthropic.com
	client     *http.Client
}

// Anthropic API request/response structures
type anthropicChatRequest struct {
	Model       string               `json:"model"`
	Messages    []anthropicMessage   `json:"messages"`
	System      string               `json:"system,omitempty"`
	MaxTokens   int                  `json:"max_tokens"`
	Tools       []anthropicTool      `json:"tools,omitempty"`
	Stream      bool                 `json:"stream"`
}

type anthropicMessage struct {
	Role    string                 `json:"role"`
	Content interface{}            `json:"content"` // string or array of content blocks
}

type anthropicTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicToolUseBlock struct {
	Type string                 `json:"type"`
	ID   string                 `json:"id"`
	Name string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

type anthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type anthropicChatResponse struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Model string `json:"model"`
	Content []struct {
		Type  string                 `json:"type"`
		Text  string                 `json:"text,omitempty"`
		ID    string                 `json:"id,omitempty"`
		Name  string                 `json:"name,omitempty"`
		Input map[string]interface{} `json:"input,omitempty"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// NewAnthropicProvider creates a new Anthropic provider.
func NewAnthropicProvider(apiKey, model, host string) *AnthropicProvider {
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	if host == "" {
		host = "https://api.anthropic.com"
	}
	return &AnthropicProvider{
		apiKey:   apiKey,
		model:    model,
		host:     host,
		client:   &http.Client{Timeout: 5 * time.Minute},
	}
}

func (a *AnthropicProvider) Name() string {
	return "anthropic"
}

func (a *AnthropicProvider) Chat(messages []Message) (*ChatResponse, error) {
	req, systemMsg := a.buildChatRequest(messages, nil)
	return a.doChat(req, systemMsg)
}

func (a *AnthropicProvider) ChatWithTools(messages []Message, tools []ToolFunction) (*ChatResponse, []ToolCall, error) {
	anthropicTools := make([]anthropicTool, len(tools))
	for i, t := range tools {
		schema := t.Parameters
		if schema == nil {
			schema = map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{},
			}
		}
		anthropicTools[i] = anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		}
	}

	req, systemMsg := a.buildChatRequest(messages, anthropicTools)
	return a.doChatWithTools(req, systemMsg)
}

// Anthropic doesn't have a native embeddings API accessible via the main API key.
// GenerateEmbedding returns nil — use Ollama or OpenAI for embeddings when using Anthropic.
func (a *AnthropicProvider) GenerateEmbedding(text string) ([]float32, error) {
	return nil, fmt.Errorf("Anthropic tidak mendukung embeddings — gunakan Ollama atau OpenAI untuk embeddings")
}

// buildChatRequest converts internal messages to Anthropic format.
func (a *AnthropicProvider) buildChatRequest(messages []Message, tools []anthropicTool) (anthropicChatRequest, string) {
	req := anthropicChatRequest{
		Model:     a.model,
		MaxTokens: 8192,
		Stream:    false,
		Tools:     tools,
	}

	var systemParts []string
	var apiMessages []anthropicMessage

	for _, m := range messages {
		switch m.Role {
		case RoleSystem:
			systemParts = append(systemParts, m.Content)
		case RoleTool:
			// Tool result — send as user message with tool_use_id
			apiMessages = append(apiMessages, anthropicMessage{
				Role: "user",
				Content: []map[string]interface{}{
					{
						"type":       "tool_result",
						"tool_use_id": m.ToolCallID,
						"content":    m.Content,
					},
				},
			})
		case RoleAssistant:
			if len(m.ToolCalls) > 0 {
				// Assistant requested tool calls
				blocks := make([]interface{}, 0, len(m.ToolCalls)+1)
				if m.Content != "" {
					blocks = append(blocks, anthropicTextBlock{Type: "text", Text: m.Content})
				}
				for _, tc := range m.ToolCalls {
					blocks = append(blocks, anthropicToolUseBlock{
						Type:  "tool_use",
						ID:    tc.ID,
						Name:  tc.Function,
						Input: tc.Args,
					})
				}
				apiMessages = append(apiMessages, anthropicMessage{
					Role:    "assistant",
					Content: blocks,
				})
			} else {
				apiMessages = append(apiMessages, anthropicMessage{
					Role:    "assistant",
					Content: m.Content,
				})
			}
		case RoleUser:
			apiMessages = append(apiMessages, anthropicMessage{
				Role:    "user",
				Content: m.Content,
			})
		}
	}

	req.Messages = apiMessages
	req.System = strings.Join(systemParts, "\n\n")
	return req, req.System
}

func (a *AnthropicProvider) doChat(req anthropicChatRequest, systemMsg string) (*ChatResponse, error) {
	// Clear tools for simple chat
	req.Tools = nil

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("gagal marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", a.host+"/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gagal menghubungi Anthropic: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Anthropic error (status %d): %s", resp.StatusCode, string(body))
	}

	var apiResp anthropicChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("gagal decode Anthropic response: %w", err)
	}

	var content string
	for _, block := range apiResp.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}

	return &ChatResponse{
		Content:     content,
		Model:       apiResp.Model,
		TotalTokens: apiResp.Usage.InputTokens + apiResp.Usage.OutputTokens,
	}, nil
}

func (a *AnthropicProvider) doChatWithTools(req anthropicChatRequest, systemMsg string) (*ChatResponse, []ToolCall, error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("gagal marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", a.host+"/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("gagal menghubungi Anthropic: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("Anthropic error (status %d): %s", resp.StatusCode, string(body))
	}

	var apiResp anthropicChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, nil, fmt.Errorf("gagal decode Anthropic response: %w", err)
	}

	var content string
	var toolCalls []ToolCall
	for _, block := range apiResp.Content {
		if block.Type == "text" {
			content += block.Text
		} else if block.Type == "tool_use" {
			toolCalls = append(toolCalls, ToolCall{
				ID:       block.ID,
				Function: block.Name,
				Args:     block.Input,
			})
		}
	}

	return &ChatResponse{
		Content:     content,
		Model:       apiResp.Model,
		TotalTokens: apiResp.Usage.InputTokens + apiResp.Usage.OutputTokens,
	}, toolCalls, nil
}
