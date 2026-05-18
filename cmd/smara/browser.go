package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	browseragent "github.com/gede-cahya/Smara-CLI/internal/browser"
)

var browserHeadful bool
var browserArtifactRoot string

var browserCmd = &cobra.Command{
	Use:   "browser",
	Short: "Jalankan Browser Subagent untuk screenshot dan testing UI",
}

var browserRunCmd = &cobra.Command{
	Use:   "run [prompt]",
	Short: "Jalankan prompt browser automation",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		prompt := strings.Join(args, " ")
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
		return nil
	},
}

func init() {
	browserRunCmd.Flags().BoolVar(&browserHeadful, "headful", false, "tampilkan browser secara visual")
	browserRunCmd.Flags().StringVar(&browserArtifactRoot, "artifacts", "", "folder output artifacts")
	browserCmd.AddCommand(browserRunCmd)
	rootCmd.AddCommand(browserCmd)
}
