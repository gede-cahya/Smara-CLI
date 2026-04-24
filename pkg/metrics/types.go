package metrics

import "time"

// Metrics holds the full operational snapshot written to metrics.json.
type Metrics struct {
	StartedAt      time.Time                  `json:"started_at"`
	UpdatedAt      time.Time                  `json:"updated_at"`
	Platforms      map[string]*PlatformMetrics `json:"platforms"`
	LLM            LLMMetrics                 `json:"llm"`
	MCP            []MCPMetrics               `json:"mcp"`
	Memory         MemoryMetrics              `json:"memory"`
	Sync           SyncMetrics                `json:"sync"`
	RecentErrors   []ErrorEntry               `json:"recent_errors"`
	ActiveSessions int                        `json:"active_sessions"`
}

// PlatformMetrics holds per-platform statistics.
type PlatformMetrics struct {
	Name         string         `json:"name"`
	Status       string         `json:"status"` // "online", "offline", "error"
	MessagesIn   int64          `json:"messages_in"`
	MessagesOut  int64          `json:"messages_out"`
	ActiveUsers  int            `json:"active_users"`
	ErrorCount   int64          `json:"error_count"`
	TopUsers     []UserActivity `json:"top_users"`
	AvgLatencyMs int64          `json:"avg_latency_ms"`
}

// LLMMetrics holds LLM usage statistics.
type LLMMetrics struct {
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	TotalRequests    int64   `json:"total_requests"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	AvgLatencyMs     int64   `json:"avg_latency_ms"`
}

// MCPMetrics holds per-MCP server metrics.
type MCPMetrics struct {
	Name       string `json:"name"`
	Connected  bool   `json:"connected"`
	ToolCount  int    `json:"tool_count"`
	CallCount  int64  `json:"call_count"`
	ErrorCount int64  `json:"error_count"`
}

// MemoryMetrics holds memory store statistics.
type MemoryMetrics struct {
	TotalMemories int   `json:"total_memories"`
	UnsyncedCount int   `json:"unsynced_count"`
	DBSizeBytes   int64 `json:"db_size_bytes"`
}

// SyncMetrics holds sync daemon status.
type SyncMetrics struct {
	Enabled       bool      `json:"enabled"`
	LastSyncAt    time.Time `json:"last_sync_at"`
	PendingDeltas int       `json:"pending_deltas"`
	Status        string    `json:"status"` // "idle", "syncing", "error"
}

// ErrorEntry represents a recent error.
type ErrorEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"` // "telegram", "llm", "mcp:blender"
	Message   string    `json:"message"`
}

// UserActivity tracks per-user stats.
type UserActivity struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Platform string `json:"platform"`
	Requests int64  `json:"requests"`
}
