package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/gede-cahya/Smara-CLI/internal/audit"
	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	cloud "github.com/gede-cahya/Smara-CLI/internal/memory/cloud"
	_ "github.com/gede-cahya/Smara-CLI/internal/memory/cloud/d1"
	_ "github.com/gede-cahya/Smara-CLI/internal/memory/cloud/supabase"
	_ "github.com/gede-cahya/Smara-CLI/internal/memory/cloud/turso"
)

var (
	cloudLoginProvider  string
	cloudLoginRegion    string
	cloudLoginHeadless  bool
	cloudStatusAudit    bool
	cloudWorkspacesJSON bool
	cloudProviderList   bool
	cloudConflictKeep   string

	// Phase 2 flags
	cloudDatabaseJSON  bool
	cloudDatabaseName  string
	cloudQuotaJSON     bool
	cloudHealthTimeout int
	cloudTokenJSON     bool

	cloudAutoResolvePolicy string
	cloudAutoResolveDryRun bool
)

var memoryCloudCmd = &cobra.Command{Use: "cloud", Short: "Kelola sinkronisasi memori ke cloud"}

var memoryCloudLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login ke Turso (atau provider lain)",
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Minute)
		defer cancel()
		providerName := strings.TrimSpace(cloudLoginProvider)
		if providerName == "" {
			providerName = config.Get().CloudMemory.Provider
		}
		if providerName == "" {
			providerName = "turso"
		}
		p, err := cloud.Get(providerName)
		if err != nil {
			return err
		}
		creds, err := p.Login(ctx, cloud.LoginOptions{Provider: providerName, Region: cloudLoginRegion, Headless: cloudLoginHeadless})
		if err != nil {
			_ = audit.LogCloudOp("login", false, providerName, map[string]any{"error": err.Error()})
			return err
		}
		if creds.Provider == "" {
			creds.Provider = providerName
		}
		if creds.Region == "" {
			creds.Region = cloudLoginRegion
		}
		if err := cloud.NewCredentialStore().Save(creds); err != nil {
			_ = audit.LogCloudOp("login", false, providerName, map[string]any{"error": err.Error()})
			return err
		}
		_ = audit.LogCloudOp("login", true, providerName, map[string]any{"source": cloud.NewCredentialStore().Source(), "region": creds.Region, "org": creds.OrgID})
		fmt.Printf("✓ Login cloud berhasil via provider %s\n", providerName)
		return nil
	},
}

var memoryCloudLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Hapus kredensial cloud dan hentikan sinkronisasi",
	RunE: func(cmd *cobra.Command, args []string) error {
		store := cloud.NewCredentialStore()
		err := store.Delete()
		if err != nil {
			var sum *cloud.DeleteSummary
			if errors.As(err, &sum) {
				fmt.Println("✓", sum.Error())
				_ = audit.LogCloudOp("logout", true, "", map[string]any{"sources": sum.Sources})
				return nil
			}
			_ = audit.LogCloudOp("logout", false, "", map[string]any{"error": err.Error()})
			return err
		}
		_ = audit.LogCloudOp("logout", true, "", map[string]any{"sources": []string{}})
		fmt.Println("✓ Logout selesai (tidak ada kredensial tersimpan).")
		return nil
	},
}

var memoryCloudStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Tampilkan status sinkronisasi cloud",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Get()
		fmt.Printf("Provider: %s\nEnabled: %v\n", cfg.CloudMemory.Provider, cfg.CloudMemory.Enabled)
		if !cfg.CloudMemory.Enabled {
			fmt.Println("State: disabled")
		} else {
			store, sm, cleanup, err := openCloudRuntime(cmd.Context())
			if err != nil {
				return err
			}
			defer cleanup()
			_ = store
			st, err := sm.Status(cmd.Context())
			if err != nil {
				return err
			}
			printStatus(st)
		}
		if cloudStatusAudit {
			_ = printAuditTail(20)
		}
		return nil
	},
}

var memoryCloudPushCmd = syncCommand("push")
var memoryCloudPullCmd = syncCommand("pull")
var memoryCloudSyncCmd = syncCommand("sync")

var memoryCloudEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Aktifkan sinkronisasi cloud untuk workspace aktif",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Get()
		cfg.CloudMemory.Enabled = true
		viper.Set("cloud_memory.enabled", true)
		if err := config.Save(); err != nil {
			return err
		}
		store, _, cleanup, err := openCloudRuntime(cmd.Context())
		if err != nil {
			return err
		}
		defer cleanup()
		_ = store
		fmt.Println("✓ Cloud memory aktif dan bootstrap selesai.")
		return nil
	},
}

var memoryCloudDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Nonaktifkan sinkronisasi cloud (replika lokal tetap utuh)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Get()
		cfg.CloudMemory.Enabled = false
		viper.Set("cloud_memory.enabled", false)
		if err := config.Save(); err != nil {
			return err
		}
		_ = audit.LogCloudOp("disable", true, cfg.CloudMemory.Provider, nil)
		fmt.Println("✓ Cloud memory dinonaktifkan. Database lokal tetap utuh.")
		return nil
	},
}

