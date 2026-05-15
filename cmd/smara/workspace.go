package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	cloud "github.com/gede-cahya/Smara-CLI/internal/memory/cloud"
	"github.com/gede-cahya/Smara-CLI/internal/ui"
)

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Kelola ruang kerja (workspace) Smara",
	Long:  "Pisahkan memori, sesi, dan konteks antar proyek dengan workspace.",
}

var workspaceListCmd = &cobra.Command{
	Use:     "list",
	Short:   "Tampilkan semua workspace",
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

var workspacePath string
var workspaceLocalOnly bool

var workspaceCreateCmd = &cobra.Command{
	Use:   "create [nama]",
	Short: "Buat workspace baru",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		path := workspacePath
		if path == "" {
			path, _ = os.Getwd()
		}

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

		if cfg.CloudMemory.Enabled && !workspaceLocalOnly {
			ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
			defer cancel()
			creds, err := cloud.NewCredentialStore().Load()
			if err != nil {
				return fmt.Errorf("workspace lokal dibuat, tapi provisioning cloud gagal: %w (gunakan --local-only untuk skip cloud)", err)
			}
			provider, err := cloud.Get(cfg.CloudMemory.Provider)
			if err != nil {
				return err
			}
			info, err := provider.EnsureDatabase(ctx, creds, name)
			if err != nil {
				return fmt.Errorf("workspace lokal dibuat, tapi database cloud gagal dibuat: %w (gunakan --local-only untuk skip cloud)", err)
			}
			if err := store.UpsertCloudDatabase(name, cfg.CloudMemory.Provider, info); err != nil {
				return err
			}
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
	workspaceCreateCmd.Flags().StringVar(&workspacePath, "path", "", "Path direktori workspace (default: cwd)")
	workspaceCreateCmd.Flags().BoolVar(&workspaceLocalOnly, "local-only", false, "skip provisioning database cloud")
}
