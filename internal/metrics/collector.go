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
	memoryTotal    int
	memoryUnsynced int
	memoryDBSize   int64
	syncEnabled    bool
	syncLastAt     time.Time
	syncPending    int
	syncStatus     string

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
		startedAt:   time.Now(),
		filePath:    filePath,
		platforms:   make(map[string]*platformCounter),
		mcpServers:  make(map[string]*mcpCounter),
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
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.ToLower(strings.TrimSpace(model))

	inputCost := func(ratePerMillion float64) float64 {
		return float64(inputTokens) * ratePerMillion / 1_000_000
	}
	outputCost := func(ratePerMillion float64) float64 {
		return float64(outputTokens) * ratePerMillion / 1_000_000
	}
	cost := func(inRate, outRate float64) float64 { return inputCost(inRate) + outputCost(outRate) }

	// Local/offline providers do not bill per token.
	if provider == "ollama" || provider == "local" || provider == "llamacpp" || provider == "unknown" {
		return 0
	}

	switch provider {
	case "anthropic":
		switch {
		case strings.Contains(model, "haiku"):
			return cost(0.25, 1.25)
		case strings.Contains(model, "sonnet"):
			return cost(3.0, 15.0)
		case strings.Contains(model, "opus"):
			return cost(15.0, 75.0)
		}
		// Safe default for current Claude Sonnet-class models.
		return cost(3.0, 15.0)
	case "openai":
		switch {
		case strings.Contains(model, "gpt-5.5"):
			return cost(5.0, 30.0)
		case strings.Contains(model, "gpt-5.4-mini"):
			return cost(0.75, 4.50)
		case strings.Contains(model, "gpt-5.4"):
			return cost(2.50, 15.0)
		case strings.Contains(model, "gpt-5-mini"):
			return cost(0.25, 2.0)
		case strings.Contains(model, "gpt-5-nano"):
			return cost(0.05, 0.40)
		case strings.Contains(model, "gpt-5-chat-latest") || strings.Contains(model, "gpt-5-codex") || strings.Contains(model, "gpt-5.2") || strings.Contains(model, "gpt-5.1") || strings.Contains(model, "gpt-5"):
			return cost(1.25, 10.0)
		case strings.Contains(model, "gpt-4o-mini"):
			return cost(0.15, 0.60)
		case strings.Contains(model, "gpt-4o"):
			return cost(5.0, 15.0)
		case strings.Contains(model, "gpt-4.1-mini"):
			return cost(0.40, 1.60)
		case strings.Contains(model, "gpt-4.1-nano"):
			return cost(0.10, 0.40)
		case strings.Contains(model, "gpt-4.1"):
			return cost(2.0, 8.0)
		case strings.Contains(model, "o3-mini") || strings.Contains(model, "o4-mini"):
			return cost(1.10, 4.40)
		}
		return cost(5.0, 15.0)
	case "openrouter":
		// OpenRouter passes many upstream model names. Match common routes first.
		switch {
		case strings.Contains(model, "claude") || strings.Contains(model, "sonnet"):
			return cost(3.0, 15.0)
		case strings.Contains(model, "haiku"):
			return cost(0.25, 1.25)
		case strings.Contains(model, "gpt-5.5"):
			return cost(5.0, 30.0)
		case strings.Contains(model, "gpt-5.4-mini"):
			return cost(0.75, 4.50)
		case strings.Contains(model, "gpt-5.4"):
			return cost(2.50, 15.0)
		case strings.Contains(model, "gpt-5-mini"):
			return cost(0.25, 2.0)
		case strings.Contains(model, "gpt-5-nano"):
			return cost(0.05, 0.40)
		case strings.Contains(model, "gpt-5"):
			return cost(1.25, 10.0)
		case strings.Contains(model, "gpt-4o-mini"):
			return cost(0.15, 0.60)
		case strings.Contains(model, "gpt-4o"):
			return cost(5.0, 15.0)
		case strings.Contains(model, "free"):
			return 0
		}
		return cost(3.0, 15.0)
	case "custom":
		// Custom is an OpenAI-compatible paid proxy (9Router). Model names
		// may have provider prefixes like cx/, sr/, bai/, tr/, tk/, etc.
		// Match based on the underlying model name after the last slash.
		switch {
		case strings.Contains(model, "free") || strings.Contains(model, "local"):
			return 0
		case strings.Contains(model, "haiku"):
			return cost(0.25, 1.25)
		case strings.Contains(model, "opus") || strings.Contains(model, "fable"):
			return cost(15.0, 75.0)
		case strings.Contains(model, "sonnet") || strings.Contains(model, "claude"):
			return cost(3.0, 15.0)
		case strings.Contains(model, "gpt-5.5"):
			return cost(5.0, 30.0)
		case strings.Contains(model, "gpt-5.4-mini"):
			return cost(0.75, 4.50)
		case strings.Contains(model, "gpt-5.4"):
			return cost(2.50, 15.0)
		case strings.Contains(model, "gpt-5-mini"):
			return cost(0.25, 2.0)
		case strings.Contains(model, "gpt-5-nano"):
			return cost(0.05, 0.40)
		case strings.Contains(model, "gpt-5"):
			return cost(1.25, 10.0)
		case strings.Contains(model, "gpt-4o-mini"):
			return cost(0.15, 0.60)
		case strings.Contains(model, "gpt-4o"):
			return cost(5.0, 15.0)
		case strings.Contains(model, "gpt-4.1-mini"):
			return cost(0.40, 1.60)
		case strings.Contains(model, "gpt-4.1"):
			return cost(2.0, 8.0)
		case strings.Contains(model, "gpt"):
			return cost(3.0, 15.0)
		case strings.Contains(model, "gemini-3.1-pro") || strings.Contains(model, "gemini-3.5"):
			return cost(2.50, 15.0)
		case strings.Contains(model, "gemini-3-flash"):
			return cost(0.15, 0.60)
		case strings.Contains(model, "gemini-2.5-pro"):
			return cost(1.25, 10.0)
		case strings.Contains(model, "gemini-2.5-flash"):
			return cost(0.15, 0.60)
		case strings.Contains(model, "gemini"):
			return cost(0.50, 2.00)
		case strings.Contains(model, "deepseek-r1") || strings.Contains(model, "deepseek-v4"):
			return cost(0.55, 2.19)
		case strings.Contains(model, "deepseek"):
			return cost(0.27, 1.10)
		case strings.Contains(model, "minimax-m3"):
			return cost(0.30, 1.50)
		case strings.Contains(model, "minimax-m2"):
			return cost(0.20, 1.10)
		case strings.Contains(model, "minimax"):
			return cost(0.10, 0.50)
		case strings.Contains(model, "glm-5"):
			return cost(0.50, 2.00)
		case strings.Contains(model, "glm-4"):
			return cost(0.10, 0.40)
		case strings.Contains(model, "glm"):
			return cost(0.20, 0.80)
		case strings.Contains(model, "kimi"):
			return cost(0.30, 1.20)
		case strings.Contains(model, "qwen3.7") || strings.Contains(model, "qwen3-coder"):
			return cost(0.50, 2.00)
		case strings.Contains(model, "qwen3"):
			return cost(0.20, 0.80)
		case strings.Contains(model, "qwen"):
			return cost(0.10, 0.40)
		case strings.Contains(model, "mimo"):
			return cost(0.15, 0.60)
		case strings.Contains(model, "grok"):
			return cost(3.0, 15.0)
		case strings.Contains(model, "mistral"):
			return cost(0.25, 1.00)
		case strings.Contains(model, "nemotron"):
			return cost(0.15, 0.60)
		case strings.Contains(model, "command"):
			return cost(0.15, 0.60)
		}
		// Conservative default for unknown 9Router models
		return cost(0.50, 2.00)
	}
	return 0
}