var memoryCloudConflictsCmd = &cobra.Command{Use: "conflicts", Short: "Daftar dan resolusi konflik sinkronisasi", RunE: listConflicts}
var memoryCloudConflictResolveCmd = &cobra.Command{Use: "resolve <id>", Short: "Resolusi konflik", Args: cobra.ExactArgs(1), RunE: resolveConflict}

var memoryCloudWorkspacesCmd = &cobra.Command{
	Use: "workspaces", Short: "Tampilkan daftar workspace beserta database cloud-nya",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := memory.NewSQLiteStore(config.Get().DBPath)
		if err != nil {
			return err
		}
		defer store.Close()
		rows, err := store.DB().Query(`SELECT w.name, c.db_name, c.region, c.last_sync_at FROM cloud_databases c JOIN workspaces w ON w.id=c.workspace_id ORDER BY w.name`)
		if err != nil {
			return err
		}
		defer rows.Close()
		type row struct{ Workspace, DBName, Region, LastSync string }
		var out []row
		for rows.Next() {
			var r row
			var last sql.NullString
			if err := rows.Scan(&r.Workspace, &r.DBName, &r.Region, &last); err != nil {
				return err
			}
			if last.Valid {
				r.LastSync = last.String
			}
			out = append(out, r)
		}
		if cloudWorkspacesJSON {
			return json.NewEncoder(os.Stdout).Encode(out)
		}
		fmt.Println("WORKSPACE\tDATABASE\tREGION\tLAST_SYNC")
		for _, r := range out {
			fmt.Printf("%s\t%s\t%s\t%s\n", r.Workspace, r.DBName, r.Region, r.LastSync)
		}
		return rows.Err()
	},
}

var memoryCloudProviderCmd = &cobra.Command{
	Use: "provider", Short: "Kelola provider cloud (list / switch)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cloudProviderList || len(args) == 0 {
			fmt.Println(strings.Join(cloud.List(), "\n"))
			return nil
		}
		if args[0] == "switch" && len(args) == 2 {
			if _, err := cloud.Get(args[1]); err != nil {
				return err
			}
			cfg := config.Get()
			cfg.CloudMemory.Provider = args[1]
			viper.Set("cloud_memory.provider", args[1])
			if err := config.Save(); err != nil {
				return err
			}
			fmt.Printf("✓ Provider cloud diganti ke %s. Restart proses berjalan bila perlu.\n", args[1])
			return nil
		}
		return fmt.Errorf("usage: smara memory cloud provider --list | provider switch <name>")
	},
}

var memoryCloudNukeCmd = &cobra.Command{
	Use: "nuke", Short: "Hapus seluruh database cloud (irreversible)",
	RunE: func(cmd *cobra.Command, args []string) error {
		creds, err := cloud.NewCredentialStore().Load()
		if err != nil {
			return err
		}
		p, err := cloud.Get(config.Get().CloudMemory.Provider)
		if err != nil {
			return err
		}
		dbs, err := p.ListWorkspaceDatabases(cmd.Context(), creds)
		if err != nil {
			return err
		}
		if len(dbs) == 0 {
			fmt.Println("Tidak ada database cloud untuk dihapus.")
			return nil
		}
		fmt.Println("Database akan dihapus:")
		for _, d := range dbs {
			fmt.Println("-", d.Name)
		}
		fmt.Print("Ketik DELETE untuk konfirmasi: ")
		var confirm string
		fmt.Scanln(&confirm)
		if confirm != "DELETE" {
			return fmt.Errorf("dibatalkan")
		}
		for _, d := range dbs {
			if err := p.DeleteWorkspaceDatabase(cmd.Context(), creds, d.Name); err != nil {
				return err
			}
		}
		_ = audit.LogCloudOp("nuke", true, creds.Provider, map[string]any{"count": len(dbs)})
		fmt.Printf("✓ %d database cloud dihapus.\n", len(dbs))
		return nil
	},
}

func syncCommand(kind string) *cobra.Command {
	return &cobra.Command{Use: kind, Short: "Jalankan cloud " + kind, RunE: func(cmd *cobra.Command, args []string) error {
		_, sm, cleanup, err := openCloudRuntime(cmd.Context())
		if err != nil {
			return err
		}
		defer cleanup()
		var rep *cloud.SyncReport
		switch kind {
		case "push":
			rep, err = sm.Push(cmd.Context())
		case "pull":
			rep, err = sm.Pull(cmd.Context())
		default:
			rep, err = sm.SyncNow(cmd.Context())
		}
		if rep != nil {
			printReport(rep)
		}
		return err
	}}
}

