package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/gede-cahya/Smara-CLI/internal/ui"
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
		wsID := cfg.ActiveWorkspaceID
		
		// Get filter flags
		tagsStr, _ := cmd.Flags().GetString("tags")
		source, _ := cmd.Flags().GetString("source")
		categoryStr, _ := cmd.Flags().GetString("category")
		dateFromStr, _ := cmd.Flags().GetString("date-from")
		dateToStr, _ := cmd.Flags().GetString("date-to")
		sortBy, _ := cmd.Flags().GetString("sort-by")
		sortDir, _ := cmd.Flags().GetString("sort-dir")

		// Build filters
		var filters memory.MemoryFilters
		filters.Limit = limit
		filters.SortBy = sortBy
		filters.SortDir = sortDir

		if tagsStr != "" {
			filters.Tags = strings.Split(tagsStr, ",")
		}
		if source != "" {
			filters.Sources = []string{source}
		}
		if categoryStr != "" {
			if catID, err := strconv.ParseInt(categoryStr, 10, 64); err == nil {
				filters.CategoryID = &catID
			}
		}
		if dateFromStr != "" {
			if date, err := time.Parse("2006-01-02", dateFromStr); err == nil {
				filters.DateFrom = &date
			}
		}
		if dateToStr != "" {
			if date, err := time.Parse("2006-01-02", dateToStr); err == nil {
				filters.DateTo = &date
			}
		}

		memories, total, err := store.ListMemoriesWithFilters(wsID, filters)
		if err != nil {
			return fmt.Errorf("gagal membaca memori: %w", err)
		}

		if len(memories) == 0 {
			fmt.Println("  Belum ada memori tersimpan.")
			return nil
		}

		fmt.Println()
		fmt.Printf("  Total: %d memori (menampilkan %d)\n", total, len(memories))
		fmt.Println()
		for _, m := range memories {
			content := m.Content
			if len(content) > 100 {
				content = content[:100] + "..."
			}
			fmt.Printf("  [%d] %s\n", m.ID, content)
			
			tagsStr := "-"
			if len(m.Tags) > 0 {
				tagsStr = strings.Join(m.Tags, ", ")
			}
			
			info := fmt.Sprintf("tags=%s source=%s  %s",
				tagsStr, m.Source,
				m.CreatedAt.Format("2006-01-02 15:04"),
			)
			
			if m.CategoryID != nil {
				info = fmt.Sprintf("cat=%d %s", *m.CategoryID, info)
			}
			if m.ExpiresAt != nil {
				info = fmt.Sprintf("expires=%s %s", m.ExpiresAt.Format("2006-01-02"), info)
			}
			
			fmt.Printf("       %s\n", info)
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

		ui.PrintSuccess("  ✓ Semua memori telah dihapus.")
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
		wsID := cfg.ActiveWorkspaceID
		
		// Check if hybrid search is requested
		hybrid, _ := cmd.Flags().GetBool("hybrid")
		
		var results []memory.SearchResult
		if hybrid {
			results, err = store.SearchHybrid(query, embedding, wsID, limit)
			if err != nil {
				return fmt.Errorf("gagal mencari memori (hybrid): %w", err)
			}
		} else {
			results, err = store.Search(embedding, wsID, limit)
			if err != nil {
				return fmt.Errorf("gagal mencari memori: %w", err)
			}
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
			fmt.Printf("  [%d] %s\n", r.Memory.ID, content)
			
			tagsStr := "-"
			if len(r.Memory.Tags) > 0 {
				tagsStr = strings.Join(r.Memory.Tags, ", ")
			}
			
			if hybrid {
				fmt.Printf("       relevansi: %.2f (vektor: %.2f) | tags=%s | source=%s | %s\n",
					r.Score, r.Similarity, tagsStr, r.Memory.Source,
					r.Memory.CreatedAt.Format("2006-01-02 15:04"),
				)
			} else {
				fmt.Printf("       relevansi: %.2f | tags=%s | source=%s | %s\n",
					r.Similarity, tagsStr, r.Memory.Source,
					r.Memory.CreatedAt.Format("2006-01-02 15:04"),
				)
			}
		}
		fmt.Println()
		return nil
	},
}

