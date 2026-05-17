package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
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
	"github.com/gede-cahya/Smara-CLI/internal/metrics"
	"github.com/gede-cahya/Smara-CLI/internal/safety"
	"github.com/gede-cahya/Smara-CLI/internal/session"
	"github.com/gede-cahya/Smara-CLI/internal/skill"
	smarassh "github.com/gede-cahya/Smara-CLI/internal/ssh"
)

const (
	maxToolResultChars  = 40000
	toolResultHeadChars = 26000
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

	// activeBudget is the live IterationBudget for the prompt currently
	// running through RunAgenticLoop. Tool handlers (e.g.
	// request_iteration_budget) read/mutate this through helper methods.
	// It is nil between prompts.
	activeBudget       *IterationBudget
	activeBudgetMu     sync.RWMutex
	autoDiscovered     bool
	workspaceID        int64 // active workspace ID
	stats              Stats // usage statistics
	safetyEngine       *safety.Engine
	cognitiveValidator *cognitive.Validator
	lspManager         *lsp.Manager

	// lastToolTrace records the sequence of tool calls (with args) made
	// during the current ProcessPrompt invocation. It is reset at the start
	// of each prompt and used by auto-skill detection at the end.
	lastToolTrace []skill.TraceStep
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
		maxIterations:   30,
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
	s.providerConfig = cfg
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.providerConfig.Model
}

// GetProvider returns the current LLM provider.
func (s *Supervisor) GetProvider() llm.Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.provider
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

