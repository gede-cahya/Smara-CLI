package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/cognitive"
	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/lsp"
	"github.com/gede-cahya/Smara-CLI/internal/mcp"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/gede-cahya/Smara-CLI/internal/repair"
	"github.com/gede-cahya/Smara-CLI/internal/safety"
	"github.com/gede-cahya/Smara-CLI/internal/session"
	"github.com/gede-cahya/Smara-CLI/internal/sync"
	"github.com/gede-cahya/Smara-CLI/internal/ui"
	"github.com/gede-cahya/Smara-CLI/internal/ui/components"
)

var (
	model     string
	offline   bool
	startMode string
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Mulai sesi interaktif Smara",
	Long: `Memulai sesi interaktif dengan agen AI Smara.
	
Alur: Config → SQLite Init → Sync Daemon → Supervisor Agent → REPL`,
	RunE: runStart,
}

func init() {
	startCmd.Flags().StringVarP(&model, "model", "m", "", "model LLM yang digunakan (default: dari config)")
	startCmd.Flags().BoolVar(&offline, "offline", false, "jalankan tanpa sync daemon")
	startCmd.Flags().StringVar(&startMode, "mode", "ask", "mode agen: ask, rush, plan, test, image, workflow, parallel")
}

func runStart(cmd *cobra.Command, args []string) error {
	startTime := time.Now()
	cfg := config.Get()

	// Auto-repair critical components before startup
	if repaired, err := repair.AutoRepairAtStartup(cfg.DBPath, ""); err != nil {
		fmt.Fprintf(os.Stderr, "[AUTO-REPAIR] Kegagalan kritis: %v\n", err)
		fmt.Fprintln(os.Stderr, "Jalankan 'smara repair' atau perbaiki secara manual.")
		os.Exit(1)
	} else if repaired {
		// Reload config after repair
		cfg = config.Get()
	}

	// Set UI version and show banner
	ui.AppVersion = version
	ui.PrintBanner(version)

	// Override model from flag if provided
	if model != "" {
		cfg.Model = model
	}

	// 1. Initialize LLM Provider
	ui.PrintInfo("Menghubungkan ke %s (%s)...", cfg.Provider, cfg.Model)

	// Build provider config with appropriate API key
	providerCfg := llm.ProviderConfig{
		Name:            cfg.Provider,
		Model:           cfg.Model,
		Host:            cfg.OllamaHost,
		APIKey:          "",
		ReasoningEffort: cfg.ReasoningEffort,
	}

	// Set API key based on provider
	switch cfg.Provider {
	case "openai":
		providerCfg.APIKey = cfg.OpenAIAPIKey
		providerCfg.Host = cfg.OpenAIBaseURL
	case "openrouter":
		providerCfg.APIKey = cfg.OpenRouterAPIKey
		providerCfg.Host = "" // uses default OpenRouter host
		if cfg.Model == "" || cfg.Model == "minimax-m2.5:cloud" {
			providerCfg.Model = cfg.OpenRouterModel
		}
	case "anthropic":
		providerCfg.APIKey = cfg.AnthropicAPIKey
		providerCfg.Host = "" // uses default Anthropic host
		if cfg.Model == "" || cfg.Model == "minimax-m2.5:cloud" {
			providerCfg.Model = cfg.AnthropicModel
		}
	case "custom":
		providerCfg.APIKey = cfg.CustomAPIKey
		providerCfg.Host = cfg.CustomBaseURL
	}

	provider, err := llm.NewProvider(providerCfg)
	if err != nil {
		return fmt.Errorf("gagal inisialisasi LLM provider: %w", err)
	}
	ui.PrintSuccess("Provider: %s — Model: %s", provider.Name(), providerCfg.Model)

	// 2. Initialize Memory Store
	ui.PrintInfo("Membuka database memori...")
	memStore, err := memory.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("gagal inisialisasi memory store: %w", err)
	}
	defer memStore.Close()
	ui.PrintSuccess("Database: %s", cfg.DBPath)

	// 3. Start Background Sync Daemon
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if !offline {
		syncCfg := sync.SyncConfig{
			SyncDir:          cfg.SyncDir,
			IntervalMin:      cfg.SyncInterval,
			Enabled:          true,
			NineDriveEnabled: cfg.NineDriveEnabled,
			NineDriveBaseURL: cfg.NineDriveBaseURL,
			NineDriveAPIKey:  cfg.NineDriveAPIKey,
		}
		daemon := sync.NewDaemon(syncCfg, memStore)
		daemon.Start(ctx)
		defer daemon.Stop()
		ui.PrintSuccess("Sync daemon aktif (interval: %d menit)", cfg.SyncInterval)
	} else {
		ui.PrintWarning("Mode offline — sync daemon dinonaktifkan")
	}

	// 4. Initialize Supervisor Agent
	supervisor := agent.NewSupervisorWithConfig(provider, providerCfg, memStore)
	defer supervisor.Close()

	// Honor configurable agent iteration cap so long agentic chains
	// (multi-step roadmaps, refactors) aren't cut off prematurely.
	if cfg.AgentMaxIterations > 0 {
		supervisor.SetMaxIterations(cfg.AgentMaxIterations)
	}

	// 4.1 Attach Safety Engine
	safetyEngine := safety.NewEngine()
	supervisor.SetSafetyEngine(safetyEngine)

	// 4.2 Attach Cognitive Validator
	cognitiveValidator := cognitive.NewValidator()
	supervisor.SetCognitiveValidator(cognitiveValidator)

	// 4.3 Attach LSP Manager
	lspManager := lsp.NewManager()
	defer lspManager.CloseAll()
	supervisor.SetLSPManager(lspManager)

	// 4.x Workspace Initialization
	activeWorkspaceName := cfg.ActiveWorkspace
	if activeWorkspaceName == "" {
		activeWorkspaceName = "default"
	}

	w, err := memStore.GetWorkspaceByName(activeWorkspaceName)
	if err != nil {
		ui.PrintWarning("Gagal memuat workspace: %v", err)
	} else if w == nil {
		// Create default workspace if it doesn't exist
		ui.PrintInfo("Membuat workspace default...")
		w, err = memStore.CreateWorkspace(activeWorkspaceName, "")
		if err != nil {
			ui.PrintWarning("Gagal membuat workspace default: %v", err)
		}
	}

	if w != nil {
		supervisor.SetWorkspaceID(w.ID)
		cfg.ActiveWorkspaceID = w.ID
		ui.PrintSuccess("Workspace Aktif: %s", w.Name)
	}

	// 4.0 Initialize Session Store
	sessStore, err := session.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		ui.PrintWarning("Gagal inisialisasi session store: %v", err)
	} else {
		supervisor.SetSessionStore(sessStore)
		// Ensure session is saved when TUI exits normally (ESC, Ctrl+Q, exit command, etc.)
		defer func() {
			if err := supervisor.SaveSession(); err != nil {
				ui.PrintWarning("Gagal menyimpan session saat keluar: %v", err)
			}
		}()
	}

	// 4.1 Initialize Sessions & Auto-Connection
	ui.PrintInfo("Memuat session dari database...")
	if err := supervisor.InitializeSessions(); err != nil {
		ui.PrintWarning("Gagal memuat session: %v", err)
	} else {
		// Attempt to auto-connect to last active session
		lastSess, err := supervisor.GetLastActiveSession()
		if err == nil && lastSess != nil {
			ui.PrintSuccess("Auto-connect ke session terakhir: %s (%s)", lastSess.Name, lastSess.ID[:8])
			supervisor.SwitchSession(lastSess.ID)
		} else {
			// No active session found, create a new one automatically
			ui.PrintInfo("Tidak ada session aktif, membuat session baru...")
			newSess, err := supervisor.CreateSession(agent.SessionConfig{
				Name:        "Auto Session",
				WorkspaceID: supervisor.GetWorkspaceID(),
				Mode:        string(agent.ModeAsk),
				MCPServers:  supervisor.ListMCPServers(),
			})
			if err == nil {
				ui.PrintSuccess("Session baru dibuat: %s (%s)", newSess.Name, newSess.ID[:8])
			}
		}
	}

	// Set initial mode (if override via flag)
	if startMode != "ask" && agent.ValidMode(startMode) {
		mode := agent.Mode(startMode)
		if mode == agent.ModeRush {
			agent.EnableRushAutoApprovalEnv()
		}
		supervisor.SetMode(mode)
	} else if agent.IsRushAutoApprovalEnabled() {
		// Env vars indicate RUSH autonomous execution; auto-set mode.
		agent.EnableRushAutoApprovalEnv()
		supervisor.SetMode(agent.ModeRush)
	}
	modeInfo := agent.GetModeInfo(supervisor.GetMode())
	ui.PrintSuccess("Mode: %s %s — %s", modeInfo.Emoji, modeInfo.Label, modeInfo.Description)

	// 5. Connect MCP Servers — auto-import from OpenCode if available
	var mcpConfigs []mcp.MCPServerConfig

	// Try to load from OpenCode config first
	ocPath := mcp.OpenCodeConfigPath()
	if ocPath != "" {
		ui.PrintInfo("OpenCode config ditemukan: %s", ocPath)
		ocServers, err := mcp.LoadOpenCodeMCPServers()
		if err == nil && len(ocServers) > 0 {
			mcpConfigs = append(mcpConfigs, ocServers...)
			ui.PrintSuccess("Mengimpor %d MCP server dari OpenCode", len(ocServers))
		}
	}

	// Try to load from Windsurf config
	wsPath := mcp.WindsurfConfigPath()
	if wsPath != "" {
		ui.PrintInfo("Windsurf config ditemukan: %s", wsPath)
		wsServers, err := mcp.LoadWindsurfMCPServers()
		if err == nil && len(wsServers) > 0 {
			mcpConfigs = append(mcpConfigs, wsServers...)
			ui.PrintSuccess("Mengimpor %d MCP server dari Windsurf", len(wsServers))
		}
	}

	// Also add any Smara-native configs
	for _, mcpCfg := range cfg.MCPServers {
		mcpType := mcpCfg.Type
		if mcpType == "" {
			mcpType = "local"
		}
		mcpConfigs = append(mcpConfigs, mcp.MCPServerConfig{
			Name:    mcpCfg.Name,
			Type:    mcpType,
			Command: mcpCfg.Command,
			Args:    mcpCfg.Args,
			URL:     mcpCfg.URL,
			Headers: mcpCfg.Headers,
			Env:     mcpCfg.Env,
			Enabled: mcpCfg.Enabled,
		})
	}

	// Deduplicate: keep only the last occurrence of each server name
	seen := make(map[string]bool)
	var deduped []mcp.MCPServerConfig
	for i := len(mcpConfigs) - 1; i >= 0; i-- {
		if seen[mcpConfigs[i].Name] {
			continue
		}
		seen[mcpConfigs[i].Name] = true
		deduped = append([]mcp.MCPServerConfig{mcpConfigs[i]}, deduped...)
	}
	mcpConfigs = deduped

	var enabledConfigs []mcp.MCPServerConfig
	for _, cfg := range mcpConfigs {
		if cfg.Enabled {
			enabledConfigs = append(enabledConfigs, cfg)
		}
	}

	connectMCPServersForStartup(supervisor, enabledConfigs)

	// Show startup time
	elapsed := time.Since(startTime)
	ui.PrintInfo("Startup: %s", elapsed.Round(time.Millisecond))
	fmt.Println()

	// 6. Handle OS signals for graceful shutdown (SIGINT=Ctrl+C, SIGTERM=kill, SIGHUP=terminal closed, SIGQUIT=Ctrl+\)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)

	go func() {
		<-sigCh
		fmt.Println()
		ui.PrintInfo("Menutup Smara...")
		// Persist current session before exit
		if err := supervisor.SaveSession(); err != nil {
			ui.PrintWarning("Gagal menyimpan session: %v", err)
		}
		cancel()
		os.Exit(0)
	}()

	// 6.5 Inject Project Context
	projectContext := loadProjectContext()
	if projectContext != "" {
		ui.PrintInfo("Memuat konteks proyek lokal...")
		supervisor.AddContext(projectContext)
	}

	// 7. Start TUI
	appModel := ui.InitialModel(supervisor, func(cmd string, args []string) {
		handleCommand(cmd, args, supervisor, memStore, nil)
	}, memStore)

	// Track current phase locally to tag stream messages
	var currentPhase string

	// Setup callback for streaming
	supervisor.SetCallback(agent.AgenticCallback{
		OnStream: func(chunk string, isThinking bool) {
			p := ui.GetGlobalProgram()
			if p != nil {
				p.Send(ui.StreamMsg{Chunk: chunk, IsThinking: isThinking, Phase: currentPhase})
			}
		},
		OnPhaseChange: func(phase, description string) {
			currentPhase = phase
			p := ui.GetGlobalProgram()
			if p != nil {
				p.Send(ui.PhaseMsg{Phase: phase, Description: description})
			}
		},
		OnLog: func(role, content string) {
			p := ui.GetGlobalProgram()
			if p != nil {
				p.Send(ui.LogMsg{
					Message: ui.ChatMessage{
						Role:    role,
						Content: content,
					},
				})
			}
		},
		OnToolCall: func(server, tool string, args map[string]interface{}) {
			p := ui.GetGlobalProgram()
			if p != nil {
				argsJSON, _ := json.Marshal(args)
				content := fmt.Sprintf("▸ %s: %s", tool, string(argsJSON))
				p.Send(ui.StreamMsg{Chunk: content + "\n", IsThinking: false, Phase: "Exploring"})
			}
		},
		OnToolResult: func(output string) {
			p := ui.GetGlobalProgram()
			if p != nil {
				content := fmt.Sprintf("◂ result: %s\n", components.HyperlinkURLs(formatToolResultPreview(output)))
				p.Send(ui.StreamMsg{Chunk: content, IsThinking: false, Phase: "Exploring"})
			}
		},
		OnConfirm: func(message string) bool {
			p := ui.GetGlobalProgram()
			if p == nil {
				return true // Auto-confirm if no TUI
			}
			respCh := make(chan bool)
			p.Send(ui.ConfirmRequestMsg{
				Message:    message,
				ResponseCh: respCh,
			})
			return <-respCh
		},
		OnExplore: func(path string, _ string) {
			p := ui.GetGlobalProgram()
			if p == nil {
				return
			}
			if results, err := ui.ExploreCodebase(path, 2); err == nil {
				exploreOutput := ui.RenderExplore(results)
				p.Send(ui.ExploreMsg{Path: path, Content: exploreOutput})
			}
		},
	})

	// Pre-load chat history into the UI if there is an active session
	if session := supervisor.GetCurrentSession(); session != nil && len(session.History) > 0 {
		var hist []struct{ Role, Content string }
		for _, msg := range session.History {
			hist = append(hist, struct{ Role, Content string }{Role: string(msg.Role), Content: msg.Content})
		}
		appModel.LoadHistory(hist)
	}

	p := ui.NewProgram(appModel)
	ui.SetGlobalProgram(p)

	// Pass mainCtx to UI if needed, but Tea program manages its own lifecycle mostly.
	if _, err := p.Run(); err != nil {
		ui.PrintError("Error starting TUI: %v", err)
	}

	return nil
}

