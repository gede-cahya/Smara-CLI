package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	stdsync "sync"

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
var skillInstallAlias string
var skillInstallOverwrite bool
var skillCreateFormat string
var skillPluginAlias string
var skillPluginOverwrite bool

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
		if strings.TrimSpace(skillRunArgs) != "" {
			var runtimeArgs map[string]interface{}
			if err := json.Unmarshal([]byte(skillRunArgs), &runtimeArgs); err != nil {
				return fmt.Errorf("--args harus JSON object valid: %w", err)
			}
			sk = sk.WithArgs(runtimeArgs)
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
	Short: "Buat skill baru dari file JSON atau Markdown",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		format := strings.ToLower(skillCreateFormat)
		if format != "json" && format != "md" && format != "markdown" {
			fmt.Fprintf(os.Stderr, "Format tidak valid: %s (pilih: json, md)\n", skillCreateFormat)
			os.Exit(1)
		}

		// Read from stdin
		fmt.Printf("Tempel %s skill (Ctrl+D untuk selesai):\n", strings.ToUpper(format))
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
		data := []byte(buf.String())

		var sk *skill.Skill
		var err error
		if format == "md" || format == "markdown" {
			sk, err = skill.ParseMarkdownSkill(data)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Markdown tidak valid: %v\n", err)
				os.Exit(1)
			}
		} else {
			sk, err = skill.FromJSON(data)
			if err != nil {
				fmt.Fprintf(os.Stderr, "JSON tidak valid: %v\n", err)
				os.Exit(1)
			}
		}
		sk.Name = name
		if format == "md" || format == "markdown" {
			if err := skill.SaveAsMarkdown(sk, nil); err != nil {
				fmt.Fprintf(os.Stderr, "Gagal simpan skill sebagai markdown: %v\n", err)
				os.Exit(1)
			}
		} else {
			if err := skill.Save(sk, nil); err != nil {
				fmt.Fprintf(os.Stderr, "Gagal simpan skill: %v\n", err)
				os.Exit(1)
			}
		}
		fmt.Printf("Skill '%s' tersimpan (%s).\n", name, format)
	},
}

