package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenRouterProvider implements the Provider interface for OpenRouter API.
// OpenRouter is an OpenAI-compatible gateway that supports 100+ models.
type OpenRouterProvider struct {
	apiKey string
	model  string
	host   string // default: https://openrouter.ai/api/v1
	client *http.Client
}

// NewOpenRouterProvider creates a new OpenRouter provider.
func NewOpenRouterProvider(apiKey, model, host string) *OpenRouterProvider {
	if model == "" {
		model = "anthropic/claude-sonnet-4"
	}
	if host == "" {
		host = "https://openrouter.ai/api/v1"
	}
	return &OpenRouterProvider{
		apiKey: apiKey,
		model:  model,
		host:   host,
		client: &http.Client{Timeout: 5 * time.Minute},
	}
}

func (o *OpenRouterProvider) Name() string {
	return "openrouter"
}

func (o *OpenRouterProvider) Chat(messages []Message) (*ChatResponse, error) {
	req := openAIChatRequest{
		Model:    o.model,
		Messages: convertMessagesToOpenAI(messages),
		Stream:   false,
	}

	resp, err := o.doChat(req)
	if err != nil {
		return nil, err
	}

	return o.parseChatResponse(resp)
}

func (o *OpenRouterProvider) ChatWithTools(messages []Message, tools []ToolFunction) (*ChatResponse, []ToolCall, error) {
	openAITools := make([]openAITool, len(tools))
	for i, t := range tools {
		openAITools[i] = openAITool{
			Type: "function",
			Function: openAIFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		}
	}

	req := openAIChatRequest{
		Model:    o.model,
		Messages: convertMessagesToOpenAI(messages),
		Tools:    openAITools,
		Stream:   false,
	}

	body, err := o.doChat(req)
	if err != nil {
		return nil, nil, err
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, nil, fmt.Errorf("gagal decode OpenRouter response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, nil, fmt.Errorf("OpenRouter response kosong")
	}

	choice := chatResp.Choices[0]
	msg := choice.Message

	var toolCalls []ToolCall
	for _, tc := range msg.ToolCalls {
		var args map[string]interface{}
		json.Unmarshal([]byte(tc.Function.Arguments), &args)
		toolCalls = append(toolCalls, ToolCall{
			ID:       tc.ID,
			Function: tc.Function.Name,
			Args:     args,
		})
	}

	return &ChatResponse{
		Content:     msg.Content,
		Model:       chatResp.Model,
		TotalTokens: chatResp.Usage.TotalTokens,
	}, toolCalls, nil
}

// ChatStream implements the Streamer interface.
func (o *OpenRouterProvider) ChatStream(messages []Message, callback StreamCallback) (*ChatResponse, error) {
	req := openAIChatRequest{
		Model:    o.model,
		Messages: convertMessagesToOpenAI(messages),
	}
	resp, _, err := streamOpenAI(o.client, o.host, o.apiKey, req, callback)
	return resp, err
}

// ChatStreamWithTools implements the Streamer interface.
func (o *OpenRouterProvider) ChatStreamWithTools(messages []Message, tools []ToolFunction, callback StreamCallback) (*ChatResponse, []ToolCall, error) {
	openAITools := make([]openAITool, len(tools))
	for i, t := range tools {
		openAITools[i] = openAITool{
			Type: "function",
			Function: openAIFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		}
	}

	req := openAIChatRequest{
		Model:    o.model,
		Messages: convertMessagesToOpenAI(messages),
		Tools:    openAITools,
	}
	return streamOpenAI(o.client, o.host, o.apiKey, req, callback)
}

// OpenRouter doesn't have native embeddings.
func (o *OpenRouterProvider) GenerateEmbedding(text string) ([]float32, error) {
	return nil, fmt.Errorf("OpenRouter tidak mendukung embeddings — gunakan Ollama atau OpenAI untuk embeddings")
}

func (o *OpenRouterProvider) doChat(req openAIChatRequest) ([]byte, error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("gagal marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", o.host+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	httpReq.Header.Set("HTTP-Referer", "https://github.com/gede-cahya/Smara-CLI")
	httpReq.Header.Set("X-Title", "Smara CLI")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gagal menghubungi OpenRouter: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenRouter error (status %d): %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

func (o *OpenRouterProvider) parseChatResponse(body []byte) (*ChatResponse, error) {
	var chatResp openAIChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("gagal decode OpenRouter response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("OpenRouter response kosong")
	}

	return &ChatResponse{
		Content:     chatResp.Choices[0].Message.Content,
		Model:       chatResp.Model,
		TotalTokens: chatResp.Usage.TotalTokens,
	}, nil
}
