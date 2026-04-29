package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequest_JSON(t *testing.T) {
	req := Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
		Params:  map[string]interface{}{"cursor": "abc"},
	}
	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded Request
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, "2.0", decoded.JSONRPC)
	assert.Equal(t, 1, decoded.ID)
	assert.Equal(t, "tools/list", decoded.Method)
}

func TestResponse_WithResult(t *testing.T) {
	result := json.RawMessage(`{"tools":[]}`)
	resp := Response{
		JSONRPC: "2.0",
		ID:      1,
		Result:  result,
		Error:   nil,
	}
	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded Response
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, "2.0", decoded.JSONRPC)
	assert.Equal(t, 1, decoded.ID)
	assert.Equal(t, result, decoded.Result)
	assert.Nil(t, decoded.Error)
}

func TestResponse_WithError(t *testing.T) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      1,
		Error:   &RPCError{Code: -32600, Message: "Invalid Request"},
	}
	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded Response
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.NotNil(t, decoded.Error)
	assert.Equal(t, -32600, decoded.Error.Code)
	assert.Equal(t, "Invalid Request", decoded.Error.Message)
}

func TestRPCError_Error(t *testing.T) {
	err := &RPCError{Code: 1, Message: "something went wrong"}
	assert.Equal(t, "something went wrong", err.Error())
}

func TestServerInfo_Struct(t *testing.T) {
	info := ServerInfo{
		Name:            "test-server",
		Version:         "1.0.0",
		ProtocolVersion: "2024-11-05",
	}
	assert.Equal(t, "test-server", info.Name)
	assert.Equal(t, "1.0.0", info.Version)
	assert.Equal(t, "2024-11-05", info.ProtocolVersion)
}

func TestTool_Struct(t *testing.T) {
	tool := Tool{
		Name:        "read_file",
		Description: "Reads a file",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{"type": "string"},
			},
		},
	}
	assert.Equal(t, "read_file", tool.Name)
	assert.Equal(t, "Reads a file", tool.Description)
	assert.NotNil(t, tool.InputSchema)
}

func TestToolListResult_Struct(t *testing.T) {
	result := ToolListResult{
		Tools: []Tool{
			{Name: "tool1"},
			{Name: "tool2"},
		},
	}
	assert.Len(t, result.Tools, 2)
}

func TestToolCallParams_Struct(t *testing.T) {
	params := ToolCallParams{
		Name:      "write_file",
		Arguments: map[string]interface{}{"path": "/tmp/test.txt"},
	}
	assert.Equal(t, "write_file", params.Name)
	assert.Equal(t, "/tmp/test.txt", params.Arguments["path"])
}

func TestToolCallContent_Struct(t *testing.T) {
	content := ToolCallContent{
		Type: "text",
		Text: "Hello world",
	}
	assert.Equal(t, "text", content.Type)
	assert.Equal(t, "Hello world", content.Text)
}

func TestToolCallResult_Struct(t *testing.T) {
	result := ToolCallResult{
		Content: []ToolCallContent{
			{Type: "text", Text: "output"},
		},
		IsError: false,
	}
	assert.Len(t, result.Content, 1)
	assert.False(t, result.IsError)
}

func TestToolCallResult_Error(t *testing.T) {
	result := ToolCallResult{
		Content: []ToolCallContent{
			{Type: "text", Text: "error occurred"},
		},
		IsError: true,
	}
	assert.True(t, result.IsError)
}

func TestInitializeParams_Struct(t *testing.T) {
	params := InitializeParams{
		ProtocolVersion: "2024-11-05",
		ClientInfo: ClientInfo{
			Name:    "smara",
			Version: "1.9.0",
		},
	}
	assert.Equal(t, "2024-11-05", params.ProtocolVersion)
	assert.Equal(t, "smara", params.ClientInfo.Name)
	assert.Equal(t, "1.9.0", params.ClientInfo.Version)
}

func TestClientInfo_Struct(t *testing.T) {
	info := ClientInfo{Name: "client", Version: "2.0.0"}
	assert.Equal(t, "client", info.Name)
	assert.Equal(t, "2.0.0", info.Version)
}

func TestMCPServerConfig_Local(t *testing.T) {
	config := MCPServerConfig{
		Name:    "local-server",
		Type:    "local",
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
		Env:     map[string]string{"NODE_ENV": "production"},
		Enabled: true,
	}
	assert.Equal(t, "local-server", config.Name)
	assert.Equal(t, "local", config.Type)
	assert.Equal(t, "npx", config.Command)
	assert.Equal(t, []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"}, config.Args)
	assert.Equal(t, "production", config.Env["NODE_ENV"])
	assert.True(t, config.Enabled)
}

func TestMCPServerConfig_Remote(t *testing.T) {
	config := MCPServerConfig{
		Name:    "remote-server",
		Type:    "remote",
		URL:     "https://example.com/mcp",
		Headers: map[string]string{"Authorization": "Bearer token123"},
		Enabled: false,
	}
	assert.Equal(t, "remote-server", config.Name)
	assert.Equal(t, "remote", config.Type)
	assert.Equal(t, "https://example.com/mcp", config.URL)
	assert.Equal(t, "Bearer token123", config.Headers["Authorization"])
	assert.False(t, config.Enabled)
}
