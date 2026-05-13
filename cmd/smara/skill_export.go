package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	_ "github.com/mattn/go-sqlite3"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

var (
	skillExportOutput string
	skillExportSource string

	skillImportInput  string
	skillImportMode   string
	skillImportDryRun bool
	skillImportJSON   bool
)

// skillExportCmd writes every skill plus auto-capture pattern metadata to
// a JSON envelope.
var skillExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export seluruh skill tree ke file JSON",
	Long: `Menulis semua skill di ~/.smara/skills/ beserta metadata auto-capture
(pattern table) ke satu file JSON. File ini bisa di-import di mesin lain
untuk memindahkan skill tree secara menyeluruh.

Contoh:
  smara skill export                         # tulis ke stdout
  smara skill export --out backup.json       # tulis ke file
  smara skill export --out backup.json --source laptop-cahya`,
	Run: func(cmd *cobra.Command, args []string) {
		db := openSQLiteForSkillIO()
		defer closeDB(db)

		if skillExportOutput == "" || skillExportOutput == "-" {
			e, err := skill.ExportAll(db, skillExportSource)
			if err != nil {
				fmt.Fprintf(os.Stderr, "❌ gagal export: %v\n", err)
				os.Exit(1)
			}
			if err := skill.WriteExport(os.Stdout, e); err != nil {
				fmt.Fprintf(os.Stderr, "❌ gagal tulis: %v\n", err)
				os.Exit(1)
			}
			return
		}

		n, err := skill.ExportToFile(db, skillExportOutput, skillExportSource)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ gagal export: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Export %d skill ke %s\n", n, skillExportOutput)
	},
}

// skillImportCmd loads an export envelope back into ~/.smara/skills/.
var skillImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import skill tree dari file JSON",
	Long: `Membaca envelope yang dihasilkan 'smara skill export' dan menulis
skill beserta metadata auto-capture ke ~/.smara/skills/.

Conflict modes:
  --mode overwrite  (default) timpa skill yang sudah ada, lineage dipreserve
  --mode skip       lewati skill yang sudah ada
  --mode rename     buat skill baru dengan suffix -2, -3, dst

Contoh:
  smara skill import --in backup.json
  smara skill import --in backup.json --mode skip
  smara skill import --in backup.json --dry-run  # preview tanpa tulis`,
	Run: func(cmd *cobra.Command, args []string) {
		if skillImportInput == "" {
			fmt.Fprintln(os.Stderr, "❌ --in wajib diisi (atau '-' untuk stdin)")
			os.Exit(1)
		}

		mode, err := skill.ValidateImportModeString(skillImportMode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}

		db := openSQLiteForSkillIO()
		defer closeDB(db)

		var result *skill.ImportResult
		if skillImportInput == "-" {
			env, err := skill.ReadExport(os.Stdin)
			if err != nil {
				fmt.Fprintf(os.Stderr, "❌ parse stdin: %v\n", err)
				os.Exit(1)
			}
			result, err = skill.ImportAll(db, env, mode, skillImportDryRun)
			if err != nil {
				fmt.Fprintf(os.Stderr, "❌ %v\n", err)
				os.Exit(1)
			}
		} else {
			result, err = skill.ImportFromFile(db, skillImportInput, mode, skillImportDryRun)
			if err != nil {
				fmt.Fprintf(os.Stderr, "❌ %v\n", err)
				os.Exit(1)
			}
		}

		if skillImportJSON {
			_ = json.NewEncoder(os.Stdout).Encode(result)
			return
		}

		prefix := ""
		if skillImportDryRun {
			prefix = "[dry-run] "
		}
		fmt.Printf("%s✓ Created  : %d\n", prefix, len(result.Created))
		fmt.Printf("%s↺ Overwrite: %d\n", prefix, len(result.Overwritten))
		fmt.Printf("%s⇝ Renamed  : %d\n", prefix, len(result.Renamed))
		fmt.Printf("%s∅ Skipped  : %d\n", prefix, len(result.Skipped))
		fmt.Printf("%s📌 Patterns: %d\n", prefix, result.PatternsLoaded)
		if len(result.Warnings) > 0 {
			fmt.Println("\n⚠ Warnings:")
			for _, w := range result.Warnings {
				fmt.Printf("  - %s\n", w)
			}
		}
		if len(result.Renamed) > 0 {
			fmt.Println("\n⇝ Renamed:")
			for orig, nw := range result.Renamed {
				fmt.Printf("  %s → %s\n", orig, nw)
			}
		}
	},
}

// openSQLiteForSkillIO opens the memory.db sqlite file directly so skill
// export/import can read/write auto_skill_patterns. Returns nil on any
// error; callers use nil-safe code paths to degrade gracefully
// (skill JSON still works, just without pattern metadata).
func openSQLiteForSkillIO() *sql.DB {
	cfg := config.Get()
	if cfg == nil || cfg.DBPath == "" {
		return nil
	}
	db, err := sql.Open("sqlite3", cfg.DBPath)
	if err != nil {
		return nil
	}
	return db
}

func closeDB(db *sql.DB) {
	if db != nil {
		_ = db.Close()
	}
}

func init() {
	skillCmd.AddCommand(skillExportCmd, skillImportCmd)

	skillExportCmd.Flags().StringVar(&skillExportOutput, "out", "", "Path file tujuan (default: stdout)")
	skillExportCmd.Flags().StringVar(&skillExportSource, "source", "", "Label origin (misal: 'laptop-cahya')")

	skillImportCmd.Flags().StringVar(&skillImportInput, "in", "", "Path file sumber (atau '-' untuk stdin)")
	skillImportCmd.Flags().StringVar(&skillImportMode, "mode", "overwrite", "Conflict mode: overwrite|skip|rename")
	skillImportCmd.Flags().BoolVar(&skillImportDryRun, "dry-run", false, "Preview tanpa menulis")
	skillImportCmd.Flags().BoolVar(&skillImportJSON, "json", false, "Output hasil sebagai JSON")
}
