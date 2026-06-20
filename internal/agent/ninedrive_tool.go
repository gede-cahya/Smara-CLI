package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/ninedrive"
)

func executeUploadTo9DriveTool(ctx context.Context, args map[string]interface{}, logCallback func(role, content string)) (string, error) {
	filePath, _ := args["file_path"].(string)
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return "", fmt.Errorf("argumen 'file_path' wajib diisi")
	}

	// Resolve relative path to absolute
	if !filepath.IsAbs(filePath) {
		cwd, _ := os.Getwd()
		filePath = filepath.Join(cwd, filePath)
	}

	// Verify file exists and is readable
	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("file tidak ditemukan atau tidak bisa diakses: %s (%w)", filePath, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("path adalah direktori, bukan file: %s", filePath)
	}

	mimeType, _ := args["mime_type"].(string)
	mimeType = strings.TrimSpace(mimeType)

	progress := func(event, message string, details map[string]interface{}) {
		emitBuiltinProgress(logCallback, "upload_to_9drive", event, message, details)
	}

	cfg := config.Get()
	if !cfg.NineDriveEnabled {
		return "", fmt.Errorf("9drive tidak aktif. Set ninedrive_enabled=true di config atau gunakan smara 9drive CLI")
	}

	apiKey := cfg.NineDriveAPIKey
	if apiKey == "" {
		return "", fmt.Errorf("9drive API key belum dikonfigurasi. Set ninedrive_api_key di config")
	}

	baseURL := cfg.NineDriveBaseURL
	if baseURL == "" {
		baseURL = "http://localhost:4000"
	}

	progress("upload_start", fmt.Sprintf("Mengupload %s (%d bytes)", filepath.Base(filePath), info.Size()), map[string]interface{}{
		"file_path": filePath,
		"file_size": info.Size(),
	})

	client := ninedrive.NewClient(baseURL, apiKey)
	result, err := client.UploadFile(ctx, filePath, mimeType)
	if err != nil {
		progress("upload_error", fmt.Sprintf("Upload gagal: %v", err), map[string]interface{}{"error": err.Error()})
		return "", fmt.Errorf("upload ke 9drive gagal: %w", err)
	}

	progress("upload_complete", fmt.Sprintf("Berhasil upload %s", filepath.Base(filePath)), map[string]interface{}{
		"file_path": filePath,
		"file_size": info.Size(),
		"response":  result.Raw,
	})

	return fmt.Sprintf("✓ Berhasil upload %s (%d bytes) ke 9drive\nResponse: %s",
		filepath.Base(filePath), info.Size(), result.Raw), nil
}
