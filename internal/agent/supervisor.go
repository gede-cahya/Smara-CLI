package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cahya/smara/internal/llm"
	"github.com/cahya/smara/internal/mcp"
	"github.com/cahya/smara/internal/memory"
)

// MCPServerInfo holds detailed MCP server information.
type MCPServerInfo struct {
	Name      string
	Connected bool
	Tools     []mcp.Tool
	Error     string
}

// Stats holds usage statistics for the supervisor.
type Stats struct {
	PromptCount     int           // Total prompts processed
	TotalTokens     int           // Total tokens used (estimate)
	TotalCost       float64       // Estimated cost in USD
	TotalDuration   time.Duration // Total processing time
	AvgTokensPerReq int           // Average tokens per request
	SessionStart    time.Time     // Session start time
	mu              sync.RWMutex
}

// AgenticCallback defines callbacks for agentic loop events.
type AgenticCallback struct {
	OnToolCall   func(server, tool string, args map[string]interface{})
	OnToolResult func(output string)
	OnIteration  func(current, max int)
}

// Supervisor orchestrates multi-agent task execution.
type Supervisor struct {
	provider        llm.Provider
	providerConfig  llm.ProviderConfig // stored for runtime model switching
	memStore        memory.MemoryStore
	sessionStore    SessionStore
	sessionRegistry *SessionRegistry
	mcpClients      map[string]*mcp.Client
	mcpInfo         map[string]MCPServerInfo // detailed MCP server info
	toolRoute       map[string]toolRouteInfo // tool name → MCP server + tool name
	workers         []*Worker
	taskCh          chan Task
	resultCh        chan TaskResult
	mu              sync.RWMutex
	maxWorkers      int
	maxIterations   int // max agentic loop iterations
	mode            Mode
	history         []llm.Message // conversation history for context
	callback        AgenticCallback
	stats           Stats // usage statistics
}

// toolRouteInfo stores routing info for a registered tool.
type toolRouteInfo struct {
	MCPServer string
	ToolName  string
}

// NewSupervisor creates a new supervisor agent.
func NewSupervisor(provider llm.Provider, memStore memory.MemoryStore) *Supervisor {
	return &Supervisor{
		provider:        provider,
		providerConfig:  llm.ProviderConfig{},
		memStore:        memStore,
		sessionRegistry: NewSessionRegistry(),
		mcpClients:      make(map[string]*mcp.Client),
		mcpInfo:         make(map[string]MCPServerInfo),
		toolRoute:       make(map[string]toolRouteInfo),
		taskCh:          make(chan Task, 100),
		resultCh:        make(chan TaskResult, 100),
		maxWorkers:      4,
		maxIterations:   10,
		mode:            ModeAsk, // default mode
		history:         make([]llm.Message, 0),
		stats:           Stats{SessionStart: time.Now()},
	}
}

// NewSupervisorWithConfig creates a supervisor with a stored provider config for runtime switching.
func NewSupervisorWithConfig(provider llm.Provider, providerCfg llm.ProviderConfig, memStore memory.MemoryStore) *Supervisor {
	s := NewSupervisor(provider, memStore)
	s.providerConfig = providerCfg
	return s
}

// GetStats returns a copy of current usage statistics.
func (s *Supervisor) GetStats() Stats {
	s.stats.mu.RLock()
	defer s.stats.mu.RUnlock()
	return s.stats
}

// updateStats updates usage statistics after a prompt is processed.
func (s *Supervisor) updateStats(tokens int, cost float64, duration time.Duration) {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	s.stats.PromptCount++
	s.stats.TotalTokens += tokens
	s.stats.TotalCost += cost
	s.stats.TotalDuration += duration
	if s.stats.PromptCount > 0 {
		s.stats.AvgTokensPerReq = s.stats.TotalTokens / s.stats.PromptCount
	}
}

// SetMode changes the agent's operating mode.
func (s *Supervisor) SetMode(mode Mode) {
	s.mode = mode
	s.history = make([]llm.Message, 0)
}

