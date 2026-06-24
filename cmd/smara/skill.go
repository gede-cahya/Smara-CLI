package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	stdsync "sync"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/mcp"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/gede-cahya/Smara-CLI/internal/skill"
	"github.com/spf13/cobra"
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Kelola reusable automation skill",
	Long:  `Buat, jalankan, edit, dan hapus skill (resep automation yang tersimpan).`,
}

var skillRunArgs string
var skillInstallAlias string
var skillInstallOverwrite bool
var skillInstallReview bool
var skillInstallApprove bool
var skillInstallAllowInvalid bool
var skillInstallAllowTools []string
var skillInstallBlockTools []string
var skillCreateFormat string
var skillPluginAlias string
var skillPluginOverwrite bool
var skillLintFormat string
var skillLintStrict bool
var skillLintAllowTools []string
var skillLintToolFile string
var skillRefinePreview bool
var skillRefineDiff bool
var skillRefineApply bool
var skillRefineProposalFile string
var skillRefineAllowInvalid bool
var skillRefineFormat string
var skillHistoryFormat string
var skillCompareFrom string
var skillCompareTo string
var skillCompareFormat string
var skillRollbackTo string
var skillRunDryRun bool
var skillRunApprove bool
var skillInspectRisk bool
var skillInspectFormat string
var skillSearchTag string
var skillSearchLocal bool
var skillTreeFormat string
var skillStatsFormat string
var skillStatsLimit int
var skillStatsAll bool
var skillAnalyticsFormat string
var skillRecommendFormat string
var skillRecommendLimit int
var skillRecommendNoHistory bool

var skillRunCmd = &cobra.Command{
	Use:   "run <name>",
	Short: "Jalankan skill yang tersimpan",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		start := time.Now()
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

		assessment := skill.AssessRisk(sk)
		if skillRunDryRun {
			printRiskAssessment(assessment)
			return nil
		}

		rushAutoApprove := agent.IsRushAutoApprovalEnabled()
		if assessment.RequiresApproval && !skillRunApprove && !rushAutoApprove {
			printRiskAssessment(assessment)
			return fmt.Errorf("skill '%s' berisiko %s dan membutuhkan approval eksplisit; jalankan ulang dengan --approve setelah review dry-run", sk.Name, assessment.Level)
		}
		if assessment.RequiresApproval {
			if rushAutoApprove && !skillRunApprove {
				fmt.Printf("Rush auto-approval aktif; bypass approval untuk skill berisiko %s: %s\n", assessment.Level, sk.Name)
			} else {
				fmt.Printf("Approval diterima untuk skill berisiko %s: %s\n", assessment.Level, sk.Name)
			}
		}

		supervisor, err := getSupervisorForSkill()
		if err != nil {
			return err
		}
		defer supervisor.Close()
		if rushAutoApprove {
			supervisor.SetMode(agent.ModeRush)
		}

		var result *skill.RunResult
		if sk.HasWorkflowComposition() {
			plan := sk.CompositionPlan()
			fmt.Println("Composition plan:")
			for i, st := range plan.Steps {
				marker := ""
				if st.Blocking {
					marker = " blocking"
				}
				fmt.Printf("  %d. %s: %s%s\n", i+1, st.Kind, st.SkillName, marker)
			}
			if len(plan.Suggests) > 0 {
				fmt.Printf("  Suggestions (tidak memblokir): %s\n", strings.Join(plan.Suggests, ", "))
			}
			composed, err := sk.RunComposed(func(depName string) (*skill.Skill, error) { return skill.Load(depName) }, supervisor.SkillExecutor())
			if err != nil {
				return fmt.Errorf("skill workflow execution error: %w", err)
			}
			result = composedRunToRunResult(sk.Name, composed)
		} else {
			result, err = sk.Run(supervisor.SkillExecutor())
			if err != nil {
				return fmt.Errorf("skill execution error: %w", err)
			}
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
		if err := logSkillRunToDefaultTracker(sk.Name, result, start); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: gagal menyimpan run history: %v\n", err)
		}
		return nil
	},
}