var skillInstallCmd = &cobra.Command{
	Use:   "install <url-or-name>",
	Short: "Install skill dari URL, registry lokal, atau marketplace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		input := args[0]

		// If input does not look like a URL, try bundled skills before remote registries.
		if !strings.Contains(input, "/") && !strings.Contains(input, ".") {
			if bundled, err := skill.ListBundledSkills(); err == nil {
				for _, item := range bundled {
					if !strings.EqualFold(item.Name, input) {
						continue
					}
					sk, err := skill.InstallBundledSkill(item.Name, skillInstallAlias, skillInstallOverwrite)
					if err != nil {
						return fmt.Errorf("gagal install bundled skill '%s': %w", item.Name, err)
					}
					fmt.Printf("Skill '%s' berhasil di-install dari bundled skills.\n", sk.Name)
					fmt.Printf("  Deskripsi: %s\n", sk.Description)
					fmt.Printf("  Steps: %d\n", len(sk.Steps))
					if len(sk.Tags) > 0 {
						fmt.Printf("  Tags: %s\n", strings.Join(sk.Tags, ", "))
					}
					return nil
				}
			}

			entries, err := agent.SearchContext7Registry(input)
			if err == nil && len(entries) > 0 {
				// Use exact name match if available
				var target *agent.Context7RegistryEntry
				for _, e := range entries {
					if strings.EqualFold(e.Name, input) {
						target = &e
						break
					}
				}
				if target == nil {
					target = &entries[0]
				}
				sk, err := agent.InstallContext7Skill(*target)
				if err != nil {
					return fmt.Errorf("gagal install skill '%s' dari Context7 registry: %w", target.Name, err)
				}
				fmt.Printf("Skill '%s' berhasil di-install dari Context7 registry.\n", sk.Name)
				fmt.Printf("  Deskripsi: %s\n", sk.Description)
				fmt.Printf("  Steps: %d\n", len(sk.Steps))
				if len(sk.Tags) > 0 {
					fmt.Printf("  Tags: %s\n", strings.Join(sk.Tags, ", "))
				}
				return nil
			}

			// Fallback: try marketplace registry search
			cfg := config.Get()
			var registries []skill.RegistryConfig
			for _, r := range cfg.SkillRegistries {
				registries = append(registries, skill.RegistryConfig{
					Name:      r.Name,
					URL:       r.URL,
					AuthToken: r.AuthToken,
				})
			}
			results, err := skill.Search(input, registries)
			if err != nil || len(results) == 0 {
				return fmt.Errorf("skill '%s' tidak ditemukan di Context7 registry maupun marketplace registry (gunakan URL langsung)", input)
			}
			// Install the first matching marketplace skill
			opts := skill.InstallOptions{
				URL:       results[0].URL,
				Alias:     skillInstallAlias,
				Overwrite: skillInstallOverwrite,
			}
			sk, err := skill.InstallFromURL(opts)
			if err != nil {
				return fmt.Errorf("gagal install skill dari marketplace: %w", err)
			}
			fmt.Printf("Skill '%s' berhasil di-install dari marketplace '%s'.\n", sk.Name, results[0].Name)
			fmt.Printf("  Deskripsi: %s\n", sk.Description)
			fmt.Printf("  Steps: %d\n", len(sk.Steps))
			if len(sk.Tags) > 0 {
				fmt.Printf("  Tags: %s\n", strings.Join(sk.Tags, ", "))
			}
			return nil
		}

		opts := skill.InstallOptions{
			URL:       input,
			Alias:     skillInstallAlias,
			Overwrite: skillInstallOverwrite,
		}

		sk, err := skill.InstallFromURL(opts)
		if err != nil {
			return fmt.Errorf("gagal install skill: %w", err)
		}
		fmt.Printf("Skill '%s' berhasil di-install.\n", sk.Name)
		fmt.Printf("  Deskripsi: %s\n", sk.Description)
		fmt.Printf("  Steps: %d\n", len(sk.Steps))
		if len(sk.Tags) > 0 {
			fmt.Printf("  Tags: %s\n", strings.Join(sk.Tags, ", "))
		}
		return nil
	},
}