// SetModel switches the LLM model/provider at runtime.
func (s *Supervisor) SetModel(provider, model string) error {
	// Build new config from stored config, updating provider and model
	cfg := s.providerConfig
	cfg.Name = provider
	if model != "" {
		cfg.Model = model
	}

	newProvider, err := llm.NewProvider(cfg)
	if err != nil {
		return fmt.Errorf("gagal switch model: %w", err)
	}

	s.provider = newProvider
	s.history = make([]llm.Message, 0)
	return nil
}

// GetProviderName returns the current provider name.
func (s *Supervisor) GetProviderName() string {
	if s.provider != nil {
		return s.provider.Name()
	}
	return "unknown"
}

// GetModel returns the current model name.
func (s *Supervisor) GetModel() string {
	// Could be extended to track current model
	return ""
}

// GetMode returns the current agent mode.
func (s *Supervisor) GetMode() Mode {
	return s.mode
}

// RegisterMCPClient adds an MCP server connection.
func (s *Supervisor) RegisterMCPClient(name string, client *mcp.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mcpClients[name] = client
	// Initialize basic info
	s.mcpInfo[name] = MCPServerInfo{
		Name:      name,
		Connected: true,
		Tools:     []mcp.Tool{},
	}
}

// UpdateMCPInfo updates detailed MCP server info after tools are listed.
func (s *Supervisor) UpdateMCPInfo(name string, tools []mcp.Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if info, ok := s.mcpInfo[name]; ok {
		info.Tools = tools
		info.Connected = true
		s.mcpInfo[name] = info
	}
	// Rebuild tool route map
	s.rebuildToolRoute(name, tools)
}

// rebuildToolRoute updates the tool routing map for a given MCP server.
func (s *Supervisor) rebuildToolRoute(serverName string, tools []mcp.Tool) {
	// Remove old routes for this server
	for key, route := range s.toolRoute {
		if route.MCPServer == serverName {
			delete(s.toolRoute, key)
		}
	}
	// Add new routes
	for _, tool := range tools {
		routeKey := tool.Name
		s.toolRoute[routeKey] = toolRouteInfo{
			MCPServer: serverName,
			ToolName:  tool.Name,
		}
	}
}

// UpdateMCPError marks an MCP server as having an error.
func (s *Supervisor) UpdateMCPError(name string, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if info, ok := s.mcpInfo[name]; ok {
		info.Connected = false
		info.Error = errMsg
		s.mcpInfo[name] = info
	}
}

// GetMCPClient retrieves an MCP client by name.
func (s *Supervisor) GetMCPClient(name string) (*mcp.Client, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.mcpClients[name]
	return c, ok
}

// GetMCPInfo returns detailed info for all MCP servers.
func (s *Supervisor) GetMCPInfo() map[string]MCPServerInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]MCPServerInfo, len(s.mcpInfo))
	for k, v := range s.mcpInfo {
		result[k] = v
	}
	return result
}

// ConvertMCPToolsToToolFunctions converts MCP tools to LLM ToolFunction format.
func (s *Supervisor) ConvertMCPToolsToToolFunctions() []llm.ToolFunction {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var tools []llm.ToolFunction
	for serverName, info := range s.mcpInfo {
		if !info.Connected {
			continue
		}
		for _, t := range info.Tools {
			tf := llm.ToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			}
			_ = serverName // route is already maintained in toolRoute
			tools = append(tools, tf)
		}
	}
	return tools
}

// executeToolCall routes a tool call to the appropriate MCP server.
func (s *Supervisor) executeToolCall(tc llm.ToolCall) (string, error) {
	s.mu.RLock()
	route, ok := s.toolRoute[tc.Function]
	client := s.mcpClients[route.MCPServer]
	s.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("tool '%s' tidak ditemukan di route map", tc.Function)
	}

	if client == nil {
		return "", fmt.Errorf("MCP server '%s' tidak terhubung", route.MCPServer)
	}

	result, err := client.CallTool(route.ToolName, tc.Args)
	if err != nil {
		return "", fmt.Errorf("gagal memanggil tool '%s': %w", tc.Function, err)
	}

	if result.IsError {
		var errText string
		for _, c := range result.Content {
			errText += c.Text
		}
		return "", fmt.Errorf("tool error: %s", errText)
	}

	var output strings.Builder
	for _, c := range result.Content {
		if c.Text != "" {
			output.WriteString(c.Text)
			output.WriteString("\n")
		}
	}

	return strings.TrimSpace(output.String()), nil
}

