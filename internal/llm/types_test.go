package llm

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRole_Constants(t *testing.T) {
	assert.Equal(t, Role("system"), RoleSystem)
	assert.Equal(t, Role("user"), RoleUser)
	assert.Equal(t, Role("assistant"), RoleAssistant)
	assert.Equal(t, Role("tool"), RoleTool)
}

func TestMessage_Struct(t *testing.T) {
	m := Message{
		Role:       RoleUser,
		Content:    "hello",
		ToolCallID: "call_123",
		ToolCalls: []ToolCall{
			{ID: "call_1", Function: "test", Args: map[string]interface{}{"x": 1}},
		},
	}
	assert.Equal(t, RoleUser, m.Role)
	assert.Equal(t, "hello", m.Content)
	assert.Equal(t, "call_123", m.ToolCallID)
	assert.Len(t, m.ToolCalls, 1)
}

func TestMessage_JSON(t *testing.T) {
	m := Message{
		Role:    RoleSystem,
		Content: "system prompt",
	}
	data, err := json.Marshal(m)
	require.NoError(t, err)

	var decoded Message
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, m.Role, decoded.Role)
	assert.Equal(t, m.Content, decoded.Content)
}

func TestChatRequest_Struct(t *testing.T) {
	req := ChatRequest{
		Model:    "gpt-4",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
		Stream:   true,
	}
	assert.Equal(t, "gpt-4", req.Model)
	assert.Len(t, req.Messages, 1)
	assert.True(t, req.Stream)
}

func TestChatResponse_Struct(t *testing.T) {
	resp := ChatResponse{
		Content:     "hello",
		Thinking:    "<think>reasoning</think>",
		Model:       "gpt-4",
		TotalTokens: 100,
	}
	assert.Equal(t, "hello", resp.Content)
	assert.Equal(t, "<think>reasoning</think>", resp.Thinking)
	assert.Equal(t, "gpt-4", resp.Model)
	assert.Equal(t, 100, resp.TotalTokens)
}

func TestChatResponse_JSON(t *testing.T) {
	resp := ChatResponse{Content: "hello", Model: "gpt-4"}
	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded ChatResponse
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, "hello", decoded.Content)
	assert.Equal(t, "gpt-4", decoded.Model)
}

func TestToolFunction_Struct(t *testing.T) {
	tf := ToolFunction{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{"type": "string"},
			},
		},
	}
	assert.Equal(t, "test_tool", tf.Name)
	assert.Equal(t, "A test tool", tf.Description)
	assert.NotNil(t, tf.Parameters)
}

func TestToolFunction_JSON(t *testing.T) {
	tf := ToolFunction{Name: "test", Description: "desc"}
	data, err := json.Marshal(tf)
	require.NoError(t, err)

	var decoded ToolFunction
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, tf.Name, decoded.Name)
	assert.Equal(t, tf.Description, decoded.Description)
}

func TestToolCall_Struct(t *testing.T) {
	tc := ToolCall{
		ID:       "call_1",
		Function: "write_file",
		Args: map[string]interface{}{
			"path":    "/tmp/test.txt",
			"content": "hello",
		},
	}
	assert.Equal(t, "call_1", tc.ID)
	assert.Equal(t, "write_file", tc.Function)
	assert.Equal(t, "/tmp/test.txt", tc.Args["path"])
}

func TestToolCall_JSON(t *testing.T) {
	tc := ToolCall{ID: "call_1", Function: "test", Args: map[string]interface{}{"x": 1}}
	data, err := json.Marshal(tc)
	require.NoError(t, err)

	var decoded ToolCall
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, tc.ID, decoded.ID)
	assert.Equal(t, tc.Function, decoded.Function)
	assert.Equal(t, float64(1), decoded.Args["x"]) // JSON numbers unmarshal as float64
}

