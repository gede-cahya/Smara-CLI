package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/ninedrive"
	"github.com/spf13/cobra"
)

var (
	nineDriveAPIKey  string
	nineDriveBaseURL string
)

var nineDriveCmd = &cobra.Command{
	Use:   "9drive",
	Short: "9drive cloud storage integration",
}

var nineDriveUploadCmd = &cobra.Command{
	Use:   "upload [file...]",
	Short: "Upload files to 9drive",
	Long: `Upload one or more files to 9drive cloud storage.

Examples:
  smara 9drive upload photo.jpg
  smara 9drive upload *.png
  smara 9drive upload --api-key 9d_live_xxx backup.zip`,
	Args: cobra.MinimumNArgs(1),
	RunE: runNineDriveUpload,
}

func init() {
	nineDriveUploadCmd.Flags().StringVar(&nineDriveAPIKey, "api-key", "", "9drive API key (overrides config)")
	nineDriveUploadCmd.Flags().StringVar(&nineDriveBaseURL, "base-url", "", "9drive base URL (overrides config)")
	nineDriveCmd.AddCommand(nineDriveUploadCmd)
	rootCmd.AddCommand(nineDriveCmd)
}

func runNineDriveUpload(cmd *cobra.Command, args []string) error {
	cfg := config.Get()

	apiKey := nineDriveAPIKey
	if apiKey == "" {
		apiKey = cfg.NineDriveAPIKey
	}
	if apiKey == "" {
		return fmt.Errorf("9drive API key not set. Use --api-key or set ninedrive_api_key in config")
	}

	baseURL := nineDriveBaseURL
	if baseURL == "" {
		baseURL = cfg.NineDriveBaseURL
	}
	if baseURL == "" {
		baseURL = "http://localhost:4000"
	}

	client := ninedrive.NewClient(baseURL, apiKey)

	uploaded := 0
	failed := 0

	for _, pattern := range args {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "✗ Invalid pattern %q: %v\n", pattern, err)
			failed++
			continue
		}
		if len(matches) == 0 {
			fmt.Fprintf(os.Stderr, "✗ No files match %q\n", pattern)
			failed++
			continue
		}

		for _, path := range matches {
			info, err := os.Stat(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "✗ %s: %v\n", path, err)
				failed++
				continue
			}
			if info.IsDir() {
				fmt.Fprintf(os.Stderr, "⊘ %s: directory, skipping\n", path)
				continue
			}

			fmt.Printf("⬆ %s (%d bytes)... ", filepath.Base(path), info.Size())
			result, err := client.UploadFile(context.Background(), path, "")
			if err != nil {
				fmt.Printf("✗ FAILED: %v\n", err)
				failed++
				continue
			}
			fmt.Printf("✓ OK\n")
			if verbose {
				fmt.Printf("  Response: %s\n", result.Raw)
			}
			uploaded++
		}
	}

	fmt.Printf("\nSummary: %d uploaded, %d failed\n", uploaded, failed)
	if failed > 0 {
		return fmt.Errorf("%d uploads failed", failed)
	}
	return nil
}
