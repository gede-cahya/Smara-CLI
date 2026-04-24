package mcp

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
)

// Transport is the interface for JSON-RPC communication.
type Transport interface {
	Send(req *Request) error
	Receive() (*Response, error)
	Close() error
}

// Client is a high-level MCP client that manages connection to an MCP server.
type Client struct {
	transport  Transport
	serverInfo *ServerInfo
	tools      []Tool
	nextID     atomic.Int64
	config     MCPServerConfig
}

// NewClient creates and initializes a new MCP client from a local (stdio) config.
func NewClient(config MCPServerConfig) (*Client, error) {
	transport, err := NewStdioTransport(config.Command, config.Args, config.Env)
	if err != nil {
		return nil, err
	}

	c := &Client{
		transport: transport,
		config:    config,
	}

	// Perform MCP initialization handshake
	if err := c.initialize(); err != nil {
		transport.Close()
		return nil, fmt.Errorf("gagal inisialisasi MCP: %w", err)
	}

	return c, nil
}

// NewRemoteClient creates an MCP client for a remote HTTP-based server.
func NewRemoteClient(config MCPServerConfig) (*Client, error) {
	transport, err := NewHTTPTransport(config.URL, config.Headers)
	if err != nil {
		return nil, err
	}

	c := &Client{
		transport: transport,
		config:    config,
	}

	// Perform MCP initialization handshake
	if err := c.initialize(); err != nil {
		transport.Close()
		return nil, fmt.Errorf("gagal inisialisasi MCP remote: %w", err)
	}

	return c, nil
}

func (c *Client) getNextID() int {
	return int(c.nextID.Add(1))
}

// initialize performs the MCP handshake.
func (c *Client) initialize() error {
	req := &Request{
		JSONRPC: "2.0",
		ID:      c.getNextID(),
		Method:  "initialize",
		Params: InitializeParams{
			ProtocolVersion: "2024-11-05",
			ClientInfo: ClientInfo{
				Name:    "smara",
				Version: "1.0.0",
			},
		},
	}

	if err := c.transport.Send(req); err != nil {
		return err
	}

	resp, err := c.transport.Receive()
	if err != nil {
		return err
	}

	if resp.Error != nil {
		return resp.Error
	}

	if err := json.Unmarshal(resp.Result, &c.serverInfo); err != nil {
		return fmt.Errorf("gagal parse server info: %w", err)
	}

	// Send initialized notification
	notif := &Request{
		JSONRPC: "2.0",
		ID:      c.getNextID(),
		Method:  "notifications/initialized",
	}
	return c.transport.Send(notif)
}

// ListTools retrieves the list of available tools from the MCP server.
func (c *Client) ListTools() ([]Tool, error) {
	req := &Request{
		JSONRPC: "2.0",
		ID:      c.getNextID(),
		Method:  "tools/list",
	}

	if err := c.transport.Send(req); err != nil {
		return nil, err
	}

	resp, err := c.transport.Receive()
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var result ToolListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("gagal parse tool list: %w", err)
	}

	c.tools = result.Tools
	return result.Tools, nil
}

// CallTool invokes a specific tool on the MCP server.
func (c *Client) CallTool(name string, args map[string]interface{}) (*ToolCallResult, error) {
	req := &Request{
		JSONRPC: "2.0",
		ID:      c.getNextID(),
		Method:  "tools/call",
		Params: ToolCallParams{
			Name:      name,
			Arguments: args,
		},
	}

	if err := c.transport.Send(req); err != nil {
		return nil, err
	}

	resp, err := c.transport.Receive()
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var result ToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("gagal parse tool result: %w", err)
	}

	return &result, nil
}

// ServerName returns the connected MCP server name.
func (c *Client) ServerName() string {
	if c.serverInfo != nil {
		return c.serverInfo.Name
	}
	return c.config.Name
}

// Close terminates the MCP client connection.
func (c *Client) Close() error {
	return c.transport.Close()
}