func handleCommand(cmd string, args []string, supervisor *agent.Supervisor, memStore memory.MemoryStore, prompt *ui.Prompt) {
	switch cmd {
	case "help":
		ui.PrintHelp()
	case "mode":
		if len(args) == 0 {
			// Show current mode and all available modes
			current := supervisor.GetMode()
			var msgParts []string
			for _, m := range agent.AllModes() {
				marker := "  "
				if m.Name == current {
					marker = "▸"
				}
				msgParts = append(msgParts, fmt.Sprintf("%s %s %s — %s", marker, m.Emoji, m.Label, m.Description))
			}
			ui.PrintInfo("Mode tersedia:\n%s", strings.Join(msgParts, "\n"))
			return
		}
		newMode := args[0]
		if !agent.ValidMode(newMode) {
			ui.PrintError("Mode tidak valid: %s (pilih: ask, rush, plan, test, workflow)", newMode)
			return
		}
		supervisor.SetMode(agent.Mode(newMode))
		info := agent.GetModeInfo(agent.Mode(newMode))
		ui.PrintModeChange(newMode, info.Emoji, info.Description)
	case "model":
		handleModelCommand(args, supervisor)
	case "memory":
		memories, err := memStore.List(config.Get().ActiveWorkspaceID, 10)
		if err != nil {
			ui.PrintError("Gagal membaca memori: %v", err)
			return
		}
		if len(memories) == 0 {
			ui.PrintInfo("Belum ada memori tersimpan.")
			return
		}
		var msgParts []string
		for _, m := range memories {
			msgParts = append(msgParts, fmt.Sprintf("[%d] %s — %s", m.ID, truncateStr(m.Content, 80), m.CreatedAt.Format("02 Jan 15:04")))
		}
		ui.PrintInfo("Memori tersimpan:\n%s", strings.Join(msgParts, "\n"))
	case "mcp":
		mcpInfo := supervisor.GetMCPInfo()
		if len(mcpInfo) == 0 {
			ui.PrintInfo("Belum ada MCP server yang terhubung.")
			return
		}
		var msgParts []string
		for name, info := range mcpInfo {
			status := "connected"
			if !info.Connected {
				status = "error"
			}
			msgParts = append(msgParts, fmt.Sprintf("%s — %s", name, status))
			if len(info.Tools) > 0 {
				for _, tool := range info.Tools {
					desc := tool.Description
					if len(desc) > 60 {
						desc = desc[:60] + "..."
					}
					msgParts = append(msgParts, fmt.Sprintf("  ├── %s: %s", tool.Name, desc))
				}
			} else if info.Error != "" {
				msgParts = append(msgParts, fmt.Sprintf("  └── Error: %s", info.Error))
			}
		}
		ui.PrintInfo("MCP Servers:\n%s", strings.Join(msgParts, "\n"))
	case "clear":
		// handled by app.go
	case "session":
		handleSessionCommand(args, supervisor)
	case "explore":
		handleExploreCommand(args)
	default:
		ui.PrintWarning("Perintah tidak dikenali: /%s", cmd)
	}
}