// ListMCPServers returns names of all connected MCP servers.
func (s *Supervisor) ListMCPServers() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.mcpClients))
	for name := range s.mcpClients {
		names = append(names, name)
	}
	return names
}

// --- Session Management ---

// CreateSession creates a new session and sets it as current.
func (s *Supervisor) CreateSession(cfg SessionConfig) (*Session, error) {
	sess, err := s.sessionRegistry.Create(cfg)
	if err != nil {
		return nil, err
	}
	// Persist to store
	if s.sessionStore != nil {
		s.sessionStore.UpdateSession(sess)
	}
	return sess, nil
}

// SwitchSession switches to a different session by ID.
func (s *Supervisor) SwitchSession(id string) error {
	return s.sessionRegistry.Switch(id)
}

// GetSession retrieves a session by ID.
func (s *Supervisor) GetSession(id string) (*Session, bool) {
	return s.sessionRegistry.Get(id)
}

// ListSessions returns all sessions.
func (s *Supervisor) ListSessions() []*Session {
	return s.sessionRegistry.List()
}

// EndCurrentSession marks the current session as ended.
func (s *Supervisor) EndCurrentSession() error {
	sess := s.sessionRegistry.Current()
	if sess == nil {
		return fmt.Errorf("tidak ada session aktif")
	}

	// Update in registry
	if err := s.sessionRegistry.EndCurrent(); err != nil {
		return err
	}

	// Persist state change
	if s.sessionStore != nil {
		s.sessionStore.UpdateSession(sess)
	}

	return nil
}

// IsCurrentSession checks if a session ID is the current session.
func (s *Supervisor) IsCurrentSession(id string) bool {
	return s.sessionRegistry.IsCurrent(id)
}

// ProcessPrompt handles a user prompt using the current agent mode.
func (s *Supervisor) ProcessPrompt(ctx context.Context, userPrompt string) (string, error) {
	startTime := time.Now()
	modeInfo := GetModeInfo(s.mode)

	// 1. Search memory for relevant context
	var memContext string
	if s.memStore != nil {
		embedding, err := s.provider.GenerateEmbedding(userPrompt)
		if err == nil && len(embedding) > 0 {
			results, err := s.memStore.Search(embedding, 3)
			if err == nil && len(results) > 0 {
				var parts []string
				for _, r := range results {
					parts = append(parts, fmt.Sprintf("- %s (relevansi: %.2f)", r.Memory.Content, r.Similarity))
				}
				memContext = "Konteks dari memori:\n" + strings.Join(parts, "\n")
			}
		}
	}

	// 2. Build messages with mode-specific system prompt
	messages := []llm.Message{
		{
			Role:    llm.RoleSystem,
			Content: modeInfo.SystemPrompt,
		},
	}

	// Add MCP tools context (only relevant for rush and plan modes)
	if s.mode == ModeRush || s.mode == ModePlan {
		mcpInfo := s.GetMCPInfo()
		if len(mcpInfo) > 0 {
			var toolDescs []string
			for serverName, info := range mcpInfo {
				if !info.Connected {
					continue
				}
				for _, tool := range info.Tools {
					toolDescs = append(toolDescs, fmt.Sprintf("- [%s] %s: %s", serverName, tool.Name, tool.Description))
				}
			}
			if len(toolDescs) > 0 {
				toolsDesc := "MCP Tools yang tersedia:\n" + strings.Join(toolDescs, "\n")
				messages = append(messages, llm.Message{
					Role:    llm.RoleSystem,
					Content: toolsDesc,
				})
			}
		}
	}

	// Add memory context
	if memContext != "" {
		messages = append(messages, llm.Message{
			Role:    llm.RoleSystem,
			Content: memContext,
		})
	}

	// Add conversation history (keep last 10 exchanges for context)
	maxHistory := 20 // 10 pairs of user+assistant
	if len(s.history) > maxHistory {
		s.history = s.history[len(s.history)-maxHistory:]
	}
	messages = append(messages, s.history...)

	// Add user prompt
	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: userPrompt,
	})

	// 3. Call LLM
	resp, err := s.provider.Chat(messages)
	if err != nil {
		return "", fmt.Errorf("gagal mendapatkan response dari LLM: %w", err)
	}

	// 4. Update conversation history
	s.history = append(s.history,
		llm.Message{Role: llm.RoleUser, Content: userPrompt},
		llm.Message{Role: llm.RoleAssistant, Content: resp.Content},
	)

	// 5. Update stats (estimate tokens: ~4 chars per token)
	inputTokens := len(userPrompt) / 4
	outputTokens := len(resp.Content) / 4
	totalTokens := inputTokens + outputTokens
	estimatedCost := float64(totalTokens) * 0.00001 // rough estimate: $0.01 per 1K tokens
	s.updateStats(totalTokens, estimatedCost, time.Since(startTime))

	// 6. Save interaction to memory
	if s.memStore != nil {
		tag := fmt.Sprintf("mode:%s", s.mode)
		content := fmt.Sprintf("Q: %s\nA: %s", userPrompt, truncate(resp.Content, 500))
		embedding, _ := s.provider.GenerateEmbedding(content)
		s.memStore.Save(content, tag, "supervisor", embedding)
	}

	return resp.Content, nil
}

