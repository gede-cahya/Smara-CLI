package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
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
	"github.com/gede-cahya/Smara-CLI/internal/platform"
	"github.com/gede-cahya/Smara-CLI/internal/platform/discord"
	"github.com/gede-cahya/Smara-CLI/internal/platform/telegram"
	"github.com/gede-cahya/Smara-CLI/internal/platform/whatsapp"
	"github.com/gede-cahya/Smara-CLI/internal/ui"
)

var (
	servePlatforms string
	serveMode      string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Jalankan Smara sebagai bot di platform messaging",
	Long: `Menjalankan Smara sebagai bot yang bisa diakses dari platform
messaging seperti Telegram dan Discord.

Contoh:
  smara serve                           # jalankan semua platform yang enabled
  smara serve --platform telegram       # hanya Telegram
  smara serve --platform telegram,discord,whatsapp  # Telegram, Discord, dan WhatsApp

Token bot diatur via config atau environment variable:
  SMARA_TELEGRAM_TOKEN=bot123:AAH...
  SMARA_DISCORD_TOKEN=MTIz...`,
	RunE: runServe,
}

func init() {
	serveCmd.Flags().StringVar(&servePlatforms, "platform", "", "platform yang dijalankan (comma-separated: telegram,discord,whatsapp)")
	serveCmd.Flags().StringVar(&serveMode, "mode", "ask", "mode agen default: ask, rush, plan")
}

