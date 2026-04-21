package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Shared OpenAI types (moved from openai.go)
type openAIChatRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Tools    []openAITool    `json:"tools,omitempty"`
	Stream   bool            `json:"stream"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIToolCallFunc `json:"function"`
	Index    int                `json:"index"` // Required for streaming to track which tool call is being updated
}

type openAIToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int           `json:"index"`
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type openAIEmbedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type openAIEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Streaming types
type openAIChatStreamResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Content   string           `json:"content,omitempty"`
			Role      string           `json:"role,omitempty"`
			ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
			Reasoning string           `json:"reasoning_content,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// convertMessagesToOpenAI converts internal messages to OpenAI format.
func convertMessagesToOpenAI(messages []Message) []openAIMessage {
	om := make([]openAIMessage, len(messages))
	for i, m := range messages {
		msg := openAIMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Args)
			msg.ToolCalls = append(msg.ToolCalls, openAIToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: openAIToolCallFunc{
					Name:      tc.Function,
					Arguments: string(argsJSON),
				},
			})
		}
		om[i] = msg
	}
	return om
}

// streamOpenAI is a shared helper for OpenAI-compatible streaming (OpenAI, OpenRouter, Custom).
func streamOpenAI(client *http.Client, host, apiKey string, req openAIChatRequest, callback StreamCallback) (*ChatResponse, []ToolCall, error) {
	req.Stream = true
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("gagal marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", host+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	
	// Add context headers for OpenRouter (ignored by others)
	httpReq.Header.Set("HTTP-Referer", "https://github.com/gede-cahya/Smara-CLI")
	httpReq.Header.Set("X-Title", "Smara CLI")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("gagal menghubungi API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var fullContent strings.Builder
	var fullThinking strings.Builder
	var finalModel string
	var toolCallsMap = make(map[int]*ToolCall)
	var toolCallsRawArgs = make(map[int]*strings.Builder)

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk openAIChatStreamResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if chunk.Model != "" {
			finalModel = chunk.Model
		}

		if len(chunk.Choices) > 0 {
			choice := chunk.Choices[0]
			delta := choice.Delta

			if delta.Content != "" {
				fullContent.WriteString(delta.Content)
				if callback != nil {
					callback(delta.Content, false)
				}
			}

			if delta.Reasoning != "" {
				fullThinking.WriteString(delta.Reasoning)
				if callback != nil {
					callback(delta.Reasoning, true)
				}
			}

			for _, tc := range delta.ToolCalls {
				idx := tc.Index
				if _, ok := toolCallsMap[idx]; !ok {
					toolCallsMap[idx] = &ToolCall{
						ID:       tc.ID,
						Function: tc.Function.Name,
					}
					toolCallsRawArgs[idx] = &strings.Builder{}
				}
				if tc.ID != "" {
					toolCallsMap[idx].ID = tc.ID
				}
				if tc.Function.Name != "" {
					toolCallsMap[idx].Function = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					toolCallsRawArgs[idx].WriteString(tc.Function.Arguments)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("error saat membaca stream: %w", err)
	}

	// Parse accumulated tool call arguments
	var toolCalls []ToolCall
	for i := 0; i < len(toolCallsMap); i++ {
		if tc, ok := toolCallsMap[i]; ok {
			raw := toolCallsRawArgs[i].String()
			if raw != "" {
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(raw), &args); err == nil {
					tc.Args = args
				}
			}
			toolCalls = append(toolCalls, *tc)
		}
	}

	return &ChatResponse{
		Content:  fullContent.String(),
		Thinking: fullThinking.String(),
		Model:    finalModel,
	}, toolCalls, nil
}
