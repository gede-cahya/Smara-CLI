package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/gede-cahya/Smara-CLI/internal/ui"
)

// ────────────────────────── link ──────────────────────────

var memoryLinkCmd = &cobra.Command{
	Use:   "link [source-id] [target-id]",
	Short: "Hubungkan dua memori secara manual",
	Long:  "Buat link antar memori dengan relation dan weight yang bisa dikustom.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		src, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("source-id tidak valid: %w", err)
		}
		tgt, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("target-id tidak valid: %w", err)
		}

		relation, _ := cmd.Flags().GetString("relation")
		weight, _ := cmd.Flags().GetFloat64("weight")
		note, _ := cmd.Flags().GetString("note")

		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return err
		}
		defer store.Close()

		link, err := store.AddLink(src, tgt, relation, weight, note)
		if err != nil {
			return err
		}
		ui.PrintSuccess("  ✓ Link #%d: [%d] —%s(w=%.2f)→ [%d]", link.ID, link.SourceID, link.Relation, link.Weight, link.TargetID)
		return nil
	},
}

var memoryUnlinkCmd = &cobra.Command{
	Use:   "unlink [link-id]",
	Short: "Hapus link antar memori",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("link-id tidak valid: %w", err)
		}
		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return err
		}
		defer store.Close()
		if err := store.RemoveLink(id); err != nil {
			return err
		}
		ui.PrintSuccess("  ✓ Link #%d dihapus", id)
		return nil
	},
}

var memoryLinksCmd = &cobra.Command{
	Use:   "links [memory-id]",
	Short: "Tampilkan link untuk satu memori",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("memory-id tidak valid: %w", err)
		}
		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return err
		}
		defer store.Close()
		links, err := store.ListLinksFor(id)
		if err != nil {
			return err
		}
		if len(links) == 0 {
			fmt.Println("  Tidak ada link.")
			return nil
		}
		fmt.Printf("\n  Memory [%d] memiliki %d link:\n\n", id, len(links))
		for _, l := range links {
			tag := "manual"
			if l.AutoLinked {
				tag = "auto"
			}
			fmt.Printf("  #%d  [%d] —%s→ [%d]  weight=%.2f  (%s)\n",
				l.ID, l.SourceID, l.Relation, l.TargetID, l.Weight, tag)
			if l.Note != "" {
				fmt.Printf("       note: %s\n", l.Note)
			}
		}
		fmt.Println()
		return nil
	},
}