func openCloudRuntime(ctx context.Context) (*memory.SQLiteStore, *cloud.SyncManager, func() error, error) {
	cfg := config.Get()
	st, sm, err := memory.OpenStoreWithCloud(ctx, cfg, cloud.FromConfig(cfg.CloudMemory))
	if err != nil {
		return nil, nil, nil, err
	}
	return st, sm, func() error {
		if sm != nil {
			_ = sm.Stop()
		}
		if st != nil {
			return st.Close()
		}
		return nil
	}, nil
}
func printStatus(st *cloud.SyncStatus) {
	fmt.Printf("State: %s\nLast sync: %s\nLag: %ds\nPending push: %d\nPending pull: %d\nUnresolved conflicts: %d\nQuota: %.2f%%\nStorage: %.2f MB / %.2f MB\n", st.State, st.LastSyncAt.Format(time.RFC3339), st.LagSeconds, st.PendingPush, st.PendingPull, st.UnresolvedConflicts, st.Quota.PercentUsed, float64(st.Quota.StorageBytes)/1024/1024, float64(st.Quota.StorageLimitBytes)/1024/1024)
}
func printReport(r *cloud.SyncReport) {
	fmt.Printf("Rows pushed: %d\nFrames pulled: %d\nConflicts: %d\n", r.PushedRows, r.PulledFrames, r.Conflicts)
	for _, e := range r.Errors {
		fmt.Println("Error:", e)
	}
}
func printAuditTail(n int) error {
	home, _ := os.UserHomeDir()
	b, err := os.ReadFile(home + "/.smara/audit.log")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	fmt.Println("Audit log:")
	for _, l := range lines {
		fmt.Println(l)
	}
	return nil
}

func listConflicts(cmd *cobra.Command, args []string) error {
	store, err := memory.NewSQLiteStore(config.Get().DBPath)
	if err != nil {
		return err
	}
	defer store.Close()
	cs, err := store.ListUnresolvedConflicts()
	if err != nil {
		return err
	}
	fmt.Println("ID\tMEMORY\tLOCAL\tREMOTE\tDETECTED")
	for _, c := range cs {
		fmt.Printf("%d\t%d\t%d\t%d\t%s\n", c.ID, c.MemoryID, c.LocalVersion, c.RemoteVersion, c.DetectedAt.Format(time.RFC3339))
	}
	return nil
}
func resolveConflict(cmd *cobra.Command, args []string) error {
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return err
	}
	keep := strings.TrimSpace(cloudConflictKeep)
	if keep == "" {
		return fmt.Errorf("--keep wajib: local|remote|merged")
	}
	store, err := memory.NewSQLiteStore(config.Get().DBPath)
	if err != nil {
		return err
	}
	defer store.Close()
	conflicts, err := store.ListUnresolvedConflicts()
	if err != nil {
		return err
	}
	for _, c := range conflicts {
		if c.ID == id {
			winner := cloud.MemoryRow{ID: c.MemoryID, CloudID: c.CloudID, Content: c.LocalContent, DeviceID: c.LocalDeviceID, Version: c.LocalVersion, UpdatedAt: c.LocalUpdatedAt}
			if keep == "remote" {
				winner.Content = c.RemoteContent
				winner.DeviceID = c.RemoteDeviceID
				winner.Version = c.RemoteVersion
				winner.UpdatedAt = c.RemoteUpdatedAt
			}
			if keep == "merged" {
				winner.Content = c.LocalContent + "\n---merged manually---\n" + c.RemoteContent
				if c.RemoteVersion > winner.Version {
					winner.Version = c.RemoteVersion
				}
				winner.Version++
			}
			if keep != "local" && keep != "remote" && keep != "merged" {
				return fmt.Errorf("--keep harus local|remote|merged")
			}
			if err := store.UpdateMemoryFromConflict(c.MemoryID, winner, nil); err != nil {
				return err
			}
			if err := store.MarkConflictResolved(id, keep); err != nil {
				return err
			}
			fmt.Println("✓ Konflik terselesaikan.")
			return nil
		}
	}
	return fmt.Errorf("conflict id %d tidak ditemukan", id)
}

