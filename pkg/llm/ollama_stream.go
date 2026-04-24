package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ChatStream sends messages to Ollama and streams the response via callback.
func (o *OllamaProvider) ChatStream(messages []Message, callback StreamCallback) (*ChatResponse, error) {
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

	reqBody := ollamaChatRequest{
		Model:    o.model,
		Messages: ollamaMessages,
		Stream:   true, // ENABLE STREAMING
		Think:    isThinkingModel(o.model),
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

	var fullContent strings.Builder
	var fullThinking strings.Builder
	var finalModel string
	var finalEvalCount int

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var chunk ollamaChatResponse
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue // skip broken chunk
		}

		if chunk.Message.Thinking != "" {
			fullThinking.WriteString(chunk.Message.Thinking)
			if callback != nil {
				callback(chunk.Message.Thinking, true)
			}
		}

		if chunk.Message.Content != "" {
			fullContent.WriteString(chunk.Message.Content)
			if callback != nil {
				callback(chunk.Message.Content, false)
			}
		}

		if chunk.Done {
			finalModel = chunk.Model
			finalEvalCount = chunk.EvalCount
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error saat membaca stream: %w", err)
	}

	return &ChatResponse{
		Content:     fullContent.String(),
		Thinking:    fullThinking.String(),
		Model:       finalModel,
		TotalTokens: finalEvalCount,
	}, nil
}

// ChatStreamWithTools sends messages with tools and streams the response.
func (o *OllamaProvider) ChatStreamWithTools(messages []Message, tools []ToolFunction, callback StreamCallback) (*ChatResponse, []ToolCall, error) {
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
		Stream:   true, // ENABLE STREAMING
		Tools:    ollamaTools,
		Think:    isThinkingModel(o.model),
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

	var fullContent strings.Builder
	var fullThinking strings.Builder
	var finalModel string
	var finalEvalCount int
	var toolCalls []ToolCall

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var chunk ollamaChatResponse
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue // skip broken chunk
		}

		// Collect tool calls
		if len(chunk.Message.ToolCalls) > 0 {
			for i, tc := range chunk.Message.ToolCalls {
				// Initialize tool call if it doesn't exist
				if i >= len(toolCalls) {
					toolCalls = append(toolCalls, ToolCall{
						ID:       fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), i),
						Function: tc.Function.Name,
						Args:     make(map[string]interface{}),
					})
				}
				// Ollama tool streaming might accumulate, but usually it returns the whole JSON for arguments
				// Wait, Ollama streaming for tool calls: it usually returns them completely when done, or streamed?
				// To be safe, we just take them when done, or overwrite.
				toolCalls[i].Function = tc.Function.Name
				toolCalls[i].Args = tc.Function.Arguments
			}
		}

		if chunk.Message.Thinking != "" {
			fullThinking.WriteString(chunk.Message.Thinking)
			if callback != nil {
				callback(chunk.Message.Thinking, true)
			}
		}

		if chunk.Message.Content != "" {
			fullContent.WriteString(chunk.Message.Content)
			if callback != nil {
				callback(chunk.Message.Content, false)
			}
		}

		if chunk.Done {
			finalModel = chunk.Model
			finalEvalCount = chunk.EvalCount
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("error saat membaca stream: %w", err)
	}

	return &ChatResponse{
		Content:     fullContent.String(),
		Thinking:    fullThinking.String(),
		Model:       finalModel,
		TotalTokens: finalEvalCount,
	}, toolCalls, nil
}
