package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	browseragent "github.com/gede-cahya/Smara-CLI/internal/browser"
)

var browserHeadful bool
var browserArtifactRoot string
var browserURL string
var browserScreenshot bool
var browserSpec string

var browserCmd = &cobra.Command{
	Use:   "browser",
	Short: "Jalankan Browser Subagent untuk screenshot dan testing UI",
}

var browserRunCmd = &cobra.Command{
	Use:   "run [prompt]",
	Short: "Jalankan prompt browser automation",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		prompt := strings.Join(args, " ")
		if browserURL != "" {
			if prompt == "" {
				prompt = "Buka " + browserURL
			} else if !strings.Contains(prompt, browserURL) {
				prompt = prompt + " " + browserURL
			}
			if browserScreenshot && !strings.Contains(strings.ToLower(prompt), "screenshot") {
				prompt += " dan ambil screenshot"
			}
		}
		if prompt == "" {
			return fmt.Errorf("prompt atau --url wajib diisi")
		}
		return runBrowserPrompt(prompt)
	},
}

var browserE2ECmd = &cobra.Command{
	Use:   "e2e --spec browser-task.md",
	Short: "Jalankan skenario E2E dari file spec Markdown/plain text",
	RunE: func(cmd *cobra.Command, args []string) error {
		if browserSpec == "" {
			return fmt.Errorf("--spec wajib diisi")
		}
		b, err := os.ReadFile(browserSpec)
		if err != nil {
			return err
		}
		return runBrowserPrompt(string(b))
	},
}

func runBrowserPrompt(prompt string) error {
	task, err := browseragent.Plan(prompt)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	res, err := browseragent.Run(ctx, task, browseragent.Options{ArtifactRoot: browserArtifactRoot, Headful: browserHeadful})
	if err != nil {
		fmt.Printf("Browser Subagent gagal: %v\n", err)
		if res.ReportPath != "" {
			fmt.Printf("Report: %s\n", res.ReportPath)
		}
		return err
	}
	fmt.Printf("Browser Subagent selesai: %s\n", res.Status)
	fmt.Printf("Artifact dir: %s\n", res.ArtifactDir)
	fmt.Printf("Screenshot: %s\n", res.ScreenshotPath)
	fmt.Printf("Report: %s\n", res.ReportPath)
	if res.RunJSONPath != "" {
		fmt.Printf("Metadata: %s\n", res.RunJSONPath)
	}
	return nil
}

func init() {
	browserRunCmd.Flags().BoolVar(&browserHeadful, "headful", false, "tampilkan browser secara visual")
	browserRunCmd.Flags().StringVar(&browserArtifactRoot, "artifacts", "", "folder output artifacts")
	browserRunCmd.Flags().StringVar(&browserURL, "url", "", "URL target browser subagent")
	browserRunCmd.Flags().BoolVar(&browserScreenshot, "screenshot", false, "ambil screenshot halaman target")
	browserE2ECmd.Flags().BoolVar(&browserHeadful, "headful", false, "tampilkan browser secara visual")
	browserE2ECmd.Flags().StringVar(&browserArtifactRoot, "artifacts", "", "folder output artifacts")
	browserE2ECmd.Flags().StringVar(&browserSpec, "spec", "", "path file spec E2E Markdown/plain text")
	browserCmd.AddCommand(browserRunCmd)
	browserCmd.AddCommand(browserE2ECmd)
	rootCmd.AddCommand(browserCmd)
}
