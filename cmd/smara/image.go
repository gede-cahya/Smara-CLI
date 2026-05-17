package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
)

var imageOpts struct {
	model    string
	out      string
	provider string
	size     string
	quality  string
	n        int
	baseURL  string
	apiKey   string
}

var imageCmd = &cobra.Command{
	Use:     "image [prompt]",
	Aliases: []string{"img", "generate-image"},
	Short:   "Generate gambar via OpenAI-compatible image API",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runImageGenerate(args[0])
	},
}

func runImageGenerate(prompt string) error {
	cfg := config.Get()
	providerName := imageOpts.provider
	if providerName == "" {
		providerName = cfg.Provider
	}
	model := imageOpts.model
	if model == "" {
		model = cfg.ImageModel
	}
	if model == "" {
		model = "gpt-image-2"
	}

	providerCfg, err := imageProviderConfig(providerName, model, cfg)
	if err != nil {
		return err
	}
	provider, err := llm.NewProvider(providerCfg)
	if err != nil {
		return fmt.Errorf("gagal inisialisasi image provider: %w", err)
	}
	generator, ok := provider.(llm.ImageGenerator)
	if !ok {
		return fmt.Errorf("provider %s belum mendukung image generation", providerName)
	}

	fmt.Printf("Generating image dengan %s (%s)...\n", providerName, model)
	result, err := generator.GenerateImage(prompt, llm.ImageGenerationOptions{
		Model:          model,
		Size:           imageOpts.size,
		Quality:        imageOpts.quality,
		N:              imageOpts.n,
		ResponseFormat: "b64_json",
	})
	if err != nil {
		return err
	}

	outPath := imageOpts.out
	if outPath == "" {
		outPath = defaultImageOutputPath(cfg.ImageOutputDir, result.Extension)
	}
	if filepath.Ext(outPath) == "" {
		outPath += result.Extension
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("gagal membuat direktori output: %w", err)
	}
	if err := os.WriteFile(outPath, result.Data, 0o644); err != nil {
		return fmt.Errorf("gagal menyimpan gambar: %w", err)
	}

	fmt.Printf("  ✓ Gambar tersimpan: %s\n", outPath)
	if result.RevisedPrompt != "" {
		fmt.Printf("  Revised prompt: %s\n", result.RevisedPrompt)
	}
	return nil
}

func imageProviderConfig(providerName, model string, cfg *config.SmaraConfig) (llm.ProviderConfig, error) {
	providerCfg := llm.ProviderConfig{Name: providerName, Model: model}
	switch providerName {
	case "custom":
		providerCfg.Host = cfg.CustomBaseURL
		providerCfg.APIKey = cfg.CustomAPIKey
	case "openai":
		providerCfg.Host = cfg.OpenAIBaseURL
		providerCfg.APIKey = cfg.OpenAIAPIKey
	default:
		return llm.ProviderConfig{}, fmt.Errorf("provider image yang didukung: custom, openai")
	}
	if imageOpts.baseURL != "" {
		providerCfg.Host = imageOpts.baseURL
	}
	if imageOpts.apiKey != "" {
		providerCfg.APIKey = imageOpts.apiKey
	}
	if strings.TrimSpace(providerCfg.Host) == "" && providerName == "custom" {
		return llm.ProviderConfig{}, fmt.Errorf("custom image provider memerlukan base URL")
	}
	return providerCfg, nil
}

func defaultImageOutputPath(outputDir, ext string) string {
	if outputDir == "" {
		outputDir = "."
	}
	if ext == "" {
		ext = ".png"
	}
	name := "smara-image-" + time.Now().Format("20060102-150405") + ext
	return filepath.Join(outputDir, name)
}

func init() {
	imageCmd.Flags().StringVar(&imageOpts.model, "model", "", "Model gambar (default: config image_model)")
	imageCmd.Flags().StringVarP(&imageOpts.out, "out", "o", "", "Path file output")
	imageCmd.Flags().StringVar(&imageOpts.provider, "provider", "", "Provider image: custom atau openai (default: provider aktif)")
	imageCmd.Flags().StringVar(&imageOpts.size, "size", "", "Ukuran gambar, misalnya 1024x1024")
	imageCmd.Flags().StringVar(&imageOpts.quality, "quality", "", "Kualitas gambar, misalnya low, medium, high, auto")
	imageCmd.Flags().IntVarP(&imageOpts.n, "n", "n", 1, "Jumlah gambar yang diminta")
	imageCmd.Flags().StringVar(&imageOpts.baseURL, "base-url", "", "Override base URL image API")
	imageCmd.Flags().StringVar(&imageOpts.apiKey, "api-key", "", "Override API key image API")
}
