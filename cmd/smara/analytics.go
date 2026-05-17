package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/metrics"
)

var analyticsDays int

var analyticsCmd = &cobra.Command{
	Use:   "analytics",
	Short: "Analitik penggunaan token, cost, model, request, prompt, dan skill",
	Long: `Menampilkan analytics Smara CLI: total prompt/request, token input-output,
estimasi cost, model yang dipakai, grafik token harian, dan skill paling sering dipakai.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Get()
		path := metrics.DefaultAnalyticsPath(cfg.DBPath)
		// Keep legacy placement predictable even if DBPath is relative/empty.
		if cfg.DBPath == "" {
			path = filepath.Join(".", "usage_analytics.jsonl")
		}
		s, err := metrics.ReadAnalyticsSummary(path, cfg.DBPath, analyticsDays)
		if err != nil {
			return err
		}
		fmt.Print(metrics.FormatAnalyticsCLI(s))
		return nil
	},
}

func init() {
	analyticsCmd.Flags().IntVar(&analyticsDays, "days", 30, "range hari analytics")
}
