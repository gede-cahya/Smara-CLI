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
	"github.com/gede-cahya/Smara-CLI/internal/ui"
	"github.com/gede-cahya/Smara-CLI/internal/web"
)

var (
	webPort           string
	webHost           string
	webNoOpen         bool
	webMode           string
	webToken          string
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
	webCmd.Flags().StringVar(&webMode, "mode", "ask", "mode agen default: ask, rush, plan")
	webCmd.Flags().StringVar(&webToken, "auth-token", "", "token akses remote opsional (header Authorization: Bearer atau ?token=)")
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
		Name:   cfg.Provider,
		Model:  cfg.Model,
		Host:   cfg.OllamaHost,
		APIKey: "",
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
		supervisor.SetMode(agent.Mode(webMode))
	}
	modeInfo := agent.GetModeInfo(supervisor.GetMode())
	ui.PrintSuccess("Mode: %s %s — %s", modeInfo.Emoji, modeInfo.Label, modeInfo.Description)

	// 3.1 Workspace Initialization
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

	type mcpConnResult struct {
		Name   string
		Client *mcp.Client
		Tools  []mcp.Tool
		Err    error
	}

	var enabledConfigs []mcp.MCPServerConfig
	for _, cfg := range mcpConfigs {
		if cfg.Enabled {
			enabledConfigs = append(enabledConfigs, cfg)
		}
	}

	if len(enabledConfigs) > 0 {
		ui.PrintInfo("Menghubungkan %d MCP server secara paralel...", len(enabledConfigs))
		results := make(chan mcpConnResult, len(enabledConfigs))
		var wg sync.WaitGroup
		for _, mcpCfg := range enabledConfigs {
			wg.Add(1)
			go func(cfg mcp.MCPServerConfig) {
				defer wg.Done()
				var client *mcp.Client
				var err error
				switch cfg.Type {
				case "remote":
					client, err = mcp.NewRemoteClient(cfg)
				default:
					client, err = mcp.NewClient(cfg)
				}
				if err != nil {
					results <- mcpConnResult{Name: cfg.Name, Err: err}
					return
				}
				tools, _ := client.ListTools()
				results <- mcpConnResult{Name: cfg.Name, Client: client, Tools: tools}
			}(mcpCfg)
		}
		go func() {
			wg.Wait()
			close(results)
		}()
		for res := range results {
			if res.Err != nil {
				ui.PrintWarning("Gagal menghubungkan MCP '%s': %v", res.Name, res.Err)
				continue
			}
			supervisor.RegisterMCPClient(res.Name, res.Client)
			if len(res.Tools) > 0 {
				supervisor.UpdateMCPInfo(res.Name, res.Tools)
				ui.PrintSuccess("MCP '%s' terhubung (%d tools)", res.Name, len(res.Tools))
			} else {
				ui.PrintSuccess("MCP '%s' terhubung", res.Name)
			}
		}
	}

	// 5. Setup metrics
	smaraDir := filepath.Dir(cfg.DBPath)
	metricsPath := filepath.Join(smaraDir, "metrics.json")
	collector := metrics.NewCollector(metricsPath, providerCfg.Name, providerCfg.Model)
	mcpInfo := supervisor.GetMCPInfo()
	for name, info := range mcpInfo {
		collector.RegisterMCP(name, info.Connected, len(info.Tools))
	}

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

	elapsed := time.Since(startTime)
	ui.PrintInfo("Startup: %s", elapsed.Round(time.Millisecond))
	fmt.Println()
	ui.PrintSuccess("🌐 Smara Web Interface berjalan!")
	fmt.Printf("   URL: http://%s\n", addr)
	fmt.Println()
	ui.PrintInfo("Tekan Ctrl+C untuk berhenti")
	fmt.Println()

	go func() {
		if err := server.Start(ctx); err != nil {
			ui.PrintError("Web server error: %v", err)
			cancel()
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println()
	ui.PrintInfo("Mematikan server...")
	cancel()
	time.Sleep(500 * time.Millisecond)
	ui.PrintSuccess("Server dihentikan.")
	return nil
}
