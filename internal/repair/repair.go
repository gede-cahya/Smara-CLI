package repair

import (
	"fmt"
	"os"

	"github.com/gede-cahya/Smara-CLI/internal/config"
)

// RepairOptions controls repair behavior.
type RepairOptions struct {
	DryRun bool
	Module string // empty = all
}

// RunRepair executes repairs for detected issues.
func RunRepair(opts RepairOptions) error {
	cfg := config.Get()
	var actionsTaken int

	// Config repair
	if opts.Module == "" || opts.Module == "config" {
		configRes := CheckConfigHealth("")
		if configRes.Status != StatusOK {
			if opts.DryRun {
				fmt.Printf("[DRY-RUN] Config: %s\n", configRes.Message)
				fmt.Println("          → Aksi: Backup & tulis default config")
				actionsTaken++
			} else {
				if configRes.Fixable {
					if err := RepairConfig(""); err != nil {
						fmt.Printf("[FAIL] Config repair: %v\n", err)
					} else {
						fmt.Println("[OK]   Config direpair (default config ditulis)")
						actionsTaken++
					}
				}
			}
		}
		// Also fix permissions if warned
		if configRes.Status == StatusWarn && !opts.DryRun {
			_ = FixConfigPermissions("")
			fmt.Println("[OK]   Config permission diperbaiki (600)")
		}
	}

	// DB repair
	if opts.Module == "" || opts.Module == "db" {
		dbRes := CheckDBHealth(cfg.DBPath)
		if dbRes.Status != StatusOK {
			if opts.DryRun {
				fmt.Printf("[DRY-RUN] DB: %s\n", dbRes.Message)
				fmt.Println("          → Aksi: Backup & recreate database")
				actionsTaken++
			} else {
				if dbRes.Fixable {
					if err := RepairDB(cfg.DBPath); err != nil {
						fmt.Printf("[FAIL] DB repair: %v\n", err)
					} else {
						fmt.Println("[OK]   DB direpair (backup + recreate)")
						actionsTaken++
					}
				}
			}
		}
	}

	// Session repair
	if opts.Module == "" || opts.Module == "session" {
		sessionResults := CheckSessionHealth()
		for _, sr := range sessionResults {
			if sr.Status != StatusOK && sr.Fixable {
				if opts.DryRun {
					fmt.Printf("[DRY-RUN] Session: %s\n", sr.Message)
					fmt.Println("          → Aksi: Mark orphaned sessions as ended + hapus stale locks")
					actionsTaken++
				} else {
					if err := RepairSessions(cfg.DBPath); err != nil {
						fmt.Printf("[FAIL] Session repair: %v\n", err)
					} else {
						fmt.Println("[OK]   Session direpair (orphaned sessions + stale locks)")
						actionsTaken++
					}
				}
				break // one repair call handles all session issues
			}
		}
	}

	// MCP repair
	if opts.Module == "" || opts.Module == "mcp" {
		mcpResults := CheckMCPHealth()
		for _, mr := range mcpResults {
			if mr.Status != StatusOK && mr.Fixable {
				if opts.DryRun {
					fmt.Printf("[DRY-RUN] MCP: %s\n", mr.Message)
					fmt.Println("          → Aksi: Reconnect semua MCP server")
					actionsTaken++
				} else {
					RepairMCP()
					fmt.Println("[OK]   MCP reconnect dijalankan")
					actionsTaken++
				}
				break
			}
		}
	}

	// Disk: no auto-repair, just warn
	if opts.Module == "" || opts.Module == "disk" {
		diskResults := CheckDiskHealth()
		for _, dr := range diskResults {
			if dr.Status != StatusOK {
				fmt.Printf("[INFO] Disk: %s\n", dr.Message)
				if dr.Suggestion != "" {
					fmt.Printf("       → %s (manual)\n", dr.Suggestion)
				}
			}
		}
	}

	if opts.DryRun {
		fmt.Printf("\nTotal aksi yang akan dijalankan: %d\n", actionsTaken)
	} else {
		fmt.Printf("\nTotal repair berhasil: %d\n", actionsTaken)
	}

	return nil
}

// AutoRepairAtStartup attempts minimal auto-repair during startup.
// It only handles critical failures that would prevent startup entirely.
func AutoRepairAtStartup(dbPath, configPath string) (bool, error) {
	repaired := false

	// Try to fix config first
	configRes := CheckConfigHealth(configPath)
	if configRes.Status == StatusFail && configRes.Fixable {
		if err := RepairConfig(configPath); err != nil {
			return false, fmt.Errorf("auto-repair config gagal: %w", err)
		}
		fmt.Fprintln(os.Stderr, "[AUTO-REPAIR] Config invalid dibackup & direset ke default. Jalankan 'smara login' untuk mengatur ulang.")
		repaired = true
	}

	// Try to fix DB
	dbRes := CheckDBHealth(dbPath)
	if dbRes.Status == StatusFail && dbRes.Fixable {
		if err := RepairDB(dbPath); err != nil {
			return false, fmt.Errorf("auto-repair DB gagal: %w", err)
		}
		fmt.Fprintln(os.Stderr, "[AUTO-REPAIR] DB corrupt dibackup & direcreate. Data lama hilang.")
		repaired = true
	}

	return repaired, nil
}