var skillPluginAddCmd = &cobra.Command{
	Use:     "add <source|npx skills add source>",
	Aliases: []string{"plugin-add"},
	Short:   "Install skill/plugin dari GitHub shorthand, URL, path lokal, atau format npx skills add",
	Long: `Install declarative Smara skill/plugin dari sumber eksternal.

Contoh:
  smara skill add pbakaus/impeccable
  smara skill add owner/repo/path/to/skill.json
  smara skill add https://example.com/skill.json
  smara skill add ./my-plugin
  smara skill add npx skills add pbakaus/impeccable
  smara skill add "npx skills add pbakaus/impeccable"

Catatan keamanan: command ini menerima format kompatibilitas npx skills add, tetapi tetap memakai installer aman Smara yang hanya membaca manifest skill JSON/Markdown dan tidak menjalankan install script eksternal.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		source, err := skill.NormalizePluginSource(args)
		if err != nil {
			return err
		}
		installed, err := skill.InstallFromPluginSource(skill.PluginInstallOptions{
			Source:    source,
			Alias:     skillPluginAlias,
			Overwrite: skillPluginOverwrite,
		})
		if err != nil {
			return fmt.Errorf("gagal install skill/plugin: %w", err)
		}
		if source != strings.Join(args, " ") {
			fmt.Printf("Terdeteksi format eksternal: %s\n", strings.Join(args, " "))
			fmt.Printf("Menggunakan installer aman Smara untuk source: %s\n", source)
		}
		fmt.Printf("Berhasil install %d skill dari %s:\n", len(installed), source)
		for _, sk := range installed {
			fmt.Printf("  - %s: %s\n", sk.Name, sk.Description)
		}
		fmt.Println("Skill bisa langsung dijalankan dengan: smara skill run <nama-skill>")
		return nil
	},
}

var skillUpdateCmd = &cobra.Command{
	Use:   "update [nama-skill]",
	Short: "Update skill yang sudah di-install dari source URL",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		sk, err := skill.UpdateSkill(name)
		if err != nil {
			return fmt.Errorf("gagal update skill '%s': %w", name, err)
		}
		fmt.Printf("Skill '%s' berhasil di-update ke versi %d.\n", sk.Name, sk.Version)
		return nil
	},
}

var skillInfoCmd = &cobra.Command{
	Use:   "info [nama-skill]",
	Short: "Tampilkan detail skill",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		sk, err := skill.Load(name)
		if err != nil {
			return fmt.Errorf("skill '%s' tidak ditemukan: %w", name, err)
		}
		fmt.Printf("Skill: %s\n", sk.Name)
		fmt.Printf("  Deskripsi: %s\n", sk.Description)
		fmt.Printf("  Versi: %d\n", sk.Version)
		if sk.Author != "" {
			fmt.Printf("  Author: %s\n", sk.Author)
		}
		if sk.SourceURL != "" {
			fmt.Printf("  Source: %s\n", sk.SourceURL)
		}
		if len(sk.Tags) > 0 {
			fmt.Printf("  Tags: %s\n", strings.Join(sk.Tags, ", "))
		}
		if len(sk.Params) > 0 {
			fmt.Println("  Parameters:")
			for _, p := range sk.Params {
				req := "optional"
				if p.Required {
					req = "required"
				}
				fmt.Printf("    - %s (%s, %s): %s\n", p.Name, p.Type, req, p.Description)
			}
		}
		fmt.Printf("  Steps (%d):\n", len(sk.Steps))
		for i, st := range sk.Steps {
			fmt.Printf("    %d. %s\n", i+1, st.Tool)
			if len(st.Args) > 0 {
				for k, v := range st.Args {
					fmt.Printf("       %s = %v\n", k, v)
				}
			}
		}
		return nil
	},
}

var skillSearchQuery string
var skillSearchRegistry string

var skillSearchCmd = &cobra.Command{
	Use:   "search [query/tag]",
	Short: "Cari skill di bundled skills, Context7 registry, dan marketplace",
	RunE: func(cmd *cobra.Command, args []string) error {
		query := ""
		if len(args) > 0 {
			query = args[0]
		}

		var allResults []string

		bundled, err := skill.ListBundledSkills()
		if err == nil && len(bundled) > 0 {
			var matches []string
			q := strings.ToLower(query)
			for _, b := range bundled {
				if q != "" && !strings.Contains(strings.ToLower(b.Name), q) && !strings.Contains(strings.ToLower(b.Description), q) && !tagsContain(b.Tags, q) {
					continue
				}
				tags := ""
				if len(b.Tags) > 0 {
					tags = fmt.Sprintf("  Tags: %s", strings.Join(b.Tags, ", "))
				}
				matches = append(matches, fmt.Sprintf("  %s — %s (v%d)%s", b.Name, b.Description, b.Version, tags))
			}
			if len(matches) > 0 {
				allResults = append(allResults, "Bundled Skills:")
				allResults = append(allResults, matches...)
			}
		}

		// 1. Search Context7 registry
		c7Entries, err := agent.SearchContext7Registry(query)
		if err == nil && len(c7Entries) > 0 {
			allResults = append(allResults, "Context7 Library Skills:")
			for _, e := range c7Entries {
				tags := ""
				if len(e.Tags) > 0 {
					tags = fmt.Sprintf("  Tags: %s", strings.Join(e.Tags, ", "))
				}
				allResults = append(allResults, fmt.Sprintf("  %s — %s%s", e.Name, e.Description, tags))
			}
		}

		// 2. Search marketplace registries
		cfg := config.Get()
		var registries []skill.RegistryConfig
		for _, r := range cfg.SkillRegistries {
			if skillSearchRegistry != "" && r.Name != skillSearchRegistry {
				continue
			}
			registries = append(registries, skill.RegistryConfig{
				Name:      r.Name,
				URL:       r.URL,
				AuthToken: r.AuthToken,
			})
		}

		if len(registries) > 0 {
			results, err := skill.Search(query, registries)
			if err == nil && len(results) > 0 {
				if len(allResults) > 0 {
					allResults = append(allResults, "")
				}
				allResults = append(allResults, "Marketplace Skills:")
				for _, entry := range results {
					meta := ""
					if entry.Author != "" {
						meta = fmt.Sprintf("    Author: %s  Downloads: %d  Rating: %.1f", entry.Author, entry.Downloads, entry.Rating)
					}
					tags := ""
					if len(entry.Tags) > 0 {
						tags = fmt.Sprintf("  Tags: %s", strings.Join(entry.Tags, ", "))
					}
					allResults = append(allResults, fmt.Sprintf("  %s — %s (v%d)%s", entry.Name, entry.Description, entry.Version, tags))
					if meta != "" {
						allResults = append(allResults, meta)
					}
				}
			}
		}

		if len(allResults) == 0 {
			fmt.Println("Tidak ada skill yang cocok di Context7 registry maupun marketplace.")
			return nil
		}

		fmt.Println(strings.Join(allResults, "\n"))
		return nil
	},
}

var skillPublishCmd = &cobra.Command{
	Use:   "publish [nama-skill]",
	Short: "Publikasikan skill ke marketplace/registry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		sk, err := skill.Load(name)
		if err != nil {
			return fmt.Errorf("skill '%s' tidak ditemukan: %w", name, err)
		}

		cfg := config.Get()
		if len(cfg.SkillRegistries) == 0 {
			return fmt.Errorf("tidak ada registry yang terdaftar (konfigurasi di skill_registries)")
		}

		// Default to first registry if only one
		regCfg := cfg.SkillRegistries[0]
		r := skill.RegistryConfig{
			Name:      regCfg.Name,
			URL:       regCfg.URL,
			AuthToken: regCfg.AuthToken,
		}

		if err := skill.Publish(sk, r); err != nil {
			return fmt.Errorf("gagal publish skill: %w", err)
		}
		return nil
	},
}

var skillRegistryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Kelola registry skill",
}

var skillRegistryListCmd = &cobra.Command{
	Use:   "list",
	Short: "Daftar registry yang terdaftar",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Get()
		if len(cfg.SkillRegistries) == 0 {
			fmt.Println("Tidak ada registry yang terdaftar.")
			return
		}
		fmt.Println("Registry yang terdaftar:")
		for _, r := range cfg.SkillRegistries {
			fmt.Printf("  - %s: %s\n", r.Name, r.URL)
		}
	},
}

var skillRegistrySyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sinkronkan cache lokal untuk semua registry",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Get()
		registries := make([]skill.RegistryConfig, 0, len(cfg.SkillRegistries))
		for _, r := range cfg.SkillRegistries {
			registries = append(registries, skill.RegistryConfig{
				Name:      r.Name,
				URL:       r.URL,
				AuthToken: r.AuthToken,
			})
		}

		if len(registries) == 0 {
			return fmt.Errorf("tidak ada registry yang terdaftar")
		}

		if err := skill.SyncRegistries(registries); err != nil {
			return fmt.Errorf("gagal sync registry: %w", err)
		}
		fmt.Println("Registry cache berhasil disinkronkan.")
		return nil
	},
}

var skillTreeCmd = &cobra.Command{
	Use:   "tree",
	Short: "Tampilkan hierarki skill tree",
	Run: func(cmd *cobra.Command, args []string) {
		tm, err := skill.BuildTree()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Gagal build tree: %v\n", err)
			os.Exit(1)
		}
		for name := range tm.AllNodes() {
			fmt.Printf("- %s\n", name)
			deps, _ := tm.GetDependencies(name)
			for _, d := range deps {
				fmt.Printf("  -> depends on: %s\n", d)
			}
			next := tm.SuggestNextSkills(name)
			for _, n := range next {
				fmt.Printf("  <- unlocks: %s\n", n)
			}
		}
	},
}

var skillStatsCmd = &cobra.Command{
	Use:   "stats [nama-skill]",
	Short: "Tampilkan statistik eksekusi skill",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Statistik skill '%s': (butuh DB tracker)\n", args[0])
	},
}

var skillRefineCmd = &cobra.Command{
	Use:   "refine [nama-skill]",
	Short: "Trigger manual refinement untuk skill",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		cfg := config.Get()
		var provider llm.Provider
		if cfg.Provider != "" {
			pc := llm.ProviderConfig{
				Name:   cfg.Provider,
				Model:  cfg.Model,
				Host:   cfg.OllamaHost,
				APIKey: cfg.OpenAIAPIKey,
			}
			var err error
			provider, err = llm.NewProvider(pc)
			if err != nil {
				provider = nil
			}
		}
		if provider == nil {
			return fmt.Errorf("LLM provider tidak tersedia, konfigurasi provider terlebih dahulu")
		}
		prompt, sk, err := skill.BuildRefinementPromptFull(name, &skill.ExecutionTracker{}, nil)
		if err != nil {
			return err
		}
		resp, _, err := skill.RefineSkill(name, &skill.ExecutionTracker{}, nil, provider)
		if err != nil {
			return err
		}
		fmt.Printf("Prompt:\n%s\n\n", prompt)
		fmt.Printf("Proposed refinement for '%s' (v%d):\n%s\n", sk.Name, sk.Version, resp)
		return nil
	},
}

var skillAnalyticsCmd = &cobra.Command{
	Use:   "analytics",
	Short: "Tampilkan global skill analytics",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Global analytics: (butuh DB tracker)")
	},
}

func tagsContain(tags []string, query string) bool {
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}

func init() {
	skillCmd.AddCommand(skillRunCmd, skillListCmd, skillDeleteCmd, skillCreateCmd)
	skillCmd.AddCommand(skillInstallCmd, skillUpdateCmd, skillInfoCmd)
	skillCmd.AddCommand(skillSearchCmd, skillPublishCmd, skillRegistryCmd)
	skillCmd.AddCommand(skillTreeCmd, skillStatsCmd, skillRefineCmd, skillAnalyticsCmd)
	skillCmd.AddCommand(skillPluginAddCmd)
	rootCmd.AddCommand(skillCmd)

	skillRunCmd.Flags().StringVar(&skillRunArgs, "args", "", "Argumen runtime skill sebagai JSON object")
	skillInstallCmd.Flags().StringVar(&skillInstallAlias, "as", "", "Alias nama skill (override nama dari JSON)")
	skillInstallCmd.Flags().BoolVar(&skillInstallOverwrite, "overwrite", false, "Timpa skill yang sudah ada")
	skillSearchCmd.Flags().StringVar(&skillSearchQuery, "query", "", "Filter kata kunci (positional juga bisa)")
	skillSearchCmd.Flags().StringVar(&skillSearchRegistry, "registry", "", "Filter nama registry tertentu")
	skillCreateCmd.Flags().StringVar(&skillCreateFormat, "format", "json", "Format input skill: json atau md (markdown)")
	skillPluginAddCmd.Flags().StringVar(&skillPluginAlias, "as", "", "Alias nama skill jika sumber hanya berisi satu skill")
	skillPluginAddCmd.Flags().BoolVar(&skillPluginOverwrite, "overwrite", false, "Timpa skill yang sudah ada")
}