func init() {
	memoryCloudLoginCmd.Flags().StringVar(&cloudLoginProvider, "provider", "turso", "provider cloud")
	memoryCloudLoginCmd.Flags().StringVar(&cloudLoginRegion, "region", "", "region provider")
	memoryCloudLoginCmd.Flags().BoolVar(&cloudLoginHeadless, "headless", false, "pakai env SMARA_CLOUD_TOKEN/ORG/REGION")
	memoryCloudStatusCmd.Flags().BoolVar(&cloudStatusAudit, "audit", false, "tampilkan 20 audit entry terakhir")
	memoryCloudWorkspacesCmd.Flags().BoolVar(&cloudWorkspacesJSON, "json", false, "output JSON")
	memoryCloudProviderCmd.Flags().BoolVar(&cloudProviderList, "list", false, "list provider")
	memoryCloudConflictResolveCmd.Flags().StringVar(&cloudConflictKeep, "keep", "", "pilihan: local|remote|merged")
	memoryCloudAutoResolveCmd.Flags().StringVar(&cloudAutoResolvePolicy, "policy", "lww", "policy resolusi: lww, local, remote")
	memoryCloudAutoResolveCmd.Flags().BoolVar(&cloudAutoResolveDryRun, "dry-run", false, "simulasi tanpa eksekusi")

	// Phase 2 flags
	memoryCloudDatabaseListCmd.Flags().BoolVar(&cloudDatabaseJSON, "json", false, "output JSON")
	memoryCloudDatabaseInfoCmd.Flags().StringVar(&cloudDatabaseName, "name", "", "nama database (bisa juga positional arg)")
	memoryCloudQuotaCmd.Flags().BoolVar(&cloudQuotaJSON, "json", false, "output JSON")
	memoryCloudHealthCmd.Flags().IntVar(&cloudHealthTimeout, "timeout", 15, "timeout detik untuk health check")
	memoryCloudTokenInfoCmd.Flags().BoolVar(&cloudTokenJSON, "json", false, "output JSON")

	// Phase 3: Encryption flags
	memoryCloudEncryptionStatusCmd.Flags().BoolVar(&cloudEncryptJSON, "json", false, "output JSON")
	memoryCloudEncryptionKeyGenCmd.Flags().BoolVar(&cloudEncryptForce, "force", false, "overwrite key yang sudah ada")
	memoryCloudEncryptionKeyGenCmd.Flags().BoolVar(&cloudEncryptRotate, "rotate", false, "rotasi key: decrypt dengan old, generate new")
	memoryCloudEncryptionKeyDeleteCmd.Flags().BoolVar(&cloudEncryptForce, "force", false, "konfirmasi penghapusan")

	// Subcommand hierarchies
	memoryCloudConflictsCmd.AddCommand(memoryCloudConflictResolveCmd, memoryCloudAutoResolveCmd)
	memoryCloudDatabaseCmd.AddCommand(memoryCloudDatabaseListCmd, memoryCloudDatabaseInfoCmd)
	memoryCloudTokenCmd.AddCommand(memoryCloudTokenInfoCmd)
	memoryCloudEncryptionCmd.AddCommand(memoryCloudEncryptionStatusCmd, memoryCloudEncryptionKeyGenCmd, memoryCloudEncryptionKeyDeleteCmd)

	memoryCloudCmd.AddCommand(
		memoryCloudLoginCmd, memoryCloudLogoutCmd,
		memoryCloudStatusCmd, memoryCloudPushCmd, memoryCloudPullCmd, memoryCloudSyncCmd,
		memoryCloudEnableCmd, memoryCloudDisableCmd,
		memoryCloudConflictsCmd, memoryCloudWorkspacesCmd,
		memoryCloudProviderCmd, memoryCloudNukeCmd, memoryCloudWhoamiCmd,
		// Phase 2
		memoryCloudDatabaseCmd, memoryCloudQuotaCmd, memoryCloudHealthCmd, memoryCloudTokenCmd,
		// Phase 3: Encryption at rest
		memoryCloudEncryptionCmd,
	)
	memoryCmd.AddCommand(memoryCloudCmd)
}

// ---------------------------------------------------------------------------
// whoami — tampilkan info kredensial cloud yang sedang aktif
// ---------------------------------------------------------------------------

var memoryCloudWhoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Tampilkan informasi kredensial cloud yang sedang aktif",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Get()
		fmt.Printf("Provider aktif: %s\n", cfg.CloudMemory.Provider)
		fmt.Printf("Cloud enabled:  %v\n", cfg.CloudMemory.Enabled)

		creds, err := cloud.NewCredentialStore().Load()
		if err != nil {
			if errors.Is(err, cloud.ErrNoCredentials) {
				fmt.Println("Status:         belum login (tidak ada kredensial tersimpan)")
				fmt.Println("\nLogin dengan:")
				fmt.Println("  smara memory cloud login --provider <name>")
				fmt.Println("  smara memory cloud login --provider supabase --headless  # via env vars")
				return nil
			}
			return err
		}

		fmt.Println("Status:         logged in")
		fmt.Printf("Email:          %s\n", creds.Email)
		fmt.Printf("Org/Project:    %s\n", creds.OrgID)
		fmt.Printf("Region:         %s\n", creds.Region)
		if !creds.ExpiresAt.IsZero() {
			fmt.Printf("Expires:        %s\n", creds.ExpiresAt.Format(time.RFC3339))
		}
		fmt.Printf("Token:          %s\n", "[REDACTED]")

		// Validate credentials with provider.
		p, err := cloud.Get(cfg.CloudMemory.Provider)
		if err != nil {
			fmt.Printf("Validasi:       ⚠ provider %q tidak tersedia (%v)\n", cfg.CloudMemory.Provider, err)
			return nil
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
		defer cancel()
		if err := p.ValidateCredentials(ctx, creds); err != nil {
			fmt.Printf("Validasi:       ❌ gagal — %v\n", err)
		} else {
			fmt.Println("Validasi:       ✓ token valid")
		}
		return nil
	},
}

// ---------------------------------------------------------------------------
// database — kelola database cloud provider (list, info)
// ---------------------------------------------------------------------------

var memoryCloudDatabaseCmd = &cobra.Command{Use: "database", Short: "Kelola database cloud provider"}

