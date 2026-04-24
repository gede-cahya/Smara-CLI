package metrics

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxRecentErrors = 20

// MetricsCollector aggregates real-time metrics from Gateway and Supervisor.
// Thread-safe — designed to be called from multiple goroutines.
type MetricsCollector struct {
	mu sync.RWMutex

	startedAt time.Time
	filePath  string

	// Platform counters
	platforms map[string]*platformCounter

	// LLM counters
	llmProvider     string
	llmModel        string
	llmRequests     int64
	llmInputTokens  int64
	llmOutputTokens int64
	llmCostUSD      float64
	llmTotalLatency int64 // cumulative ms for average calculation

	// MCP counters
	mcpServers map[string]*mcpCounter

	// Memory & sync
	memoryTotal   int
	memoryUnsynced int
	memoryDBSize  int64
	syncEnabled   bool
	syncLastAt    time.Time
	syncPending   int
	syncStatus    string

	// Sessions
	activeSessions int

	// Errors ring buffer
	recentErrors []ErrorEntry
}

type platformCounter struct {
	status      string
	messagesIn  int64
	messagesOut int64
	errorCount  int64
	users       map[string]*userCounter // userID → counter
	latencies   []int64                 // recent latencies in ms
}

type userCounter struct {
	username string
	requests int64
}

type mcpCounter struct {
	connected  bool
	toolCount  int
	callCount  int64
	errorCount int64
}

// NewCollector creates a new MetricsCollector.
func NewCollector(filePath, provider, model string) *MetricsCollector {
	return &MetricsCollector{
		startedAt:  time.Now(),
		filePath:   filePath,
		platforms:  make(map[string]*platformCounter),
		mcpServers: make(map[string]*mcpCounter),
		llmProvider: provider,
		llmModel:    model,
		syncStatus:  "idle",
	}
}

// RegisterPlatform marks a platform as online.
func (c *MetricsCollector) RegisterPlatform(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.platforms[name] = &platformCounter{
		status: "online",
		users:  make(map[string]*userCounter),
	}
}

// RegisterMCP marks an MCP server status.
func (c *MetricsCollector) RegisterMCP(name string, connected bool, toolCount int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mcpServers[name] = &mcpCounter{
		connected: connected,
		toolCount: toolCount,
	}
}

// RecordMessageIn increments the incoming message counter for a platform.
func (c *MetricsCollector) RecordMessageIn(platform, userID, username string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.getOrCreatePlatform(platform)
	p.messagesIn++
	u, ok := p.users[userID]
	if !ok {
		u = &userCounter{username: username}
		p.users[userID] = u
	}
	u.requests++
}

// RecordMessageOut increments the outgoing message counter.
func (c *MetricsCollector) RecordMessageOut(platform string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.getOrCreatePlatform(platform)
	p.messagesOut++
}

// RecordLLMUsage records token usage from a single LLM call.
func (c *MetricsCollector) RecordLLMUsage(inputTokens, outputTokens int, latencyMs int64, costUSD float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.llmRequests++
	c.llmInputTokens += int64(inputTokens)
	c.llmOutputTokens += int64(outputTokens)
	c.llmCostUSD += costUSD
	c.llmTotalLatency += latencyMs
}

// RecordMCPCall records a tool call to an MCP server.
func (c *MetricsCollector) RecordMCPCall(serverName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if mc, ok := c.mcpServers[serverName]; ok {
		mc.callCount++
	}
}

// RecordMCPError records an MCP error.
func (c *MetricsCollector) RecordMCPError(serverName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if mc, ok := c.mcpServers[serverName]; ok {
		mc.errorCount++
	}
}

// RecordError adds an error to the ring buffer.
func (c *MetricsCollector) RecordError(source, message string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := ErrorEntry{
		Timestamp: time.Now(),
		Source:    source,
		Message:   message,
	}
	c.recentErrors = append(c.recentErrors, entry)
	if len(c.recentErrors) > maxRecentErrors {
		c.recentErrors = c.recentErrors[len(c.recentErrors)-maxRecentErrors:]
	}
}

// RecordLatency records response latency for a platform.
func (c *MetricsCollector) RecordLatency(platform string, latencyMs int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.getOrCreatePlatform(platform)
	p.latencies = append(p.latencies, latencyMs)
	if len(p.latencies) > 100 {
		p.latencies = p.latencies[len(p.latencies)-100:]
	}
}

// UpdateMemoryStats updates memory/sync metrics.
func (c *MetricsCollector) UpdateMemoryStats(total, unsynced int, dbSize int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.memoryTotal = total
	c.memoryUnsynced = unsynced
	c.memoryDBSize = dbSize
}