func handleExploreCommand(args []string) {
	path := "."
	depth := 2

	for _, arg := range args {
		if arg == "--interactive" {
			ui.PrintInfo("Meluncurkan explore interaktif...")
			if err := ui.RunExploreInteractive(path, depth); err != nil {
				ui.PrintError("Gagal membuka explore interaktif: %v", err)
			}
			return
		}
		if strings.HasPrefix(arg, "--depth=") {
			d, err := strconv.Atoi(strings.TrimPrefix(arg, "--depth="))
			if err == nil && d > 0 {
				depth = d
			}
		} else if arg != "" && !strings.HasPrefix(arg, "-") {
			path = arg
		}
	}

	results, err := ui.ExploreCodebase(path, depth)
	if err != nil {
		ui.PrintError("Gagal mengeksplorasi codebase: %v", err)
		return
	}
	ui.PrintInfo("Struktur direktori %s:\n%s", path, ui.RenderExplore(results))
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// formatToolResultPreview keeps live CLI tool previews readable. Raw HTTP
// responses and verbose build logs can be very noisy, so show compact
// structured summaries instead of dumping the whole output in chat.
func formatToolResultPreview(output string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(output, "▶", ""))
	if trimmed == "" {
		return "(kosong)"
	}
	if summary, ok := compactHTTPResponsePreview(trimmed); ok {
		return summary
	}
	if summary, ok := compactBuildLogPreview(trimmed); ok {
		return summary
	}
	return truncateStr(singleLine(trimmed), 300)
}

