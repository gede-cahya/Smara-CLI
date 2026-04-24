// Package llm provides LLM provider abstractions for Smara CLI.
package llm

// Role represents the role of a message sender.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool" // for tool call results in agentic loop
)

// Message represents a single message in a conversation.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // ID of the tool call this message responds to
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // Tool calls requested by the assistant
}

// ChatRequest represents a request to generate a chat completion.
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

// ChatResponse represents the response from a chat completion.
type ChatResponse struct {
	Content     string `json:"content"`
	Thinking    string `json:"thinking,omitempty"` // reasoning/thinking content from thinking models (qwen3, deepseek-r1, etc.)
	Model       string `json:"model"`
	TotalTokens int    `json:"total_tokens,omitempty"`
}

// ToolFunction describes a function that an LLM can call.
type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// ToolCall represents the LLM requesting a tool invocation.
type ToolCall struct {
	ID       string                 `json:"id"`
	Function string                 `json:"function"`
	Args     map[string]interface{} `json:"arguments"`
}