var memoryUpdateCmd = &cobra.Command{
	Use:   "update [id]",
	Short: "Update memori yang ada",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("ID memori tidak valid: %w", err)
		}

		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("gagal membuka database: %w", err)
		}
		defer store.Close()

		// Get current memory
		current, err := store.GetMemoryByID(id)
		if err != nil {
			return fmt.Errorf("gagal membaca memori: %w", err)
		}
		if current == nil {
			return fmt.Errorf("memori #%d tidak ditemukan", id)
		}

		// Get update flags
		content, _ := cmd.Flags().GetString("content")
		tagsStr, _ := cmd.Flags().GetString("tags")
		source, _ := cmd.Flags().GetString("source")
		categoryStr, _ := cmd.Flags().GetString("category")
		ttlStr, _ := cmd.Flags().GetString("ttl")

		updates := make(map[string]interface{})

		if content != "" {
			updates["content"] = content
		}
		if tagsStr != "" {
			updates["tags"] = strings.Split(tagsStr, ",")
		}
		if source != "" {
			updates["source"] = source
		}
		if categoryStr != "" {
			if catID, err := strconv.ParseInt(categoryStr, 10, 64); err == nil {
				updates["category_id"] = &catID
			}
		}
		if ttlStr != "" {
			days, err := time.ParseDuration(ttlStr)
			if err == nil {
				expiry := time.Now().Add(days)
				updates["expires_at"] = &expiry
			}
		}

		if len(updates) == 0 {
			return fmt.Errorf("tidak ada field yang diupdate")
		}

		if err := store.UpdateMemory(id, updates); err != nil {
			return fmt.Errorf("gagal update memori: %w", err)
		}

		ui.PrintSuccess("  ✓ Memori #%d berhasil diupdate", id)
		return nil
	},
}

var memoryGetCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Tampilkan detail memori",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("ID memori tidak valid: %w", err)
		}

		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("gagal membuka database: %w", err)
		}
		defer store.Close()

		mem, err := store.GetMemoryByID(id)
		if err != nil {
			return fmt.Errorf("gagal membaca memori: %w", err)
		}
		if mem == nil {
			return fmt.Errorf("memori #%d tidak ditemukan", id)
		}

		fmt.Println()
		fmt.Printf("  ID:          %d\n", mem.ID)
		fmt.Printf("  Workspace:   %d\n", mem.WorkspaceID)
		if mem.CategoryID != nil {
			fmt.Printf("  Category:    %d\n", *mem.CategoryID)
		}
		fmt.Printf("  Content:     %s\n", mem.Content)
		fmt.Printf("  Tags:        %s\n", strings.Join(mem.Tags, ", "))
		fmt.Printf("  Source:      %s\n", mem.Source)
		fmt.Printf("  Version:     %d\n", mem.Version)
		fmt.Printf("  Created:     %s\n", mem.CreatedAt.Format("2006-01-02 15:04"))
		fmt.Printf("  Updated:     %s\n", mem.UpdatedAt.Format("2006-01-02 15:04"))
		if mem.ExpiresAt != nil {
			fmt.Printf("  Expires:     %s\n", mem.ExpiresAt.Format("2006-01-02"))
		}
		if len(mem.Metadata) > 0 {
			fmt.Printf("  Metadata:    %v\n", mem.Metadata)
		}
		fmt.Println()
		return nil
	},
}

var memoryCleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Hapus memori kadaluarsa dan bersihkan database",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("gagal membuka database: %w", err)
		}
		defer store.Close()

		dryRun, _ := cmd.Flags().GetBool("dry-run")

		if dryRun {
			stats, err := store.GetMemoryStats(cfg.ActiveWorkspaceID)
			if err != nil {
				return fmt.Errorf("gagal dapatkan statistik: %w", err)
			}
			fmt.Printf("  [DRY RUN] Akan dihapus: %d memori kadaluarsa\n", stats["expired"])
			return nil
		}

		results, err := store.RunCleanup(cfg.ActiveWorkspaceID)
		if err != nil {
			return fmt.Errorf("gagal cleanup: %w", err)
		}

		ui.PrintSuccess("  ✓ Cleanup selesai")
		fmt.Printf("    - Memori kadaluarsa dihapus: %d\n", results["expired_deleted"])
		fmt.Printf("    - Kategori orphan dihapus: %d\n", results["orphan_categories_deleted"])
		if results["fts_optimized"] == true {
			fmt.Printf("    - FTS5 dioptimalkan\n")
		}
		fmt.Println()
		return nil
	},
}

var memoryExportCmd = &cobra.Command{
	Use:   "export [file]",
	Short: "Export memori ke file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filename := args[0]
		format, _ := cmd.Flags().GetString("format")
		includeEmbeddings, _ := cmd.Flags().GetBool("include-embeddings")
		includeMetadata, _ := cmd.Flags().GetBool("include-metadata")

		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("gagal membuka database: %w", err)
		}
		defer store.Close()

		options := memory.ExportOptions{
			Format:            memory.ExportFormat(format),
			IncludeEmbeddings: includeEmbeddings,
			IncludeMetadata:   includeMetadata,
		}

		data, actualFilename, err := store.ExportMemories(cfg.ActiveWorkspaceID, options)
		if err != nil {
			return fmt.Errorf("gagal export memori: %w", err)
		}

		// Use provided filename or generated one
		if filename != "" && !strings.HasSuffix(filename, "/") {
			actualFilename = filename
		}

		if err := os.WriteFile(actualFilename, data, 0644); err != nil {
			return fmt.Errorf("gagal simpan file: %w", err)
		}

		ui.PrintSuccess("  ✓ Memori diekspor ke %s (%d bytes)", actualFilename, len(data))
		return nil
	},
}

