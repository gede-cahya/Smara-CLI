package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/mcp"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/gede-cahya/Smara-CLI/internal/metrics"
	internalSync "github.com/gede-cahya/Smara-CLI/internal/sync"
	"github.com/gede-cahya/Smara-CLI/internal/ui"
	"github.com/gede-cahya/Smara-CLI/internal/web"
)

var (
	webPort           string
	webHost           string
	webNoOpen         bool
	webMode           string
	webToken          string
	webSkipMCP        bool
	desktopAgentAddr  string
	desktopAgentToken string
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Jalankan Smara Web Interface",
	Long: `Menjalankan antarmuka web interaktif untuk Smara.

Server HTTP dengan WebSocket real-time chat, manajemen memori,
workspace, konfigurasi, dan dashboard monitoring.

Contoh:
  smara web              # jalankan di localhost:8080
  smara web --port 3000  # jalankan di port 3000
  smara web --host 0.0.0.0 --port 80  # listen di semua interface`,
	RunE: runWeb,
}

func init() {
	webCmd.Flags().StringVar(&webPort, "port", "8080", "port HTTP server")
	webCmd.Flags().StringVar(&webHost, "host", "127.0.0.1", "host HTTP server (use 0.0.0.0 untuk akses dari network)")
	webCmd.Flags().BoolVar(&webNoOpen, "no-open", false, "jangan buka browser otomatis")
	webCmd.Flags().StringVar(&webMode, "mode", "ask", "mode agen default: ask, rush, plan, test, image, workflow")
	webCmd.Flags().StringVar(&webToken, "auth-token", "", "token akses remote opsional (header Authorization: Bearer atau ?token=)")
	webCmd.Flags().BoolVar(&webSkipMCP, "skip-mcp", false, "lewati koneksi MCP saat startup web")
	webCmd.Flags().StringVar(&desktopAgentAddr, "desktop-agent", "", "URL desktop-agent untuk auto-pair remote desktop, contoh http://127.0.0.1:8765")
	webCmd.Flags().StringVar(&desktopAgentToken, "desktop-token", "", "Token desktop-agent untuk auto-pair")
	rootCmd.AddCommand(webCmd)
}

