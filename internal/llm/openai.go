package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenAIProvider implements the Provider interface for OpenAI API.
type OpenAIProvider struct {
	apiKey string
	model  string
	host   string // default: https://api.openai.com/v1
	client *http.Client
}

// OpenAI API request/response structures
type openAIChatRequest struct {
	Model       string           `json:"model"`
	Messages    []openAIMessage  `json:"messages"`
	Tools       []openAITool     `json:"tools,omitempty"`
	Stream      bool             `json:"stream"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAITool struct {
	Type     string          `json:"type"`
	Function openAIFunction  `json:"function"`
}

type openAIFunction struct {
	Name       string                 `json:"name"`
	Description string                `json:"description"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

type openAIToolCall struct {
	ID       string                `json:"id"`
	Type     string                `json:"type"`
	Function openAIToolCallFunc    `json:"function"`
}

type openAIToolCallFunc struct {
	Name      string                 `json:"name"`
	Arguments string                 `json:"arguments"`
}

type openAIChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int             `json:"index"`
		Message      openAIMessage   `json:"message"`
		FinishReason string          `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type openAIEmbedRequest struct {
	Model string   `json:"model"`
	Input string   `json:"input"`
}

type openAIEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// NewOpenAIProvider creates a new OpenAI provider.
func NewOpenAIProvider(apiKey, model, host string) *OpenAIProvider {
	if model == "" {
		model = "gpt-4o"
	}
	if host == "" {
		host = "https://api.openai.com/v1"
	}
	return &OpenAIProvider{
		apiKey: apiKey,
		model:  model,
		host:   host,
		client: &http.Client{Timeout: 5 * time.Minute},
	}
}

func (o *OpenAIProvider) Name() string {
	return "openai"
}

func (o *OpenAIProvider) Chat(messages []Message) (*ChatResponse, error) {
	req := openAIChatRequest{
		Model:    o.model,
		Messages: o.convertMessages(messages),
		Stream:   false,
	}

	resp, err := o.doChat(req)
	if err != nil {
		return nil, err
	}

	return o.parseChatResponse(resp)
}

func (o *OpenAIProvider) ChatWithTools(messages []Message, tools []ToolFunction) (*ChatResponse, []ToolCall, error) {
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
		Messages: o.convertMessages(messages),
		Tools:    openAITools,
		Stream:   false,
	}

	body, err := o.doChat(req)
	if err != nil {
		return nil, nil, err
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, nil, fmt.Errorf("gagal decode OpenAI response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, nil, fmt.Errorf("OpenAI response kosong")
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

func (o *OpenAIProvider) GenerateEmbedding(text string) ([]float32, error) {
	reqBody := openAIEmbedRequest{
		Model: "text-embedding-3-small",
		Input: text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("gagal marshal embed request: %w", err)
	}

	req, err := http.NewRequest("POST", o.host+"/embeddings", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gagal menghubungi OpenAI: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI error (status %d): %s", resp.StatusCode, string(body))
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

// convertMessages converts internal messages to OpenAI format.
func (o *OpenAIProvider) convertMessages(messages []Message) []openAIMessage {
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

func (o *OpenAIProvider) doChat(req openAIChatRequest) ([]byte, error) {
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

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gagal menghubungi OpenAI: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI error (status %d): %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

func (o *OpenAIProvider) parseChatResponse(body []byte) (*ChatResponse, error) {
	var chatResp openAIChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("gagal decode OpenAI response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("OpenAI response kosong")
	}

	return &ChatResponse{
		Content:     chatResp.Choices[0].Message.Content,
		Model:       chatResp.Model,
		TotalTokens: chatResp.Usage.TotalTokens,
	}, nil
}