// GetMCPClients returns all connected MCP clients.
func (s *Supervisor) GetMCPClients() map[string]*mcp.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]*mcp.Client, len(s.mcpClients))
	for k, v := range s.mcpClients {
		result[k] = v
	}
	return result
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
				query, _ := tc.Args["query"].(string)
				if strings.TrimSpace(query) == "" {
					return "", fmt.Errorf("argumen 'query' wajib diisi")
				}

				// Prefer semantic search; fall back to FTS when embeddings
				// are unavailable (custom provider without /embeddings) or
				// return no matches.
				var results []memory.SearchResult
				if s.provider != nil {
					if embedding, embErr := s.provider.GenerateEmbedding(query); embErr == nil && len(embedding) > 0 {
						if semRes, semErr := s.memStore.Search(embedding, s.workspaceID, 5); semErr == nil {
							results = semRes
						}
					}
				}

				// FTS fallback when semantic returned nothing or embedding was
				// unavailable.
				if len(results) == 0 {
					ftsQuery := sanitizeFTSQuery(query)
					if ftsQuery != "" {
						if fts, err := s.memStore.SearchFullText(ftsQuery, s.workspaceID, memory.MemoryFilters{Limit: 5}); err == nil {
							for _, m := range fts {
								results = append(results, memory.SearchResult{Memory: m})
							}
						}
					}
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

			if tc.Function == "skill_run" {
				name := getStr(tc.Args, "skill_name")
				if name == "" {
					return "", fmt.Errorf("argumen 'skill_name' wajib diisi")
				}
				sk, err := skill.Load(name)
				if err != nil {
					return "", fmt.Errorf("skill '%s' tidak ditemukan: %w", name, err)
				}
				res, err := sk.Run(s.SkillExecutor())
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("Skill '%s' dijalankan. Sukses=%v. %s", name, res.Success, res.Summary), nil
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

	// Fallback: Context7 tools via connected MCP servers if not in toolRoute
	// (handles cases where ListTools() discovery failed but server is connected)
	if !ok && (tc.Function == "resolve" || tc.Function == "get-library-documentation") {
		for name, mcpClient := range s.mcpClients {
			if mcpClient == nil || !strings.Contains(strings.ToLower(name), "context7") {
				continue
			}
			s.mu.RUnlock()
			result, err := mcpClient.CallTool(tc.Function, tc.Args)
			if err == nil && !result.IsError {
				var output strings.Builder
				for _, c := range result.Content {
					if c.Text != "" {
						output.WriteString(c.Text)
						output.WriteString("\n")
					}
				}
				out := strings.TrimSpace(output.String())
				if out != "" {
					return out, nil
				}
			}
			s.mu.RLock()
		}
	}
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
// the supervisor's existing tool routing (built-in + MCP + nested skills).
// If toolName is prefixed with "skill:", it loads and executes the named skill
// from the skill store recursively (max depth 5 to prevent infinite loops).
func (s *Supervisor) SkillExecutor() skill.StepExecutor {
	return s.skillExecutorWithDepth(0)
}

func (s *Supervisor) skillExecutorWithDepth(depth int) skill.StepExecutor {
	const maxDepth = 5
	return func(toolName string, args map[string]interface{}) (string, error) {
		if strings.HasPrefix(toolName, "skill:") {
			if depth >= maxDepth {
				return "", fmt.Errorf("skill recursion depth exceeded (%d)", maxDepth)
			}
			skName := strings.TrimPrefix(toolName, "skill:")
			sk, err := skill.Load(skName)
			if err != nil {
				return "", fmt.Errorf("skill '%s' not found: %w", skName, err)
			}
			sk = sk.WithArgs(args)
			result, err := sk.Run(s.skillExecutorWithDepth(depth + 1))
			if err != nil {
				return "", err
			}
			return result.Summary, nil
		}
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
	// Save reference to old current session before creating new one
	var oldCurrent *Session
	if cfg.CarryOverCount > 0 {
		oldCurrent = s.sessionRegistry.Current()
	}

	sess, err := s.sessionRegistry.Create(cfg)
	if err != nil {
		return nil, err
	}

	// Carry over last N message pairs from old current session if requested
	if cfg.CarryOverCount > 0 && oldCurrent != nil && oldCurrent.ID != sess.ID {
		msgCount := cfg.CarryOverCount * 2 // each turn = user + assistant
		if len(oldCurrent.History) < msgCount {
			msgCount = len(oldCurrent.History)
		}
		if msgCount > 0 {
			sess.History = make([]llm.Message, msgCount)
			copy(sess.History, oldCurrent.History[len(oldCurrent.History)-msgCount:])
		}
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

// DeleteSession deletes a session by ID from both registry and store.
func (s *Supervisor) DeleteSession(id string) error {
	if err := s.sessionRegistry.Delete(id); err != nil {
		return err
	}
	if s.sessionStore != nil {
		if err := s.sessionStore.DeleteSession(id); err != nil {
			return err
		}
	}
	return nil
}

// SearchResult holds a single session search result with relevance.
type SearchResult struct {
	Session *session.Session
	Score   float64
	Snippet string
}

// SearchSessions searches sessions using AI embeddings for semantic relevance.
func (s *Supervisor) SearchSessions(query string, topN int) ([]SearchResult, error) {
	sessions := s.ListSessions()
	if len(sessions) == 0 {
		return nil, nil
	}
	if s.provider == nil {
		return nil, fmt.Errorf("LLM provider tidak tersedia untuk generate embedding")
	}

	queryEmb, err := s.provider.GenerateEmbedding(query)
	if err != nil {
		return nil, fmt.Errorf("gagal generate embedding query: %w", err)
	}

	var results []SearchResult
	for _, sess := range sessions {
		// Build searchable text from session name, context, and history
		var parts []string
		parts = append(parts, sess.Name)
		if sess.Context != "" {
			parts = append(parts, sess.Context)
		}
		for _, msg := range sess.History {
			parts = append(parts, msg.Content)
		}
		text := strings.Join(parts, "\n")
		if len(text) > 8000 {
			text = text[:8000]
		}

		textEmb, err := s.provider.GenerateEmbedding(text)
		if err != nil {
			continue // skip sessions that fail embedding
		}

		score := cosineSimilarity(queryEmb, textEmb)

		// Build snippet: first matching line or first history message
		snippet := ""
		lines := strings.Split(text, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if len(line) > 10 {
				snippet = line
				break
			}
		}
		if len(snippet) > 120 {
			snippet = snippet[:120] + "..."
		}

		results = append(results, SearchResult{
			Session: sess,
			Score:   score,
			Snippet: snippet,
		})
	}

	// Sort by score descending
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if topN > 0 && len(results) > topN {
		results = results[:topN]
	}
	return results, nil
}

// cosineSimilarity computes cosine similarity between two float32 vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// GetSessionHistory returns the history messages for a session by ID.
func (s *Supervisor) GetSessionHistory(id string) ([]llm.Message, error) {
	sess, ok := s.sessionRegistry.Get(id)
	if !ok {
		return nil, fmt.Errorf("session tidak ditemukan: %s", id)
	}
	result := make([]llm.Message, len(sess.History))
	copy(result, sess.History)
	return result, nil
}

// ProcessPrompt handles a user prompt using the current agent mode.
func (s *Supervisor) ProcessPrompt(ctx context.Context, userPrompt string) (*PromptResult, error) {
	// Auto-discovery and exploration removed to respect user preference for manual triggers.

	// Reset the auto-skill-detection trace for this new prompt.
	s.lastToolTrace = nil

	// Safety net: if the user's prompt is a clear self-introduction
	// ("nama saya X", "panggil saya Y", etc), capture it to long-term
	// memory immediately — regardless of whether the LLM later remembers
	// to call the `remember` tool. This guarantees identity info is
	// always recorded the first time it's mentioned.
	if intro, ok := detectIntroduction(userPrompt); ok && s.memStore != nil {
		// Best-effort embedding (custom providers without /embeddings will
		// just save with NULL embedding — FTS will still find it).
		var emb []float32
		if s.provider != nil {
			emb, _ = s.provider.GenerateEmbedding(intro)
		}
		_, _ = s.memStore.Save(intro, "user_preference", "auto-intro", s.workspaceID, emb)
	}

	// Pre-process: Search relevant memories

	startTime := time.Now()
	modeInfo := GetModeInfo(s.mode)

	// 1. Search memory for relevant context
	var memContext string
	if s.memStore != nil {
		memContext = buildMemoryContext(s.memStore, s.provider, userPrompt, s.workspaceID)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// 2. Build messages with mode-specific system prompt
	sysPrompt := modeInfo.SystemPrompt
	if hostCtx, err := smarassh.AllHosts(); err == nil && hostCtx != "(tidak ada host SSH tersimpan)" {
		sysPrompt += "\n\nHost VPS/Server yang tersimpan (gunakan saat user menyebut vps/server/remote):\n" + hostCtx
	}
	sysPrompt += buildSkillContext()
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

	// Inject graph context when user asks about codebase
	if BuiltinDB != nil && IsCodebaseQuery(userPrompt) {
		graphCtx, _ := BuildGraphContext(BuiltinDB, userPrompt, s.workspaceID)
		if graphCtx != "" {
			messages = append(messages, llm.Message{
				Role:    llm.RoleSystem,
				Content: graphCtx,
			})
		}
	}

	// Auto-resolve Context7 documentation for libraries mentioned in the prompt
	if s.mcpClients != nil && len(s.mcpClients) > 0 {
		injector := NewContext7Injector()
		enrichedPrompt, _, err := injector.DetectAndInject(userPrompt, s.SkillExecutor())
		if err == nil && enrichedPrompt != userPrompt {
			// Replace the user prompt with enriched version that includes Context7 docs
			userPrompt = enrichedPrompt
		}
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
	var finalThoughts []string
	var finalToolsExecuted []string

	// Use agentic loop if tools are available, regardless of mode (with different behavior)
	tools := s.ConvertMCPToolsToToolFunctions()

	if len(tools) > 0 {
		resp, thinking, thoughts, executed, err := s.RunAgenticLoop(ctx, userPrompt)
		if err != nil {
			return nil, err
		}
		finalResp = resp
		finalThinking = thinking
		finalThoughts = thoughts
		finalToolsExecuted = executed
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

	if ctx.Err() != nil {
		return nil, ctx.Err()
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

	// 5. Update stats (estimate tokens: ~4 chars per token) and cost using
	// provider/model-specific pricing.
	inputTokens := len(userPrompt) / 4
	outputTokens := len(finalResp) / 4
	totalTokens := inputTokens + outputTokens
	providerName, modelName := s.GetModelInfo()
	if providerName == "" || providerName == "unknown" {
		providerName = s.GetProviderName()
	}
	estimatedCost := metrics.EstimateCost(providerName, modelName, int64(inputTokens), int64(outputTokens))

	duration := time.Since(startTime)
	s.updateStats(totalTokens, estimatedCost, duration)

	s.mu.Lock()
	s.stats.InputTokens += inputTokens
	s.stats.OutputTokens += outputTokens
	s.stats.LastDuration = duration
	s.mu.Unlock()

	result := &PromptResult{
		Response:      finalResp,
		Thinking:      finalThinking,
		Thoughts:      finalThoughts,
		ToolsExecuted: finalToolsExecuted,
		InputTokens:   inputTokens,
		OutputTokens:  outputTokens,
		TotalTokens:   totalTokens,
		Duration:      duration,
	}

	// 6. Save interaction to memory
	if s.memStore != nil {
		tag := fmt.Sprintf("mode:%s", s.mode)
		content := fmt.Sprintf("Q: %s\nA: %s", userPrompt, truncate(finalResp, 500))
		embedding, _ := s.provider.GenerateEmbedding(content)
		s.memStore.Save(content, tag, "supervisor", s.workspaceID, embedding)
	}

	// 7. Auto-skill detection: record trace and capture if a repeating pattern
	// has been observed. Runs asynchronously so it never blocks the reply.
	if len(s.lastToolTrace) >= 2 && BuiltinDB != nil {
		trace := skill.ExecutionTrace{
			PromptText:  userPrompt,
			Steps:       append([]skill.TraceStep(nil), s.lastToolTrace...),
			CompletedAt: time.Now(),
		}
		go s.autoDetectAndCapture(trace)
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
		memContext = buildMemoryContext(s.memStore, s.provider, userPrompt, s.workspaceID)
	}

	// 2. Build initial messages
	sysPrompt2 := modeInfo.SystemPrompt
	if hostCtx2, err := smarassh.AllHosts(); err == nil && hostCtx2 != "(tidak ada host SSH tersimpan)" {
		sysPrompt2 += "\n\nHost VPS/Server yang tersimpan (gunakan saat user menyebut vps/server/remote):\n" + hostCtx2
	}
	sysPrompt2 += buildSkillContext()
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

	// 4. Agentic loop with adaptive iteration budget.
	// Budget is mode-aware (ASK small, WORKFLOW large) and self-extending
	// while the model is making progress. Stuck-loop detector terminates
	// early when the same tool call repeats too often.
	budget := NewIterationBudget(s.mode, s.maxIterations)
	s.activeBudgetMu.Lock()
	s.activeBudget = budget
	s.activeBudgetMu.Unlock()
	SetActiveBudgetController(s)
	defer func() {
		s.activeBudgetMu.Lock()
		s.activeBudget = nil
		s.activeBudgetMu.Unlock()
		SetActiveBudgetController(nil)
	}()
	for iteration := 0; budget.ShouldContinue(iteration); iteration++ {
		if ctx.Err() != nil {
			return "", "", nil, nil, ctx.Err()
		}

		// Callback: report iteration
		if s.callback.OnIteration != nil {
			s.callback.OnIteration(iteration+1, budget.Limit())
		}

		// Call LLM with tools
		var resp *llm.ChatResponse
		var toolCalls []llm.ToolCall
		var err error

		// Wrap stream callback to emit phase changes (skip if context cancelled).
		// Snapshot user callbacks INTO LOCAL VARS so a concurrent SetCallback
		// (e.g. handleWSChat clearing callbacks after ProcessPrompt returns)
		// does not race with in-flight streaming chunks. Without this snapshot,
		// `s.callback.OnPhaseChange` could be non-nil at the outer `if` check
		// and become nil by the time the inner stream handler fires — that's
		// the panic at supervisor.go:1471 we observed in production.
		var streamCb llm.StreamCallback
		cbSnap := s.snapshotCallback()
		onPhase := cbSnap.OnPhaseChange
		onStream := cbSnap.OnStream
		if onPhase != nil {
			streamCb = func(chunk string, isThinking bool, phaseHint llm.PhaseHint) {
				defer func() {
					if r := recover(); r != nil {
						// Stream callback panic must not kill the LLM goroutine.
						// Drop the chunk; the agent will still get the final response.
					}
				}()
				if ctx.Err() != nil {
					return
				}
				phaseName := phaseNameFromHint(phaseHint)
				if phaseName != "" && onPhase != nil {
					onPhase(phaseName, phaseDescFromHint(phaseHint))
				}
				if onStream != nil {
					onStream(chunk, isThinking)
				}
			}
		} else {
			streamCb = func(chunk string, isThinking bool, _ llm.PhaseHint) {
				defer func() {
					if r := recover(); r != nil {
					}
				}()
				if ctx.Err() != nil {
					return
				}
				if onStream != nil {
					onStream(chunk, isThinking)
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

		if ctx.Err() != nil {
			return "", "", nil, nil, ctx.Err()
		}

		// Accumulate thinking
		if resp.Thinking != "" {
			allThinking = append(allThinking, resp.Thinking)
		}

		// Some LLMs (e.g. deepseek-v4-flash) emit tool calls as DSML/XML inside content
		// alongside or instead of native tool_calls. Always strip DSML from content and
		// merge any extracted tool calls with native ones.
		if resp != nil {
			extracted, cleaned := llm.ExtractToolCallsFromContent(resp.Content)
			resp.Content = cleaned
			toolCalls = append(toolCalls, extracted...)
		}

		if len(toolCalls) == 0 {
			// No tool calls — LLM gave final answer
			if s.callback.OnPhaseChange != nil {
				s.callback.OnPhaseChange("Generating", "Formulating final response...")
			}
			// If the "final" answer turned out to be pure DSML (happens when
			// DeepSeek-style models end with a tool-call block but the
			// extractor already converted those into real tool_calls) the
			// cleaned content is empty. Fall back to thoughts+tools so the
			// user sees something actionable instead of a blank bubble.
			content := strings.TrimSpace(resp.Content)
			if content == "" {
				content = synthesizeFallbackSummary(thoughts, toolsExecuted)
			}
			return content, strings.Join(allThinking, "\n\n"), thoughts, toolsExecuted, nil
		}

		// If we are here, LLM wants to call tools
		if s.callback.OnPhaseChange != nil {
			s.callback.OnPhaseChange("Exploring", "Gathering data with tools...")
		}

		// Capture this intermediate content as a "Thought"
		if resp.Content != "" {
			thoughts = append(thoughts, resp.Content)
		}

		// Update toolsExecuted list AND record fingerprints for stuck-loop detection.
		stuckLoop := false
		for _, tc := range toolCalls {
			toolsExecuted = append(toolsExecuted, tc.Function)
			// Record full call for auto-skill pattern detection.
			s.lastToolTrace = append(s.lastToolTrace, skill.TraceStep{
				Tool: tc.Function,
				Args: tc.Args,
			})
			// Update budget; flag if a stuck loop is detected.
			if budget.RecordToolCalls(tc.Function, tc.Args) {
				stuckLoop = true
			}
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

		var imageToolOutputs []string

		// Execute each tool and add results to messages
		for _, tc := range toolCalls {
			if ctx.Err() != nil {
				return "", "", nil, nil, ctx.Err()
			}
			result, err := s.executeToolCall(tc)
			toolOutput := result
			if err != nil {
				toolOutput = fmt.Sprintf("Error: %s", err)
			}
			toolOutput = truncateToolResultForContext(toolOutput)
			if tc.Function == "generate_image" && err == nil && strings.Contains(toolOutput, "Path:") {
				imageToolOutputs = append(imageToolOutputs, toolOutput)
			}

			// Callback: report result
			if s.callback.OnToolResult != nil {
				s.callback.OnToolResult(toolOutput)
			}

			// Add tool result as a user message with tool_call_id
			toolMsg := llm.Message{
				Role:       llm.RoleTool,
				Content:    toolOutput,
				ToolCallID: tc.ID,
			}
			messages = append(messages, toolMsg)
		}

		if len(imageToolOutputs) > 0 {
			return strings.Join(imageToolOutputs, "\n\n"), strings.Join(allThinking, "\n\n"), thoughts, toolsExecuted, nil
		}

		// Stuck-loop bailout: if we detect the same tool+args repeating,
		// inject a steering message and exit the loop early so we don't
		// keep wasting tokens on a clearly stuck model.
		if stuckLoop {
			messages = append(messages, llm.Message{
				Role: llm.RoleSystem,
				Content: "STOP: Tool yang sama dipanggil berulang dengan argumen yang sama. " +
					"Berhenti memanggil tool. Jelaskan ke user apa yang sudah dilakukan, " +
					"apa hasilnya, dan apa yang menyebabkan loop ini (mis. permission denied, " +
					"service tidak ada, atau argumen salah). Berikan saran langkah berikutnya.",
			})
			break
		}

		// Loop continues — LLM will process tool results and either call more tools or give final answer
	}

	// Max iterations reached — try to get a final answer. We explicitly
	// instruct the model NOT to emit tool calls anymore so the final turn
	// is pure prose. Some models (notably DeepSeek-v4 family) like to
	// continue emitting DSML even in the final answer; stripping those
	// tags leaves an empty string and the user sees "Tidak ada output teks"
	// — that is the bug we're patching here.
	if s.callback.OnPhaseChange != nil {
		s.callback.OnPhaseChange("Generating", "Max iterations reached. Formulating final response...")
	}

	messages = append(messages, llm.Message{
		Role:    llm.RoleSystem,
		Content: "BATAS iterasi tool tercapai. STOP calling tools. Sekarang tuliskan JAWABAN FINAL untuk user dalam Bahasa Indonesia, berdasarkan semua hasil tool yang sudah terkumpul di atas.\n\nATURAN PENTING:\n- JANGAN keluarkan tool_calls atau format DSML <|DSML|...>.\n- Jawaban harus berupa teks biasa / markdown normal.\n- Ringkas apa yang sudah dilakukan dan state akhir dari tugas.\n- Jika tugas belum selesai, katakan secara eksplisit apa yang belum selesai dan kenapa.",
	})

	resp, err := s.provider.Chat(messages)
	if err != nil {
		return "", "", nil, nil, fmt.Errorf("gagal mendapatkan response final: %w", err)
	}

	// Also strip DSML on the fallback path — some models still emit DSML
	// tool-call markup in the final answer, which would otherwise leak to
	// the platform reply.
	var finalContent string
	if resp != nil {
		_, cleaned := llm.ExtractToolCallsFromContent(resp.Content)
		finalContent = strings.TrimSpace(cleaned)
	}

	// If the fallback call still produced an empty response (pure DSML
	// that got stripped), synthesize a useful summary from the
	// intermediate thoughts and tool list instead of returning "".
	// This is what prevents the "✓ Selesai. (Tidak ada output teks)"
	// state when the model refuses to settle on a final answer.
	if finalContent == "" {
		finalContent = synthesizeFallbackSummary(thoughts, toolsExecuted)
	}

	if s.callback.OnPhaseChange != nil {
		s.callback.OnPhaseChange("Done", "Finished")
	}

	return finalContent, strings.Join(allThinking, "\n\n"), thoughts, toolsExecuted, nil
}

func truncateToolResultForContext(output string) string {
	if len(output) <= maxToolResultChars {
		return output
	}

	tailChars := maxToolResultChars - toolResultHeadChars
	if tailChars < 0 {
		tailChars = 0
	}
	omitted := len(output) - toolResultHeadChars - tailChars
	if omitted < 0 {
		omitted = 0
	}

	return output[:toolResultHeadChars] + fmt.Sprintf("\n\n[... %d characters omitted from tool result to keep the LLM context within limits ...]\n\n", omitted) + output[len(output)-tailChars:]
}

// synthesizeFallbackSummary builds a human-readable recap when the LLM
// couldn't produce a clean final answer. Uses any plaintext intermediate
// thoughts plus the list of tools it ran so the user at least knows what
// happened and can make a follow-up decision.
func synthesizeFallbackSummary(thoughts, toolsExecuted []string) string {
	var sb strings.Builder
	sb.WriteString("⚠ Smara mencapai batas iterasi tool tanpa menyelesaikan jawaban final.\n\n")

	// Pick the last few non-empty thoughts — they're usually the most
	// contextual ("Saya cek X, ternyata Y, lanjut Z").
	kept := 0
	for i := len(thoughts) - 1; i >= 0 && kept < 3; i-- {
		t := strings.TrimSpace(thoughts[i])
		if t == "" {
			continue
		}
		// Strip any leftover DSML shards that didn't make it through the
		// per-iteration strip (defense in depth).
		_, cleaned := llm.ExtractToolCallsFromContent(t)
		cleaned = strings.TrimSpace(cleaned)
		if cleaned == "" {
			continue
		}
		if len(cleaned) > 300 {
			cleaned = cleaned[:300] + "…"
		}
		sb.WriteString("• " + cleaned + "\n")
		kept++
	}
	if kept == 0 {
		sb.WriteString("(Tidak ada komentar intermediate yang bisa ditampilkan.)\n")
	}

	if len(toolsExecuted) > 0 {
		sb.WriteString(fmt.Sprintf("\n🔧 Tools dijalankan (%d): ", len(toolsExecuted)))
		// Dedupe sambil preserve order, batas 10
		seen := map[string]bool{}
		var list []string
		for _, t := range toolsExecuted {
			if !seen[t] {
				seen[t] = true
				list = append(list, t)
				if len(list) >= 10 {
					break
				}
			}
		}
		sb.WriteString(strings.Join(list, ", "))
		if len(seen) > 10 {
			sb.WriteString(fmt.Sprintf(" …dan %d lainnya", len(seen)-10))
		}
	}

	sb.WriteString("\n\nSaran: pecah tugas jadi langkah lebih kecil atau kirim pertanyaan baru yang lebih spesifik.")
	return sb.String()
}

// ClearHistory clears the conversation history and session registry history.
func (s *Supervisor) ClearHistory() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = make([]llm.Message, 0)
	if sess := s.sessionRegistry.Current(); sess != nil {
		sess.History = make([]llm.Message, 0)
		sess.UpdatedAt = time.Now()
		if s.sessionStore != nil {
			s.sessionStore.UpdateSession(sess)
		}
	}
}

// SaveSession persists the current session to the store.
func (s *Supervisor) SaveSession() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess := s.sessionRegistry.Current(); sess != nil {
		sess.UpdatedAt = time.Now()
		if s.sessionStore != nil {
			return s.sessionStore.UpdateSession(sess)
		}
	}
	return nil
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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callback = cb
}

// snapshotCallback returns a value-copy of the current callback under the
// supervisor lock. Use this from goroutines (stream handlers, etc.) instead
// of dereferencing s.callback fields directly to avoid races with concurrent
// SetCallback writes.
func (s *Supervisor) snapshotCallback() AgenticCallback {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.callback
}

// RequestIterationExtension allows the running prompt to extend its own
// iteration budget. Returns the structured ExtensionRequest result. If no
// prompt is currently running the call is denied.
func (s *Supervisor) RequestIterationExtension(amount int, reason string) ExtensionRequest {
	s.activeBudgetMu.RLock()
	b := s.activeBudget
	s.activeBudgetMu.RUnlock()
	if b == nil {
		return ExtensionRequest{Denial: "tidak ada prompt aktif (budget hanya hidup selama ProcessPrompt)"}
	}
	return b.RequestExtension(amount, reason)
}

// IterationBudgetSnapshot returns a snapshot of the active budget, or zero
// value if no prompt is running.
func (s *Supervisor) IterationBudgetSnapshot() (BudgetSnapshot, bool) {
	s.activeBudgetMu.RLock()
	b := s.activeBudget
	s.activeBudgetMu.RUnlock()
	if b == nil {
		return BudgetSnapshot{}, false
	}
	return b.Snapshot(), true
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
		"skill_delete",
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