var memoryAutolinkCmd = &cobra.Command{
	Use:   "autolink",
	Short: "Auto-hubungkan memori berdasarkan kemiripan embedding",
	Long: `Membangun link otomatis antar memori dalam workspace aktif berdasarkan
cosine similarity. Hanya berjalan untuk memori yang punya embedding.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		threshold, _ := cmd.Flags().GetFloat64("threshold")
		topK, _ := cmd.Flags().GetInt("top-k")
		replace, _ := cmd.Flags().GetBool("replace")
		strategy, _ := cmd.Flags().GetString("strategy")
		aggressive, _ := cmd.Flags().GetBool("aggressive")
		hubLinks, _ := cmd.Flags().GetBool("hub-links")
		attachIsolated, _ := cmd.Flags().GetBool("attach-isolated")
		hubThreshold, _ := cmd.Flags().GetFloat64("hub-threshold")
		if aggressive {
			strategy = string(memory.AutoLinkModeAggressive)
		}
		if strategy == string(memory.AutoLinkModeAggressive) {
			if threshold == 0 {
				threshold = 0.28
			}
			if topK == 0 {
				topK = 10
			}
		}

		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return err
		}
		defer store.Close()

		fmt.Println("  Menjalankan auto-link…")
		report, err := store.AutoLinkSmart(memory.AutoLinkOptions{
			WorkspaceID:    cfg.ActiveWorkspaceID,
			Threshold:      threshold,
			MaxPerNode:     topK,
			Replace:        replace,
			Strategy:       strategy,
			HubLinks:       hubLinks,
			AttachIsolated: attachIsolated,
			HubThreshold:   hubThreshold,
		})
		if err != nil {
			return err
		}

		modeLabel := map[memory.AutoLinkMode]string{
			memory.AutoLinkModeSemantic:   "🧠 semantic (embedding)",
			memory.AutoLinkModeLexical:    "📝 lexical (Jaccard token overlap)",
			memory.AutoLinkModeAggressive: "🕸️ aggressive (multi-signal + hub/topic)",
			memory.AutoLinkModeNone:       "—",
		}[report.Mode]
		ui.PrintSuccess("  ✓ %d link otomatis dibuat — mode: %s", report.Created, modeLabel)
		fmt.Printf("    Memori: %d total, %d punya embedding (%.0f%%) · threshold %.2f · top-k %d\n",
			report.MemoriesScanned, report.WithEmbedding, report.EmbeddingRatio*100, report.Threshold, report.TopK)
		if report.FellBackToLexical {
			fmt.Println("    Catatan: provider tidak menyediakan embeddings,")
			fmt.Println("    fallback ke lexical similarity (Jaccard token overlap).")
			fmt.Println("    Untuk hasil semantic yang lebih bagus, pakai provider")
			fmt.Println("    dengan endpoint /embeddings (OpenAI atau Ollama lokal).")
		}
		if report.Created == 0 && report.Mode != memory.AutoLinkModeNone {
			fmt.Println("    Tidak ada pasangan memori yang melewati threshold.")
			fmt.Printf("    Coba turunkan --threshold (sekarang %.2f).\n", threshold)
		}
		return nil
	},
}

// ────────────────────────── graph (visual) ──────────────────────────

var memoryGraphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Visualisasikan memory graph (interaktif di browser)",
	Long: `Buka visualisasi interaktif memory graph di browser. Memori jadi node,
link jadi edge. Auto-link bisa dijalankan dulu dengan: smara memory autolink.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		limit, _ := cmd.Flags().GetInt("limit")
		noOpen, _ := cmd.Flags().GetBool("no-open")
		exportPath, _ := cmd.Flags().GetString("export")

		cfg := config.Get()
		store, err := memory.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return err
		}
		defer store.Close()

		// JSON export mode
		if exportPath != "" {
			data, err := store.BuildGraph(cfg.ActiveWorkspaceID, limit)
			if err != nil {
				return err
			}
			b, _ := json.MarshalIndent(data, "", "  ")
			if err := writeFileBytes(exportPath, b); err != nil {
				return err
			}
			ui.PrintSuccess("  ✓ Graph diekspor ke %s (%d nodes, %d edges)", exportPath, len(data.Nodes), len(data.Edges))
			return nil
		}

		// Find a free port if requested one is taken.
		if !portAvailable(port) {
			fmt.Printf("  Port %d sudah dipakai, mencari port lain…\n", port)
			port = findFreePort(port + 1)
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/api/graph", func(w http.ResponseWriter, r *http.Request) {
			data, err := store.BuildGraph(cfg.ActiveWorkspaceID, limit)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			_ = json.NewEncoder(w).Encode(data)
		})
		mux.HandleFunc("/api/memory/", func(w http.ResponseWriter, r *http.Request) {
			idStr := strings.TrimPrefix(r.URL.Path, "/api/memory/")
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				http.Error(w, "invalid id", 400)
				return
			}
			m, err := store.GetMemoryByID(id)
			if err != nil || m == nil {
				http.Error(w, "not found", 404)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			_ = json.NewEncoder(w).Encode(m)
		})
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(memoryGraphHTML))
		})

		addr := fmt.Sprintf("127.0.0.1:%d", port)
		url := "http://" + addr
		srv := &http.Server{Addr: addr, Handler: mux}

		// Graceful shutdown on Ctrl+C.
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		go func() {
			<-ctx.Done()
			shutdownCtx, c := context.WithTimeout(context.Background(), 2*time.Second)
			defer c()
			_ = srv.Shutdown(shutdownCtx)
		}()

		fmt.Println()
		ui.PrintSuccess("  ✓ Memory graph siap di %s", url)
		fmt.Println("    Tekan Ctrl+C untuk berhenti.")
		fmt.Println()

		if !noOpen {
			_ = openBrowser(url)
		}

		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	},
}

// ────────────────────────── helpers ──────────────────────────

func portAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func findFreePort(start int) int {
	for p := start; p < start+50; p++ {
		if portAvailable(p) {
			return p
		}
	}
	// Last resort: ask kernel for any port.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func writeFileBytes(path string, b []byte) error {
	return os.WriteFile(path, b, 0644)
}

// ────────────────────────── init ──────────────────────────

func init() {
	memoryLinkCmd.Flags().String("relation", "related", "tipe relasi (related, follows, contradicts, refines, similar)")
	memoryLinkCmd.Flags().Float64("weight", 0.5, "kekuatan link 0..1")
	memoryLinkCmd.Flags().String("note", "", "catatan opsional")

	memoryAutolinkCmd.Flags().Float64("threshold", 0, "minimum similarity/score 0..1 (default smart=0.78, aggressive=0.28)")
	memoryAutolinkCmd.Flags().Int("top-k", 0, "jumlah link maksimum per memori (default smart=5, aggressive=10)")
	memoryAutolinkCmd.Flags().Bool("replace", true, "hapus auto-link sebelumnya sebelum menjalankan")
	memoryAutolinkCmd.Flags().String("strategy", "smart", "engine: smart, aggressive")
	memoryAutolinkCmd.Flags().Bool("aggressive", false, "shortcut untuk --strategy aggressive --threshold 0.28 --top-k 10")
	memoryAutolinkCmd.Flags().Bool("hub-links", true, "aktifkan topic/hub grouping untuk aggressive autolink")
	memoryAutolinkCmd.Flags().Bool("attach-isolated", true, "hubungkan memory isolated ke neighbor/topic terbaik")
	memoryAutolinkCmd.Flags().Float64("hub-threshold", 0.18, "minimum score untuk attach isolated node")

	memoryGraphCmd.Flags().Int("port", 7878, "port HTTP server lokal")
	memoryGraphCmd.Flags().Int("limit", 0, "batasi jumlah node (0 = semua)")
	memoryGraphCmd.Flags().Bool("no-open", false, "jangan buka browser otomatis")
	memoryGraphCmd.Flags().String("export", "", "export graph ke file JSON dan keluar")
}