func compactHTTPResponsePreview(output string) (string, bool) {
	firstLine := firstNonEmptyLine(output)
	if !strings.HasPrefix(firstLine, "HTTP/") {
		return "", false
	}

	status := firstLine
	contentType := ""
	server := ""
	cache := ""
	matchedPath := ""
	body := ""

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "▶"))
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "content-type:"):
			contentType = strings.TrimSpace(line[len("content-type:"):])
		case strings.HasPrefix(lower, "server:"):
			server = strings.TrimSpace(line[len("server:"):])
		case strings.HasPrefix(lower, "x-vercel-cache:"):
			cache = strings.TrimSpace(line[len("x-vercel-cache:"):])
		case strings.HasPrefix(lower, "cf-cache-status:") && cache == "":
			cache = strings.TrimSpace(line[len("cf-cache-status:"):])
		case strings.HasPrefix(lower, "x-matched-path:"):
			matchedPath = strings.TrimSpace(line[len("x-matched-path:"):])
		case strings.HasPrefix(line, "{") || strings.HasPrefix(line, "["):
			body = line
		}
	}

	parts := []string{"🌐 HTTP", "status " + status}
	if body != "" {
		parts = append(parts, "body "+truncateStr(body, 120))
	}
	if contentType != "" {
		parts = append(parts, "type "+contentType)
	}
	if matchedPath != "" {
		parts = append(parts, "path "+matchedPath)
	}
	if cache != "" {
		parts = append(parts, "cache "+cache)
	}
	if server != "" {
		parts = append(parts, "server "+server)
	}
	return strings.Join(parts, " · "), true
}

