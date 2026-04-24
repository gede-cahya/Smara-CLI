package main

import (
	"fmt"

	"github.com/gede-cahya/Smara-CLI/internal/ui"
	"github.com/spf13/cobra"
)

var exploreCmd = &cobra.Command{
	Use:   "explore [path]",
	Short: "Eksplorasi struktur proyek dan codebase",
	Long:  "Menampilkan struktur direktori dan file secara visual untuk memahami konteks proyek.",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}

		depth, _ := cmd.Flags().GetInt("depth")

		interactive, _ := cmd.Flags().GetBool("interactive")
		if interactive {
			return ui.RunExploreInteractive(path, depth)
		}

		results, err := ui.ExploreCodebase(path, depth)
		if err != nil {
			return fmt.Errorf("gagal mengeksplorasi codebase: %w", err)
		}

		fmt.Print(ui.RenderExplore(results))
		return nil
	},
}

func init() {
	exploreCmd.Flags().IntP("depth", "d", 2, "Kedalaman direktori yang akan ditampilkan")
	exploreCmd.Flags().BoolP("interactive", "i", false, "Buka mode eksplorasi interaktif (TUI)")
	rootCmd.AddCommand(exploreCmd)
}