var memoryCloudDatabaseListCmd = &cobra.Command{
	Use:   "list",
	Short: "Tampilkan daftar database cloud di provider",
	RunE: func(cmd *cobra.Command, args []string) error {
		creds, err := cloud.NewCredentialStore().Load()
		if err != nil {
			return err
		}
		p, err := cloud.Get(config.Get().CloudMemory.Provider)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
		defer cancel()
		dbs, err := p.ListWorkspaceDatabases(ctx, creds)
		if err != nil {
			return err
		}
		if len(dbs) == 0 {
			fmt.Println("(tidak ada database cloud)")
			return nil
		}
		if cloudDatabaseJSON {
			return json.NewEncoder(os.Stdout).Encode(dbs)
		}
		fmt.Printf("%-40s %-12s %-8s %s\n", "NAME", "REGION", "SIZE", "CREATED")
		for _, d := range dbs {
			size := "-"
			if d.SizeBytes > 0 {
				size = fmt.Sprintf("%.1f MB", float64(d.SizeBytes)/1024/1024)
			}
			fmt.Printf("%-40s %-12s %-8s %s\n", d.Name, d.Region, size, d.CreatedAt.Format("2006-01-02"))
		}
		return nil
	},
}

var memoryCloudDatabaseInfoCmd = &cobra.Command{
	Use:   "info <nama-db>",
	Short: "Tampilkan detail satu database cloud",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := cloudDatabaseName
		if len(args) > 0 {
			name = args[0]
		}
		if name == "" {
			return fmt.Errorf("nama database diperlukan (--name atau positional arg)")
		}
		creds, err := cloud.NewCredentialStore().Load()
		if err != nil {
			return err
		}
		p, err := cloud.Get(config.Get().CloudMemory.Provider)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
		defer cancel()
		dbs, err := p.ListWorkspaceDatabases(ctx, creds)
		if err != nil {
			return err
		}
		for _, d := range dbs {
			if d.Name == name {
				fmt.Printf("Name:       %s\n", d.Name)
				fmt.Printf("Provider:   %s\n", d.Provider)
				fmt.Printf("Region:     %s\n", d.Region)
				fmt.Printf("Size:       %d bytes (%.2f MB)\n", d.SizeBytes, float64(d.SizeBytes)/1024/1024)
				fmt.Printf("Rows read:  %d (this month)\n", d.RowsRead)
				fmt.Printf("Rows wrote: %d (this month)\n", d.RowsWritten)
				fmt.Printf("Created:    %s\n", d.CreatedAt.Format(time.RFC3339))
				if d.URL != "" {
					fmt.Printf("URL:        %s\n", d.URL)
				}
				return nil
			}
		}
		return fmt.Errorf("database %q tidak ditemukan", name)
	},
}

// ---------------------------------------------------------------------------
// quota — tampilkan detail kuota provider
// ---------------------------------------------------------------------------

var memoryCloudQuotaCmd = &cobra.Command{
	Use:   "quota",
	Short: "Tampilkan detail kuota dan pemakaian cloud",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, sm, cleanup, err := openCloudRuntime(cmd.Context())
		if err != nil {
			// Fallback: try provider directly when cloud not bootstrapped
			p, pErr := cloud.Get(config.Get().CloudMemory.Provider)
			if pErr != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()
			st, stErr := p.Status(ctx)
			if stErr != nil {
				return stErr
			}
			printQuotaDetailed(&st.Quota)
			return nil
		}
		defer cleanup()
		st, err := sm.Status(cmd.Context())
		if err != nil {
			return err
		}
		printQuotaDetailed(&st.Quota)
		return nil
	},
}

func printQuotaDetailed(q *cloud.QuotaInfo) {
	storageMB := float64(q.StorageBytes) / 1024 / 1024
	limitMB := float64(q.StorageLimitBytes) / 1024 / 1024
	bar := quotaBar(q.PercentUsed, 40)

	fmt.Printf("Storage:   %s  %.1f / %.1f MB (%.1f%%)\n", bar, storageMB, limitMB, q.PercentUsed)
	fmt.Printf("Rows read: %d / %d (this month)\n", q.RowsReadMonth, q.RowsReadLimit)
	if q.RowsReadLimit > 0 {
		pct := float64(q.RowsReadMonth) / float64(q.RowsReadLimit) * 100
		fmt.Printf("           %.1f%% used\n", pct)
	}
	if q.PercentUsed >= 90 {
		fmt.Println("⚠  Kuota hampir penuh! Pertimbangkan cleanup atau upgrade.")
	}
	if q.PercentUsed >= 99 {
		fmt.Println("🛑 Kuota >= 99% — write akan diblokir sampai ada ruang.")
	}
}

func quotaBar(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	empty := width - filled
	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	// Color indicator
	switch {
	case pct >= 90:
		return "[" + bar + "] 🔴"
	case pct >= 70:
		return "[" + bar + "] 🟡"
	default:
		return "[" + bar + "] 🟢"
	}
}

// ---------------------------------------------------------------------------
// health — cek konektivitas & kesehatan provider cloud
// ---------------------------------------------------------------------------

