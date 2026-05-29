package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/ui"
)

// ── Custom provider preset definitions ──────────────────────────────────────

// customPreset holds a built-in preset for common OpenAI-compatible providers.
type customPreset struct {
	Name        string
	URL         string
	Description string
	NeedsKey    bool   // false = key optional (e.g. local servers)
	DefaultModel string // suggested default model (empty = user must supply)
}

var customPresets = map[string]customPreset{
	"9router": {
		Name:        "9router",
		URL:         "http://localhost:20128/v1",
		Description: "9router local proxy / multi-provider router",
		NeedsKey:    true,
	},
	"lmstudio": {
		Name:         "LM Studio",
		URL:          "http://localhost:1234/v1",
		Description:  "LM Studio local inference server",
		NeedsKey:     false,
		DefaultModel: "local-model",
	},
	"ollama-api": {
		Name:         "Ollama (OpenAI-compat)",
		URL:          "http://localhost:11434/v1",
		Description:  "Ollama via OpenAI-compatible endpoint",
		NeedsKey:     false,
		DefaultModel: "llama3.1",
	},
	"vllm": {
		Name:         "vLLM",
		URL:          "http://localhost:8000/v1",
		Description:  "vLLM high-performance inference server",
		NeedsKey:     false,
		DefaultModel: "model",
	},
	"localai": {
		Name:         "LocalAI",
		URL:          "http://localhost:8080/v1",
		Description:  "LocalAI self-hosted inference",
		NeedsKey:     false,
		DefaultModel: "model",
	},
	"litellm": {
		Name:        "LiteLLM",
		URL:         "http://localhost:4000/v1",
		Description: "LiteLLM proxy — unified multi-provider gateway",
		NeedsKey:    true,
	},
	"openrouter": {
		Name:        "OpenRouter (custom)",
		URL:         "https://openrouter.ai/api/v1",
		Description: "OpenRouter multi-model gateway via custom provider",
		NeedsKey:    true,
	},
	"together": {
		Name:        "Together AI",
		URL:         "https://api.together.xyz/v1",
		Description: "Together AI cloud inference",
		NeedsKey:    true,
	},
	"groq": {
		Name:        "Groq Cloud",
		URL:         "https://api.groq.com/openai/v1",
		Description: "Groq ultra-fast cloud inference",
		NeedsKey:    true,
	},
}

// ── Flag variables for providerCustomCmd ─────────────────────────────────────

var (
	customFlagURL         string
	customFlagKey         string
	customFlagModel       string
	customFlagName        string
	customFlagPreset      string
	customFlagShow        bool
	customFlagListPresets bool
	customFlagNoStream    bool
	customFlagTest        bool
)

// ── Cobra commands ──────────────────────────────────────────────────────────

var providerCmd = &cobra.Command{
	Use:   "provider",
	Short: "Kelola provider LLM",
	Long:  "Lihat, ganti, dan test provider LLM yang tersedia.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProviderList()
	},
}

var providerSetCmd = &cobra.Command{
	Use:   "set <name>",
	Short: "Ganti provider aktif",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProviderSet(args[0])
	},
}

var providerSetModelCmd = &cobra.Command{
	Use:   "set-model <model>",
	Short: "Ganti model untuk provider aktif",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProviderSetModel(args[0])
	},
}

var providerTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Test koneksi provider aktif",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProviderTest()
	},
}

var providerListCmd = &cobra.Command{
	Use:     "list",
	Short:   "Tampilkan semua provider dan model yang tersedia",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProviderList()
	},
}

var providerSelectCmd = &cobra.Command{
	Use:   "select",
	Short: "Pilih provider dan model secara interaktif (TUI)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ui.ShowProviderSelector()
	},
}

var providerCustomCmd = &cobra.Command{
	Use:   "custom",
	Short: "Setup custom OpenAI-compatible provider dengan mudah",
	Long: `Setup custom provider dalam satu command.

Contoh:
  # One-liner — set semuanya sekaligus
  smara provider custom --url http://localhost:20128/v1 --key sk-xxx --model cx/gpt-5.5 --name 9router

  # Dengan preset — URL otomatis terisi
  smara provider custom --preset 9router --key sk-xxx --model cx/gpt-5.5

  # Preset lokal tanpa API key
  smara provider custom --preset lmstudio --model llama3

  # Lihat preset yang tersedia
  smara provider custom --list-presets

  # Lihat konfigurasi custom saat ini
  smara provider custom --show

  # Setup + langsung test koneksi
  smara provider custom --preset 9router --key sk-xxx --model cx/gpt-5.5 --test`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProviderCustom(cmd)
	},
}

