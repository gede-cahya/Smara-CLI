package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/desktopbridge"
)

var desktopAgentOpts struct {
	addr         string
	auditLog     string
	mode         string
	allowCommand []string
	token        string
}
var desktopAgentCmd = &cobra.Command{
	Use:   "desktop-agent",
	Short: "Phase 2 Linux Desktop Bridge: local API untuk kontrol desktop Linux",
	Long: `Menjalankan service lokal sebagai jembatan Smara ke desktop Linux.

Endpoint utama:
- GET  /health, /screenshot, /window/active, /windows, /clipboard/read
- POST /window/focus, /app/open, /clipboard/write, /mouse/move
- POST /click, /double-click, /right-click, /scroll, /type, /hotkey
- POST /command/run, /stop, /resume

Semua aksi dicatat ke audit log JSONL. Endpoint /stop mengaktifkan emergency stop.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc := desktopbridge.New(desktopbridge.Options{Addr: desktopAgentOpts.addr, AuditLog: desktopAgentOpts.auditLog, Mode: desktopAgentOpts.mode, AllowCommand: desktopAgentOpts.allowCommand, Token: desktopAgentOpts.token})
		fmt.Printf("🖥️  Smara desktop-agent listening at http://%s\n", svc.Opt.Addr)
		fmt.Println("Mode      :", svc.Opt.Mode)
		fmt.Println("Audit log :", svc.Opt.AuditLog)
		fmt.Println("Stop      : Ctrl+C atau POST /stop untuk emergency stop")
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		return svc.ListenAndServe(ctx)
	},
}

func init() {
	desktopAgentCmd.Flags().StringVar(&desktopAgentOpts.addr, "addr", "127.0.0.1:8765", "Alamat listen local API")
	desktopAgentCmd.Flags().StringVar(&desktopAgentOpts.auditLog, "audit-log", "", "Path audit log JSONL")
	desktopAgentCmd.Flags().StringVar(&desktopAgentOpts.mode, "mode", "supervised", "Safety mode: read_only/supervised/autopilot")
	desktopAgentCmd.Flags().StringSliceVar(&desktopAgentOpts.allowCommand, "allow-command", nil, "Allowlist command desktop yang boleh dijalankan; bisa diulang/dipisah koma")
	desktopAgentCmd.Flags().StringVar(&desktopAgentOpts.token, "token", "", "Access token untuk pairing remote Smara Web")
	rootCmd.AddCommand(desktopAgentCmd)
}
