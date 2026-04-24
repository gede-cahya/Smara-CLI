package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/gede-cahya/Smara-CLI/internal/ui"
)

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Kelola ruang kerja (workspace) Smara",
	Long:  "Pisahkan memori, sesi, dan konteks antar proyek dengan workspace.",
}

var workspaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "Tampilkan semua workspace",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return err
		}
		defer store.Close()

		workspaces, err := store.ListWorkspaces()
		if err != nil {
			return err
		}

		active := cfg.ActiveWorkspace
		fmt.Println("\n  RUANG KERJA (WORKSPACES):")
		for _, w := range workspaces {
			prefix := "  "
			suffix := ""
			if w.Name == active {
				prefix = "👉"
				suffix = " (aktif)"
			}
			fmt.Printf("%s %-15s %s%s\n", prefix, w.Name, w.Path, suffix)
		}
		fmt.Println()
		return nil
	},
}

var workspaceCreateCmd = &cobra.Command{
	Use:   "create [nama]",
	Short: "Buat workspace baru",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		path, _ := os.Getwd()
		
		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return err
		}
		defer store.Close()

		w, err := store.CreateWorkspace(name, path)
		if err != nil {
			return fmt.Errorf("gagal membuat workspace: %w", err)
		}

		ui.PrintSuccess("Workspace '%s' berhasil dibuat di %s", w.Name, w.Path)
		return nil
	},
}

var workspaceUseCmd = &cobra.Command{
	Use:   "use [nama]",
	Short: "Ganti workspace aktif",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		
		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return err
		}
		defer store.Close()

		w, err := store.GetWorkspaceByName(name)
		if err != nil {
			return err
		}
		if w == nil {
			return fmt.Errorf("workspace '%s' tidak ditemukan", name)
		}

		if err := config.Set("active_workspace", name); err != nil {
			return err
		}

		ui.PrintSuccess("Sekarang menggunakan workspace: %s", name)
		return nil
	},
}

func init() {
	workspaceCmd.AddCommand(workspaceListCmd, workspaceCreateCmd, workspaceUseCmd)
}
