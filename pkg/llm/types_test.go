package llm

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRoleConstants(t *testing.T) {
	assert.Equal(t, Role("system"), RoleSystem)
	assert.Equal(t, Role("user"), RoleUser)
	assert.Equal(t, Role("assistant"), RoleAssistant)
	assert.Equal(t, Role("tool"), RoleTool)
}

func TestMessage_MarshalJSON(t *testing.T) {
	m := Message{
		Role:       RoleUser,
		Content:    "hello",
		ToolCallID: "tc_123",
		ToolCalls:  []ToolCall{{ID: "tc_1", Function: "test_func", Args: map[string]interface{}{"key": "val"}}},
	}
	data, err := json.Marshal(m)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"role":"user"`)
	assert.Contains(t, string(data), `"content":"hello"`)
	assert.Contains(t, string(data), `"tool_call_id":"tc_123"`)
}

func TestMessage_UnmarshalJSON(t *testing.T) {
	jsonData := `{"role":"assistant","content":"hi","tool_calls":[{"id":"1","function":"f"}]}`
	var m Message
	err := json.Unmarshal([]byte(jsonData), &m)
	assert.NoError(t, err)
	assert.Equal(t, RoleAssistant, m.Role)
	assert.Equal(t, "hi", m.Content)
	assert.Len(t, m.ToolCalls, 1)
	assert.Equal(t, "1", m.ToolCalls[0].ID)
}

func TestChatRequest_MarshalJSON(t *testing.T) {
	cr := ChatRequest{
		Model:    "gpt-4",
		Messages: []Message{{Role: RoleSystem, Content: "sys"}},
		Stream:   true,
	}
	data, err := json.Marshal(cr)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"model":"gpt-4"`)
	assert.Contains(t, string(data), `"stream":true`)
}

func TestChatResponse_MarshalJSON(t *testing.T) {
	cr := ChatResponse{
		Content:     "output",
		Thinking:    "thoughts",
		Model:       "model-x",
		TotalTokens: 42,
	}
	data, err := json.Marshal(cr)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"content":"output"`)
	assert.Contains(t, string(data), `"thinking":"thoughts"`)
	assert.Contains(t, string(data), `"total_tokens":42`)
}

func TestToolCall_MarshalJSON(t *testing.T) {
	tc := ToolCall{
		ID:       "call_1",
		Function: "read_file",
		Args:     map[string]interface{}{"path": "/tmp/test"},
	}
	data, err := json.Marshal(tc)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"id":"call_1"`)
	assert.Contains(t, string(data), `"function":"read_file"`)
	assert.Contains(t, string(data), `"arguments"`)
}

func TestToolFunction_MarshalJSON(t *testing.T) {
	tf := ToolFunction{
		Name:        "write_file",
		Description: "Write to file",
		Parameters:  map[string]interface{}{"type": "object"},
	}
	data, err := json.Marshal(tf)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"name":"write_file"`)
}

func TestMessage_OmitEmpty(t *testing.T) {
	m := Message{Role: RoleUser, Content: "test"}
	data, err := json.Marshal(m)
	assert.NoError(t, err)
	assert.NotContains(t, string(data), "tool_call_id")
	assert.NotContains(t, string(data), "tool_calls")
}
