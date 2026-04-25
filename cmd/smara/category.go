package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/gede-cahya/Smara-CLI/internal/ui"
)

var categoryCmd = &cobra.Command{
	Use:   "category",
	Short: "Kelola kategori memori",
	Long:  "Buat, daftar, dan kelola kategori untuk mengorganisir memori dalam workspace.",
}

var categoryListCmd = &cobra.Command{
	Use:   "list",
	Short: "Tampilkan daftar kategori",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("gagal membuka database: %w", err)
		}
		defer store.Close()

		all, _ := cmd.Flags().GetBool("all")
		
		var categories []memory.Category
		
		if all {
			categories, err = store.ListCategories(cfg.ActiveWorkspaceID, true)
		} else {
			categories, err = store.ListCategories(cfg.ActiveWorkspaceID, false)
		}
		
		if err != nil {
			return fmt.Errorf("gagal membaca kategori: %w", err)
		}

		if len(categories) == 0 {
			fmt.Println("  Belum ada kategori.")
			return nil
		}

		fmt.Println()
		fmt.Printf("  Kategori di workspace '%s' (%d):\n\n", cfg.ActiveWorkspace, len(categories))
		
		for _, cat := range categories {
			prefix := "  "
			if cat.ParentID != nil {
				prefix = "    ↳ "
			}
			fmt.Printf("%s[%d] %s\n", prefix, cat.ID, cat.Name)
			if cat.Description != "" {
				fmt.Printf("       %s\n", cat.Description)
			}
			fmt.Printf("       Dibuat: %s\n", cat.CreatedAt.Format("2006-01-02 15:04"))
			fmt.Println()
		}
		
		return nil
	},
}

var categoryCreateCmd = &cobra.Command{
	Use:   "create [nama]",
	Short: "Buat kategori baru",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		description, _ := cmd.Flags().GetString("description")
		parentStr, _ := cmd.Flags().GetString("parent")

		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("gagal membuka database: %w", err)
		}
		defer store.Close()

		var parentID *int64
		if parentStr != "" {
			if pid, err := strconv.ParseInt(parentStr, 10, 64); err == nil {
				parentID = &pid
			}
		}

		newCat := &memory.Category{
			Name:        name,
			Description: description,
			WorkspaceID: cfg.ActiveWorkspaceID,
			ParentID:    parentID,
		}

		cat, err := store.CreateCategory(newCat.Name, newCat.Description, newCat.WorkspaceID, newCat.ParentID)
		if err != nil {
			return fmt.Errorf("gagal membuat kategori: %w", err)
		}

		ui.PrintSuccess("  ✓ Kategori '%s' berhasil dibuat (ID: %d)", cat.Name, cat.ID)
		return nil
	},
}

var categoryGetCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Tampilkan detail kategori",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("ID kategori tidak valid: %w", err)
		}

		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("gagal membuka database: %w", err)
		}
		defer store.Close()

		cat, err := store.GetCategory(id)
		if err != nil {
			return fmt.Errorf("gagal membaca kategori: %w", err)
		}
		if cat == nil {
			return fmt.Errorf("kategori #%d tidak ditemukan", id)
		}

		fmt.Println()
		fmt.Printf("  ID:          %d\n", cat.ID)
		fmt.Printf("  Workspace:   %d\n", cat.WorkspaceID)
		fmt.Printf("  Nama:        %s\n", cat.Name)
		if cat.Description != "" {
			fmt.Printf("  Deskripsi:   %s\n", cat.Description)
		}
		if cat.ParentID != nil {
			fmt.Printf("  Parent ID:   %d\n", *cat.ParentID)
		}
		fmt.Printf("  Dibuat:      %s\n", cat.CreatedAt.Format("2006-01-02 15:04"))
		fmt.Println()
		
		// Show memory count
		stats, err := store.GetCategoryStats(id)
		if err == nil {
			fmt.Printf("  Statistik:\n")
			fmt.Printf("    - Jumlah memori: %d\n", stats["memory_count"])
			fmt.Printf("    - Subkategori:   %d\n", stats["subcategory_count"])
		}
		
		return nil
	},
}

var categoryUpdateCmd = &cobra.Command{
	Use:   "update [id]",
	Short: "Update kategori",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("ID kategori tidak valid: %w", err)
		}

		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("gagal membuka database: %w", err)
		}
		defer store.Close()

		updates := make(map[string]interface{})

		if name, _ := cmd.Flags().GetString("name"); name != "" {
			updates["name"] = name
		}
		if desc, _ := cmd.Flags().GetString("description"); desc != "" {
			updates["description"] = desc
		}
		if parentStr, _ := cmd.Flags().GetString("parent"); parentStr != "" {
			if pid, err := strconv.ParseInt(parentStr, 10, 64); err == nil {
				updates["parent_id"] = &pid
			}
		}

		if len(updates) == 0 {
			return fmt.Errorf("tidak ada field yang diupdate")
		}

		if err := store.UpdateCategory(id, updates); err != nil {
			return fmt.Errorf("gagal update kategori: %w", err)
		}

		ui.PrintSuccess("  ✓ Kategori #%d berhasil diupdate", id)
		return nil
	},
}

var categoryDeleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Hapus kategori",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("ID kategori tidak valid: %w", err)
		}

		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("gagal membuka database: %w", err)
		}
		defer store.Close()

		reassign, _ := cmd.Flags().GetInt64("reassign-to")
		var reassignID *int64
		if reassign > 0 {
			reassignID = &reassign
		}

		if err := store.DeleteCategory(id, reassignID); err != nil {
			return fmt.Errorf("gagal hapus kategori: %w", err)
		}

		ui.PrintSuccess("  ✓ Kategori #%d berhasil dihapus", id)
		if reassignID != nil {
			fmt.Printf("    Memori dialihkan ke kategori #%d\n", *reassignID)
		}
		return nil
	},
}

var categoryStatsCmd = &cobra.Command{
	Use:   "stats [id]",
	Short: "Tampilkan statistik kategori",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("ID kategori tidak valid: %w", err)
		}

		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("gagal membuka database: %w", err)
		}
		defer store.Close()

		stats, err := store.GetCategoryStats(id)
		if err != nil {
			return fmt.Errorf("gagal dapatkan statistik: %w", err)
		}

		fmt.Println()
		fmt.Printf("  Statistik Kategori #%d:\n\n", id)
		fmt.Printf("    Memori:       %d\n", stats["memory_count"])
		fmt.Printf("    Subkategori:  %d\n", stats["subcategory_count"])
		fmt.Println()
		
		return nil
	},
}

func init() {
	categoryListCmd.Flags().Bool("all", false, "tampilkan semua kategori termasuk subkategori")
	
	categoryCreateCmd.Flags().String("description", "", "deskripsi kategori")
	categoryCreateCmd.Flags().String("parent", "", "ID kategori parent")
	
	categoryUpdateCmd.Flags().String("name", "", "nama baru")
	categoryUpdateCmd.Flags().String("description", "", "deskripsi baru")
	categoryUpdateCmd.Flags().String("parent", "", "ID parent baru")
	
	categoryDeleteCmd.Flags().Int64("reassign-to", 0, "pindahkan memori ke kategori ini")

	categoryCmd.AddCommand(
		categoryListCmd,
		categoryCreateCmd,
		categoryGetCmd,
		categoryUpdateCmd,
		categoryDeleteCmd,
		categoryStatsCmd,
	)
}