func runServe(cmd *cobra.Command, args []string) error {
	startTime := time.Now()
	cfg := config.Get()

	ui.PrintBanner(version)
	ui.PrintInfo("🌐 Memulai Smara Platform Bot Server...")

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

	if agent.ValidMode(serveMode) {
		supervisor.SetMode(agent.Mode(serveMode))
	}
	modeInfo := agent.GetModeInfo(supervisor.GetMode())
	ui.PrintSuccess("Mode: %s %s — %s", modeInfo.Emoji, modeInfo.Label, modeInfo.Description)

	// 4. Connect MCP Servers
	var mcpConfigs []mcp.MCPServerConfig

	// Try to load from OpenCode config
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

	// Try to load from Windsurf config
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

	// Also add Smara-native configs
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

	// Connect to all MCP servers in parallel
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

				// List available tools
				tools, _ := client.ListTools()
				results <- mcpConnResult{Name: cfg.Name, Client: client, Tools: tools}
			}(mcpCfg)
		}

		// Close channel when all goroutines finish
		go func() {
			wg.Wait()
			close(results)
		}()

		// Collect results and register to supervisor
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

	// 5. Create Gateway
	gateway := platform.NewGateway(supervisor)

	// Set up auth manager
	auth := platform.NewAuthManager()

	// 6. Determine which platforms to enable
	enabledPlatforms := determinePlatforms(cfg, servePlatforms)

	if len(enabledPlatforms) == 0 {
		return fmt.Errorf("tidak ada platform yang diaktifkan. Gunakan --platform atau atur di config")
	}

	// 7. Connect platform adapters
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, p := range enabledPlatforms {
		switch p {
		case "telegram":
			token := cfg.Platforms.Telegram.Token
			if envToken := os.Getenv("SMARA_TELEGRAM_TOKEN"); envToken != "" {
				token = envToken
			}
			if token == "" {
				ui.PrintWarning("Telegram: token tidak ditemukan, skipping")
				continue
			}

			tg := telegram.New()
			adapterCfg := platform.AdapterConfig{
				Token:        token,
				AllowedUsers: cfg.Platforms.Telegram.AllowedUsers,
				RateLimit: platform.RateLimitConfig{
					RequestsPerMinute: cfg.Platforms.Telegram.RateLimit,
					BurstSize:         cfg.Platforms.Telegram.RateBurst,
				},
			}
			if err := tg.Connect(ctx, adapterCfg); err != nil {
				ui.PrintError("Telegram: gagal terhubung: %v", err)
				continue
			}
			gateway.RegisterAdapter(tg)
			auth.SetAllowedUsers("telegram", cfg.Platforms.Telegram.AllowedUsers)
			auth.SetBlockedUsers("telegram", cfg.Platforms.Telegram.BlockedUsers)
			ui.PrintSuccess("✅ Telegram adapter aktif")

		case "discord":
			token := cfg.Platforms.Discord.Token
			if envToken := os.Getenv("SMARA_DISCORD_TOKEN"); envToken != "" {
				token = envToken
			}
			if token == "" {
				ui.PrintWarning("Discord: token tidak ditemukan, skipping")
				continue
			}

			dc := discord.New()
			adapterCfg := platform.AdapterConfig{
				Token:        token,
				GuildIDs:     cfg.Platforms.Discord.GuildIDs,
				AllowedRoles: cfg.Platforms.Discord.AllowedRoles,
				AllowedUsers: cfg.Platforms.Discord.AllowedUsers,
				RateLimit: platform.RateLimitConfig{
					RequestsPerMinute: cfg.Platforms.Discord.RateLimit,
					BurstSize:         cfg.Platforms.Discord.RateBurst,
				},
			}
			if err := dc.Connect(ctx, adapterCfg); err != nil {
				ui.PrintError("Discord: gagal terhubung: %v", err)
				continue
			}
			gateway.RegisterAdapter(dc)
			auth.SetAllowedUsers("discord", cfg.Platforms.Discord.AllowedUsers)
			auth.SetBlockedUsers("discord", cfg.Platforms.Discord.BlockedUsers)
			ui.PrintSuccess("✅ Discord adapter aktif")

		case "whatsapp":
			wa := whatsapp.New()
			adapterCfg := platform.AdapterConfig{
				AllowedUsers: cfg.Platforms.WhatsApp.AllowedNumbers,
				RateLimit: platform.RateLimitConfig{
					RequestsPerMinute: cfg.Platforms.WhatsApp.RateLimit,
					BurstSize:         cfg.Platforms.WhatsApp.RateBurst,
				},
				Extra: map[string]string{
					"session_dir": cfg.Platforms.WhatsApp.SessionDir,
				},
			}
			if err := wa.Connect(ctx, adapterCfg); err != nil {
				ui.PrintError("WhatsApp: gagal terhubung: %v", err)
				continue
			}
			gateway.RegisterAdapter(wa)
			auth.SetAllowedUsers("whatsapp", cfg.Platforms.WhatsApp.AllowedNumbers)
			ui.PrintSuccess("✅ WhatsApp adapter aktif")
		}
	}

	gateway.SetAuth(auth)

	// Set up rate limiter from config
	rl := platform.NewRateLimiter(platform.RateLimitConfig{
		RequestsPerMinute: 20, // default
		BurstSize:         5,
	})
	gateway.SetRateLimiter(rl)

	// Set up metrics collector
	smaraDir := filepath.Dir(cfg.DBPath)
	metricsPath := filepath.Join(smaraDir, "metrics.json")
	collector := metrics.NewCollector(metricsPath, providerCfg.Name, providerCfg.Model)

	// Register platforms in metrics
	for _, p := range enabledPlatforms {
		collector.RegisterPlatform(p)
	}

	// Register MCP servers in metrics
	mcpInfo := supervisor.GetMCPInfo()
	for name, info := range mcpInfo {
		collector.RegisterMCP(name, info.Connected, len(info.Tools))
	}

	gateway.SetMetrics(collector)

	// Apply configurable per-prompt timeout (workflow tasks can be long).
	if cfg.PlatformPromptTimeout > 0 {
		gateway.SetPromptTimeout(time.Duration(cfg.PlatformPromptTimeout) * time.Second)
		ui.PrintInfo("Prompt timeout: %ds (dari config)", cfg.PlatformPromptTimeout)
	}
	if cfg.AgentMaxIterations > 0 {
		supervisor.SetMaxIterations(cfg.AgentMaxIterations)
		ui.PrintInfo("Agent max iterations: %d (dari config)", cfg.AgentMaxIterations)
	}
	collector.Start(ctx, 2*time.Second)
	ui.PrintSuccess("Metrics collector aktif → %s", metricsPath)

	elapsed := time.Since(startTime)
	ui.PrintInfo("Startup: %s", elapsed.Round(time.Millisecond))

	fmt.Println()
	ui.PrintSuccess("🌀 Smara Bot Server berjalan!")
	ui.PrintInfo("Platform aktif: %s", strings.Join(enabledPlatforms, ", "))
	ui.PrintInfo("Tekan Ctrl+C untuk berhenti")
	fmt.Println()

	// 8. Handle OS signals for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println()
		log.Println("[serve] Menerima sinyal shutdown...")
		cancel()
		gateway.Stop()
	}()

	// 9. Start gateway (blocks until context cancelled)
	if err := gateway.Start(ctx); err != nil {
		if ctx.Err() != nil {
			// Normal shutdown
			ui.PrintInfo("Server dihentikan.")
			return nil
		}
		return fmt.Errorf("gateway error: %w", err)
	}

	return nil
}

// determinePlatforms figures out which platforms to enable based on CLI flags and config.
func determinePlatforms(cfg *config.SmaraConfig, flagPlatforms string) []string {
	// If --platform flag is set, use that
	if flagPlatforms != "" {
		var platforms []string
		for _, p := range strings.Split(flagPlatforms, ",") {
			p = strings.TrimSpace(strings.ToLower(p))
			if p != "" {
				platforms = append(platforms, p)
			}
		}
		return platforms
	}

	// Otherwise, check config for enabled platforms
	var platforms []string
	if cfg.Platforms.Telegram.Enabled {
		platforms = append(platforms, "telegram")
	}
	if cfg.Platforms.Discord.Enabled {
		platforms = append(platforms, "discord")
	}
	if cfg.Platforms.WhatsApp.Enabled {
		platforms = append(platforms, "whatsapp")
	}

	return platforms
}