func runProviderList() error {
	cfg := config.Get()
	providers := llm.AvailableProviders()

	fmt.Println()
	fmt.Println("🌀 Provider LLM Tersedia")
	fmt.Println(strings.Repeat("─", 60))

	providerNames := []string{"ollama", "openai", "openrouter", "anthropic", "custom"}
	for _, name := range providerNames {
		info := providers[name]
		current := ""
		if cfg.Provider == name {
			current = " ← aktif"
		}

		status := ""
		if info.NeedsAPIKey {
			key := getAPIKeyForProvider(name, cfg)
			if key != "" {
				status = "✓ " + maskKey(key)
			} else {
				status = "✗ belum login"
			}
		} else {
			status = "✓ siap (local)"
		}

		fmt.Printf("\n  %s (%s)%s\n", name, status, current)
		fmt.Printf("    %s\n", info.Description)
		fmt.Printf("    Model: %s\n", strings.Join(info.Models, ", "))
	}
	fmt.Println()
	return nil
}

func runProviderSet(name string) error {
	providers := llm.AvailableProviders()
	info, ok := providers[name]
	if !ok {
		return fmt.Errorf("provider tidak dikenali: %s (tersedia: %s)", name, strings.Join([]string{"ollama", "openai", "openrouter", "anthropic", "custom"}, ", "))
	}

	cfg := config.Get()

	// Check if API key is needed and present
	if info.NeedsAPIKey {
		key := getAPIKeyForProvider(name, cfg)
		if key == "" {
			return fmt.Errorf("provider '%s' memerlukan API key — jalankan 'smara login --provider %s'", name, name)
		}
	}

	if err := config.Set("provider", name); err != nil {
		return fmt.Errorf("gagal set provider: %w", err)
	}

	// Also set the model for this provider
	modelKey := modelConfigKey(name)
	if modelKey != "" {
		model := getProviderModel(name, cfg)
		if model != "" {
			config.Set("model", model)
		}
	}

	fmt.Printf("  ✓ Provider aktif: %s\n", name)
	return nil
}

func runProviderSetModel(model string) error {
	cfg := config.Get()
	provider := cfg.Provider

	// Set the model for the current provider
	if err := config.Set("model", model); err != nil {
		return fmt.Errorf("gagal set model: %w", err)
	}

	// Also update provider-specific model key
	modelKey := modelConfigKey(provider)
	if modelKey != "" {
		config.Set(modelKey, model)
	}

	fmt.Printf("  ✓ Model untuk %s: %s\n", provider, model)
	return nil
}

