package main

import (
	"fmt"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/dashboard"
)

var (
	dashOnce    bool
	dashRefresh string
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Tampilkan dashboard monitoring Smara",
	Long: `Membuka dashboard TUI real-time untuk memantau status platform bot,
penggunaan LLM, MCP servers, sessions, memory, dan error.

Dashboard membaca metrik dari smara serve yang sedang berjalan.
Jika serve tidak aktif, menampilkan data tersimpan dari database.

Contoh:
  smara dashboard                  # buka TUI interaktif
  smara dashboard --once           # snapshot sekali (non-interaktif)
  smara dashboard --refresh 5s     # custom refresh interval`,
	RunE: runDashboard,
}

func init() {
	dashboardCmd.Flags().BoolVar(&dashOnce, "once", false, "tampilkan snapshot sekali dan keluar")
	dashboardCmd.Flags().StringVar(&dashRefresh, "refresh", "2s", "interval refresh data (e.g. 2s, 5s, 10s)")
}

func runDashboard(cmd *cobra.Command, args []string) error {
	cfg := config.Get()

	smaraDir := filepath.Dir(cfg.DBPath)
	metricsPath := filepath.Join(smaraDir, "metrics.json")

	// Non-interactive mode
	if dashOnce {
		output := dashboard.RenderOnce(metricsPath, cfg.DBPath, version)
		fmt.Println(output)
		return nil
	}

	// Parse refresh interval
	interval, err := time.ParseDuration(dashRefresh)
	if err != nil {
		interval = 2 * time.Second
	}
	if interval < 500*time.Millisecond {
		interval = 500 * time.Millisecond
	}

	// Start interactive TUI
	model := dashboard.NewDashboardModel(metricsPath, cfg.DBPath, version, interval)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error menjalankan dashboard: %w", err)
	}

	return nil
}
