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
		Messages: c.convertMessages(messages),
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
		Messages: c.convertMessages(messages),
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

func (c *CustomProvider) GenerateEmbedding(text string) ([]float32, error) {
	reqBody := openAIEmbedRequest{
		Model: "text-embedding-3-small",
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
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error (status %d): %s", resp.StatusCode, string(body))
	}

	var embedResp openAIEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, fmt.Errorf("gagal decode embed response: %w", err)
	}

	if len(embedResp.Data) == 0 {
		return nil, fmt.Errorf("tidak ada embedding yang dikembalikan")
	}

	return embedResp.Data[0].Embedding, nil
}

func (c *CustomProvider) convertMessages(messages []Message) []openAIMessage {
	om := make([]openAIMessage, len(messages))
	for i, m := range messages {
		msg := openAIMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		for j, tc := range m.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Args)
			msg.ToolCalls = append(msg.ToolCalls, openAIToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: openAIToolCallFunc{
					Name:      tc.Function,
					Arguments: string(argsJSON),
				},
			})
			_ = j
		}
		om[i] = msg
	}
	return om
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