func runProviderTest() error {
	cfg := config.Get()

	// Build provider config
	providerCfg := llm.ProviderConfig{
		Name:            cfg.Provider,
		Model:           cfg.Model,
		Host:            cfg.OllamaHost,
		APIKey:          getAPIKeyForProvider(cfg.Provider, cfg),
		ReasoningEffort: cfg.ReasoningEffort,
	}

	// Get correct host for cloud providers
	switch cfg.Provider {
	case "openai":
		providerCfg.Host = ""
	case "openrouter":
		providerCfg.Host = ""
	case "anthropic":
		providerCfg.Host = ""
	case "custom":
		providerCfg.Host = cfg.CustomBaseURL
		providerCfg.APIKey = cfg.CustomAPIKey
	}

	provider, err := llm.NewProvider(providerCfg)
	if err != nil {
		return fmt.Errorf("gagal inisialisasi provider: %w", err)
	}

	fmt.Printf("Testing koneksi ke %s (%s)...\n", provider.Name(), cfg.Model)

	// Simple test message
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Reply with 'OK' if you can read this."},
	}

	resp, err := provider.Chat(messages)
	if err != nil {
		fmt.Printf("  ✗ Koneksi gagal: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("  ✓ Koneksi berhasil! (%s, %d tokens)\n", resp.Model, resp.TotalTokens)
	fmt.Printf("  Response: %s\n", strings.TrimSpace(resp.Content))
	return nil
}

func getProviderModel(name string, cfg *config.SmaraConfig) string {
	switch name {
	case "openai":
		return cfg.OpenAIModel
	case "openrouter":
		return cfg.OpenRouterModel
	case "anthropic":
		return cfg.AnthropicModel
	case "custom":
		return cfg.CustomModel
	case "ollama":
		return cfg.Model
	}
	return ""
}

func modelConfigKey(name string) string {
	switch name {
	case "openai":
		return "openai_model"
	case "openrouter":
		return "openrouter_model"
	case "anthropic":
		return "anthropic_model"
	case "custom":
		return "custom_model"
	}
	return ""
}

func init() {
	// Register flags for providerCustomCmd
	providerCustomCmd.Flags().StringVar(&customFlagURL, "url", "", "Base URL for the custom provider (e.g. http://localhost:20128/v1)")
	providerCustomCmd.Flags().StringVar(&customFlagKey, "key", "", "API key for the custom provider")
	providerCustomCmd.Flags().StringVar(&customFlagModel, "model", "", "Model name to use (e.g. cx/gpt-5.5)")
	providerCustomCmd.Flags().StringVar(&customFlagName, "name", "", "Display name for the custom provider")
	providerCustomCmd.Flags().StringVar(&customFlagPreset, "preset", "", "Use a built-in preset (e.g. 9router, lmstudio, groq)")
	providerCustomCmd.Flags().BoolVar(&customFlagShow, "show", false, "Show current custom provider configuration")
	providerCustomCmd.Flags().BoolVar(&customFlagListPresets, "list-presets", false, "List all available presets")
	providerCustomCmd.Flags().BoolVar(&customFlagNoStream, "no-stream", false, "Disable streaming for this provider")
	providerCustomCmd.Flags().BoolVar(&customFlagTest, "test", false, "Auto-test connection after setup")

	providerCmd.AddCommand(providerSetCmd, providerSetModelCmd, providerTestCmd, providerListCmd, providerSelectCmd, providerCustomCmd)
}

// ── runProviderCustom handles all modes of `smara provider custom` ──────────

func runProviderCustom(cmd *cobra.Command) error {
	// Mode 1: --show
	if customFlagShow {
		return runProviderCustomShow()
	}

	// Mode 2: --list-presets
	if customFlagListPresets {
		return runProviderCustomListPresets()
	}

	// Mode 3: Setup (one-liner or preset)
	return runProviderCustomSetup()
}

func runProviderCustomShow() error {
	cfg := config.Get()

	fmt.Println()
	name := cfg.CustomProviderName
	if name == "" {
		name = "custom"
	}
	fmt.Printf("🌀 Custom Provider: %s\n", name)
	fmt.Println(strings.Repeat("─", 42))

	// Name
	fmt.Printf("  %-10s: %s\n", "Name", name)

	// Base URL
	urlVal := cfg.CustomBaseURL
	if urlVal == "" {
		urlVal = "(belum diset)"
	}
	fmt.Printf("  %-10s: %s\n", "Base URL", urlVal)

	// API Key
	keyVal := cfg.CustomAPIKey
	if keyVal == "" {
		keyVal = "(belum diset)"
	} else {
		keyVal = maskKey(keyVal)
	}
	fmt.Printf("  %-10s: %s\n", "API Key", keyVal)

	// Model
	modelVal := cfg.CustomModel
	if modelVal == "" {
		modelVal = "(belum diset)"
	}
	fmt.Printf("  %-10s: %s\n", "Model", modelVal)

	// Stream
	streamVal := "enabled"
	if cfg.CustomDisableStream {
		streamVal = "disabled"
	}
	fmt.Printf("  %-10s: %s\n", "Stream", streamVal)

	// Active status
	if cfg.Provider == "custom" {
		fmt.Printf("  %-10s: ← aktif\n", "Status")
	} else {
		fmt.Printf("  %-10s: tidak aktif (provider aktif: %s)\n", "Status", cfg.Provider)
	}

	fmt.Println()
	return nil
}

func runProviderCustomListPresets() error {
	fmt.Println()
	fmt.Println("🌀 Preset Custom Provider")
	fmt.Println(strings.Repeat("─", 65))

	// Sort keys for consistent display
	keys := make([]string, 0, len(customPresets))
	for k := range customPresets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		p := customPresets[k]
		keyInfo := "🔑 key required"
		if !p.NeedsKey {
			keyInfo = "🔓 no key needed"
		}
		fmt.Printf("\n  %-14s %s\n", k, p.Description)
		fmt.Printf("  %-14s URL: %s  (%s)\n", "", p.URL, keyInfo)
		if p.DefaultModel != "" {
			fmt.Printf("  %-14s Default model: %s\n", "", p.DefaultModel)
		}
	}

	fmt.Println()
	fmt.Println("Gunakan: smara provider custom --preset <name> --key <key> --model <model>")
	fmt.Println()
	return nil
}

