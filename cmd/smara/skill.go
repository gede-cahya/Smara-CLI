package main

import (
	"fmt"
	"os"
	stdsync "sync"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/mcp"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Kelola reusable automation skill",
	Long:  `Buat, jalankan, edit, dan hapus skill (resep automation yang tersimpan).`,
}

var skillRunArgs string

var skillRunCmd = &cobra.Command{
	Use:   "run [nama-skill]",
	Short: "Jalankan skill yang tersimpan",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		sk, err := skill.Load(name)
		if err != nil {
			return fmt.Errorf("skill '%s' tidak ditemukan: %w", name, err)
		}
		fmt.Printf("Menjalankan skill: %s\n", sk.Summary())

		// Create lightweight supervisor for tool execution
		supervisor, err := getSupervisorForSkill()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Gagal inisialisasi supervisor: %v\n", err)
			fmt.Println("Gunakan TUI (smara start) untuk eksekusi penuh dengan MCP support.")
			os.Exit(1)
		}
		defer supervisor.Close()

		executor := supervisor.SkillExecutor()
		result, err := sk.Run(executor)
		if err != nil {
			return fmt.Errorf("skill execution error: %w", err)
		}

		fmt.Println()
		if result.Success {
			fmt.Println("Skill berhasil dieksekusi!")
		} else {
			fmt.Println("Skill gagal pada salah satu step.")
		}
		for i, sr := range result.StepResults {
			status := "OK"
			if sr.Error != nil {
				status = fmt.Sprintf("ERROR: %v", sr.Error)
			}
			out := sr.Output
			if len(out) > 200 {
				out = out[:200] + "..."
			}
			fmt.Printf("  Step %d: %s → %s\n    Output: %s\n", i+1, sr.Tool, status, out)
		}
		return nil
	},
}

// getSupervisorForSkill creates a lightweight supervisor for CLI skill execution.
// It initializes the LLM provider (if configured), memory store, and connects MCP servers.
func getSupervisorForSkill() (*agent.Supervisor, error) {
	cfg := config.Get()

	// 1. Initialize LLM Provider (optional — some tools don't need it)
	var provider llm.Provider
	var providerCfg llm.ProviderConfig
	if cfg.Provider != "" {
		providerCfg = llm.ProviderConfig{
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
		var err error
		provider, err = llm.NewProvider(providerCfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: LLM provider gagal diinisialisasi: %v\n", err)
			provider = nil
		}
	}

	// 2. Initialize Memory Store
	var memStore memory.MemoryStore
	if cfg.DBPath != "" {
		var err error
		memStore, err = memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Memory store gagal diinisialisasi: %v\n", err)
		}
	}
	if memStore != nil {
		defer func() {
			// Note: don't close here — supervisor will use it during execution
			// memStore is closed by supervisor.Close() via its own lifecycle
		}()
	}

	// 3. Create Supervisor
	supervisor := agent.NewSupervisorWithConfig(provider, providerCfg, memStore)

	// 4. Connect MCP Servers from config
	var mcpConfigs []mcp.MCPServerConfig

	// Try OpenCode config
	ocPath := mcp.OpenCodeConfigPath()
	if ocPath != "" {
		ocServers, err := mcp.LoadOpenCodeMCPServers()
		if err == nil && len(ocServers) > 0 {
			mcpConfigs = append(mcpConfigs, ocServers...)
		}
	}

	// Try Windsurf config
	wsPath := mcp.WindsurfConfigPath()
	if wsPath != "" {
		wsServers, err := mcp.LoadWindsurfMCPServers()
		if err == nil && len(wsServers) > 0 {
			mcpConfigs = append(mcpConfigs, wsServers...)
		}
	}

	// Smara-native configs
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

	// Connect enabled servers in parallel
	var enabledConfigs []mcp.MCPServerConfig
	for _, cfg := range mcpConfigs {
		if cfg.Enabled {
			enabledConfigs = append(enabledConfigs, cfg)
		}
	}

	if len(enabledConfigs) > 0 {
		type mcpConnResult struct {
			Name   string
			Client *mcp.Client
			Tools  []mcp.Tool
			Err    error
		}
		results := make(chan mcpConnResult, len(enabledConfigs))
		var wg stdsync.WaitGroup

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
				fmt.Fprintf(os.Stderr, "Warning: MCP '%s' gagal terhubung: %v\n", res.Name, res.Err)
				continue
			}
			supervisor.RegisterMCPClient(res.Name, res.Client)
			if len(res.Tools) > 0 {
				supervisor.UpdateMCPInfo(res.Name, res.Tools)
				fmt.Printf("  MCP '%s' terhubung (%d tools)\n", res.Name, len(res.Tools))
			} else {
				supervisor.UpdateMCPInfo(res.Name, []mcp.Tool{})
				fmt.Printf("  MCP '%s' terhubung\n", res.Name)
			}
		}
	}

	return supervisor, nil
}

var skillListCmd = &cobra.Command{
	Use:   "list",
	Short: "Daftar skill yang tersimpan",
	Run: func(cmd *cobra.Command, args []string) {
		names, err := skill.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Gagal list skill: %v\n", err)
			os.Exit(1)
		}
		if len(names) == 0 {
			fmt.Println("Belum ada skill tersimpan.")
			return
		}
		fmt.Println("Skill tersimpan:")
		for _, n := range names {
			sk, _ := skill.Load(n)
			if sk != nil {
				fmt.Printf("  - %s: %s\n", n, sk.Description)
			} else {
				fmt.Printf("  - %s\n", n)
			}
		}
	},
}

var skillDeleteCmd = &cobra.Command{
	Use:   "delete [nama-skill]",
	Short: "Hapus skill",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		if err := skill.Delete(name, nil); err != nil {
			fmt.Fprintf(os.Stderr, "Gagal hapus skill: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Skill '%s' dihapus.\n", name)
	},
}

var skillCreateCmd = &cobra.Command{
	Use:   "create [nama-skill]",
	Short: "Buat skill baru dari file JSON",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		// Read JSON from stdin
		fmt.Println("Tempel JSON skill (Ctrl+D untuk selesai):")
		var buf strings.Builder
		var b [1024]byte
		for {
			n, err := os.Stdin.Read(b[:])
			if n > 0 {
				buf.Write(b[:n])
			}
			if err != nil {
				break
			}
		}
		sk, err := skill.FromJSON([]byte(buf.String()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "JSON tidak valid: %v\n", err)
			os.Exit(1)
		}
		sk.Name = name
		if err := skill.Save(sk, nil); err != nil {
			fmt.Fprintf(os.Stderr, "Gagal simpan skill: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Skill '%s' tersimpan.\n", name)
	},
}

func init() {
	skillCmd.AddCommand(skillRunCmd, skillListCmd, skillDeleteCmd, skillCreateCmd)
	rootCmd.AddCommand(skillCmd)
}