var memoryImportCmd = &cobra.Command{
	Use:   "import [file]",
	Short: "Import memori dari file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filename := args[0]
		format, _ := cmd.Flags().GetString("format")
		skipDuplicates, _ := cmd.Flags().GetBool("skip-duplicates")

		data, err := os.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("gagal baca file: %w", err)
		}

		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("gagal membuka database: %w", err)
		}
		defer store.Close()

		options := memory.ImportOptions{
			SkipDuplicates: skipDuplicates,
		}

		imported, err := store.ImportMemories(cfg.ActiveWorkspaceID, data, memory.ExportFormat(format), options)
		if err != nil {
			return fmt.Errorf("gagal import memori: %w", err)
		}

		ui.PrintSuccess("  ✓ %d memori berhasil diimpor", imported)
		return nil
	},
}

var memoryHistoryCmd = &cobra.Command{
	Use:   "history [id]",
	Short: "Tampilkan riwayat versi memori",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("ID memori tidak valid: %w", err)
		}

		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("gagal membuka database: %w", err)
		}
		defer store.Close()

		report, err := store.GetVersionHistory(id)
		if err != nil {
			return fmt.Errorf("gagal dapatkan riwayat: %w", err)
		}

		fmt.Println()
		fmt.Println(report)
		return nil
	},
}

var memoryRollbackCmd = &cobra.Command{
	Use:   "rollback [id] [version]",
	Short: "Rollback memori ke versi sebelumnya",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("ID memori tidak valid: %w", err)
		}

		versionID, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("ID versi tidak valid: %w", err)
		}

		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("gagal membuka database: %w", err)
		}
		defer store.Close()

		if err := store.RollbackMemory(id, versionID); err != nil {
			return fmt.Errorf("gagal rollback memori: %w", err)
		}

		ui.PrintSuccess("  ✓ Memori #%d berhasil di-rollback ke versi #%d", id, versionID)
		return nil
	},
}

func init() {
	memoryListCmd.Flags().IntP("limit", "n", 20, "jumlah memori yang ditampilkan")
	memoryListCmd.Flags().String("tags", "", "filter berdasarkan tags (pisahkan dengan koma)")
	memoryListCmd.Flags().String("source", "", "filter berdasarkan source")
	memoryListCmd.Flags().String("category", "", "filter berdasarkan category ID")
	memoryListCmd.Flags().String("date-from", "", "filter dari tanggal (YYYY-MM-DD)")
	memoryListCmd.Flags().String("date-to", "", "filter sampai tanggal (YYYY-MM-DD)")
	memoryListCmd.Flags().String("sort-by", "created_at", "field untuk sorting (created_at, updated_at)")
	memoryListCmd.Flags().String("sort-dir", "DESC", "arah sorting (ASC, DESC)")

	memorySearchCmd.Flags().IntP("limit", "n", 5, "jumlah hasil pencarian")
	memorySearchCmd.Flags().Bool("hybrid", false, "gunakan hybrid search (semantic + keyword)")

	memoryUpdateCmd.Flags().String("content", "", "konten baru")
	memoryUpdateCmd.Flags().String("tags", "", "tags baru (pisahkan dengan koma)")
	memoryUpdateCmd.Flags().String("source", "", "source baru")
	memoryUpdateCmd.Flags().String("category", "", "category ID baru")
	memoryUpdateCmd.Flags().String("ttl", "", "TTL baru (contoh: 30d, 1w)")

	memoryCleanupCmd.Flags().Bool("dry-run", false, "tampilkan apa yang akan dihapus tanpa menghapus")

	memoryExportCmd.Flags().StringP("format", "f", "json", "format export (json, csv, markdown, zip)")
	memoryExportCmd.Flags().Bool("include-embeddings", false, "sertakan vektor embeddings")
	memoryExportCmd.Flags().Bool("include-metadata", false, "sertakan metadata")

	memoryImportCmd.Flags().StringP("format", "f", "json", "format import (json, csv)")
	memoryImportCmd.Flags().Bool("skip-duplicates", true, "lewati memori duplikat")

	memoryCmd.AddCommand(
		memoryListCmd,
		memorySearchCmd,
		memoryClearCmd,
		memoryUpdateCmd,
		memoryGetCmd,
		memoryCleanupCmd,
		memoryExportCmd,
		memoryImportCmd,
		memoryHistoryCmd,
		memoryRollbackCmd,
	)
}

