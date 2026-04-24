package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// CustomProvider implements the Provider interface for custom OpenAI-compatible APIs.
type CustomProvider struct {
	name    string
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewCustomProvider creates a new custom provider.
func NewCustomProvider(name, apiKey, model, baseURL string) *CustomProvider {
	if model == "" {
		model = "gpt-4o"
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &CustomProvider{
		name:    name,
		apiKey:  apiKey,
		model:   model,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

func (c *CustomProvider) Name() string {
	return c.name
}

func (c *CustomProvider) Chat(messages []Message) (*ChatResponse, error) {
	req := openAIChatRequest{
		Model:    c.model,
		Messages: convertMessagesToOpenAI(messages),
		Stream:   false,
	}

	resp, err := c.doChat(req)
	if err != nil {
		return nil, err
	}

	return c.parseChatResponse(resp)
}

func (c *CustomProvider) ChatWithTools(messages []Message, tools []ToolFunction) (*ChatResponse, []ToolCall, error) {
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
		Model:    c.model,
		Messages: convertMessagesToOpenAI(messages),
		Tools:    openAITools,
		Stream:   false,
	}

	body, err := c.doChat(req)
	if err != nil {
		return nil, nil, err
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, nil, fmt.Errorf("gagal decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, nil, fmt.Errorf("response kosong")
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
func (c *CustomProvider) ChatStream(messages []Message, callback StreamCallback) (*ChatResponse, error) {
	req := openAIChatRequest{
		Model:    c.model,
		Messages: convertMessagesToOpenAI(messages),
	}
	resp, _, err := streamOpenAI(c.client, c.baseURL, c.apiKey, req, callback)
	return resp, err
}

// ChatStreamWithTools implements the Streamer interface.
func (c *CustomProvider) ChatStreamWithTools(messages []Message, tools []ToolFunction, callback StreamCallback) (*ChatResponse, []ToolCall, error) {
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
		Model:    c.model,
		Messages: convertMessagesToOpenAI(messages),
		Tools:    openAITools,
	}
	return streamOpenAI(c.client, c.baseURL, c.apiKey, req, callback)
}

func (c *CustomProvider) GenerateEmbedding(text string) ([]float32, error) {
	// Fallback mechanism for embedding model
	model := "text-embedding-3-small"
	if c.model == "minimax-auto" {
		// If using the proxy auto-model, we might need a different embedding model
		// or let the proxy handle it. For now, we try to be more generic.
		model = "text-embedding-ada-002" 
	}

	reqBody := openAIEmbedRequest{
		Model: model,
		Input: text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("gagal marshal embed request: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL+"/embeddings", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gagal menghubungi provider: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Log and return nil error to avoid breaking the chat flow if embeddings are not available
		// This is common in custom/local providers
		return nil, nil 
	}

	var embedResp openAIEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, nil // Graceful fallback
	}

	if len(embedResp.Data) == 0 {
		return nil, nil
	}

	return embedResp.Data[0].Embedding, nil
}

func (c *CustomProvider) doChat(req openAIChatRequest) ([]byte, error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("gagal marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gagal menghubungi provider: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error (status %d): %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

func (c *CustomProvider) parseChatResponse(body []byte) (*ChatResponse, error) {
	var chatResp openAIChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("gagal decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("response kosong")
	}

	return &ChatResponse{
		Content:     chatResp.Choices[0].Message.Content,
		Model:       chatResp.Model,
		TotalTokens: chatResp.Usage.TotalTokens,
	}, nil
}