func composedRunToRunResult(skillName string, composed *skill.ComposedRunResult) *skill.RunResult {
	if composed == nil {
		return &skill.RunResult{SkillName: skillName, Success: false, Summary: "skill workflow returned no result"}
	}
	result := &skill.RunResult{SkillName: skillName, Success: composed.Success, Summary: composed.Summary}
	for _, run := range composed.Results {
		result.StepResults = append(result.StepResults, run.StepResults...)
	}
	return result
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

var skillHistoryCmd = &cobra.Command{
	Use:   "history [nama-skill]",
	Short: "Tampilkan riwayat versi skill",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, current, err := skill.History(args[0])
		if err != nil {
			return err
		}
		if skillHistoryFormat == "json" {
			data, _ := json.MarshalIndent(struct {
				Current *skill.Skill         `json:"current"`
				Lineage []skill.LineageEntry `json:"lineage"`
			}{current, entries}, "", "  ")
			fmt.Println(string(data))
			return nil
		}
		fmt.Printf("History skill '%s' (current v%d):\n", current.Name, current.Version)
		if len(entries) == 0 {
			fmt.Println("  Belum ada lineage/history.")
		} else {
			for _, e := range entries {
				when := e.RefinedAt.Format("2006-01-02 15:04:05")
				fmt.Printf("  - v%d | %s | steps:%d | source:%s | %s\n", e.Version, when, e.StepCount, e.RefinedFrom, e.Description)
			}
		}
		fmt.Printf("  - v%d | current | steps:%d | %s\n", current.Version, len(current.Steps), current.Description)
		return nil
	},
}

var skillCompareCmd = &cobra.Command{
	Use:   "compare [nama-skill] --from v1 --to v2",
	Short: "Bandingkan dua versi skill",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		from, err := skill.ResolveVersion(skillCompareFrom)
		if err != nil {
			return err
		}
		to, err := skill.ResolveVersion(skillCompareTo)
		if err != nil {
			return err
		}
		res, err := skill.CompareVersions(args[0], from, to)
		if err != nil {
			return err
		}
		if skillCompareFormat == "json" {
			fmt.Println(res.JSON())
			return nil
		}
		fmt.Printf("Compare skill '%s': v%d → v%d\n", res.Name, res.FromVersion, res.ToVersion)
		if len(res.Changes) == 0 {
			fmt.Println("  Tidak ada perubahan.")
			return nil
		}
		for _, ch := range res.Changes {
			fmt.Printf("  - %s berubah\n", ch.Field)
		}
		return nil
	},
}

var skillRollbackCmd = &cobra.Command{
	Use:   "rollback [nama-skill] --to v2",
	Short: "Rollback skill ke versi lama dan simpan sebagai versi baru",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		to, err := skill.ResolveVersion(skillRollbackTo)
		if err != nil {
			return err
		}
		sk, err := skill.Rollback(args[0], to, nil)
		if err != nil {
			return err
		}
		fmt.Printf("Skill '%s' rollback ke v%d dan disimpan sebagai v%d. History lama tetap dipertahankan.\n", sk.Name, to, sk.Version)
		return nil
	},
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

var skillRecommendCmd = &cobra.Command{
	Use:   "recommend <query>",
	Short: "Rekomendasikan skill terbaik berdasarkan query",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")
		skills, err := loadAllLocalSkills()
		if err != nil {
			return err
		}
		var stats skill.RecommendationStatsProvider
		var closeFn func()
		if !skillRecommendNoHistory {
			tracker, closer, err := openDefaultSkillTracker()
			if err == nil {
				stats = tracker
				closeFn = closer
			} else {
				fmt.Fprintf(os.Stderr, "Warning: history tracker tidak tersedia: %v\n", err)
			}
		}
		if closeFn != nil {
			defer closeFn()
		}
		recs := skill.RecommendSkills(query, skills, skill.RecommendationOptions{Limit: skillRecommendLimit, StatsProvider: stats})
		if strings.EqualFold(skillRecommendFormat, "json") {
			data, err := json.MarshalIndent(recs, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}
		if len(recs) == 0 {
			fmt.Println("Tidak ada skill yang cocok. Coba query yang lebih spesifik.")
			return nil
		}
		fmt.Printf("Rekomendasi skill untuk: %q\n", query)
		for i, rec := range recs {
			clarify := ""
			if rec.Clarify {
				clarify = " (confidence rendah, perlu klarifikasi)"
			}
			fmt.Printf("%d. %s — score %.1f, confidence %s%s\n", i+1, rec.SkillName, rec.Score, rec.Confidence, clarify)
			if len(rec.Reasons) > 0 {
				fmt.Printf("   alasan: %s\n", strings.Join(rec.Reasons, "; "))
			}
		}
		return nil
	},
}

