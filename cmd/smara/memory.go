package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cahya/smara/internal/config"
	"github.com/cahya/smara/internal/memory"
)

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Kelola memori agen Smara",
	Long:  "Lihat, cari, dan bersihkan memori yang tersimpan di database.",
}

var memoryListCmd = &cobra.Command{
	Use:   "list",
	Short: "Tampilkan memori terbaru",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("gagal membuka database: %w", err)
		}
		defer store.Close()

		limit, _ := cmd.Flags().GetInt("limit")
		memories, err := store.List(limit)
		if err != nil {
			return fmt.Errorf("gagal membaca memori: %w", err)
		}

		if len(memories) == 0 {
			fmt.Println("  Belum ada memori tersimpan.")
			return nil
		}

		fmt.Println()
		for _, m := range memories {
			content := m.Content
			if len(content) > 100 {
				content = content[:100] + "..."
			}
			fmt.Printf("  [%d] %s\n", m.ID, content)
			fmt.Printf("       tags=%s source=%s  %s\n",
				m.Tags, m.Source,
				m.CreatedAt.Format("2006-01-02 15:04"),
			)
		}
		fmt.Println()
		return nil
	},
}

var memoryClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Hapus semua memori",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("gagal membuka database: %w", err)
		}
		defer store.Close()

		if err := store.Clear(); err != nil {
			return fmt.Errorf("gagal menghapus memori: %w", err)
		}

		fmt.Println("  ✓ Semua memori telah dihapus.")
		return nil
	},
}

var memorySearchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Cari memori berdasarkan query",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("gagal membuka database: %w", err)
		}
		defer store.Close()

		// For search, we need an LLM to generate embeddings
		fmt.Println("  ℹ Pencarian semantik memerlukan Ollama. Gunakan 'smara memory list' untuk listing biasa.")
		return nil
	},
}

func init() {
	memoryListCmd.Flags().IntP("limit", "n", 20, "jumlah memori yang ditampilkan")
	memoryCmd.AddCommand(memoryListCmd, memorySearchCmd, memoryClearCmd)
}