// RunAgenticLoop executes the agentic loop: LLM → tool calls → execute → feed back → repeat.
// Returns the final text response when the LLM stops calling tools.
func (s *Supervisor) RunAgenticLoop(ctx context.Context, userPrompt string) (string, error) {
	modeInfo := GetModeInfo(s.mode)

	// 1. Search memory for relevant context
	var memContext string
	if s.memStore != nil {
		embedding, err := s.provider.GenerateEmbedding(userPrompt)
		if err == nil && len(embedding) > 0 {
			results, err := s.memStore.Search(embedding, 3)
			if err == nil && len(results) > 0 {
				var parts []string
				for _, r := range results {
					parts = append(parts, fmt.Sprintf("- %s (relevansi: %.2f)", r.Memory.Content, r.Similarity))
				}
				memContext = "Konteks dari memori:\n" + strings.Join(parts, "\n")
			}
		}
	}

	// 2. Build initial messages
	messages := []llm.Message{
		{
			Role:    llm.RoleSystem,
			Content: modeInfo.SystemPrompt,
		},
	}

	// Add MCP tools context
	if s.mode == ModeRush || s.mode == ModePlan {
		mcpInfo := s.GetMCPInfo()
		if len(mcpInfo) > 0 {
			var toolDescs []string
			for serverName, info := range mcpInfo {
				if !info.Connected {
					continue
				}
				for _, tool := range info.Tools {
					toolDescs = append(toolDescs, fmt.Sprintf("- [%s] %s: %s", serverName, tool.Name, tool.Description))
				}
			}
			if len(toolDescs) > 0 {
				toolsDesc := "MCP Tools yang tersedia:\n" + strings.Join(toolDescs, "\n")
				messages = append(messages, llm.Message{
					Role:    llm.RoleSystem,
					Content: toolsDesc,
				})
			}
		}
	}

	// Add memory context
	if memContext != "" {
		messages = append(messages, llm.Message{
			Role:    llm.RoleSystem,
			Content: memContext,
		})
	}

	// Add conversation history
	maxHistory := 20
	if len(s.history) > maxHistory {
		s.history = s.history[len(s.history)-maxHistory:]
	}
	messages = append(messages, s.history...)

	// Add user prompt
	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: userPrompt,
	})

	// 3. Get available tools
	tools := s.ConvertMCPToolsToToolFunctions()

	// 4. Agentic loop
	for iteration := 0; iteration < s.maxIterations; iteration++ {
		// Callback: report iteration
		if s.callback.OnIteration != nil {
			s.callback.OnIteration(iteration+1, s.maxIterations)
		}

		// Call LLM with tools
		var resp *llm.ChatResponse
		var toolCalls []llm.ToolCall
		var err error

		if len(tools) > 0 {
			resp, toolCalls, err = s.provider.ChatWithTools(messages, tools)
		} else {
			resp, err = s.provider.Chat(messages)
		}

		if err != nil {
			return "", fmt.Errorf("gagal mendapatkan response dari LLM: %w", err)
		}

		// Check if LLM wants to call tools
		if len(toolCalls) == 0 {
			// No tool calls — LLM gave final answer
			// Update history and save to memory
			s.history = append(s.history,
				llm.Message{Role: llm.RoleUser, Content: userPrompt},
				llm.Message{Role: llm.RoleAssistant, Content: resp.Content},
			)

			if s.memStore != nil {
				tag := fmt.Sprintf("mode:%s", s.mode)
				content := fmt.Sprintf("Q: %s\nA: %s", userPrompt, truncate(resp.Content, 500))
				embedding, _ := s.provider.GenerateEmbedding(content)
				s.memStore.Save(content, tag, "supervisor", embedding)
			}

			return resp.Content, nil
		}

		// LLM requested tool calls — execute them
		// Add assistant message with tool calls to history
		assistantMsg := llm.Message{
			Role:      llm.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: toolCalls,
		}
		messages = append(messages, assistantMsg)

		// Callback: report tool calls
		if s.callback.OnToolCall != nil {
			for _, tc := range toolCalls {
				s.callback.OnToolCall("", tc.Function, tc.Args)
			}
		}

		// Execute each tool and add results to messages
		for _, tc := range toolCalls {
			result, err := s.executeToolCall(tc)

			// Callback: report result
			if s.callback.OnToolResult != nil {
				if err != nil {
					s.callback.OnToolResult(fmt.Sprintf("Error: %s", err))
				} else {
					s.callback.OnToolResult(result)
				}
			}

			// Add tool result as a user message with tool_call_id
			toolMsg := llm.Message{
				Role:       llm.RoleTool,
				Content:    result,
				ToolCallID: tc.ID,
			}
			if err != nil {
				toolMsg.Content = fmt.Sprintf("Error: %s", err)
			}
			messages = append(messages, toolMsg)
		}

		// Loop continues — LLM will process tool results and either call more tools or give final answer
	}

	// Max iterations reached — try to get a final answer
	messages = append(messages, llm.Message{
		Role:    llm.RoleSystem,
		Content: "Maksimal iterasi tool tercapai. Berikan jawaban final berdasarkan informasi yang sudah dikumpulkan.",
	})

	resp, err := s.provider.Chat(messages)
	if err != nil {
		return "", fmt.Errorf("gagal mendapatkan response final: %w", err)
	}

	s.history = append(s.history,
		llm.Message{Role: llm.RoleUser, Content: userPrompt},
		llm.Message{Role: llm.RoleAssistant, Content: resp.Content},
	)

	return resp.Content, nil
}

// SetMaxIterations sets the maximum number of agentic loop iterations.
func (s *Supervisor) SetMaxIterations(n int) {
	s.maxIterations = n
}

// SetSessionStore sets the session persistence store.
func (s *Supervisor) SetSessionStore(store SessionStore) {
	s.sessionStore = store
}

// SetCallback sets the agentic callback functions.
func (s *Supervisor) SetCallback(cb AgenticCallback) {
	s.callback = cb
}

// Close shuts down all MCP client connections.
func (s *Supervisor) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, client := range s.mcpClients {
		client.Close()
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ExecuteTask runs a single task with timeout.
func (s *Supervisor) ExecuteTask(ctx context.Context, task Task) TaskResult {
	timeout := 60 * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resultCh := make(chan TaskResult, 1)

	go func() {
		worker := NewWorker(s.provider, s.mcpClients)
		result := worker.Execute(ctx, task)
		resultCh <- result
	}()

	select {
	case result := <-resultCh:
		return result
	case <-ctx.Done():
		return TaskResult{
			TaskID: task.ID,
			Status: TaskFailed,
			Error:  "task timeout",
		}
	}
}