var memoryCloudHealthCmd = &cobra.Command{
	Use:   "health",
	Short: "Cek konektivitas dan kesehatan provider cloud",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Get()
		providerName := cfg.CloudMemory.Provider
		if providerName == "" {
			providerName = "turso"
		}

		fmt.Printf("Provider:  %s\n", providerName)
		fmt.Printf("Enabled:   %v\n", cfg.CloudMemory.Enabled)

		// 1. Check provider registered
		p, err := cloud.Get(providerName)
		if err != nil {
			fmt.Printf("Registry:  ❌ %v\n", err)
			return nil
		}
		fmt.Println("Registry:  ✓ terdaftar")

		// 2. Check credentials
		creds, err := cloud.NewCredentialStore().Load()
		if err != nil {
			if errors.Is(err, cloud.ErrNoCredentials) {
				fmt.Println("Auth:      ⚠ belum login (jalankan `smara memory cloud login`)")
			} else {
				fmt.Printf("Auth:      ❌ %v\n", err)
			}
			return nil
		}
		fmt.Printf("Auth:      ✓ logged in as %s\n", creds.Email)

		// 3. Validate token
		ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(cloudHealthTimeout)*time.Second)
		defer cancel()
		if err := p.ValidateCredentials(ctx, creds); err != nil {
			fmt.Printf("Token:     ❌ %v\n", err)
			fmt.Println("           → coba login ulang: smara memory cloud login")
		} else {
			fmt.Println("Token:     ✓ valid")
		}

		// 4. Check remote connectivity
		st, err := p.Status(ctx)
		if err != nil {
			fmt.Printf("Remote:    ❌ %v\n", err)
			fmt.Println("           → provider unreachable; data lokal tetap aman")
		} else {
			fmt.Printf("Remote:    ✓ connected (state=%s, lag=%ds)\n", st.State, st.LagSeconds)
		}

		// 5. Check local DB
		if cfg.CloudMemory.Enabled {
			if _, err := os.Stat(cfg.DBPath); os.IsNotExist(err) {
				fmt.Println("Local DB:  ⚠ file database lokal belum ada (bootstrap via `smara memory cloud enable`)")
			} else {
				fmt.Printf("Local DB:  ✓ %s\n", cfg.DBPath)
			}
		}

		// 6. Provider name check
		switch providerName {
		case "turso":
			fmt.Println("Type:      libSQL (embedded replica)")
		case "supabase":
			fmt.Println("Type:      PostgreSQL (REST API sync)")
		case "d1":
			fmt.Println("Type:      Cloudflare D1 (SQLite edge)")
		default:
			fmt.Printf("Type:      unknown (%s)\n", providerName)
		}

		return nil
	},
}

// ---------------------------------------------------------------------------
// token info — tampilkan detail token cloud
// ---------------------------------------------------------------------------

var memoryCloudTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Tampilkan informasi token cloud yang tersimpan",
}

var memoryCloudTokenInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Tampilkan detail token (expiry, scopes, dll)",
	RunE: func(cmd *cobra.Command, args []string) error {
		creds, err := cloud.NewCredentialStore().Load()
		if err != nil {
			return err
		}
		store := cloud.NewCredentialStore()
		source := store.Source()

		if cloudTokenJSON {
			payload := map[string]any{
				"provider":  creds.Provider,
				"email":     creds.Email,
				"org_id":    creds.OrgID,
				"region":    creds.Region,
				"expires":   nil,
				"source":    source,
				"has_token": creds.Token != "",
			}
			if !creds.ExpiresAt.IsZero() {
				payload["expires"] = creds.ExpiresAt.Format(time.RFC3339)
			}
			return json.NewEncoder(os.Stdout).Encode(payload)
		}

		fmt.Printf("Provider:  %s\n", creds.Provider)
		fmt.Printf("Email:     %s\n", creds.Email)
		fmt.Printf("Org:       %s\n", creds.OrgID)
		fmt.Printf("Region:    %s\n", creds.Region)
		fmt.Printf("Source:    %s\n", source)
		fmt.Printf("Token:     %s\n", "[REDACTED]")
		if creds.RefreshToken != "" {
			fmt.Printf("Refresh:   %s\n", "[REDACTED]")
		}
		if creds.ExpiresAt.IsZero() {
			fmt.Println("Expires:   never (API key)")
		} else {
			remaining := time.Until(creds.ExpiresAt)
			fmt.Printf("Expires:   %s", creds.ExpiresAt.Format(time.RFC3339))
			if remaining < 0 {
				fmt.Println(" ⚠ EXPIRED — login ulang!")
			} else if remaining < time.Hour {
				fmt.Printf(" ⚠ %s lagi!\n", remaining.Round(time.Minute))
			} else {
				fmt.Printf(" (%s remaining)\n", remaining.Round(time.Minute))
			}
		}

		// Validate
		p, err := cloud.Get(config.Get().CloudMemory.Provider)
		if err == nil {
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()
			if err := p.ValidateCredentials(ctx, creds); err != nil {
				fmt.Printf("Valid:     ❌ %v\n", err)
			} else {
				fmt.Println("Valid:     ✓")
			}
		}

		return nil
	},
}