var skillSuggestCmd = &cobra.Command{
	Use:   "suggest <query>",
	Short: "Alias untuk skill recommend",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return skillRecommendCmd.RunE(cmd, args)
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
				URL:          results[0].URL,
				Alias:        skillInstallAlias,
				Overwrite:    skillInstallOverwrite,
				ReviewOnly:   skillInstallReview,
				Approve:      skillInstallApprove,
				AllowInvalid: skillInstallAllowInvalid,
				AllowedTools: skillInstallAllowTools,
				BlockedTools: skillInstallBlockTools,
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
			URL:          input,
			Alias:        skillInstallAlias,
			Overwrite:    skillInstallOverwrite,
			ReviewOnly:   skillInstallReview,
			Approve:      skillInstallApprove,
			AllowInvalid: skillInstallAllowInvalid,
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

var skillInspectCmd = &cobra.Command{
	Use:   "inspect [nama-skill]",
	Short: "Inspect detail skill termasuk metadata, dependency, lineage, dan risk assessment",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sk, err := skill.Load(args[0])
		if err != nil {
			return fmt.Errorf("skill '%s' tidak ditemukan: %w", args[0], err)
		}
		if skillInspectFormat == "json" {
			payload := struct {
				Skill *skill.Skill         `json:"skill"`
				Risk  skill.RiskAssessment `json:"risk"`
			}{Skill: sk, Risk: skill.AssessRisk(sk)}
			data, _ := json.MarshalIndent(payload, "", "  ")
			fmt.Println(string(data))
			return nil
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
		if len(sk.CategoryPath) > 0 {
			fmt.Printf("  Category: %s\n", strings.Join(sk.CategoryPath, " > "))
		}
		if sk.ParentID != "" {
			fmt.Printf("  Parent: %s\n", sk.ParentID)
		}
		if len(sk.Dependencies) > 0 {
			fmt.Printf("  Dependencies: %s\n", strings.Join(sk.Dependencies, ", "))
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
		}
		if len(sk.Lineage) > 0 {
			fmt.Printf("  Lineage versions: %d\n", len(sk.Lineage))
		}
		if skillInspectRisk {
			printRiskAssessment(skill.AssessRisk(sk))
		}
		return nil
	},
}

func printRiskAssessment(assessment skill.RiskAssessment) {
	fmt.Printf("Risk assessment: %s\n", strings.ToUpper(assessment.Level))
	fmt.Printf("  Requires approval: %t\n", assessment.RequiresApproval)
	if len(assessment.Categories) > 0 {
		fmt.Printf("  Categories: %s\n", strings.Join(assessment.Categories, ", "))
	}
	if len(assessment.Reasons) > 0 {
		fmt.Println("  Reasons:")
		for _, r := range assessment.Reasons {
			fmt.Printf("    - %s\n", r)
		}
	}
	if len(assessment.SimulationSummary) > 0 {
		fmt.Println("  Dry-run summary:")
		for _, s := range assessment.SimulationSummary {
			fmt.Printf("    - %s\n", s)
		}
	}
}

var skillSearchQuery string
var skillSearchRegistry string

var skillSearchCmd = &cobra.Command{
	Use:   "search [query/tag]",
	Short: "Cari skill lokal, bundled, Context7 registry, dan marketplace",
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.TrimSpace(skillSearchQuery)
		if query == "" && len(args) > 0 {
			query = args[0]
		}

		var allResults []string
		q := strings.ToLower(query)
		tagFilter := strings.ToLower(strings.TrimSpace(skillSearchTag))

		localNames, err := skill.List()
		if err == nil && len(localNames) > 0 {
			var matches []string
			for _, name := range localNames {
				sk, err := skill.Load(name)
				if err != nil || sk == nil {
					continue
				}
				if q != "" && !strings.Contains(strings.ToLower(sk.Name), q) && !strings.Contains(strings.ToLower(sk.Description), q) && !tagsContain(sk.Tags, q) {
					continue
				}
				if tagFilter != "" && !tagsContain(sk.Tags, tagFilter) {
					continue
				}
				tags := ""
				if len(sk.Tags) > 0 {
					tags = fmt.Sprintf("  Tags: %s", strings.Join(sk.Tags, ", "))
				}
				matches = append(matches, fmt.Sprintf("  %s — %s (v%d)%s", sk.Name, sk.Description, sk.Version, tags))
			}
			if len(matches) > 0 {
				allResults = append(allResults, "Local Skills:")
				allResults = append(allResults, matches...)
			}
		}

		if skillSearchLocal {
			if len(allResults) == 0 {
				fmt.Println("Tidak ada skill lokal yang cocok.")
				return nil
			}
			fmt.Println(strings.Join(allResults, "\n"))
			return nil
		}

		bundled, err := skill.ListBundledSkills()
		if err == nil && len(bundled) > 0 {
			var matches []string
			for _, b := range bundled {
				if q != "" && !strings.Contains(strings.ToLower(b.Name), q) && !strings.Contains(strings.ToLower(b.Description), q) && !tagsContain(b.Tags, q) {
					continue
				}
				if tagFilter != "" && !tagsContain(b.Tags, tagFilter) {
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
	RunE: func(cmd *cobra.Command, args []string) error {
		tm, err := skill.BuildTree()
		if err != nil {
			return fmt.Errorf("gagal build tree: %w", err)
		}
		if skillTreeFormat == "json" {
			nodes, edges := tm.ToGraphJSON()
			data, _ := json.MarshalIndent(struct {
				Nodes []map[string]interface{} `json:"nodes"`
				Edges []map[string]interface{} `json:"edges"`
			}{nodes, edges}, "", "  ")
			fmt.Println(string(data))
			return nil
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
		return nil
	},
}

var skillRunsCmd = &cobra.Command{
	Use:   "runs [nama-skill]",
	Short: "Tampilkan run history skill",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tracker, closeFn, err := openDefaultSkillTracker()
		if err != nil {
			return err
		}
		defer closeFn()
		name := ""
		if len(args) > 0 {
			name = args[0]
		}
		runs, err := tracker.GetTimeline(name, skillStatsLimit)
		if err != nil {
			return err
		}
		if skillStatsFormat == "json" {
			data, _ := json.MarshalIndent(runs, "", "  ")
			fmt.Println(string(data))
			return nil
		}
		if len(runs) == 0 {
			fmt.Println("Belum ada run history.")
			return nil
		}
		for _, r := range runs {
			fmt.Printf("- %s | %s | %s | %dms", r.StartedAt.Format("2006-01-02 15:04:05"), r.SkillName, r.Status, r.DurationMs)
			if r.FailedStep > 0 {
				fmt.Printf(" | failed_step:%d", r.FailedStep)
			}
			if r.VersionID != "" {
				fmt.Printf(" | version:%s", r.VersionID)
			}
			fmt.Println()
			if r.ErrorMessage != "" {
				fmt.Printf("  error: %s\n", r.ErrorMessage)
			}
		}
		return nil
	},
}

var skillStatsCmd = &cobra.Command{
	Use:   "stats [nama-skill]",
	Short: "Tampilkan statistik eksekusi skill",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tracker, closeFn, err := openDefaultSkillTracker()
		if err != nil {
			return err
		}
		defer closeFn()
		if len(args) == 0 || skillStatsAll {
			top, err := tracker.GetTopSkills(skillStatsLimit)
			if err != nil {
				return err
			}
			if skillStatsFormat == "json" {
				data, _ := json.MarshalIndent(top, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Println("Statistik skill global:")
			for _, s := range top {
				fmt.Printf("  - %s: %d run, success %.1f%%\n", s.Name, s.RunCount, s.SuccessRate)
			}
			return nil
		}
		total, successCount, avgMs, lastRun, err := tracker.GetStats(args[0])
		if err != nil {
			return err
		}
		payload := map[string]interface{}{"skill_name": args[0], "total_runs": total, "successful_runs": successCount, "failed_runs": total - successCount, "avg_duration_ms": avgMs}
		if lastRun != nil {
			payload["last_run"] = lastRun
		}
		if skillStatsFormat == "json" {
			data, _ := json.MarshalIndent(payload, "", "  ")
			fmt.Println(string(data))
			return nil
		}
		rate := 0.0
		if total > 0 {
			rate = float64(successCount) / float64(total) * 100
		}
		fmt.Printf("Statistik skill '%s':\n", args[0])
		fmt.Printf("  Total run: %d\n  Sukses: %d\n  Gagal: %d\n  Success rate: %.1f%%\n  Avg duration: %dms\n", total, successCount, total-successCount, rate, avgMs)
		if lastRun != nil {
			fmt.Printf("  Last run: %s\n", lastRun.Format("2006-01-02 15:04:05"))
		}
		return nil
	},
}

var skillRefineCmd = &cobra.Command{
	Use:   "refine [nama-skill]",
	Short: "Preview, diff, atau apply refinement untuk skill",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(skillRefineProposalFile) == "" {
			return fmt.Errorf("gunakan --proposal <file.json> untuk refine preview/diff/apply")
		}
		return runSkillRefineWithProposal(args[0])
	},
}

func runSkillRefineWithProposal(name string) error {
	original, err := skill.Load(name)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(skillRefineProposalFile)
	if err != nil {
		return fmt.Errorf("gagal membaca proposal: %w", err)
	}
	proposed, err := skill.FromJSON(data)
	if err != nil {
		return err
	}
	proposed = skill.NormalizeRefinementProposal(original, proposed)
	knownTools, err := skillKnownTools()
	if err != nil {
		return err
	}
	opts := skill.LintOptions{KnownTools: knownTools}
	preview := skill.BuildRefinementPreview(original, proposed, opts)
	if skillRefineDiff {
		fmt.Println(skill.MarshalSkillDiff(original, proposed))
	}
	if skillRefinePreview || !skillRefineApply {
		printRefinementPreview(preview)
	}
	if !skillRefineApply {
		return nil
	}
	applied, preview, err := skill.ApplyRefinementWithLint(name, data, nil, "manual", opts, skillRefineAllowInvalid)
	if err != nil {
		printRefinementPreview(preview)
		return err
	}
	fmt.Printf("Refinement applied: %s v%d\n", applied.Name, applied.Version)
	return nil
}

func printRefinementPreview(preview skill.RefinementPreview) {
	if skillRefineFormat == "json" {
		data, _ := preview.ToJSON()
		fmt.Println(string(data))
		return
	}
	fmt.Printf("Refinement preview: %s -> %s\n", preview.OriginalName, preview.ProposedName)
	for _, item := range preview.Summary {
		fmt.Printf("- %s\n", item)
	}
	if len(preview.Lint.Issues) == 0 {
		fmt.Println("Lint: PASS")
		return
	}
	fmt.Println("Lint issues:")
	for _, issue := range preview.Lint.Issues {
		fmt.Printf("[%s] %s.%s: %s\n", strings.ToUpper(issue.Severity), issue.Skill, issue.Field, issue.Message)
	}
}

var skillAnalyticsCmd = &cobra.Command{
	Use:   "analytics",
	Short: "Tampilkan global skill analytics",
	RunE: func(cmd *cobra.Command, args []string) error {
		tracker, closeFn, err := openDefaultSkillTracker()
		if err != nil {
			return err
		}
		defer closeFn()
		data, err := tracker.GlobalAnalytics()
		if err != nil {
			return err
		}
		if skillAnalyticsFormat == "json" {
			b, _ := json.MarshalIndent(data, "", "  ")
			fmt.Println(string(b))
			return nil
		}
		fmt.Printf("Global analytics:\n  Total run: %v\n  Sukses: %v\n  Overall rate: %.1f%%\n", data["total_runs"], data["successful_runs"], data["overall_rate"])
		return nil
	},
}
var skillLintCmd = &cobra.Command{
	Use:   "lint [nama-skill]",
	Short: "Lint satu atau semua skill",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		report, err := runSkillLint(args)
		if err != nil {
			return err
		}
		printSkillLintReport(report)
		if report.HasErrors() || (skillLintStrict && lintWarningCount(report) > 0) {
			return fmt.Errorf("skill lint gagal: %d error, %d warning", lintErrorCount(report), lintWarningCount(report))
		}
		return nil
	},
}

var skillValidateCmd = &cobra.Command{
	Use:   "validate [nama-skill]",
	Short: "Validasi satu atau semua skill dan gagal pada warning/error",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		prevStrict := skillLintStrict
		skillLintStrict = true
		defer func() { skillLintStrict = prevStrict }()
		report, err := runSkillLint(args)
		if err != nil {
			return err
		}
		printSkillLintReport(report)
		if report.HasErrors() || lintWarningCount(report) > 0 {
			return fmt.Errorf("skill validation gagal: %d error, %d warning", lintErrorCount(report), lintWarningCount(report))
		}
		return nil
	},
}

func runSkillLint(args []string) (skill.LintReport, error) {
	knownTools, err := skillKnownTools()
	if err != nil {
		return skill.LintReport{}, err
	}
	if len(args) == 0 {
		return skill.LintAllWithKnownTools(knownTools)
	}

	name := args[0]
	sk, err := skill.Load(name)
	if err != nil {
		return skill.LintReport{}, fmt.Errorf("skill '%s' tidak ditemukan: %w", name, err)
	}
	existing := map[string]bool{}
	names, err := skill.List()
	if err != nil {
		return skill.LintReport{}, err
	}
	for _, n := range names {
		existing[n] = true
	}
	return skill.LintSkillWithOptions(sk, skill.LintOptions{Existing: existing, KnownTools: knownTools}), nil
}

func skillKnownTools() (map[string]bool, error) {
	known := map[string]bool{}
	for _, tool := range agent.GetBuiltinTools() {
		known[tool.Name] = true
	}
	for _, tool := range defaultExternalSkillTools() {
		known[tool] = true
	}
	for _, raw := range skillLintAllowTools {
		for _, tool := range splitToolList(raw) {
			known[tool] = true
		}
	}
	if strings.TrimSpace(skillLintToolFile) != "" {
		tools, err := loadSkillLintToolFile(skillLintToolFile)
		if err != nil {
			return nil, err
		}
		for _, tool := range tools {
			known[tool] = true
		}
	}
	return known, nil
}

func defaultExternalSkillTools() []string {
	return []string{
		"execute_blender_code",
		"get-library-documentation",
		"get_scene_info",
		"resolve",
		"resolve-library-id",
	}
}

func splitToolList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		tool := strings.TrimSpace(part)
		if tool != "" {
			out = append(out, tool)
		}
	}
	return out
}

