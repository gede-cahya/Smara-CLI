package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
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
	Short: "Cari memori berdasarkan query secara semantik",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := args[0]
		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("gagal membuka database: %w", err)
		}
		defer store.Close()

		// Use ollama with qwen3.6 model for embeddings
		fmt.Printf("  Membuat embedding untuk query: '%s'...\n", query)
		ollamaProvider := llm.NewOllamaProvider("qwen3.6", cfg.OllamaHost)
		embedding, err := ollamaProvider.GenerateEmbedding(query)
		if err != nil {
			return fmt.Errorf("gagal membuat embedding (pastikan Ollama berjalan dan model qwen3.6 tersedia): %w", err)
		}

		limit, _ := cmd.Flags().GetInt("limit")
		results, err := store.Search(embedding, limit)
		if err != nil {
			return fmt.Errorf("gagal mencari memori: %w", err)
		}

		if len(results) == 0 {
			fmt.Println("  Tidak ditemukan memori yang relevan.")
			return nil
		}

		fmt.Println()
		for _, r := range results {
			content := r.Memory.Content
			if len(content) > 100 {
				content = content[:100] + "..."
			}
			// Use ui colors if needed, but since ui package is not imported here, we use plain or basic terminal codes.
			fmt.Printf("  [%d] %s\n", r.Memory.ID, content)
			fmt.Printf("       relevansi: %.2f | tags=%s | source=%s | %s\n",
				r.Similarity, r.Memory.Tags, r.Memory.Source,
				r.Memory.CreatedAt.Format("2006-01-02 15:04"),
			)
		}
		fmt.Println()
		return nil
	},
}

func init() {
	memoryListCmd.Flags().IntP("limit", "n", 20, "jumlah memori yang ditampilkan")
	memoryCmd.AddCommand(memoryListCmd, memorySearchCmd, memoryClearCmd)
}
