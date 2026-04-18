package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login ke provider LLM (OpenAI, OpenRouter, Anthropic)",
	Long: `Simpan API key untuk provider LLM cloud.

Contoh:
  smara login                          # wizard interaktif
  smara login --provider openai        # login ke OpenAI
  smara login --provider openrouter    # login ke OpenRouter
  smara login --provider anthropic     # login ke Anthropic`,
	RunE: runLogin,
}

var loginProviderFlag string
var loginKeyFlag string
var loginCustomFlag bool

func runLogin(cmd *cobra.Command, args []string) error {
	cfg := config.Get()

	// If no provider specified, show current status and prompt
	if loginProviderFlag == "" {
		fmt.Println()
		fmt.Println("🌀 Smara LLM Providers")
		fmt.Println(strings.Repeat("─", 50))

		providers := llm.AvailableProviders()
		providerNames := []string{"ollama", "openai", "openrouter", "anthropic", "custom"}

		for _, name := range providerNames {
			info := providers[name]
			status := "✗ belum"
			if !info.NeedsAPIKey {
				status = "✓ siap"
			} else {
				key := getAPIKeyForProvider(name, cfg)
				if key != "" {
					masked := maskKey(key)
					status = "✓ " + masked
				}
			}

			current := ""
			if cfg.Provider == name {
				current = " ← aktif"
			}

			fmt.Printf("  %-12s %s%s\n", name, status, current)
		}
		fmt.Println()
		fmt.Println("Gunakan: smara login --provider <name> untuk login")
		fmt.Println("         smara provider set <name> untuk mengganti provider")
		fmt.Println()
		return nil
	}

	// Validate provider name
	providers := llm.AvailableProviders()
	info, ok := providers[loginProviderFlag]
	if !ok {
		return fmt.Errorf("provider tidak dikenali: %s (tersedia: ollama, openai, openrouter, anthropic, custom)", loginProviderFlag)
	}

	if !info.NeedsAPIKey {
		return fmt.Errorf("provider '%s' tidak memerlukan API key", loginProviderFlag)
	}

	// Handle custom provider specially
	if loginProviderFlag == "custom" {
		return runLoginCustom()
	}

	// Get API key
	apiKey := loginKeyFlag
	if apiKey == "" {
		// Interactive prompt
		fmt.Printf("Masukkan API key untuk %s: ", info.Name)
		fmt.Scanln(&apiKey)
		apiKey = strings.TrimSpace(apiKey)
	}

	if apiKey == "" {
		return fmt.Errorf("API key tidak boleh kosong")
	}

	// Save to config
	keyName := apiKeyConfigKey(loginProviderFlag)
	if err := config.Set(keyName, apiKey); err != nil {
		return fmt.Errorf("gagal menyimpan API key: %w", err)
	}

	fmt.Printf("  ✓ API key untuk %s berhasil disimpan\n", info.Name)

	// Auto-switch to this provider if none set
	if cfg.Provider == "ollama" {
		if err := config.Set("provider", loginProviderFlag); err != nil {
			return fmt.Errorf("gagal set provider: %w", err)
		}
		fmt.Printf("  ✓ Provider aktif diganti ke %s\n", loginProviderFlag)
	}

	return nil
}

func getAPIKeyForProvider(name string, cfg *config.SmaraConfig) string {
	switch name {
	case "openai":
		return cfg.OpenAIAPIKey
	case "openrouter":
		return cfg.OpenRouterAPIKey
	case "anthropic":
		return cfg.AnthropicAPIKey
	}
	return ""
}

func apiKeyConfigKey(name string) string {
	switch name {
	case "openai":
		return "openai_api_key"
	case "openrouter":
		return "openrouter_api_key"
	case "anthropic":
		return "anthropic_api_key"
	}
	return ""
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return "****" + key[len(key)-2:]
	}
	return key[:4] + "..." + key[len(key)-4:]
}

func runLoginCustom() error {
	cfg := config.Get()

	fmt.Println()
	fmt.Println("🌀 Custom Provider Setup")
	fmt.Println(strings.Repeat("─", 40))

	// Get provider name
	providerName := cfg.CustomProviderName
	if providerName == "" {
		fmt.Print("Nama provider (misal: my-ollama, local-ai): ")
		fmt.Scanln(&providerName)
		providerName = strings.TrimSpace(providerName)
	}
	if providerName == "" {
		return fmt.Errorf("nama provider tidak boleh kosong")
	}

	// Get base URL
	baseURL := cfg.CustomBaseURL
	if baseURL == "" {
		fmt.Print("Base URL (misal: http://localhost:11434/v1): ")
		fmt.Scanln(&baseURL)
		baseURL = strings.TrimSpace(baseURL)
	}
	if baseURL == "" {
		return fmt.Errorf("base URL tidak boleh kosong")
	}

	// Get API key
	apiKey := loginKeyFlag
	if apiKey == "" {
		fmt.Print("API Key: ")
		fmt.Scanln(&apiKey)
		apiKey = strings.TrimSpace(apiKey)
	}
	if apiKey == "" {
		return fmt.Errorf("API key tidak boleh kosong")
	}

	// Get model
	model := cfg.CustomModel
	if model == "" {
		fmt.Print("Model name (misal: gpt-4o, llama3): ")
		fmt.Scanln(&model)
		model = strings.TrimSpace(model)
	}
	if model == "" {
		return fmt.Errorf("model tidak boleh kosong")
	}

	// Save all to config
	if err := config.Set("custom_provider_name", providerName); err != nil {
		return fmt.Errorf("gagal menyimpan nama provider: %w", err)
	}
	if err := config.Set("custom_base_url", baseURL); err != nil {
		return fmt.Errorf("gagal menyimpan base URL: %w", err)
	}
	if err := config.Set("custom_api_key", apiKey); err != nil {
		return fmt.Errorf("gagal menyimpan API key: %w", err)
	}
	if err := config.Set("custom_model", model); err != nil {
		return fmt.Errorf("gagal menyimpan model: %w", err)
	}

	// Set as active provider
	if err := config.Set("provider", "custom"); err != nil {
		return fmt.Errorf("gagal set provider: %w", err)
	}

	fmt.Println()
	fmt.Printf("  ✓ Custom provider '%s' configured\n", providerName)
	fmt.Printf("  ✓ Base URL: %s\n", baseURL)
	fmt.Printf("  ✓ Model: %s\n", model)
	fmt.Printf("  ✓ Provider aktif: custom (%s)\n", providerName)
	fmt.Println()

	return nil
}

func init() {
	loginCmd.Flags().StringVar(&loginProviderFlag, "provider", "", "Provider tujuan (openai, openrouter, anthropic, custom)")
	loginCmd.Flags().StringVar(&loginKeyFlag, "key", "", "API key langsung (non-interaktif)")
	loginCmd.Flags().BoolVar(&loginCustomFlag, "custom", false, "Setup custom provider")
}
