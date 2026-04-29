package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTransport implements the Transport interface for testing.
type mockTransport struct {
	requests  []*Request
	responses []*Response
	index     int
	closeErr  error
	err       error
	closed    bool
}

func (m *mockTransport) Send(req *Request) error {
	if m.err != nil {
		return m.err
	}
	m.requests = append(m.requests, req)
	return nil
}

func (m *mockTransport) Receive() (*Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.index >= len(m.responses) {
		return nil, assert.AnError
	}
	resp := m.responses[m.index]
	m.index++
	return resp, nil
}

func (m *mockTransport) Close() error {
	m.closed = true
	return m.closeErr
}

func newClientWithTransport(config MCPServerConfig, transport Transport) (*Client, error) {
	c := &Client{transport: transport}
	if err := c.initialize(); err != nil {
		return nil, err
	}
	return c, nil
}

func newTestClient(t *testing.T) *Client {
	transport := &mockTransport{
		responses: []*Response{
			{
				JSONRPC: "2.0",
				ID:      0,
				Result:  mustMarshal(ServerInfo{Name: "test-server", Version: "1.0.0"}),
			},
		},
	}
	client, err := newClientWithTransport(MCPServerConfig{Name: "test"}, transport)
	require.NoError(t, err)
	return client
}

func mustMarshal(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return json.RawMessage(data)
}

func TestNewClient_Initialization(t *testing.T) {
	transport := &mockTransport{
		responses: []*Response{
			{
				JSONRPC: "2.0",
				ID:      0,
				Result:  mustMarshal(ServerInfo{Name: "test-server", Version: "1.0.0"}),
			},
		},
	}

	client, err := newClientWithTransport(MCPServerConfig{Name: "test"}, transport)
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, "test-server", client.GetServerInfo().Name)
	assert.Equal(t, "1.0.0", client.GetServerInfo().Version)
}

func TestClient_ListTools(t *testing.T) {
	transport := &mockTransport{
		responses: []*Response{
			{
				JSONRPC: "2.0",
				ID:      0,
				Result:  mustMarshal(ServerInfo{Name: "test"}),
			},
			{
				JSONRPC: "2.0",
				ID:      1,
				Result: mustMarshal(ToolListResult{
					Tools: []Tool{
						{Name: "tool1", Description: "First tool"},
						{Name: "tool2", Description: "Second tool"},
					},
				}),
			},
		},
	}

	client, err := newClientWithTransport(MCPServerConfig{Name: "test"}, transport)
	require.NoError(t, err)

	tools, err := client.ListTools()
	require.NoError(t, err)
	assert.Len(t, tools, 2)
	assert.Equal(t, "tool1", tools[0].Name)
	assert.Equal(t, "First tool", tools[0].Description)
	assert.Equal(t, "tool2", tools[1].Name)
}

func TestClient_ListTools_Error(t *testing.T) {
	transport := &mockTransport{
		responses: []*Response{
			{
				JSONRPC: "2.0",
				ID:      0,
				Result:  mustMarshal(ServerInfo{Name: "test"}),
			},
			{
				JSONRPC: "2.0",
				ID:      1,
				Error:   &RPCError{Code: -32600, Message: "Invalid request"},
			},
		},
	}

	client, err := newClientWithTransport(MCPServerConfig{Name: "test"}, transport)
	require.NoError(t, err)

	_, err = client.ListTools()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid request")
}

func TestClient_CallTool(t *testing.T) {
	transport := &mockTransport{
		responses: []*Response{
			{
				JSONRPC: "2.0",
				ID:      0,
				Result:  mustMarshal(ServerInfo{Name: "test"}),
			},
			{
				JSONRPC: "2.0",
				ID:      1,
				Result: mustMarshal(ToolCallResult{
					Content: []ToolCallContent{
						{Type: "text", Text: "result"},
					},
				}),
			},
		},
	}

	client, err := newClientWithTransport(MCPServerConfig{Name: "test"}, transport)
	require.NoError(t, err)

	result, err := client.CallTool("test_tool", map[string]interface{}{"arg": "value"})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Content, 1)
	assert.Equal(t, "result", result.Content[0].Text)
}

func TestClient_ServerInfo(t *testing.T) {
	transport := &mockTransport{
		responses: []*Response{
			{
				JSONRPC: "2.0",
				ID:      0,
				Result:  mustMarshal(ServerInfo{Name: "mcp-server", Version: "2.0.0"}),
			},
		},
	}

	client, err := newClientWithTransport(MCPServerConfig{Name: "test"}, transport)
	require.NoError(t, err)

	info := client.GetServerInfo()
	assert.Equal(t, "mcp-server", info.Name)
	assert.Equal(t, "2.0.0", info.Version)
}