func loadSkillLintToolFile(path string) ([]string, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("gagal baca tool registry '%s': %w", path, err)
	}

	var names []string
	if err := json.Unmarshal(data, &names); err == nil {
		return names, nil
	}

	var wrapped struct {
		Tools []string `json:"tools"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && len(wrapped.Tools) > 0 {
		return wrapped.Tools, nil
	}

	return nil, fmt.Errorf("tool registry '%s' harus JSON array string atau object {\"tools\": [...]}", path)
}

func printSkillLintReport(report skill.LintReport) {
	if skillLintFormat == "json" {
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
		return
	}

	if len(report.Issues) == 0 {
		fmt.Println("Skill lint PASS: tidak ada issue.")
		return
	}
	for _, issue := range report.Issues {
		loc := issue.Skill
		if issue.Field != "" {
			if loc != "" {
				loc += "."
			}
			loc += issue.Field
		}
		if loc == "" {
			loc = "skill"
		}
		fmt.Printf("[%s] %s: %s\n", strings.ToUpper(issue.Severity), loc, issue.Message)
	}
	fmt.Printf("Skill lint selesai: %d error, %d warning.\n", lintErrorCount(report), lintWarningCount(report))
}

func lintErrorCount(report skill.LintReport) int {
	count := 0
	for _, issue := range report.Issues {
		if issue.Severity == "error" {
			count++
		}
	}
	return count
}

func lintWarningCount(report skill.LintReport) int {
	count := 0
	for _, issue := range report.Issues {
		if issue.Severity == "warning" {
			count++
		}
	}
	return count
}

func tagsContain(tags []string, query string) bool {
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}

func loadAllLocalSkills() ([]*skill.Skill, error) {
	names, err := skill.List()
	if err != nil {
		return nil, fmt.Errorf("gagal list skill: %w", err)
	}
	skills := make([]*skill.Skill, 0, len(names))
	for _, name := range names {
		sk, err := skill.Load(name)
		if err != nil || sk == nil {
			continue
		}
		skills = append(skills, sk)
	}
	return skills, nil
}

func openDefaultSkillTracker() (*skill.ExecutionTracker, func(), error) {
	cfg := config.Get()
	dbPath := cfg.DBPath
	if strings.TrimSpace(dbPath) == "" {
		dbPath = filepath.Join(os.TempDir(), "smara-skill-history.db")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, nil, fmt.Errorf("gagal membuat direktori tracker: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("gagal membuka tracker DB: %w", err)
	}
	tracker, err := skill.NewExecutionTracker(db)
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	return tracker, func() { _ = db.Close() }, nil
}

func logSkillRunToDefaultTracker(skillName string, result *skill.RunResult, start time.Time) error {
	tracker, closeFn, err := openDefaultSkillTracker()
	if err != nil {
		return err
	}
	defer closeFn()
	workspace, _ := os.Getwd()
	runID := fmt.Sprintf("run-%d", start.UnixNano())
	versionID := ""
	if sk, err := skill.Load(skillName); err == nil && sk != nil {
		versionID = fmt.Sprintf("v%d", sk.Version)
	}
	return tracker.LogRunWithMetadata(skillName, versionID, skillRunApprove, runID, "cli", workspace, "skill-run", result, start)
}

func init() {
	skillCmd.AddCommand(skillRunCmd, skillListCmd, skillDeleteCmd, skillCreateCmd)
	skillCmd.AddCommand(skillInstallCmd, skillUpdateCmd, skillInfoCmd)
	skillCmd.AddCommand(skillSearchCmd, skillPublishCmd, skillRegistryCmd)
	skillCmd.AddCommand(skillTreeCmd, skillStatsCmd, skillRunsCmd, skillRefineCmd, skillAnalyticsCmd, skillInspectCmd)
	skillCmd.AddCommand(skillRecommendCmd, skillSuggestCmd)
	skillCmd.AddCommand(skillLintCmd, skillValidateCmd)
	skillCmd.AddCommand(skillHistoryCmd, skillCompareCmd, skillRollbackCmd)
	skillCmd.AddCommand(skillPluginAddCmd)
	rootCmd.AddCommand(skillCmd)

	skillRunCmd.Flags().StringVar(&skillRunArgs, "args", "", "Argumen runtime skill sebagai JSON object")
	skillRunCmd.Flags().BoolVar(&skillRunDryRun, "dry-run", false, "Tampilkan simulasi/risk summary tanpa menjalankan skill")
	skillRunCmd.Flags().BoolVar(&skillRunApprove, "approve", false, "Setujui eksekusi skill berisiko tinggi/kritis")
	skillInstallCmd.Flags().StringVar(&skillInstallAlias, "as", "", "Alias nama skill (override nama dari JSON)")
	skillInstallCmd.Flags().BoolVar(&skillInstallOverwrite, "overwrite", false, "Timpa skill yang sudah ada")
	skillInstallCmd.Flags().BoolVar(&skillInstallReview, "review", false, "Tampilkan security review tanpa menyimpan skill")
	skillInstallCmd.Flags().BoolVar(&skillInstallApprove, "approve", false, "Setujui install skill remote/berisiko setelah review")
	skillInstallCmd.Flags().BoolVar(&skillInstallAllowInvalid, "allow-invalid", false, "Izinkan install walau lint/validasi skill gagal")
	skillSearchCmd.Flags().StringVar(&skillSearchQuery, "query", "", "Filter kata kunci (positional juga bisa)")
	skillSearchCmd.Flags().StringVar(&skillSearchRegistry, "registry", "", "Filter nama registry tertentu")
	skillSearchCmd.Flags().StringVar(&skillSearchTag, "tag", "", "Filter skill berdasarkan tag")
	skillSearchCmd.Flags().BoolVar(&skillSearchLocal, "local", false, "Cari hanya di skill lokal")
	skillCreateCmd.Flags().StringVar(&skillCreateFormat, "format", "json", "Format input skill: json atau md (markdown)")
	skillPluginAddCmd.Flags().StringVar(&skillPluginAlias, "as", "", "Alias nama skill jika sumber hanya berisi satu skill")
	skillPluginAddCmd.Flags().BoolVar(&skillPluginOverwrite, "overwrite", false, "Timpa skill yang sudah ada")
	skillLintCmd.Flags().StringVar(&skillLintFormat, "format", "text", "Format output lint: text atau json")
	skillLintCmd.Flags().BoolVar(&skillLintStrict, "strict", false, "Anggap warning sebagai failure")
	skillLintCmd.Flags().StringSliceVar(&skillLintAllowTools, "allow-tool", nil, "Whitelist tool eksternal tambahan (boleh comma-separated, repeatable)")
	skillLintCmd.Flags().StringVar(&skillLintToolFile, "tool-registry", "", "File JSON daftar tool eksternal: [\"tool\"] atau {\"tools\":[\"tool\"]}")
	skillValidateCmd.Flags().StringVar(&skillLintFormat, "format", "text", "Format output validasi: text atau json")
	skillValidateCmd.Flags().StringSliceVar(&skillLintAllowTools, "allow-tool", nil, "Whitelist tool eksternal tambahan (boleh comma-separated, repeatable)")
	skillValidateCmd.Flags().StringVar(&skillLintToolFile, "tool-registry", "", "File JSON daftar tool eksternal: [\"tool\"] atau {\"tools\":[\"tool\"]}")
	skillRefineCmd.Flags().BoolVar(&skillRefinePreview, "preview", false, "Tampilkan ringkasan perubahan dan hasil lint tanpa apply")
	skillRefineCmd.Flags().BoolVar(&skillRefineDiff, "diff", false, "Tampilkan diff JSON skill lama vs proposal")
	skillRefineCmd.Flags().BoolVar(&skillRefineApply, "apply", false, "Apply proposal setelah auto-lint")
	skillRefineCmd.Flags().StringVar(&skillRefineProposalFile, "proposal", "", "File JSON proposal refinement untuk preview/diff/apply")
	skillRefineCmd.Flags().BoolVar(&skillRefineAllowInvalid, "allow-invalid", false, "Izinkan apply walau lint proposal memiliki error")
	skillRefineCmd.Flags().StringVar(&skillRefineFormat, "format", "text", "Format output preview: text atau json")
	skillHistoryCmd.Flags().StringVar(&skillHistoryFormat, "format", "text", "Format output history: text atau json")
	skillCompareCmd.Flags().StringVar(&skillCompareFrom, "from", "", "Versi sumber, contoh: v1 atau 1")
	skillCompareCmd.Flags().StringVar(&skillCompareTo, "to", "", "Versi tujuan, contoh: v2 atau 2")
	skillCompareCmd.Flags().StringVar(&skillCompareFormat, "format", "text", "Format output compare: text atau json")
	skillRollbackCmd.Flags().StringVar(&skillRollbackTo, "to", "", "Versi target rollback, contoh: v2 atau 2")
	skillInspectCmd.Flags().BoolVar(&skillInspectRisk, "risk", false, "Tampilkan risk assessment dan dry-run summary")
	skillInspectCmd.Flags().StringVar(&skillInspectFormat, "format", "text", "Format output inspect: text atau json")
	skillTreeCmd.Flags().StringVar(&skillTreeFormat, "format", "text", "Format output tree: text atau json/graph")
	skillStatsCmd.Flags().StringVar(&skillStatsFormat, "format", "text", "Format output stats: text atau json")
	skillStatsCmd.Flags().IntVar(&skillStatsLimit, "limit", 10, "Batas jumlah item untuk stats global")
	skillStatsCmd.Flags().BoolVar(&skillStatsAll, "all", false, "Tampilkan statistik semua skill")
	skillRunsCmd.Flags().StringVar(&skillStatsFormat, "format", "text", "Format output runs: text atau json")
	skillRunsCmd.Flags().IntVar(&skillStatsLimit, "limit", 20, "Batas jumlah run history")
	skillAnalyticsCmd.Flags().StringVar(&skillAnalyticsFormat, "format", "text", "Format output analytics: text atau json")
	skillRecommendCmd.Flags().StringVar(&skillRecommendFormat, "format", "text", "Format output recommend: text atau json")
	skillRecommendCmd.Flags().IntVar(&skillRecommendLimit, "limit", 5, "Batas jumlah rekomendasi")
	skillRecommendCmd.Flags().BoolVar(&skillRecommendNoHistory, "no-history", false, "Jangan gunakan run history untuk scoring")
	skillSuggestCmd.Flags().StringVar(&skillRecommendFormat, "format", "text", "Format output suggest: text atau json")
	skillSuggestCmd.Flags().IntVar(&skillRecommendLimit, "limit", 5, "Batas jumlah rekomendasi")
	skillSuggestCmd.Flags().BoolVar(&skillRecommendNoHistory, "no-history", false, "Jangan gunakan run history untuk scoring")
}
