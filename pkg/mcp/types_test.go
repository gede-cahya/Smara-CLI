package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequest_MarshalJSON(t *testing.T) {
	r := Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
		Params:  map[string]string{"filter": "test"},
	}
	data, err := json.Marshal(r)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"jsonrpc":"2.0"`)
	assert.Contains(t, string(data), `"id":1`)
	assert.Contains(t, string(data), `"method":"tools/list"`)
	assert.Contains(t, string(data), `"params"`)
}

func TestResponse_MarshalJSON(t *testing.T) {
	r := Response{
		JSONRPC: "2.0",
		ID:      1,
		Result:  json.RawMessage(`{"tools":[]}`),
	}
	data, err := json.Marshal(r)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"result":{"tools":[]}`)
}

func TestResponse_WithError(t *testing.T) {
	r := Response{
		JSONRPC: "2.0",
		ID:      2,
		Error:   &RPCError{Code: -32600, Message: "Invalid Request"},
	}
	data, err := json.Marshal(r)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"error":{"code":-32600,"message":"Invalid Request"}`)
}

func TestRPCError_Error(t *testing.T) {
	e := &RPCError{Code: -32601, Message: "Method not found"}
	assert.Equal(t, "Method not found", e.Error())
}

func TestTool_MarshalJSON(t *testing.T) {
	tool := Tool{
		Name:        "read_file",
		Description: "Read a file",
		InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
	}
	data, err := json.Marshal(tool)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"name":"read_file"`)
	assert.Contains(t, string(data), `"description":"Read a file"`)
	assert.Contains(t, string(data), `"inputSchema"`)
}

func TestToolListResult_MarshalJSON(t *testing.T) {
	result := ToolListResult{
		Tools: []Tool{
			{Name: "tool1"},
			{Name: "tool2"},
		},
	}
	data, err := json.Marshal(result)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"tools"`)
}

func TestToolCallParams_MarshalJSON(t *testing.T) {
	params := ToolCallParams{
		Name:      "write_file",
		Arguments: map[string]interface{}{"path": "/tmp/x", "content": "hello"},
	}
	data, err := json.Marshal(params)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"name":"write_file"`)
	assert.Contains(t, string(data), `"arguments"`)
}

func TestToolCallResult_MarshalJSON(t *testing.T) {
	result := ToolCallResult{
		Content: []ToolCallContent{
			{Type: "text", Text: "Done"},
		},
		IsError: false,
	}
	data, err := json.Marshal(result)
	assert.NoError(t, err)
	// IsError has omitempty, so false is omitted
	assert.NotContains(t, string(data), `"isError"`)

	// Test with IsError = true
	result2 := ToolCallResult{IsError: true}
	data2, err := json.Marshal(result2)
	assert.NoError(t, err)
	assert.Contains(t, string(data2), `"isError":true`)
}

func TestToolCallContent_MarshalJSON(t *testing.T) {
	c := ToolCallContent{Type: "image", Text: "base64data"}
	data, err := json.Marshal(c)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"type":"image"`)
	assert.Contains(t, string(data), `"text":"base64data"`)
}

func TestInitializeParams_MarshalJSON(t *testing.T) {
	params := InitializeParams{
		ProtocolVersion: "2024-11-05",
		ClientInfo:      ClientInfo{Name: "smara", Version: "1.0"},
	}
	data, err := json.Marshal(params)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"protocolVersion":"2024-11-05"`)
	assert.Contains(t, string(data), `"clientInfo":{"name":"smara","version":"1.0"}`)
}

func TestMCPServerConfig_MarshalJSON(t *testing.T) {
	cfg := MCPServerConfig{
		Name:    "test-server",
		Type:    "local",
		Command: "node",
		Args:    []string{"server.js"},
		URL:     "",
		Headers: map[string]string{"Authorization": "Bearer test"},
		Env:     map[string]string{"NODE_ENV": "production"},
		Enabled: true,
	}
	data, err := json.Marshal(cfg)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"name":"test-server"`)
	assert.Contains(t, string(data), `"enabled":true`)
	assert.Contains(t, string(data), `"type":"local"`)
}

func TestServerInfo_MarshalJSON(t *testing.T) {
	info := ServerInfo{Name: "test", Version: "1.0", ProtocolVersion: "2024-11-05"}
	data, err := json.Marshal(info)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"name":"test"`)
}