func runWeb(cmd *cobra.Command, args []string) error {
	startTime := time.Now()
	cfg := config.Get()

	ui.AppVersion = version
	ui.PrintBanner(version)
	ui.PrintInfo("🌐 Memulai Smara Web Interface...")

	// Override model from flag if provided
	if model != "" {
		cfg.Model = model
	}

	// 1. Initialize LLM Provider
	ui.PrintInfo("Menghubungkan ke %s (%s)...", cfg.Provider, cfg.Model)

	providerCfg := llm.ProviderConfig{
		Name:            cfg.Provider,
		Model:           cfg.Model,
		Host:            cfg.OllamaHost,
		APIKey:          "",
		ReasoningEffort: cfg.ReasoningEffort,
	}

	switch cfg.Provider {
	case "openai":
		providerCfg.APIKey = cfg.OpenAIAPIKey
		providerCfg.Host = cfg.OpenAIBaseURL
	case "openrouter":
		providerCfg.APIKey = cfg.OpenRouterAPIKey
		if cfg.Model == "" || cfg.Model == "minimax-m2.5:cloud" {
			providerCfg.Model = cfg.OpenRouterModel
		}
	case "anthropic":
		providerCfg.APIKey = cfg.AnthropicAPIKey
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

	// 3. Initialize Supervisor Agent
	supervisor := agent.NewSupervisorWithConfig(provider, providerCfg, memStore)
	defer supervisor.Close()

	if agent.ValidMode(webMode) {
		mode := agent.Mode(webMode)
		if mode == agent.ModeRush {
			agent.EnableRushAutoApprovalEnv()
		}
		supervisor.SetMode(mode)
	}
	if agent.IsRushAutoApprovalEnabled() {
		agent.EnableRushAutoApprovalEnv()
		supervisor.SetMode(agent.ModeRush)
	}
	activeWorkspaceName := cfg.ActiveWorkspace
	if activeWorkspaceName == "" {
		activeWorkspaceName = "default"
	}
	w, err := memStore.GetWorkspaceByName(activeWorkspaceName)
	if err != nil {
		ui.PrintWarning("Gagal memuat workspace: %v", err)
	} else if w == nil {
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

	// 4. Connect MCP Servers
	var mcpConfigs []mcp.MCPServerConfig

	ocPath := mcp.OpenCodeConfigPath()
	if ocPath != "" {
		ui.PrintInfo("OpenCode config ditemukan: %s", ocPath)
		ocServers, err := mcp.LoadOpenCodeMCPServers()
		if err == nil && len(ocServers) > 0 {
			mcpConfigs = append(mcpConfigs, ocServers...)
			ui.PrintSuccess("Mengimpor %d MCP server dari OpenCode", len(ocServers))
		} else if err != nil {
			ui.PrintWarning("Gagal memuat OpenCode config: %v", err)
		}
	}

	wsPath := mcp.WindsurfConfigPath()
	if wsPath != "" {
		ui.PrintInfo("Windsurf config ditemukan: %s", wsPath)
		wsServers, err := mcp.LoadWindsurfMCPServers()
		if err == nil && len(wsServers) > 0 {
			mcpConfigs = append(mcpConfigs, wsServers...)
			ui.PrintSuccess("Mengimpor %d MCP server dari Windsurf", len(wsServers))
		} else if err != nil {
			ui.PrintWarning("Gagal memuat Windsurf config: %v", err)
		}
	}

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

	// Deduplicate
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

	// 5. Setup metrics
	smaraDir := filepath.Dir(cfg.DBPath)
	metricsPath := filepath.Join(smaraDir, "metrics.json")
	collector := metrics.NewCollector(metricsPath, providerCfg.Name, providerCfg.Model)

	// Apply configurable agent iteration cap (matches `smara serve` behavior
	// so long roadmap-style chains don't get cut off in web mode).
	if cfg.AgentMaxIterations > 0 {
		supervisor.SetMaxIterations(cfg.AgentMaxIterations)
		ui.PrintInfo("Agent max iterations: %d (dari config)", cfg.AgentMaxIterations)
	}
	if cfg.AgentRequestTimeoutSec > 0 {
		ui.PrintInfo("Agent per-turn timeout: %ds (dari config)", cfg.AgentRequestTimeoutSec)
	}

	// 6. Start web server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr := fmt.Sprintf("%s:%s", webHost, webPort)
	server := web.NewServer(addr, supervisor, memStore, collector, cfg)
	server.WebSessions = web.NewWebSessionManager(provider, providerCfg, memStore, activeWorkspaceName, cfg.ActiveWorkspaceID, cfg.AgentMaxIterations, filepath.Join(filepath.Dir(cfg.DBPath), "web-sessions.json"))
	server.WebSessions.SetMCPConnections(supervisor.GetMCPClients(), supervisor.GetMCPInfo())
	server.RemoteDesktop = web.NewRemoteDesktopManager(filepath.Join(filepath.Dir(cfg.DBPath), "remote-desktop-devices.json"))
	if desktopAgentAddr != "" {
		if _, err := server.RemoteDesktop.Upsert("local-desktop", desktopAgentAddr, desktopAgentToken); err != nil {
			ui.PrintWarning("Gagal pair desktop-agent: %v", err)
		} else {
			ui.PrintSuccess("Desktop agent paired: %s", desktopAgentAddr)
		}
	}
	if webToken != "" {
		server.AuthToken = webToken
	}

	// 5.5 Start Background Sync Daemon
	syncCfg := internalSync.SyncConfig{
		SyncDir:          cfg.SyncDir,
		IntervalMin:      cfg.SyncInterval,
		Enabled:          true,
		NineDriveEnabled: cfg.NineDriveEnabled,
		NineDriveBaseURL: cfg.NineDriveBaseURL,
		NineDriveAPIKey:  cfg.NineDriveAPIKey,
	}
	daemon := internalSync.NewDaemon(syncCfg, memStore)
	daemon.Start(ctx)
	defer daemon.Stop()
	ui.PrintSuccess("Sync daemon aktif (interval: %d menit)", cfg.SyncInterval)

	go func() {
		if err := server.Start(ctx); err != nil {
			ui.PrintError("Web server error: %v", err)
			cancel()
		}
	}()

	elapsed := time.Since(startTime)
	ui.PrintInfo("Startup: %s", elapsed.Round(time.Millisecond))
	fmt.Println()
	ui.PrintSuccess("🌐 Smara Web Interface berjalan!")
	fmt.Printf("   URL: http://%s\n", addr)
	fmt.Println()
	ui.PrintInfo("Tekan Ctrl+C untuk berhenti")
	fmt.Println()

	var mcpWG sync.WaitGroup
	if webSkipMCP {
		ui.PrintInfo("MCP startup dilewati (--skip-mcp)")
	} else if len(enabledConfigs) > 0 {
		ui.PrintInfo("MCP sedang dihubungkan paralel di background; web sudah dapat digunakan.")
		mcpWG.Add(1)
		go func() {
			defer mcpWG.Done()
			connectMCPServersForStartup(supervisor, enabledConfigs)
			mcpInfo := supervisor.GetMCPInfo()
			for name, info := range mcpInfo {
				collector.RegisterMCP(name, info.Connected, len(info.Tools))
			}
			server.WebSessions.SetMCPConnections(supervisor.GetMCPClients(), mcpInfo)
			ui.PrintSuccess("Koneksi MCP background selesai: %d server aktif", len(supervisor.GetMCPClients()))
		}()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println()
	ui.PrintInfo("Mematikan server...")
	cancel()
	mcpWG.Wait()
	time.Sleep(500 * time.Millisecond)
	ui.PrintSuccess("Server dihentikan.")
	return nil
}
