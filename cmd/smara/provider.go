package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cahya/smara/internal/config"
	"github.com/cahya/smara/internal/llm"
	"github.com/cahya/smara/internal/ui"
)

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
		Name:   cfg.Provider,
		Model:  cfg.Model,
		Host:   cfg.OllamaHost,
		APIKey: getAPIKeyForProvider(cfg.Provider, cfg),
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
	providerCmd.AddCommand(providerSetCmd, providerSetModelCmd, providerTestCmd, providerListCmd, providerSelectCmd)
}
