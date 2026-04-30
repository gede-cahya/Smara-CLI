package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/cognitive"
	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/lsp"
	"github.com/gede-cahya/Smara-CLI/internal/mcp"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/gede-cahya/Smara-CLI/internal/safety"
	smarassh "github.com/gede-cahya/Smara-CLI/internal/ssh"
	"github.com/gede-cahya/Smara-CLI/internal/session"
	"github.com/gede-cahya/Smara-CLI/internal/skill"
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
	InputTokens     int           // Total input tokens
	OutputTokens    int           // Total output tokens
	LastDuration    time.Duration // Duration of the last request
	mu              sync.RWMutex
}

// PromptResult contains the result and statistics of a prompt processing.
type PromptResult struct {
	Response      string
	Thinking      string   // The <thinking> block content
	Thoughts      []string // Intermediate reasoning text
	ToolsExecuted []string // List of tools run during this prompt
	InputTokens   int
	OutputTokens  int
	TotalTokens   int
	Duration      time.Duration
}

// AgenticCallback defines callbacks for agentic loop events.
type AgenticCallback struct {
	OnToolCall    func(server, tool string, args map[string]interface{})
	OnToolResult  func(output string)
	OnIteration   func(current, max int)
	OnStream      func(chunk string, isThinking bool)
	OnPhaseChange func(phase, description string)
	OnLog         func(role, content string)
	OnConfirm     func(message string) bool
	OnExplore     func(path string, results string)
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
	autoDiscovered      bool
	workspaceID         int64 // active workspace ID
	stats               Stats // usage statistics
	safetyEngine        *safety.Engine
	cognitiveValidator  *cognitive.Validator
	lspManager          *lsp.Manager
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
	// Attempt to wire DB for built-in tools (user_model, schedule_reminder, etc.)
	if dbStore, ok := memStore.(*memory.SQLiteStore); ok {
		BuiltinDB = dbStore.DB()
	}
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
	// History is now preserved across mode changes to maintain conversation context

	// Sync safety engine mode to enforce read-only in Plan Mode only
	if s.safetyEngine != nil {
		if mode == ModePlan {
			s.safetyEngine.SetMode(safety.ModePlan)
		} else {
			s.safetyEngine.SetMode(safety.ModeBuild)
		}
	}
}

// SetSafetyEngine sets the safety engine for execution mode enforcement.
func (s *Supervisor) SetSafetyEngine(engine *safety.Engine) {
	s.safetyEngine = engine
	// Sync current supervisor mode into safety engine
	if s.mode == ModePlan {
		engine.SetMode(safety.ModePlan)
	} else {
		engine.SetMode(safety.ModeBuild)
	}
}

// SetCognitiveValidator sets the cognitive schema validator for tool argument validation.
func (s *Supervisor) SetCognitiveValidator(validator *cognitive.Validator) {
	s.cognitiveValidator = validator
}

// SetLSPManager sets the LSP Manager for code intelligence tools.
func (s *Supervisor) SetLSPManager(manager *lsp.Manager) {
	s.lspManager = manager
}

// SetWorkspaceID sets the active workspace for this supervisor.
func (s *Supervisor) SetWorkspaceID(id int64) {
	s.workspaceID = id
}

// GetWorkspaceID returns the active workspace ID.
func (s *Supervisor) GetWorkspaceID() int64 {
	return s.workspaceID
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
	// We don't wipe history here anymore to maintain session context across model switches
	return nil
}

// GetProviderName returns the current provider name.
func (s *Supervisor) GetProviderName() string {
	if s.provider != nil {
		return s.provider.Name()
	}
	return "unknown"
}

// AddContext adds system context to the current session history.
func (s *Supervisor) AddContext(context string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, llm.Message{
		Role:    llm.RoleSystem,
		Content: context,
	})
}