func runProviderCustomSetup() error {
	var (
		finalURL   = customFlagURL
		finalKey   = customFlagKey
		finalModel = customFlagModel
		finalName  = customFlagName
	)

	// If preset is specified, fill in defaults from the preset
	if customFlagPreset != "" {
		preset, ok := customPresets[strings.ToLower(customFlagPreset)]
		if !ok {
			available := make([]string, 0, len(customPresets))
			for k := range customPresets {
				available = append(available, k)
			}
			sort.Strings(available)
			return fmt.Errorf("preset tidak dikenali: %s\nTersedia: %s\nJalankan 'smara provider custom --list-presets' untuk detail",
				customFlagPreset, strings.Join(available, ", "))
		}

		// Use preset URL if --url not explicitly given
		if finalURL == "" {
			finalURL = preset.URL
		}
		// Use preset name if --name not explicitly given
		if finalName == "" {
			finalName = preset.Name
		}
		// Use preset default model if --model not given
		if finalModel == "" && preset.DefaultModel != "" {
			finalModel = preset.DefaultModel
		}
		// Validate key requirement
		if preset.NeedsKey && finalKey == "" {
			return fmt.Errorf("preset '%s' memerlukan API key — tambahkan --key <api-key>", customFlagPreset)
		}
		// For presets that don't need a key, use a dummy if empty
		if !preset.NeedsKey && finalKey == "" {
			finalKey = "no-key"
		}
	}

	// Validate required fields
	if finalURL == "" {
		return fmt.Errorf("base URL wajib diisi — tambahkan --url <url> atau gunakan --preset <name>\nJalankan 'smara provider custom --help' untuk contoh")
	}
	if finalModel == "" {
		return fmt.Errorf("model wajib diisi — tambahkan --model <model-name>")
	}
	if finalKey == "" {
		return fmt.Errorf("API key wajib diisi — tambahkan --key <api-key>\nUntuk server lokal tanpa key, gunakan --preset <name>")
	}

	// Auto-generate name from URL host if not provided
	if finalName == "" {
		// Extract a reasonable name from the URL
		name := finalURL
		name = strings.TrimPrefix(name, "http://")
		name = strings.TrimPrefix(name, "https://")
		if idx := strings.Index(name, "/"); idx > 0 {
			name = name[:idx]
		}
		if idx := strings.Index(name, ":"); idx > 0 {
			name = name[:idx]
		}
		finalName = name
	}

	// Save all settings
	settings := []struct {
		key, value string
	}{
		{"custom_provider_name", finalName},
		{"custom_base_url", finalURL},
		{"custom_api_key", finalKey},
		{"custom_model", finalModel},
		{"model", finalModel},
		{"provider", "custom"},
	}

	for _, s := range settings {
		if err := config.Set(s.key, s.value); err != nil {
			return fmt.Errorf("gagal menyimpan %s: %w", s.key, err)
		}
	}

	// Handle --no-stream
	if customFlagNoStream {
		if err := config.Set("custom_disable_stream", "true"); err != nil {
			return fmt.Errorf("gagal menyimpan custom_disable_stream: %w", err)
		}
	} else {
		// Explicitly enable streaming if not --no-stream
		if err := config.Set("custom_disable_stream", "false"); err != nil {
			return fmt.Errorf("gagal menyimpan custom_disable_stream: %w", err)
		}
	}

	// Print summary
	fmt.Println()
	fmt.Printf("🌀 Custom Provider: %s\n", finalName)
	fmt.Println(strings.Repeat("─", 42))
	fmt.Printf("  ✓ Name     : %s\n", finalName)
	fmt.Printf("  ✓ Base URL : %s\n", finalURL)
	fmt.Printf("  ✓ API Key  : %s\n", maskKey(finalKey))
	fmt.Printf("  ✓ Model    : %s\n", finalModel)
	if customFlagNoStream {
		fmt.Printf("  ✓ Stream   : disabled\n")
	} else {
		fmt.Printf("  ✓ Stream   : enabled\n")
	}
	fmt.Printf("  ✓ Provider aktif: custom (%s)\n", finalName)
	fmt.Println()

	// Handle --test
	if customFlagTest {
		return runProviderTest()
	}

	return nil
}
