package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/mcp"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/gede-cahya/Smara-CLI/internal/sync"
	"github.com/gede-cahya/Smara-CLI/internal/ui"
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
	startCmd.Flags().StringVar(&startMode, "mode", "ask", "mode agen: ask, rush, plan")
}

func runStart(cmd *cobra.Command, args []string) error {
	startTime := time.Now()
	cfg := config.Get()

	// Show banner
	ui.PrintBanner()

	// Override model from flag if provided
	if model != "" {
		cfg.Model = model
	}

	// 1. Initialize LLM Provider
	ui.PrintInfo("Menghubungkan ke %s (%s)...", cfg.Provider, cfg.Model)

	// Build provider config with appropriate API key
	providerCfg := llm.ProviderConfig{
		Name:   cfg.Provider,
		Model:  cfg.Model,
		Host:   cfg.OllamaHost,
		APIKey: "",
	}

	// Set API key based on provider
	switch cfg.Provider {
	case "openai":
		providerCfg.APIKey = cfg.OpenAIAPIKey
		providerCfg.Host = "" // uses default OpenAI host
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
			SyncDir:     cfg.SyncDir,
			IntervalMin: cfg.SyncInterval,
			Enabled:     true,
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

	// Set initial mode
	if agent.ValidMode(startMode) {
		supervisor.SetMode(agent.Mode(startMode))
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

	// Also add any Smara-native configs
	for _, mcpCfg := range cfg.MCPServers {
		mcpConfigs = append(mcpConfigs, mcp.MCPServerConfig{
			Name:    mcpCfg.Name,
			Type:    "local",
			Command: mcpCfg.Command,
			Args:    mcpCfg.Args,
			Env:     mcpCfg.Env,
			Enabled: true,
		})
	}

	// Connect to each MCP server
	for _, mcpCfg := range mcpConfigs {
		if !mcpCfg.Enabled {
			continue
		}
		ui.PrintInfo("Menghubungkan MCP: %s (%s)...", mcpCfg.Name, mcpCfg.Type)

		var client *mcp.Client
		var err error

		switch mcpCfg.Type {
		case "remote":
			client, err = mcp.NewRemoteClient(mcpCfg)
		default: // "local"
			client, err = mcp.NewClient(mcpCfg)
		}

		if err != nil {
			ui.PrintWarning("Gagal menghubungkan MCP '%s': %v", mcpCfg.Name, err)
			continue
		}
		supervisor.RegisterMCPClient(mcpCfg.Name, client)

		// List available tools
		tools, err := client.ListTools()
		if err == nil {
			supervisor.UpdateMCPInfo(mcpCfg.Name, tools)
			ui.PrintSuccess("MCP '%s' terhubung (%d tools)", mcpCfg.Name, len(tools))
		} else {
			ui.PrintSuccess("MCP '%s' terhubung", mcpCfg.Name)
		}
	}

	// Show startup time
	elapsed := time.Since(startTime)
	ui.PrintInfo("Startup: %s", elapsed.Round(time.Millisecond))
	fmt.Println()

	// 6. Handle OS signals for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println()
		ui.PrintInfo("Menutup Smara...")
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
			ui.PrintError("Mode tidak valid: %s (pilih: ask, rush, plan)", newMode)
			return
		}
		supervisor.SetMode(agent.Mode(newMode))
		info := agent.GetModeInfo(agent.Mode(newMode))
		ui.PrintModeChange(newMode, info.Emoji, info.Description)
	case "model":
		handleModelCommand(args, supervisor)
	case "memory":
		memories, err := memStore.List(10)
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
	default:
		ui.PrintWarning("Perintah tidak dikenali: /%s", cmd)
	}
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func handleSessionCommand(args []string, supervisor *agent.Supervisor) {
	if len(args) == 0 {
		ui.PrintError("Gunakan: /session [list|info|switch|new|end]")
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
		if len(args) < 2 {
			ui.PrintError("Gunakan: /session info <id>")
			return
		}
		session, ok := supervisor.GetSession(args[1])
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
		ui.PrintInfo(strings.Join(msgParts, "\n"))

	case "switch":
		if len(args) < 2 {
			ui.PrintError("Gunakan: /session switch <id>")
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
		if len(args) > 1 {
			name = strings.Join(args[1:], " ")
		}
		session, err := supervisor.CreateSession(agent.SessionConfig{
			Name:       name,
			Mode:       string(supervisor.GetMode()),
			MCPServers: supervisor.ListMCPServers(),
		})
		if err != nil {
			ui.PrintError("Gagal membuat session: %v", err)
			return
		}
		ui.PrintSuccess("Session baru dibuat: %s [%s]\nMode: %s | MCP: %d servers", session.Name, session.ID[:8], session.Mode, len(session.MCPServers))

	case "end":
		if err := supervisor.EndCurrentSession(); err != nil {
			ui.PrintError("Gagal mengakhiri session: %v", err)
		} else {
			ui.PrintSuccess("Session diakhiri.")
		}

	default:
		ui.PrintError("Sub-command tidak dikenali: %s (list|info|switch|new|end)", subCmd)
	}
}

func handleModelCommand(args []string, supervisor *agent.Supervisor) {
	if len(args) == 0 {
		// Show current model and available options
		providers := llm.AvailableProviders()
		currentProvider := supervisor.GetProviderName()

		fmt.Println()
		fmt.Printf("  %sModel saat ini:%s %s\n", ui.Dim, ui.Reset, currentProvider)
		fmt.Println()
		fmt.Printf("  %sGunakan: /model <provider> [model]%s\n", ui.Dim, ui.Reset)
		fmt.Println()
		fmt.Printf("  %sProvider tersedia:%s\n", ui.Dim, ui.Reset)
		for name, info := range providers {
			marker := "  "
			if name == currentProvider {
				marker = fmt.Sprintf("%s▸%s", ui.Green, ui.Reset)
			}
			keyIndicator := ""
			if info.NeedsAPIKey {
				keyIndicator = fmt.Sprintf(" %s🔑%s", ui.Yellow, ui.Reset)
			}
			fmt.Printf("  %s %s%s — %s%s\n", marker, ui.Cyan, name, info.Description, keyIndicator)
			if len(info.Models) > 0 && name == currentProvider {
				for _, m := range info.Models {
					fmt.Printf("    %s├──%s %s\n", ui.Dim, ui.Reset, m)
				}
			}
		}
		fmt.Println()
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
		ui.PrintError("Provider tidak valid: %s", provider)
		fmt.Printf("  Provider tersedia: ")
		for name := range providers {
			fmt.Printf("%s ", name)
		}
		fmt.Println()
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

	ui.PrintSuccess("Model switched ke: %s", provider)
	if model != "" {
		fmt.Printf("  Model: %s\n", model)
	} else if info, ok := providers[provider]; ok && len(info.Models) > 0 {
		fmt.Printf("  Model default: %s\n", info.Models[0])
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
