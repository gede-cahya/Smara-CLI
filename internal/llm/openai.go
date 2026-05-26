package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenAIProvider implements the Provider interface for OpenAI API.
type OpenAIProvider struct {
	apiKey          string
	model           string
	host            string // default: https://api.openai.com/v1
	reasoningEffort string
	client          *http.Client
}

// NewOpenAIProvider creates a new OpenAI provider.
func NewOpenAIProvider(apiKey, model, host string, reasoningEffort ...string) *OpenAIProvider {
	if model == "" {
		model = "gpt-4o"
	}
	if host == "" {
		host = "https://api.openai.com/v1"
	}
	effort := ""
	if len(reasoningEffort) > 0 {
		effort = normalizeReasoningEffort(reasoningEffort[0])
	}
	return &OpenAIProvider{
		apiKey:          apiKey,
		model:           model,
		host:            host,
		reasoningEffort: effort,
		client:          &http.Client{Timeout: 15 * time.Minute},
	}
}

func (o *OpenAIProvider) Name() string {
	return "openai"
}

func (o *OpenAIProvider) GenerateImage(prompt string, opts ImageGenerationOptions) (*ImageGenerationResult, error) {
	return o.GenerateImageWithContext(context.Background(), prompt, opts)
}

func (o *OpenAIProvider) GenerateImageWithContext(ctx context.Context, prompt string, opts ImageGenerationOptions) (*ImageGenerationResult, error) {
	opts.Prompt = prompt
	return generateOpenAIImageWithContext(ctx, o.client, o.host, o.apiKey, o.model, opts)
}

func (o *OpenAIProvider) EditImage(imagePath, prompt string, opts ImageEditOptions) (*ImageGenerationResult, error) {
	return o.EditImageWithContext(context.Background(), imagePath, prompt, opts)
}

func (o *OpenAIProvider) EditImageWithContext(ctx context.Context, imagePath, prompt string, opts ImageEditOptions) (*ImageGenerationResult, error) {
	opts.Prompt = prompt
	return editOpenAIImageWithContext(ctx, o.client, o.host, o.apiKey, o.model, imagePath, opts)
}
func (o *OpenAIProvider) Chat(messages []Message) (*ChatResponse, error) {
	req := openAIChatRequest{
		Model:           o.model,
		Messages:        convertMessagesToOpenAI(messages),
		Stream:          false,
		ReasoningEffort: o.reasoningEffort,
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
		Model:           o.model,
		Messages:        convertMessagesToOpenAI(messages),
		Tools:           openAITools,
		Stream:          false,
		ReasoningEffort: o.reasoningEffort,
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

// ChatStream implements the Streamer interface.
func (o *OpenAIProvider) ChatStream(messages []Message, callback StreamCallback) (*ChatResponse, error) {
	return o.ChatStreamWithContext(context.Background(), messages, callback)
}

func (o *OpenAIProvider) ChatStreamWithContext(ctx context.Context, messages []Message, callback StreamCallback) (*ChatResponse, error) {
	req := openAIChatRequest{
		Model:           o.model,
		Messages:        convertMessagesToOpenAI(messages),
		ReasoningEffort: o.reasoningEffort,
	}
	resp, _, err := streamOpenAIWithContext(ctx, o.client, o.host, o.apiKey, req, callback)
	return resp, err
}

// ChatStreamWithTools implements the Streamer interface.
func (o *OpenAIProvider) ChatStreamWithTools(messages []Message, tools []ToolFunction, callback StreamCallback) (*ChatResponse, []ToolCall, error) {
	return o.ChatStreamWithToolsWithContext(context.Background(), messages, tools, callback)
}

func (o *OpenAIProvider) ChatStreamWithToolsWithContext(ctx context.Context, messages []Message, tools []ToolFunction, callback StreamCallback) (*ChatResponse, []ToolCall, error) {
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
		Model:           o.model,
		Messages:        convertMessagesToOpenAI(messages),
		Tools:           openAITools,
		ReasoningEffort: o.reasoningEffort,
	}
	return streamOpenAIWithContext(ctx, o.client, o.host, o.apiKey, req, callback)
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