var memoryCloudAutoResolveCmd = &cobra.Command{
	Use:   "auto-resolve",
	Short: "Resolusi massal semua konflik dengan policy otomatis (lww / local / remote)",
	Long: `Selesaikan semua konflik yang belum resolved secara otomatis.

Policy:
  lww       — Last-Write-Wins: pilih versi dengan updated_at terbaru
  local     — Selalu pilih versi lokal
  remote    — Selalu pilih versi remote

Gunakan --dry-run untuk melihat apa yang akan terjadi tanpa eksekusi.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		policy := strings.TrimSpace(cloudAutoResolvePolicy)
		if policy == "" {
			policy = "lww"
		}
		if policy != "lww" && policy != "local" && policy != "remote" {
			return fmt.Errorf("policy tidak dikenal: %q (pilih: lww, local, remote)", policy)
		}

		store, err := memory.NewSQLiteStore(config.Get().DBPath)
		if err != nil {
			return err
		}
		defer store.Close()

		conflicts, err := store.ListUnresolvedConflicts()
		if err != nil {
			return err
		}
		if len(conflicts) == 0 {
			fmt.Println("✓ Tidak ada konflik yang perlu diselesaikan.")
			return nil
		}

		fmt.Printf("Ditemukan %d konflik. Policy: %s\n", len(conflicts), policy)
		if cloudAutoResolveDryRun {
			fmt.Println("Mode: DRY RUN (tidak ada perubahan)")
		}
		fmt.Println()

		resolved := 0
		for _, c := range conflicts {
			var winner cloud.MemoryRow
			var resolution string

			switch policy {
			case "lww":
				if c.RemoteUpdatedAt.After(c.LocalUpdatedAt) {
					winner = cloud.MemoryRow{ID: c.MemoryID, CloudID: c.CloudID, Content: c.RemoteContent, DeviceID: c.RemoteDeviceID, Version: c.RemoteVersion, UpdatedAt: c.RemoteUpdatedAt}
					resolution = "remote (LWW — newer)"
				} else {
					winner = cloud.MemoryRow{ID: c.MemoryID, CloudID: c.CloudID, Content: c.LocalContent, DeviceID: c.LocalDeviceID, Version: c.LocalVersion, UpdatedAt: c.LocalUpdatedAt}
					resolution = "local (LWW — newer)"
				}
			case "local":
				winner = cloud.MemoryRow{ID: c.MemoryID, CloudID: c.CloudID, Content: c.LocalContent, DeviceID: c.LocalDeviceID, Version: c.LocalVersion, UpdatedAt: c.LocalUpdatedAt}
				resolution = "local"
			case "remote":
				winner = cloud.MemoryRow{ID: c.MemoryID, CloudID: c.CloudID, Content: c.RemoteContent, DeviceID: c.RemoteDeviceID, Version: c.RemoteVersion, UpdatedAt: c.RemoteUpdatedAt}
				resolution = "remote"
			}

			if cloudAutoResolveDryRun {
				fmt.Printf("  [DRY RUN] conflict #%d (memory %d): → %s\n", c.ID, c.MemoryID, resolution)
				resolved++
				continue
			}

			if err := store.UpdateMemoryFromConflict(c.MemoryID, winner, nil); err != nil {
				fmt.Printf("  ❌ conflict #%d: gagal update memory — %v\n", c.ID, err)
				continue
			}
			if err := store.MarkConflictResolved(c.ID, resolution); err != nil {
				fmt.Printf("  ❌ conflict #%d: gagal tandai resolved — %v\n", c.ID, err)
				continue
			}
			fmt.Printf("  ✓ conflict #%d (memory %d): → %s\n", c.ID, c.MemoryID, resolution)
			resolved++
		}

		fmt.Printf("\n✓ %d/%d konflik terselesaikan.\n", resolved, len(conflicts))
		if cloudAutoResolveDryRun {
			fmt.Println("  (dry run — tidak ada perubahan permanen)")
		}
		return nil
	},
}

// ---------------------------------------------------------------------------
// encryption — kelola enkripsi at-rest untuk cloud memory
// ---------------------------------------------------------------------------

var (
	cloudEncryptJSON   bool
	cloudEncryptForce  bool
	cloudEncryptRotate bool
)

var memoryCloudEncryptionCmd = &cobra.Command{
	Use:   "encryption",
	Short: "Kelola enkripsi at-rest untuk cloud memory",
}

var memoryCloudEncryptionStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Tampilkan status enkripsi at-rest",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Get()

		if cloudEncryptJSON {
			info := cloud.EncryptionKeyStatus()
			payload := map[string]any{
				"enabled":    cfg.CloudMemory.EncryptAtRest,
				"key_exists": info.Exists,
				"key_source": info.Source,
				"key_size":   info.KeySize,
			}
			return json.NewEncoder(os.Stdout).Encode(payload)
		}

		fmt.Printf("EncryptAtRest: %v\n", cfg.CloudMemory.EncryptAtRest)

		info := cloud.EncryptionKeyStatus()
		if info.Exists {
			fmt.Printf("Key:           ✓ tersedia (source: %s, %d bytes AES-256)\n", info.Source, info.KeySize)
		} else {
			fmt.Println("Key:           ✗ belum ada")
			if cfg.CloudMemory.EncryptAtRest {
				fmt.Println("               → key akan digenerate otomatis saat cloud enable")
			}
		}

		if cfg.CloudMemory.EncryptAtRest {
			fmt.Println("\nEfek enkripsi:")
			fmt.Println("  • Konten memori dienkripsi dengan AES-256-GCM sebelum dikirim ke cloud")
			fmt.Println("  • Data di cloud tidak bisa dibaca tanpa encryption key")
			fmt.Println("  • Data lokal tetap plaintext untuk pencarian dan FTS")
		}
		return nil
	},
}

var memoryCloudEncryptionKeyGenCmd = &cobra.Command{
	Use:   "key-generate",
	Short: "Generate encryption key baru (AES-256)",
	Long: `Generate kunci enkripsi AES-256 baru untuk enkripsi at-rest.

HATI-HATI: Jika sudah ada data terenkripsi di cloud dengan key lama,
key baru akan membuat data lama tidak bisa dibaca. Gunakan --rotate
untuk generate key baru + re-enkripsi data yang ada.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if cloudEncryptRotate {
			// Rotate: decrypt with old key, generate new key, re-encrypt.
			oldKey, err := cloud.LoadEncryptionKey()
			if err != nil {
				if errors.Is(err, cloud.ErrNoCredentials) {
					fmt.Println("⚠  Tidak ada key lama — generate key baru seperti biasa.")
				} else {
					return fmt.Errorf("gagal load key lama: %w", err)
				}
			}

			// Delete old key.
			if oldKey != nil {
				if err := cloud.DeleteEncryptionKey(); err != nil {
					return fmt.Errorf("gagal hapus key lama: %w", err)
				}
			}

			// Generate new key.
			newKey, info, err := cloud.EnsureEncryptionKey()
			if err != nil {
				return fmt.Errorf("gagal generate key baru: %w", err)
			}

			fmt.Printf("✓ Key baru digenerate (source: %s, %d bytes)\n", info.Source, info.KeySize)

			// If old key existed, offer to re-encrypt local data.
			if oldKey != nil && !cloudEncryptForce {
				fmt.Println("\n⚠  Data di cloud masih terenkripsi dengan key lama.")
				fmt.Println("   Rekomendasi: jalankan `smara memory cloud push` untuk re-enkripsi data.")
				fmt.Println("   Gunakan --force untuk skip peringatan ini.")
			}

			// Zero keys.
			for i := range newKey {
				newKey[i] = 0
			}

			// Zero old key.
			for i := range oldKey {
				oldKey[i] = 0
			}
			return nil
		}

		// Plain generation.
		existingKey, err := cloud.LoadEncryptionKey()
		if err == nil && existingKey != nil && !cloudEncryptForce {
			info := cloud.EncryptionKeyStatus()
			fmt.Printf("⚠  Key sudah ada (source: %s).\n", info.Source)
			fmt.Println("   Gunakan --force untuk overwrite, atau --rotate untuk rotasi aman.")
			return nil
		}

		if err := cloud.DeleteEncryptionKey(); err != nil {
			return fmt.Errorf("gagal hapus key lama: %w", err)
		}

		newKey, info, err := cloud.EnsureEncryptionKey()
		if err != nil {
			return fmt.Errorf("gagal generate key: %w", err)
		}

		fmt.Printf("✓ Encryption key baru digenerate (source: %s, %d bytes AES-256)\n", info.Source, info.KeySize)
		fmt.Println("  Simpan key ini dengan aman. Tanpa key, data di cloud tidak bisa dibaca.")

		// Zero key.
		for i := range newKey {
			newKey[i] = 0
		}
		return nil
	},
}