// discoverProjectContext mencari file penting di direktori saat ini dan menambahkannya sebagai konteks.
func (s *Supervisor) discoverProjectContext() {
	if s.autoDiscovered {
		return
	}
	s.autoDiscovered = true

	cwd, _ := os.Getwd()
	foundContext := fmt.Sprintf("Current Working Directory (CWD): %s\n", cwd)

	// List files in current directory (level 1 & 2)
	foundContext += "Workspace Structure (Limited Depth):\n"
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == "." {
			return nil
		}

		// Skip hidden directories and large folders
		if d.IsDir() && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" || d.Name() == "vendor" || d.Name() == "bin") {
			return filepath.SkipDir
		}

		rel, _ := filepath.Rel(".", path)
		depth := strings.Count(rel, string(os.PathSeparator))
		if depth >= 2 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		indent := strings.Repeat("  ", depth)
		name := d.Name()
		if d.IsDir() {
			name += "/"
		}
		foundContext += fmt.Sprintf("%s- %s\n", indent, name)
		return nil
	})
	if err != nil {
		foundContext += fmt.Sprintf("(Gagal memetakan struktur: %v)\n", err)
	}

	importantFiles := []string{"go.mod", "package.json", "README.md", "Makefile", ".gitignore", "docker-compose.yml", "requirements.txt"}
	foundContext += "\nDetected Important File Contents (Preview):\n"

	for _, file := range importantFiles {
		if _, err := os.Stat(file); err == nil {
			data, err := os.ReadFile(file)
			if err == nil {
				content := string(data)
				if len(content) > 1000 {
					content = content[:1000] + "..."
				}
				foundContext += fmt.Sprintf("\n--- %s ---\n%s\n", file, content)
			}
		}
	}

	if foundContext != "" {
		s.AddContext("CONTEXT: Proyek Terdeteksi Otomatis\n" + foundContext)
		if s.callback.OnLog != nil {
			s.callback.OnLog("system", fmt.Sprintf("Berhasil memetakan workspace di %s", cwd))
		}
	}
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
	if old, ok := s.mcpClients[name]; ok {
		old.Close()
	}
	s.mcpClients[name] = client
	// Initialize basic info
	s.mcpInfo[name] = MCPServerInfo{
		Name:      name,
		Connected: true,
		Tools:     []mcp.Tool{},
	}
}

