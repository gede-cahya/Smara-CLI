// Package mcp implements the Model Context Protocol (MCP) JSON-RPC client.
package mcp

import "encoding/json"

// JSON-RPC 2.0 types

// Request represents a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// Response represents a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return e.Message
}

// MCP-specific types

// ServerInfo describes an MCP server's capabilities.
type ServerInfo struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	ProtocolVersion string `json:"protocolVersion"`
}

// Tool describes a tool offered by an MCP server.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema,omitempty"`
}

// ToolListResult is the response from tools/list.
type ToolListResult struct {
	Tools []Tool `json:"tools"`
}

// ToolCallParams are the parameters for tools/call.
type ToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// ToolCallContent represents a single content item in a tool call result.
type ToolCallContent struct {
	Type string `json:"type"` // "text", "image", "resource"
	Text string `json:"text,omitempty"`
}

// ToolCallResult is the response from tools/call.
type ToolCallResult struct {
	Content []ToolCallContent `json:"content"`
	IsError bool              `json:"isError,omitempty"`
}

// InitializeParams are sent during MCP handshake.
type InitializeParams struct {
	ProtocolVersion string     `json:"protocolVersion"`
	ClientInfo      ClientInfo `json:"clientInfo"`
	Capabilities    struct{}   `json:"capabilities"`
}

// ClientInfo identifies this MCP client.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPServerConfig represents user-configured MCP server settings.
type MCPServerConfig struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"`    // "local" or "remote"
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	URL     string            `json:"url,omitempty"`     // for remote servers
	Headers map[string]string `json:"headers,omitempty"` // for remote servers
	Env     map[string]string `json:"env,omitempty"`
	Enabled bool              `json:"enabled"`
}