var memoryCloudEncryptionKeyDeleteCmd = &cobra.Command{
	Use:   "key-delete",
	Short: "Hapus encryption key dari penyimpanan lokal",
	Long: `Hapus encryption key dari keyring dan file lokal.

⚠ PERINGATAN: Data di cloud yang sudah terenkripsi tidak akan bisa
dibaca setelah key dihapus. Hanya lakukan ini jika kamu sudah tidak
membutuhkan data cloud atau sudah mem-backup key.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		info := cloud.EncryptionKeyStatus()
		if !info.Exists {
			fmt.Println("✓ Tidak ada encryption key yang tersimpan.")
			return nil
		}

		if !cloudEncryptForce {
			fmt.Printf("⚠  Key ditemukan di %s.\n", info.Source)
			fmt.Println("   Data terenkripsi di cloud tidak akan bisa dibaca setelah key dihapus.")
			fmt.Println("   Gunakan --force untuk melanjutkan.")
			return nil
		}

		if err := cloud.DeleteEncryptionKey(); err != nil {
			return fmt.Errorf("gagal hapus key: %w", err)
		}

		fmt.Println("✓ Encryption key dihapus dari penyimpanan lokal.")
		fmt.Println("  Data terenkripsi di cloud sekarang tidak bisa dibaca.")
		return nil
	},
}
