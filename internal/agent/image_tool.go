package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
)

func executeGenerateImageTool(ctx context.Context, args map[string]interface{}, logCallback func(role, content string)) (string, error) {
	prompt, _ := args["prompt"].(string)
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", fmt.Errorf("argumen 'prompt' tidak valid")
	}
	progress := func(event, message string, details map[string]interface{}) {
		emitBuiltinProgress(logCallback, "generate_image", event, message, details)
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
	progress("tool_progress", "Menyiapkan image provider.", map[string]interface{}{"provider": providerName, "model": model, "size": stringArg(args, "size", ""), "quality": stringArg(args, "quality", "")})

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

	opts := llm.ImageGenerationOptions{
		Model:          model,
		Size:           stringArg(args, "size", ""),
		Quality:        stringArg(args, "quality", ""),
		N:              intArg(args, "n", 1),
		ResponseFormat: "b64_json",
	}
	progress("tool_progress", "Mengirim request generate image ke provider.", map[string]interface{}{"provider": providerName, "model": model, "prompt_chars": len(prompt)})
	var result *llm.ImageGenerationResult
	if ctxGenerator, ok := provider.(llm.ImageGeneratorWithContext); ok {
		result, err = ctxGenerator.GenerateImageWithContext(ctx, prompt, opts)
	} else {
		result, err = generator.GenerateImage(prompt, opts)
	}
	if err != nil {
		return "", err
	}
	progress("tool_progress", "Response image diterima, menyiapkan file output.", map[string]interface{}{"model": result.Model, "mime": result.MIME, "bytes": len(result.Data)})

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
	progress("tool_verify", "Gambar berhasil disimpan.", map[string]interface{}{"path": outPath, "bytes": len(result.Data)})

	var b strings.Builder
	fmt.Fprintf(&b, "Gambar berhasil dibuat.\nPath: %s\nModel: %s\nMIME: %s\nMarkdown: ![generated image](%s)", outPath, result.Model, result.MIME, outPath)
	if result.RevisedPrompt != "" {
		fmt.Fprintf(&b, "\nRevised prompt: %s", result.RevisedPrompt)
	}
	return b.String(), nil
}

func executeEditImageTool(ctx context.Context, args map[string]interface{}, logCallback func(role, content string)) (string, error) {
	prompt, _ := args["prompt"].(string)
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", fmt.Errorf("argumen 'prompt' tidak valid")
	}
	progress := func(event, message string, details map[string]interface{}) {
		emitBuiltinProgress(logCallback, "edit_image", event, message, details)
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
	progress("tool_progress", "Image input siap diedit.", map[string]interface{}{"image_path": imagePath, "bytes": info.Size()})

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
	progress("tool_progress", "Menyiapkan image edit provider.", map[string]interface{}{"provider": providerName, "model": model, "size": stringArg(args, "size", ""), "quality": stringArg(args, "quality", "")})

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

	var result *llm.ImageGenerationResult
	for attempt := 1; attempt <= 3; attempt++ {
		progress("tool_progress", "Mengirim request edit image ke provider.", map[string]interface{}{"provider": providerName, "model": model, "attempt": attempt, "prompt_chars": len(prompt)})
		opts := llm.ImageEditOptions{
			Model:          model,
			Size:           stringArg(args, "size", ""),
			Quality:        stringArg(args, "quality", ""),
			N:              intArg(args, "n", 1),
			ResponseFormat: "b64_json",
			MaskPath:       stringArg(args, "mask_path", ""),
		}
		if ctxEditor, ok := provider.(llm.ImageEditorWithContext); ok {
			result, err = ctxEditor.EditImageWithContext(ctx, imagePath, prompt, opts)
		} else {
			result, err = editor.EditImage(imagePath, prompt, opts)
		}
		if err == nil {
			break
		}
		msg := strings.ToLower(err.Error())
		retryable := strings.Contains(msg, "status 408") || strings.Contains(msg, "status 429") || strings.Contains(msg, "status 500") || strings.Contains(msg, "status 502") || strings.Contains(msg, "status 503") || strings.Contains(msg, "status 504") || strings.Contains(msg, "stream disconnected") || strings.Contains(msg, "internal_server_error")
		if !retryable || attempt == 3 {
			break
		}
		progress("tool_progress", "Edit image gagal sementara, retry dengan backoff.", map[string]interface{}{"attempt": attempt, "error": err.Error()})
		time.Sleep(time.Duration(attempt*2) * time.Second)
	}
	if err != nil {
		return "", err
	}
	progress("tool_progress", "Response edit image diterima, menyiapkan file output.", map[string]interface{}{"model": result.Model, "mime": result.MIME, "bytes": len(result.Data)})

	if outPath == "" {
		outPath = defaultEditedImagePath(cfg.ImageOutputDir, result.Extension)
	}
	if filepath.Ext(outPath) == "" {
		outPath += result.Extension
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return "", fmt.Errorf("gagal membuat direktori output edit: %w", err)
	}
	if err := os.WriteFile(outPath, result.Data, 0o644); err != nil {
		return "", fmt.Errorf("gagal menyimpan gambar hasil edit: %w", err)
	}
	progress("tool_verify", "Gambar hasil edit berhasil disimpan.", map[string]interface{}{"path": outPath, "bytes": len(result.Data)})

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
		// Prefer dedicated image provider config; fall back to chat provider.
		providerCfg.Host = cfg.CustomBaseURL
		providerCfg.APIKey = cfg.CustomAPIKey
		if strings.TrimSpace(cfg.ImageBaseURL) != "" {
			providerCfg.Host = cfg.ImageBaseURL
		}
		if strings.TrimSpace(cfg.ImageAPIKey) != "" {
			providerCfg.APIKey = cfg.ImageAPIKey
		}
	case "openai":
		providerCfg.Host = cfg.OpenAIBaseURL
		providerCfg.APIKey = cfg.OpenAIAPIKey
		if strings.TrimSpace(cfg.ImageBaseURL) != "" {
			providerCfg.Host = cfg.ImageBaseURL
		}
		if strings.TrimSpace(cfg.ImageAPIKey) != "" {
			providerCfg.APIKey = cfg.ImageAPIKey
		}
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
