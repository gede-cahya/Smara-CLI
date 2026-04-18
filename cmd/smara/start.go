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

	"github.com/cahya/smara/internal/agent"
	"github.com/cahya/smara/internal/config"
	"github.com/cahya/smara/internal/llm"
	"github.com/cahya/smara/internal/mcp"
	"github.com/cahya/smara/internal/memory"
	"github.com/cahya/smara/internal/sync"
	"github.com/cahya/smara/internal/ui"
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

	// Create a cancelable context for the main loop
	mainCtx, mainCancel := context.WithCancel(context.Background())

	go func() {
		<-sigCh
		fmt.Println()
		ui.PrintInfo("Menutup Smara...")
		mainCancel()
		cancel()
		os.Exit(0)
	}()

	// 7. Start REPL
	ui.PrintHelp()
	prompt := ui.NewPrompt()
	prompt.SetMode(string(supervisor.GetMode()))

	// Wire up Tab key → mode cycling
	prompt.OnModeChange(func(newMode string) {
		supervisor.SetMode(agent.Mode(newMode))
	})

	for {
		// Check if we should continue
		select {
		case <-mainCtx.Done():
			ui.PrintInfo("Sampai jumpa! 👋")
			return nil
		default:
		}

		// Create new context for this iteration
		iterCtx, iterCancel := context.WithCancel(mainCtx)

		input, err := prompt.ReadLineWithCancel(iterCtx)
		if err != nil {
			// Check if cancelled
			if mainCtx.Err() != nil {
				ui.PrintInfo("Sampai jumpa! 👋")
				return nil
			}
			// If it's an interrupt (Ctrl+C during input), continue to next prompt
			if err == ui.ErrInterrupted {
				iterCancel()
				continue
			}
			// EOF or other error
			iterCancel()
			break
		}

		iterCancel() // Cancel function no longer needed after getting input

		if input == "" {
			continue
		}

		if ui.IsExitCommand(input) {
			ui.PrintInfo("Sampai jumpa! 👋")
			break
		}

		// Handle slash commands
		if ui.IsCommand(input) {
			cmdName, cmdArgs := ui.ParseCommand(input)
			handleCommand(cmdName, cmdArgs, supervisor, memStore, prompt)
			continue
		}

		// Process as prompt to supervisor
		currentMode := string(supervisor.GetMode())
		ui.PrintInfo("Memproses [%s]...", currentMode)

		// Create a display-only cancel indicator
		respCh := make(chan struct {
			response string
			err      error
		})

		go func() {
			response, err := supervisor.ProcessPrompt(mainCtx, input)
			respCh <- struct {
				response string
				err      error
			}{response, err}
		}()

		// Wait for response or cancellation
		select {
		case <-mainCtx.Done():
			ui.PrintWarning("Proses dibatalkan")
			continue
		case result := <-respCh:
			if result.err != nil {
				// Check if it was a cancel
				if mainCtx.Err() != nil {
					ui.PrintWarning("Proses dibatalkan")
				} else {
					ui.PrintError("Error: %v", result.err)
				}
				continue
			}
			ui.PrintAgent(result.response, currentMode)
			stats := supervisor.GetStats()
			ui.PrintUsageStats(stats.PromptCount, stats.TotalTokens, stats.AvgTokensPerReq, stats.TotalCost, stats.TotalDuration.String())
		}
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
			fmt.Println()
			for _, m := range agent.AllModes() {
				marker := "  "
				if m.Name == current {
					marker = fmt.Sprintf("%s▸%s", ui.Green, ui.Reset)
				}
				color := ui.ModeColors[string(m.Name)]
				fmt.Printf("  %s %s%s %s%s — %s\n", marker, color, m.Emoji, m.Label, ui.Reset, m.Description)
			}
			fmt.Println()
			return
		}
		newMode := args[0]
		if !agent.ValidMode(newMode) {
			ui.PrintError("Mode tidak valid: %s (pilih: ask, rush, plan)", newMode)
			return
		}
		supervisor.SetMode(agent.Mode(newMode))
		prompt.SetMode(newMode)
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
		fmt.Println()
		for _, m := range memories {
			fmt.Printf("  %s[%d]%s %s%s%s — %s\n",
				ui.Dim, m.ID, ui.Reset,
				ui.Cyan, truncateStr(m.Content, 80), ui.Reset,
				m.CreatedAt.Format("02 Jan 15:04"),
			)
		}
		fmt.Println()
	case "mcp":
		mcpInfo := supervisor.GetMCPInfo()
		if len(mcpInfo) == 0 {
			ui.PrintInfo("Belum ada MCP server yang terhubung.")
			ui.PrintInfo("Tambahkan di ~/.smara/config.yaml pada bagian 'mcp_servers'")
			return
		}
		fmt.Println()
		for name, info := range mcpInfo {
			status := fmt.Sprintf("%s●%s connected", ui.Green, ui.Reset)
			if !info.Connected {
				status = fmt.Sprintf("%s✗%s error", ui.Red, ui.Reset)
			}
			fmt.Printf("  %s%s — %s\n", ui.Cyan, name, status)
			if len(info.Tools) > 0 {
				for _, tool := range info.Tools {
					desc := tool.Description
					if len(desc) > 60 {
						desc = desc[:60] + "..."
					}
					fmt.Printf("    %s├──%s %s %s%s%s\n", ui.Dim, ui.Reset, ui.Yellow, tool.Name, ui.Dim, desc)
				}
			} else if info.Error != "" {
				fmt.Printf("    %s└──%s %s%s%s\n", ui.Dim, ui.Reset, ui.Red, info.Error, ui.Reset)
			}
		}
		fmt.Println()
	case "clear":
		fmt.Print("\033[2J\033[H")
	case "session":
		handleSessionCommand(args, supervisor, prompt)
	default:
		ui.PrintWarning("Perintah tidak dikenali: /%s", cmd)
		ui.PrintHelp()
	}
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func handleSessionCommand(args []string, supervisor *agent.Supervisor, prompt *ui.Prompt) {
	if len(args) == 0 {
		ui.PrintError("Gunakan: /session [list|info|switch|new|end]")
		return
	}

	subCmd := args[0]

	switch subCmd {
	case "list":
		sessions := supervisor.ListSessions()
		if len(sessions) == 0 {
			ui.PrintInfo("Belum ada session aktif.")
			ui.PrintInfo("Gunakan /session new untuk membuat session baru.")
			return
		}
		fmt.Println()
		for _, s := range sessions {
			marker := "  "
			if supervisor.IsCurrentSession(s.ID) {
				marker = fmt.Sprintf("%s▸%s", ui.Green, ui.Reset)
			}
			stateIcon := "🟢"
			if s.State == agent.SessionEnded {
				stateIcon = "⚫"
			} else if s.State == agent.SessionPaused {
				stateIcon = "🟡"
			}
			fmt.Printf("  %s %s%s%s %s — %s, %d tasks\n",
				marker, ui.Cyan, s.ID[:8], ui.Reset,
				stateIcon, s.Name, len(s.Tasks))
		}
		fmt.Println()

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
		fmt.Printf("\n  Session: %s %s[%s]%s\n", session.Name, ui.Cyan, session.ID[:8], ui.Reset)
		fmt.Printf("  State: %s\n", session.State)
		fmt.Printf("  Mode: %s\n", session.Mode)
		fmt.Printf("  Created: %s\n", session.CreatedAt.Format("02 Jan 2006 15:04"))
		fmt.Printf("  Updated: %s\n", session.UpdatedAt.Format("02 Jan 2006 15:04"))
		fmt.Printf("  History: %d messages\n", len(session.History))
		fmt.Printf("  Tasks: %d\n", len(session.Tasks))
		fmt.Printf("  MCP Servers: %s\n", strings.Join(session.MCPServers, ", "))
		if len(session.History) > 0 {
			fmt.Println()
			fmt.Printf("  %sRiwayat percakapan:%s\n", ui.Dim, ui.Reset)
			for i, msg := range session.History {
				if i >= 6 { // Show last 3 exchanges
					fmt.Printf("  %s... (%d more)%s\n", ui.Dim, len(session.History)-6, ui.Reset)
					break
				}
				role := "User"
				if msg.Role == llm.RoleAssistant {
					role = "Agent"
				}
				content := msg.Content
				if len(content) > 60 {
					content = content[:60] + "..."
				}
				fmt.Printf("  %s[%s]%s %s\n", ui.Dim, role, ui.Reset, content)
			}
		}
		fmt.Println()

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
		ui.PrintSuccess("Session baru dibuat: %s %s[%s]%s", session.Name, ui.Cyan, session.ID[:8], ui.Reset)
		fmt.Printf("  Mode: %s | MCP: %d servers\n", session.Mode, len(session.MCPServers))

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
