package main

import (
	"github.com/spf13/cobra"
	"github.com/gede-cahya/Smara-CLI/internal/repair"
)

var (
	doctorJSON   bool
	doctorModule string
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnosis kesehatan Smara CLI",
	Long: `Menjalankan pemeriksaan menyeluruh terhadap komponen Smara:
  - Database SQLite
  - File konfigurasi
  - Koneksi MCP server
  - Session store
  - Ruang disk & permission

Gunakan --json untuk output machine-readable.`,
	RunE: runDoctor,
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "output JSON")
	doctorCmd.Flags().StringVar(&doctorModule, "module", "", "filter modul: db, config, mcp, session, disk")
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	opts := repair.DoctorOptions{
		JSON:   doctorJSON,
		Module: doctorModule,
	}
	_, err := repair.RunDoctor(opts)
	return err
}