func compactBuildLogPreview(output string) (string, bool) {
	lines := normalizedNonEmptyLines(output)
	if len(lines) == 0 {
		return "", false
	}

	buildLines := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(line, "Building:") {
			buildLines = append(buildLines, strings.TrimSpace(strings.TrimPrefix(line, "Building:")))
		}
	}
	if len(buildLines) == 0 {
		return "", false
	}

	var status string
	var runtime string
	var framework string
	var command string
	var warning string
	var current string

	for _, line := range buildLines {
		lower := strings.ToLower(line)
		switch {
		case strings.Contains(lower, "installing dependencies"):
			status = "installing dependencies"
		case strings.Contains(lower, "checked ") && strings.Contains(lower, "packages"):
			status = line
		case strings.Contains(lower, "detected next.js version"):
			framework = strings.TrimSpace(strings.TrimPrefix(line, "Detected "))
		case strings.HasPrefix(lower, "bun install"):
			runtime = line
		case strings.Contains(lower, "running "):
			command = strings.Trim(line[strings.Index(lower, "running ")+len("running "):], " \\\" ")
		case strings.HasPrefix(line, "$"):
			command = strings.TrimSpace(strings.TrimPrefix(line, "$"))
		case strings.HasPrefix(line, "⚠") || strings.Contains(lower, "warning") || strings.Contains(lower, "deprecated"):
			warning = strings.TrimSpace(line)
		case strings.Contains(lower, "creating an optimized production build"):
			current = "creating optimized production build"
		case strings.Contains(lower, "compiled successfully") || strings.Contains(lower, "build completed") || strings.Contains(lower, "success"):
			current = "build completed"
		}
	}

	if current == "" {
		current = status
	}
	if current == "" {
		current = buildLines[len(buildLines)-1]
	}

	parts := []string{"🏗️ Build", "status " + current}
	if framework != "" {
		parts = append(parts, framework)
	}
	if runtime != "" {
		parts = append(parts, runtime)
	}
	if command != "" {
		parts = append(parts, "cmd "+command)
	}
	if warning != "" {
		parts = append(parts, "warning "+truncateStr(warning, 100))
	}
	return strings.Join(parts, " · "), true
}