// UnregisterMCPClient disconnects and removes an MCP server.
func (s *Supervisor) UnregisterMCPClient(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if client, ok := s.mcpClients[name]; ok {
		if client != nil {
			client.Close()
		}
		delete(s.mcpClients, name)
	}
	delete(s.mcpInfo, name)
	// Remove all tool routes for this server
	for key, route := range s.toolRoute {
		if route.MCPServer == name {
			delete(s.toolRoute, key)
		}
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

	// 1. Add built-in agentic tools
	tools := GetBuiltinTools()

	// 2. If Plan Mode with safety engine, filter to read-only tools
	if s.mode == ModePlan && s.safetyEngine != nil {
		var filtered []llm.ToolFunction
		for _, t := range tools {
			if safety.IsReadOnlyTool(t.Name) {
				filtered = append(filtered, t)
			}
		}
		tools = filtered
	}

	// 3. Add MCP tools
	for serverName, info := range s.mcpInfo {
		if !info.Connected {
			continue
		}
		for _, t := range info.Tools {
			if s.mode == ModePlan && s.safetyEngine != nil && !safety.IsReadOnlyTool(t.Name) {
				continue
			}
			// Prefix MCP tools with their server name if there's a conflict
			// but for now we just append them directly
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
	// Safety engine enforcement: block write/execute/delete tools in Plan Mode
	if s.safetyEngine != nil {
		ok, reason := s.safetyEngine.CanExecute(tc.Function)
		if !ok {
			s.safetyEngine.RecordDraft(tc.Function, tc.Args)
			return "", fmt.Errorf("safety block: %s", reason)
		}
	}

	// Cognitive validator: validate tool arguments against registered schemas
	// Skip validation for builtin tools — their schemas are known and trusted
	if s.cognitiveValidator != nil {
		isBuiltin := false
		for _, bt := range GetBuiltinTools() {
			if bt.Name == tc.Function {
				isBuiltin = true
				break
			}
		}
		if !isBuiltin {
			result := s.cognitiveValidator.Validate(tc.Function, tc.Args)
			if !result.Valid {
				return "", fmt.Errorf("cognitive validation failed: %s", strings.Join(result.Errors, "; "))
			}
		}
	}

	// Check if confirmation is needed for critical tools
	if s.isCriticalCall(s.mode, tc.Function, tc.Args) && s.callback.OnConfirm != nil {
		if !s.callback.OnConfirm(fmt.Sprintf("Tool: %s\nArgs: %v", tc.Function, tc.Args)) {
			return "User membatalkan eksekusi tool ini.", nil
		}
	}

	// Check if it is a built-in tool first
	for _, bt := range GetBuiltinTools() {
		if bt.Name == tc.Function {
			// Handle memory tools directly in supervisor because they need memStore and provider
			if tc.Function == "remember" {
				if s.memStore == nil {
					return "Memory store tidak tersedia.", nil
				}
				if s.provider == nil {
					return "LLM provider tidak tersedia untuk generate embedding.", nil
				}
				content, _ := tc.Args["content"].(string)
				embedding, err := s.provider.GenerateEmbedding(content)
				if err != nil {
					return "", fmt.Errorf("gagal generate embedding: %w", err)
				}
				_, err = s.memStore.Save(content, "user_preference", "agent", s.workspaceID, embedding)
				if err != nil {
					return "", fmt.Errorf("gagal menyimpan memori: %w", err)
				}
				return "Informasi berhasil disimpan ke memori jangka panjang.", nil
			}

			if tc.Function == "search_memories" {
				if s.memStore == nil {
					return "Memory store tidak tersedia.", nil
				}
				if s.provider == nil {
					return "LLM provider tidak tersedia untuk generate embedding.", nil
				}
				query, _ := tc.Args["query"].(string)
				embedding, err := s.provider.GenerateEmbedding(query)
				if err != nil {
					return "", fmt.Errorf("gagal generate embedding: %w", err)
				}
				results, err := s.memStore.Search(embedding, s.workspaceID, 5)
				if err != nil {
					return "", fmt.Errorf("gagal mencari memori: %w", err)
				}
				if len(results) == 0 {
					return "Tidak ada memori yang relevan ditemukan.", nil
				}
				var sb strings.Builder
				sb.WriteString("Hasil pencarian memori:\n")
				for _, r := range results {
					sb.WriteString(fmt.Sprintf("- %s\n", r.Memory.Content))
				}
				return sb.String(), nil
			}

			if strings.HasPrefix(tc.Function, "lsp_") {
				if s.lspManager == nil {
					return "LSP Manager tidak tersedia.", nil
				}
				cwd, _ := os.Getwd()
				lang := "go"
				filePath, _ := tc.Args["file_path"].(string)
				if filePath != "" {
					ext := filepath.Ext(filePath)
					switch ext {
					case ".go":
						lang = "go"
					case ".py":
						lang = "python"
					case ".ts", ".js", ".tsx", ".jsx":
						lang = "typescript"
					}
				}
				client, err := s.lspManager.GetOrCreateClient(lang, cwd)
				if err != nil {
					return "", fmt.Errorf("gagal inisialisasi LSP client: %w", err)
				}

				switch tc.Function {
				case "lsp_document_symbols":
					res, err := client.DocumentSymbol(filePath)
					if err != nil {
						return "", err
					}
					b, _ := json.MarshalIndent(res, "", "  ")
					return string(b), nil
				default:
					var line, char int
					if l, ok := tc.Args["line"].(float64); ok {
						line = int(l)
					}
					if c, ok := tc.Args["character"].(float64); ok {
						char = int(c)
					}

					switch tc.Function {
					case "lsp_hover":
						res, err := client.Hover(filePath, line, char)
						if err != nil {
							return "", err
						}
						b, _ := json.MarshalIndent(res, "", "  ")
						return string(b), nil
					case "lsp_definition":
						res, err := client.Definition(filePath, line, char)
						if err != nil {
							return "", err
						}
						b, _ := json.MarshalIndent(res, "", "  ")
						return string(b), nil
					case "lsp_references":
						res, err := client.References(filePath, line, char)
						if err != nil {
							return "", err
						}
						b, _ := json.MarshalIndent(res, "", "  ")
						return string(b), nil
					}
				}
			}

			// Handle MCP connection/disconnection tools directly in supervisor
			if tc.Function == "connect_mcp" {
				name := getStr(tc.Args, "name")
				mcpType := getStr(tc.Args, "type")
				if name == "" || mcpType == "" {
					return "", fmt.Errorf("argumen 'name' dan 'type' wajib diisi")
				}
				mcpCfg := mcp.MCPServerConfig{
					Name:    name,
					Type:    mcpType,
					Enabled: true,
				}
				if mcpType == "local" {
					mcpCfg.Command = getStr(tc.Args, "command")
					if mcpCfg.Command == "" {
						return "", fmt.Errorf("argumen 'command' wajib diisi untuk type=local")
					}
					if argsArr, ok := tc.Args["args"].([]interface{}); ok {
						for _, a := range argsArr {
							if s, ok := a.(string); ok {
								mcpCfg.Args = append(mcpCfg.Args, s)
							}
						}
					}
					if envObj, ok := tc.Args["env"].(map[string]interface{}); ok {
						mcpCfg.Env = make(map[string]string)
						for k, v := range envObj {
							if s, ok := v.(string); ok {
								mcpCfg.Env[k] = s
							}
						}
					}
				} else if mcpType == "remote" {
					mcpCfg.URL = getStr(tc.Args, "url")
					if mcpCfg.URL == "" {
						return "", fmt.Errorf("argumen 'url' wajib diisi untuk type=remote")
					}
					if headersObj, ok := tc.Args["headers"].(map[string]interface{}); ok {
						mcpCfg.Headers = make(map[string]string)
						for k, v := range headersObj {
							if s, ok := v.(string); ok {
								mcpCfg.Headers[k] = s
							}
						}
					}
				}
				var client *mcp.Client
				var err error
				switch mcpType {
				case "remote":
					client, err = mcp.NewRemoteClient(mcpCfg)
				default:
					client, err = mcp.NewClient(mcpCfg)
				}
				if err != nil {
					return "", fmt.Errorf("gagal menghubungkan MCP '%s': %w", name, err)
				}
				tools, err := client.ListTools()
				if err != nil {
					client.Close()
					return "", fmt.Errorf("gagal list tools dari MCP '%s': %w", name, err)
				}
				s.RegisterMCPClient(name, client)
				s.UpdateMCPInfo(name, tools)
				// Persist to config
				if err := config.AddMCPServer(config.MCPServer{
					Name:    mcpCfg.Name,
					Type:    mcpCfg.Type,
					Command: mcpCfg.Command,
					Args:    mcpCfg.Args,
					URL:     mcpCfg.URL,
					Headers: mcpCfg.Headers,
					Env:     mcpCfg.Env,
					Enabled: true,
				}); err != nil {
					return fmt.Sprintf("MCP '%s' terhubung dengan %d tools (gagal menyimpan config: %v)", name, len(tools), err), nil
				}
				return fmt.Sprintf("MCP '%s' terhubung dengan %d tools", name, len(tools)), nil
			}

			if tc.Function == "disconnect_mcp" {
				name := getStr(tc.Args, "name")
				if name == "" {
					return "", fmt.Errorf("argumen 'name' wajib diisi")
				}
				s.UnregisterMCPClient(name)
				if err := config.RemoveMCPServer(name); err != nil {
					return fmt.Sprintf("MCP '%s' diputuskan (gagal menghapus config: %v)", name, err), nil
				}
				return fmt.Sprintf("MCP '%s' diputuskan dan dihapus dari config", name), nil
			}

			var logFn func(string, string)
			if s.callback.OnLog != nil {
				logFn = s.callback.OnLog
			}
			return ExecuteBuiltinTool(tc.Function, tc.Args, logFn)
		}
	}

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

// SkillExecutor returns a StepExecutor that routes tool calls through
// the supervisor's existing tool routing (built-in + MCP).
func (s *Supervisor) SkillExecutor() skill.StepExecutor {
	return func(toolName string, args map[string]interface{}) (string, error) {
		tc := llm.ToolCall{
			Function: toolName,
			Args:     args,
		}
		return s.executeToolCall(tc)
	}
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
	if err := s.sessionRegistry.Switch(id); err != nil {
		return err
	}
	// Sync history from session to supervisor internal history
	if sess := s.sessionRegistry.Current(); sess != nil {
		s.mu.Lock()
		s.history = make([]llm.Message, len(sess.History))
		copy(s.history, sess.History)
		s.mu.Unlock()
	}
	return nil
}

// GetSession retrieves a session by ID.
func (s *Supervisor) GetSession(id string) (*Session, bool) {
	return s.sessionRegistry.Get(id)
}

// GetCurrentSession returns the currently active session.
func (s *Supervisor) GetCurrentSession() *Session {
	return s.sessionRegistry.Current()
}

// ListSessions returns all sessions.
func (s *Supervisor) ListSessions() []*Session {
	return s.sessionRegistry.List()
}

// InitializeSessions loads existing sessions from store into the registry.
func (s *Supervisor) InitializeSessions() error {
	if s.sessionStore == nil {
		return nil
	}

	sessions, err := s.sessionStore.ListSessionsByWorkspace(s.workspaceID)
	if err != nil {
		return err
	}

	for i := range sessions {
		s.sessionRegistry.Register(&sessions[i])
	}

	return nil
}

// GetLastActiveSession returns the last active session from store.
func (s *Supervisor) GetLastActiveSession() (*Session, error) {
	if s.sessionStore == nil {
		return nil, nil
	}

	// Just use store helper
	if store, ok := s.sessionStore.(*session.SQLiteStore); ok {
		return store.GetLastActiveSessionByWorkspace(s.workspaceID)
	}
	return nil, nil
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
func (s *Supervisor) ProcessPrompt(ctx context.Context, userPrompt string) (*PromptResult, error) {
	// Auto-discovery and exploration removed to respect user preference for manual triggers.

	// Pre-process: Search relevant memories

	startTime := time.Now()
	modeInfo := GetModeInfo(s.mode)

	// 1. Search memory for relevant context
	var memContext string
	if s.memStore != nil {
		embedding, err := s.provider.GenerateEmbedding(userPrompt)
		if err == nil && len(embedding) > 0 {
			results, err := s.memStore.Search(embedding, s.workspaceID, 3)
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
	sysPrompt := modeInfo.SystemPrompt
	if hostCtx, err := smarassh.AllHosts(); err == nil && hostCtx != "(tidak ada host SSH tersimpan)" {
		sysPrompt += "\n\nHost VPS/Server yang tersimpan (gunakan saat user menyebut vps/server/remote):\n" + hostCtx
	}
	if BuiltinDB != nil {
		if profile, err := LoadProfile(BuiltinDB); err == nil {
			sysPrompt += "\n\n" + profile.ToContext()
		}
	}

	messages := []llm.Message{
		{
			Role:    llm.RoleSystem,
			Content: sysPrompt,
		},
	}

	// Add MCP tools context (now added to all modes if tools are available)
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
			toolsDesc := "Tools yang tersedia (gunakan via function calling):\n" + strings.Join(toolDescs, "\n")
			messages = append(messages, llm.Message{
				Role:    llm.RoleSystem,
				Content: toolsDesc,
			})
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

	// 3. Call LLM (branch based on mode)
	var finalResp string
	var finalThinking string

	// Use agentic loop if tools are available, regardless of mode (with different behavior)
	tools := s.ConvertMCPToolsToToolFunctions()

	if len(tools) > 0 {
		resp, thinking, thoughts, executed, err := s.RunAgenticLoop(ctx, userPrompt)
		if err != nil {
			return nil, err
		}

		finalResp = resp
		finalThinking = thinking
		return &PromptResult{
			Response:      finalResp,
			Thinking:      finalThinking,
			Thoughts:      thoughts,
			ToolsExecuted: executed,
			InputTokens:   s.stats.InputTokens,
			OutputTokens:  s.stats.OutputTokens,
			TotalTokens:   s.stats.InputTokens + s.stats.OutputTokens,
			Duration:      time.Since(startTime),
		}, nil
	} else {
		var resp *llm.ChatResponse
		var err error
		if streamer, ok := s.provider.(llm.Streamer); ok {
			// Emit initial Thinking phase before stream starts
			if s.callback.OnPhaseChange != nil {
				s.callback.OnPhaseChange("Thinking", "Analyzing the request and planning approach...")
			}
			// Wrap stream callback to emit phase changes
			streamCb := func(chunk string, isThinking bool, phaseHint llm.PhaseHint) {
				if s.callback.OnPhaseChange != nil {
					phaseName := phaseNameFromHint(phaseHint)
					if phaseName != "" {
						s.callback.OnPhaseChange(phaseName, phaseDescFromHint(phaseHint))
					}
				}
				if s.callback.OnStream != nil {
					s.callback.OnStream(chunk, isThinking)
				}
			}
			resp, err = streamer.ChatStream(messages, streamCb)
		} else {
			resp, err = s.provider.Chat(messages)
		}

		if err != nil {
			return nil, fmt.Errorf("gagal mendapatkan response dari LLM: %w", err)
		}
		finalResp = resp.Content
		finalThinking = resp.Thinking
	}

	// 4. Update conversation history (both local and session)
	userMsg := llm.Message{Role: llm.RoleUser, Content: userPrompt}
	assistantMsg := llm.Message{Role: llm.RoleAssistant, Content: finalResp}

	s.history = append(s.history, userMsg, assistantMsg)

	if sess := s.sessionRegistry.Current(); sess != nil {
		sess.History = append(sess.History, userMsg, assistantMsg)
		sess.UpdatedAt = time.Now()
		if s.sessionStore != nil {
			s.sessionStore.UpdateSession(sess)
		}
	}

	// 5. Update stats (estimate tokens: ~4 chars per token)
	inputTokens := len(userPrompt) / 4
	outputTokens := len(finalResp) / 4
	totalTokens := inputTokens + outputTokens
	estimatedCost := float64(totalTokens) * 0.00001

	duration := time.Since(startTime)
	s.updateStats(totalTokens, estimatedCost, duration)

	s.mu.Lock()
	s.stats.InputTokens += inputTokens
	s.stats.OutputTokens += outputTokens
	s.stats.LastDuration = duration
	s.mu.Unlock()

	result := &PromptResult{
		Response:     finalResp,
		Thinking:     finalThinking,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  totalTokens,
		Duration:     duration,
	}

	// 6. Save interaction to memory
	if s.memStore != nil {
		tag := fmt.Sprintf("mode:%s", s.mode)
		content := fmt.Sprintf("Q: %s\nA: %s", userPrompt, truncate(finalResp, 500))
		embedding, _ := s.provider.GenerateEmbedding(content)
		s.memStore.Save(content, tag, "supervisor", s.workspaceID, embedding)
	}

	return result, nil
}

// GetModelInfo returns the current provider and model name.
func (s *Supervisor) GetModelInfo() (string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.providerConfig.Name, s.providerConfig.Model
}

// RunAgenticLoop executes the agentic loop: LLM → tool calls → execute → feed back → repeat.
// Returns the final text response when the LLM stops calling tools.
func (s *Supervisor) RunAgenticLoop(ctx context.Context, userPrompt string) (string, string, []string, []string, error) {
	modeInfo := GetModeInfo(s.mode)

	// 1. Search memory for relevant context
	var memContext string
	if s.memStore != nil {
		embedding, err := s.provider.GenerateEmbedding(userPrompt)
		if err == nil && len(embedding) > 0 {
			results, err := s.memStore.Search(embedding, s.workspaceID, 3)
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
	sysPrompt2 := modeInfo.SystemPrompt
	if hostCtx2, err := smarassh.AllHosts(); err == nil && hostCtx2 != "(tidak ada host SSH tersimpan)" {
		sysPrompt2 += "\n\nHost VPS/Server yang tersimpan (gunakan saat user menyebut vps/server/remote):\n" + hostCtx2
	}
	if BuiltinDB != nil {
		if profile, err := LoadProfile(BuiltinDB); err == nil {
			sysPrompt2 += "\n\n" + profile.ToContext()
		}
	}
	messages := []llm.Message{
		{
			Role:    llm.RoleSystem,
			Content: sysPrompt2,
		},
	}

	// Add MCP tools context
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
			toolsDesc := "Tools yang tersedia (gunakan via function calling):\n" + strings.Join(toolDescs, "\n")
			messages = append(messages, llm.Message{
				Role:    llm.RoleSystem,
				Content: toolsDesc,
			})
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

	var allThinking []string
	var toolsExecuted []string
	var thoughts []string

	// Emit initial Thinking phase
	if s.callback.OnPhaseChange != nil {
		s.callback.OnPhaseChange("Thinking", "Analyzing the request and planning approach...")
	}

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

		// Wrap stream callback to emit phase changes
		var streamCb llm.StreamCallback
		if s.callback.OnPhaseChange != nil {
			streamCb = func(chunk string, isThinking bool, phaseHint llm.PhaseHint) {
				phaseName := phaseNameFromHint(phaseHint)
				if phaseName != "" {
					s.callback.OnPhaseChange(phaseName, phaseDescFromHint(phaseHint))
				}
				if s.callback.OnStream != nil {
					s.callback.OnStream(chunk, isThinking)
				}
			}
		} else {
			streamCb = func(chunk string, isThinking bool, _ llm.PhaseHint) {
				if s.callback.OnStream != nil {
					s.callback.OnStream(chunk, isThinking)
				}
			}
		}

		if streamer, ok := s.provider.(llm.Streamer); ok {
			if len(tools) > 0 {
				resp, toolCalls, err = streamer.ChatStreamWithTools(messages, tools, streamCb)
			} else {
				resp, err = streamer.ChatStream(messages, streamCb)
			}
		} else {
			if len(tools) > 0 {
				resp, toolCalls, err = s.provider.ChatWithTools(messages, tools)
			} else {
				resp, err = s.provider.Chat(messages)
			}
		}

		if err != nil {
			return "", "", nil, nil, fmt.Errorf("gagal mendapatkan response dari LLM: %w", err)
		}

		// Accumulate thinking
		if resp.Thinking != "" {
			allThinking = append(allThinking, resp.Thinking)
		}

		// Fallback: some LLMs emit tool calls as DSML/XML inside content instead of native tool_calls field
		if len(toolCalls) == 0 && resp != nil && strings.Contains(resp.Content, "<| DSML |") {
			extracted, cleaned := llm.ExtractToolCallsFromContent(resp.Content)
			if len(extracted) > 0 {
				toolCalls = extracted
				resp.Content = cleaned
			}
		}

		if len(toolCalls) == 0 {
			// No tool calls — LLM gave final answer
			if s.callback.OnPhaseChange != nil {
				s.callback.OnPhaseChange("Generating", "Formulating final response...")
			}
			return resp.Content, strings.Join(allThinking, "\n\n"), thoughts, toolsExecuted, nil
		}

		// If we are here, LLM wants to call tools
		if s.callback.OnPhaseChange != nil {
			s.callback.OnPhaseChange("Exploring", "Gathering data with tools...")
		}

		// Capture this intermediate content as a "Thought"
		if resp.Content != "" {
			thoughts = append(thoughts, resp.Content)
		}

		// Update toolsExecuted list
		for _, tc := range toolCalls {
			toolsExecuted = append(toolsExecuted, tc.Function)
		}

		// LLM requested tool calls — execute them
		// Add assistant message with tool calls to history
		// Preserve reasoning content for DeepSeek-style models that require it
		assistantMsg := llm.Message{
			Role:             llm.RoleAssistant,
			Content:          resp.Content,
			ToolCalls:        toolCalls,
			ReasoningContent: resp.Thinking,
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
	if s.callback.OnPhaseChange != nil {
		s.callback.OnPhaseChange("Generating", "Max iterations reached. Formulating final response...")
	}

	messages = append(messages, llm.Message{
		Role:    llm.RoleSystem,
		Content: "Maksimal iterasi tool tercapai. Berikan jawaban final berdasarkan informasi yang sudah dikumpulkan.",
	})

	resp, err := s.provider.Chat(messages)
	if err != nil {
		return "", "", nil, nil, fmt.Errorf("gagal mendapatkan response final: %w", err)
	}

	if s.callback.OnPhaseChange != nil {
		s.callback.OnPhaseChange("Done", "Finished")
	}

	return resp.Content, strings.Join(allThinking, "\n\n"), thoughts, toolsExecuted, nil
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

// isCriticalCall returns true if the tool call requires user confirmation.
func (s *Supervisor) isCriticalCall(mode Mode, name string, args map[string]interface{}) bool {
	// Sensitive paths check (regardless of tool or mode)
	if path, ok := args["path"].(string); ok {
		if isSensitivePath(path) {
			return true
		}
	}

	// Mode RUSH bypasses non-sensitive critical tools for speed
	if mode == ModeRush {
		return false
	}

	// Standard critical tools list
	critical := []string{
		"run_command",
		"write_file",
		"delete_file",
		"edit_file",
		"ssh_exec",
		"skill_run",
	}
	for _, c := range critical {
		if name == c {
			return true
		}
	}
	return false
}

// isSensitivePath checks if the given path contains sensitive information.
func isSensitivePath(path string) bool {
	sensitivePatterns := []string{
		".env",
		".pem",
		".key",
		"id_rsa",
		"id_ed25519",
		"shadow",
		"passwd",
		"credential",
		"secret",
		"token",
		"config.json", // can be sensitive
	}

	lowerPath := strings.ToLower(path)
	for _, p := range sensitivePatterns {
		if strings.Contains(lowerPath, p) {
			return true
		}
	}
	return false
}

// phaseNameFromHint maps an llm.PhaseHint to a UI phase name.
func phaseNameFromHint(hint llm.PhaseHint) string {
	switch hint {
	case llm.PhaseThinking:
		return "Thinking"
	case llm.PhaseAnalyzing:
		return "Analyzing"
	case llm.PhaseExploring:
		return "Exploring"
	case llm.PhaseGenerating:
		return "Generating"
	default:
		return ""
	}
}

// phaseDescFromHint maps an llm.PhaseHint to a human-readable description.
func phaseDescFromHint(hint llm.PhaseHint) string {
	switch hint {
	case llm.PhaseThinking:
		return "Reasoning about the problem..."
	case llm.PhaseAnalyzing:
		return "Analyzing context and constraints..."
	case llm.PhaseExploring:
		return "Gathering data with tools..."
	case llm.PhaseGenerating:
		return "Formulating response..."
	default:
		return ""
	}
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