// UpdateSyncStatus updates the sync daemon status.
func (c *MetricsCollector) UpdateSyncStatus(enabled bool, lastSync time.Time, pending int, status string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.syncEnabled = enabled
	c.syncLastAt = lastSync
	c.syncPending = pending
	c.syncStatus = status
}

// UpdateActiveSessions sets the active session count.
func (c *MetricsCollector) UpdateActiveSessions(count int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.activeSessions = count
}

// Snapshot creates an immutable copy of current metrics.
func (c *MetricsCollector) Snapshot() *Metrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	m := &Metrics{
		StartedAt:      c.startedAt,
		UpdatedAt:      time.Now(),
		Platforms:      make(map[string]*PlatformMetrics),
		ActiveSessions: c.activeSessions,
		LLM: LLMMetrics{
			Provider:         c.llmProvider,
			Model:            c.llmModel,
			TotalRequests:    c.llmRequests,
			InputTokens:      c.llmInputTokens,
			OutputTokens:     c.llmOutputTokens,
			EstimatedCostUSD: c.llmCostUSD,
		},
		Memory: MemoryMetrics{
			TotalMemories: c.memoryTotal,
			UnsyncedCount: c.memoryUnsynced,
			DBSizeBytes:   c.memoryDBSize,
		},
		Sync: SyncMetrics{
			Enabled:       c.syncEnabled,
			LastSyncAt:    c.syncLastAt,
			PendingDeltas: c.syncPending,
			Status:        c.syncStatus,
		},
	}

	// LLM avg latency
	if c.llmRequests > 0 {
		m.LLM.AvgLatencyMs = c.llmTotalLatency / c.llmRequests
	}

	// Platforms
	for name, pc := range c.platforms {
		pm := &PlatformMetrics{
			Name:        name,
			Status:      pc.status,
			MessagesIn:  pc.messagesIn,
			MessagesOut: pc.messagesOut,
			ErrorCount:  pc.errorCount,
			ActiveUsers: len(pc.users),
		}

		// Average latency
		if len(pc.latencies) > 0 {
			var total int64
			for _, l := range pc.latencies {
				total += l
			}
			pm.AvgLatencyMs = total / int64(len(pc.latencies))
		}

		// Top users (top 5)
		type userEntry struct {
			id string
			uc *userCounter
		}
		var entries []userEntry
		for id, uc := range pc.users {
			entries = append(entries, userEntry{id, uc})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].uc.requests > entries[j].uc.requests
		})
		top := 5
		if len(entries) < top {
			top = len(entries)
		}
		for _, e := range entries[:top] {
			pm.TopUsers = append(pm.TopUsers, UserActivity{
				UserID:   e.id,
				Username: e.uc.username,
				Platform: name,
				Requests: e.uc.requests,
			})
		}

		m.Platforms[name] = pm
	}

	// MCP
	for name, mc := range c.mcpServers {
		m.MCP = append(m.MCP, MCPMetrics{
			Name:       name,
			Connected:  mc.connected,
			ToolCount:  mc.toolCount,
			CallCount:  mc.callCount,
			ErrorCount: mc.errorCount,
		})
	}

	// Errors (copy)
	m.RecentErrors = make([]ErrorEntry, len(c.recentErrors))
	copy(m.RecentErrors, c.recentErrors)

	return m
}

// Start begins the background metrics writer goroutine.
func (c *MetricsCollector) Start(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				c.writeOnce()
				return
			case <-ticker.C:
				c.writeOnce()
			}
		}
	}()
}

func (c *MetricsCollector) writeOnce() {
	snapshot := c.Snapshot()
	_ = WriteMetrics(c.filePath, snapshot)
}

func (c *MetricsCollector) getOrCreatePlatform(name string) *platformCounter {
	p, ok := c.platforms[name]
	if !ok {
		p = &platformCounter{
			status: "online",
			users:  make(map[string]*userCounter),
		}
		c.platforms[name] = p
	}
	return p
}

// EstimateCost estimates LLM cost based on provider and model.
func EstimateCost(provider, model string, inputTokens, outputTokens int64) float64 {
	switch provider {
	case "anthropic":
		switch {
		case strings.Contains(model, "haiku"):
			return float64(inputTokens)*0.25/1_000_000 + float64(outputTokens)*1.25/1_000_000
		case strings.Contains(model, "sonnet"):
			return float64(inputTokens)*3.0/1_000_000 + float64(outputTokens)*15.0/1_000_000
		case strings.Contains(model, "opus"):
			return float64(inputTokens)*15.0/1_000_000 + float64(outputTokens)*75.0/1_000_000
		}
	case "openai":
		return float64(inputTokens)*5.0/1_000_000 + float64(outputTokens)*15.0/1_000_000
	case "openrouter":
		return float64(inputTokens)*3.0/1_000_000 + float64(outputTokens)*15.0/1_000_000
	}
	return 0
}