func normalizedNonEmptyLines(s string) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "▶"))
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func firstNonEmptyLine(s string) string {
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "▶"))
		if line != "" {
			return line
		}
	}
	return ""
}

func singleLine(s string) string {
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

func handleSessionCommand(args []string, supervisor *agent.Supervisor) {
	if len(args) == 0 {
		ui.PrintError("Gunakan: /session [list|info|switch|new|end|delete|search]")
		return
	}

	subCmd := args[0]

	switch subCmd {
	case "list":
		sessions := supervisor.ListSessions()
		if len(sessions) == 0 {
			ui.PrintInfo("Belum ada session aktif. Gunakan /session new")
			return
		}
		var msgParts []string
		for _, s := range sessions {
			marker := "  "
			if supervisor.IsCurrentSession(s.ID) {
				marker = "▸"
			}
			stateIcon := "🟢"
			if s.State == agent.SessionEnded {
				stateIcon = "⚫"
			} else if s.State == agent.SessionPaused {
				stateIcon = "🟡"
			}
			msgParts = append(msgParts, fmt.Sprintf("%s %s %s [%s] — %d tasks", marker, stateIcon, s.Name, s.ID[:8], len(s.Tasks)))
		}
		ui.PrintInfo("Daftar Session:\n%s", strings.Join(msgParts, "\n"))

	case "info":
		var session *session.Session
		var ok bool
		if len(args) < 2 {
			// Show current session info if no ID provided
			if s := supervisor.GetCurrentSession(); s != nil {
				session = s
				ok = true
			} else {
				ui.PrintError("Tidak ada session aktif. Gunakan: /session info <id>")
				return
			}
		} else {
			session, ok = supervisor.GetSession(args[1])
		}
		if !ok {
			ui.PrintError("Session tidak ditemukan: %s", args[1])
			return
		}
		var msgParts []string
		msgParts = append(msgParts, fmt.Sprintf("Session: %s [%s]", session.Name, session.ID[:8]))
		msgParts = append(msgParts, fmt.Sprintf("State: %s", session.State))
		msgParts = append(msgParts, fmt.Sprintf("Mode: %s", session.Mode))
		msgParts = append(msgParts, fmt.Sprintf("History: %d messages", len(session.History)))
		msgParts = append(msgParts, fmt.Sprintf("Tasks: %d", len(session.Tasks)))
		msgParts = append(msgParts, fmt.Sprintf("MCP: %s", strings.Join(session.MCPServers, ", ")))
		ui.PrintInfo("%s", strings.Join(msgParts, "\n"))

	case "switch":
		if len(args) < 2 {
			// List available sessions with ID so user can copy-paste
			sessions := supervisor.ListSessions()
			if len(sessions) == 0 {
				ui.PrintInfo("Belum ada session. Gunakan /session new")
				return
			}
			var msgParts []string
			msgParts = append(msgParts, "Session tersedia (gunakan /session switch <id>):")
			for _, s := range sessions {
				marker := "  "
				if supervisor.IsCurrentSession(s.ID) {
					marker = "▸"
				}
				msgParts = append(msgParts, fmt.Sprintf("%s %s [%s]", marker, s.Name, s.ID[:8]))
			}
			ui.PrintInfo("%s", strings.Join(msgParts, "\n"))
			return
		}
		if err := supervisor.SwitchSession(args[1]); err != nil {
			ui.PrintError("Gagal switch session: %v", err)
			return
		}
		session, _ := supervisor.GetSession(args[1])
		ui.PrintSuccess("Berpindah ke session: %s (%s)", session.Name, args[1][:8])

	case "new":
		name := "Session"
		carryOver := 0
		nameParts := []string{}
		for _, arg := range args[1:] {
			if strings.HasPrefix(arg, "--carry-over=") {
				if c, err := strconv.Atoi(strings.TrimPrefix(arg, "--carry-over=")); err == nil && c > 0 {
					carryOver = c
				}
			} else if arg == "--carry-over" {
				carryOver = 3 // default carry-over
			} else {
				nameParts = append(nameParts, arg)
			}
		}
		if len(nameParts) > 0 {
			name = strings.Join(nameParts, " ")
		}
		session, err := supervisor.CreateSession(agent.SessionConfig{
			Name:           name,
			Mode:           string(supervisor.GetMode()),
			MCPServers:     supervisor.ListMCPServers(),
			CarryOverCount: carryOver,
		})
		if err != nil {
			ui.PrintError("Gagal membuat session: %v", err)
			return
		}
		extra := ""
		if carryOver > 0 {
			extra = fmt.Sprintf(" | Carry-over: %d turns", carryOver)
		}
		ui.PrintSuccess("Session baru dibuat: %s [%s]\nMode: %s | MCP: %d servers%s", session.Name, session.ID[:8], session.Mode, len(session.MCPServers), extra)

	case "end":
		if err := supervisor.EndCurrentSession(); err != nil {
			ui.PrintError("Gagal mengakhiri session: %v", err)
		} else {
			ui.PrintSuccess("Session diakhiri.")
		}

	case "delete":
		if len(args) < 2 {
			ui.PrintError("Gunakan: /session delete <id>")
			return
		}
		id := args[1]
		if supervisor.IsCurrentSession(id) {
			ui.PrintError("Tidak dapat menghapus session yang sedang aktif. Gunakan /session end terlebih dahulu.")
			return
		}
		if err := supervisor.DeleteSession(id); err != nil {
			ui.PrintError("Gagal menghapus session: %v", err)
		} else {
			ui.PrintSuccess("Session %s dihapus.", id[:8])
		}

	case "search":
		if len(args) < 2 {
			ui.PrintError("Gunakan: /session search <query>")
			return
		}
		query := strings.Join(args[1:], " ")
		results, err := supervisor.SearchSessions(query, 5)
		if err != nil {
			ui.PrintError("Gagal mencari session: %v", err)
			return
		}
		if len(results) == 0 {
			ui.PrintInfo("Tidak ada session yang cocok dengan query: %s", query)
			return
		}
		var msgParts []string
		msgParts = append(msgParts, fmt.Sprintf("Hasil pencarian untuk '%s' (top %d):", query, len(results)))
		for i, r := range results {
			marker := "  "
			if supervisor.IsCurrentSession(r.Session.ID) {
				marker = "▸"
			}
			msgParts = append(msgParts, fmt.Sprintf("%s [%d] %s [%s] — relevansi: %.2f", marker, i+1, r.Session.Name, r.Session.ID[:8], r.Score))
			if r.Snippet != "" {
				msgParts = append(msgParts, fmt.Sprintf("     %s", r.Snippet))
			}
		}
		ui.PrintInfo("%s", strings.Join(msgParts, "\n"))

	default:
		ui.PrintError("Sub-command tidak dikenali: %s (list|info|switch|new|end|delete|search)", subCmd)
	}
}

func handleModelCommand(args []string, supervisor *agent.Supervisor) {
	if len(args) == 0 {
		// Show current model and available options
		providers := llm.AvailableProviders()
		currentProvider := supervisor.GetProviderName()

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Model saat ini: %s\n", currentProvider))
		sb.WriteString(fmt.Sprintf("Gunakan: /model <provider> [model]\n\n"))
		sb.WriteString("Provider tersedia:\n")
		for name, info := range providers {
			marker := "  "
			if name == currentProvider {
				marker = "▸ "
			}
			keyIndicator := ""
			if info.NeedsAPIKey {
				keyIndicator = " 🔑"
			}
			sb.WriteString(fmt.Sprintf("  %s%s — %s%s\n", marker, name, info.Description, keyIndicator))
			if len(info.Models) > 0 && name == currentProvider {
				for _, m := range info.Models {
					sb.WriteString(fmt.Sprintf("    ├── %s\n", m))
				}
			}
		}
		ui.PrintInfo("%s", sb.String())
		return
	}

	provider := args[0]
	model := ""
	if len(args) > 1 {
		model = args[1]
	}

	// Validate provider
	providers := llm.AvailableProviders()
	if _, ok := providers[provider]; !ok {
		var names []string
		for name := range providers {
			names = append(names, name)
		}
		ui.PrintError("Provider tidak valid: %s (tersedia: %s)", provider, strings.Join(names, ", "))
		return
	}

	// Check API key requirement
	cfg := config.Get()
	var hasKey bool
	switch provider {
	case "openai":
		hasKey = cfg.OpenAIAPIKey != ""
	case "openrouter":
		hasKey = cfg.OpenRouterAPIKey != ""
	case "anthropic":
		hasKey = cfg.AnthropicAPIKey != ""
	case "custom":
		hasKey = cfg.CustomAPIKey != ""
	case "ollama":
		hasKey = true // no API key needed
	}

	if !hasKey {
		ui.PrintError("API key belum diatur untuk provider %s", provider)
		ui.PrintInfo("Gunakan: smara login --provider %s", provider)
		return
	}

	// Switch model
	if err := supervisor.SetModel(provider, model); err != nil {
		ui.PrintError("Gagal switch model: %v", err)
		return
	}

	if model != "" {
		ui.PrintSuccess("Model switched ke: %s — %s", provider, model)
	} else if info, ok := providers[provider]; ok && len(info.Models) > 0 {
		ui.PrintSuccess("Model switched ke: %s — %s (default)", provider, info.Models[0])
	} else {
		ui.PrintSuccess("Model switched ke: %s", provider)
	}
}

// loadProjectContext reads project files to provide initial context.
func loadProjectContext() string {
	var contextParts []string

	// Read README.md
	if content, err := os.ReadFile("README.md"); err == nil {
		contentStr := string(content)
		if len(contentStr) > 2000 {
			contentStr = contentStr[:2000] + "\n... (dipotong)"
		}
		contextParts = append(contextParts, "Isi README.md:\n```\n"+contentStr+"\n```")
	}

	// Basic folder structure
	if entries, err := os.ReadDir("."); err == nil {
		var dirs, files []string
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, e.Name()+"/")
			} else {
				files = append(files, e.Name())
			}
		}
		contextParts = append(contextParts, "Struktur root direktori proyek:\nFolder: "+strings.Join(dirs, ", ")+"\nFile: "+strings.Join(files, ", "))
	}

	if len(contextParts) > 0 {
		return "Kamu sedang berada dalam sebuah direktori proyek lokal. Berikut adalah informasi konteks dari proyek ini:\n\n" + strings.Join(contextParts, "\n\n")
	}

	return ""
}
