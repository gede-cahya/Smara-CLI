package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/pkg/config"
	"github.com/gede-cahya/Smara-CLI/pkg/llm"
)

func executeGenerateImageTool(args map[string]interface{}) (string, error) {
	prompt, _ := args["prompt"].(string)
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", fmt.Errorf("argumen 'prompt' tidak valid")
	}

	cfg := config.Get()
	model := stringArg(args, "model", cfg.ImageModel)
	if model == "" {
		model = "gpt-image-2"
	}
	providerName := stringArg(args, "provider", cfg.Provider)
	if providerName == "" {
		providerName = "custom"
	}
	outPath := stringArg(args, "output_path", "")
	if outPath == "" {
		outPath = stringArg(args, "out", "")
	}

	providerCfg, err := imageToolProviderConfig(providerName, model, cfg, args)
	if err != nil {
		return "", err
	}
	provider, err := llm.NewProvider(providerCfg)
	if err != nil {
		return "", fmt.Errorf("gagal inisialisasi image provider: %w", err)
	}
	generator, ok := provider.(llm.ImageGenerator)
	if !ok {
		return "", fmt.Errorf("provider %s belum mendukung image generation", providerName)
	}

	result, err := generator.GenerateImage(prompt, llm.ImageGenerationOptions{
		Model:          model,
		Size:           stringArg(args, "size", ""),
		Quality:        stringArg(args, "quality", ""),
		N:              intArg(args, "n", 1),
		ResponseFormat: "b64_json",
	})
	if err != nil {
		return "", err
	}

	if outPath == "" {
		outPath = defaultGeneratedImagePath(cfg.ImageOutputDir, result.Extension)
	}
	if filepath.Ext(outPath) == "" {
		outPath += result.Extension
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return "", fmt.Errorf("gagal membuat direktori output: %w", err)
	}
	if err := os.WriteFile(outPath, result.Data, 0o644); err != nil {
		return "", fmt.Errorf("gagal menyimpan gambar: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Gambar berhasil dibuat.\nPath: %s\nModel: %s\nMIME: %s\nMarkdown: ![generated image](%s)", outPath, result.Model, result.MIME, outPath)
	if result.RevisedPrompt != "" {
		fmt.Fprintf(&b, "\nRevised prompt: %s", result.RevisedPrompt)
	}
	return b.String(), nil
}

func executeEditImageTool(args map[string]interface{}) (string, error) {
	prompt, _ := args["prompt"].(string)
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", fmt.Errorf("argumen 'prompt' tidak valid")
	}
	imagePath := stringArg(args, "image_path", "")
	if imagePath == "" {
		imagePath = stringArg(args, "input_image_path", "")
	}
	if imagePath == "" {
		return "", fmt.Errorf("argumen 'image_path' wajib diisi")
	}
	info, err := os.Stat(imagePath)
	if err != nil {
		return "", fmt.Errorf("image_path tidak bisa diakses: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("image_path harus berupa file, bukan direktori")
	}

	cfg := config.Get()
	model := stringArg(args, "model", cfg.ImageModel)
	if model == "" {
		model = "gpt-image-2"
	}
	providerName := stringArg(args, "provider", cfg.Provider)
	if providerName == "" {
		providerName = "custom"
	}
	outPath := stringArg(args, "output_path", "")
	if outPath == "" {
		outPath = stringArg(args, "out", "")
	}

	providerCfg, err := imageToolProviderConfig(providerName, model, cfg, args)
	if err != nil {
		return "", err
	}
	provider, err := llm.NewProvider(providerCfg)
	if err != nil {
		return "", fmt.Errorf("gagal inisialisasi image edit provider: %w", err)
	}
	editor, ok := provider.(llm.ImageEditor)
	if !ok {
		return "", fmt.Errorf("provider %s belum mendukung image editing / image-to-image", providerName)
	}

	result, err := editor.EditImage(imagePath, prompt, llm.ImageEditOptions{
		Model:          model,
		Size:           stringArg(args, "size", ""),
		Quality:        stringArg(args, "quality", ""),
		N:              intArg(args, "n", 1),
		ResponseFormat: "b64_json",
	})
	if err != nil {
		return "", err
	}

	if outPath == "" {
		outPath = defaultEditedImagePath(cfg.ImageOutputDir, result.Extension)
	}
	if filepath.Ext(outPath) == "" {
		outPath += result.Extension
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return "", fmt.Errorf("gagal membuat direktori output: %w", err)
	}
	if err := os.WriteFile(outPath, result.Data, 0o644); err != nil {
		return "", fmt.Errorf("gagal menyimpan gambar hasil edit: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Gambar berhasil diedit.\nInput: %s\nPath: %s\nModel: %s\nMIME: %s\nMarkdown: ![edited image](%s)", imagePath, outPath, result.Model, result.MIME, outPath)
	if result.RevisedPrompt != "" {
		fmt.Fprintf(&b, "\nRevised prompt: %s", result.RevisedPrompt)
	}
	return b.String(), nil
}

func imageToolProviderConfig(providerName, model string, cfg *config.SmaraConfig, args map[string]interface{}) (llm.ProviderConfig, error) {
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
	if baseURL := stringArg(args, "base_url", ""); baseURL != "" {
		providerCfg.Host = baseURL
	}
	if apiKey := stringArg(args, "api_key", ""); apiKey != "" {
		providerCfg.APIKey = apiKey
	}
	if strings.TrimSpace(providerCfg.Host) == "" && providerName == "custom" {
		return llm.ProviderConfig{}, fmt.Errorf("custom image provider memerlukan base URL")
	}
	return providerCfg, nil
}

func defaultEditedImagePath(outputDir, ext string) string {
	if outputDir == "" {
		outputDir = "."
	}
	if ext == "" {
		ext = ".png"
	}
	return filepath.Join(outputDir, "smara-image-edit-"+time.Now().Format("20060102-150405")+ext)
}

func defaultGeneratedImagePath(outputDir, ext string) string {
	if outputDir == "" {
		outputDir = "."
	}
	if ext == "" {
		ext = ".png"
	}
	return filepath.Join(outputDir, "smara-image-"+time.Now().Format("20060102-150405")+ext)
}

func stringArg(args map[string]interface{}, key, fallback string) string {
	if v, ok := args[key].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func intArg(args map[string]interface{}, key string, fallback int) int {
	switch v := args[key].(type) {
	case int:
		if v > 0 {
			return v
		}
	case float64:
		if v > 0 {
			return int(v)
		}
	}
	return fallback
}
