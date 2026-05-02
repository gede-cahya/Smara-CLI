package main

import (
	"github.com/spf13/cobra"
	"github.com/gede-cahya/Smara-CLI/internal/repair"
)

var (
	repairDryRun bool
	repairModule string
)

var repairCmd = &cobra.Command{
	Use:   "repair",
	Short: "Perbaiki masalah Smara CLI secara otomatis",
	Long: `Menjalankan pemeriksaan dan perbaikan otomatis untuk:
  - Database corrupt → backup & recreate
  - Config invalid → backup & tulis default
  - Session orphan → mark ended + hapus stale locks
  - MCP disconnect → reconnect

Gunakan --dry-run untuk preview tanpa mutasi.`,
	RunE: runRepair,
}

func init() {
	repairCmd.Flags().BoolVar(&repairDryRun, "dry-run", false, "preview tanpa memutasi file")
	repairCmd.Flags().StringVar(&repairModule, "module", "", "filter modul: db, config, mcp, session, disk")
	rootCmd.AddCommand(repairCmd)
}

func runRepair(cmd *cobra.Command, args []string) error {
	opts := repair.RepairOptions{
		DryRun: repairDryRun,
		Module: repairModule,
	}
	return repair.RunRepair(opts)
}
