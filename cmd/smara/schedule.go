package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/scheduler"
)

var scheduleDaemonInterval time.Duration

var scheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Jadwalkan workflow Smara",
}

var (
	scheduleRetriesFlag       int
	scheduleRetryIntervalFlag int
	scheduleDependsOnFlag     string
)

var scheduleAddCmd = &cobra.Command{
	Use:   "add [spec] [workflow]",
	Short: "Tambahkan jadwal workflow",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		job, err := scheduler.Add(args[0], args[1])
		if err != nil {
			return err
		}
		if scheduleRetriesFlag > 0 {
			job.MaxRetries = scheduleRetriesFlag
			job.RetryIntervalSec = scheduleRetryIntervalFlag
		}
		if scheduleDependsOnFlag != "" {
			job.DependsOn = scheduleDependsOnFlag
		}
		if scheduleRetriesFlag > 0 || scheduleDependsOnFlag != "" {
			jobs, _ := scheduler.List()
			for i, j := range jobs {
				if j.ID == job.ID {
					jobs[i] = job
					break
				}
			}
			_ = scheduler.SaveAll(jobs)
		}

		fmt.Printf("Schedule %s dibuat: %s -> %s (next: %s)\n", job.ID, job.Spec, job.Workflow, job.NextRunAt.Format(time.RFC3339))
		return nil
	},
}

var scheduleListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "Tampilkan jadwal workflow",
	RunE: func(cmd *cobra.Command, args []string) error {
		jobs, err := scheduler.List()
		if err != nil {
			return err
		}
		if len(jobs) == 0 {
			fmt.Println("Belum ada schedule.")
			return nil
		}
		fmt.Printf("%-14s %-16s %-20s %-22s %-12s %s\n", "ID", "SPEC", "WORKFLOW", "NEXT", "STATUS", "AFTER")
		for _, job := range jobs {
			last := "-"
			if job.LastRunAt != nil {
				last = job.LastRunAt.Format("2006-01-02 15:04") + " (" + job.LastStatus + ")"
			}
			after := "-"
			if job.DependsOn != "" {
				after = job.DependsOn
			}
			fmt.Printf("%-14s %-16s %-20s %-22s %-12s %s\n", job.ID, job.Spec, job.Workflow, job.NextRunAt.Format("2006-01-02 15:04:05"), last, after)
		}
		return nil
	},
}

var scheduleRemoveCmd = &cobra.Command{
	Use:   "remove [id]",
	Short: "Hapus jadwal workflow",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := scheduler.Remove(args[0]); err != nil {
			return err
		}
		fmt.Printf("Schedule %s dihapus.\n", args[0])
		return nil
	},
}

var scheduleRunDueCmd = &cobra.Command{
	Use:   "run-due",
	Short: "Jalankan workflow yang sudah jatuh tempo",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDueSchedules()
	},
}

var scheduleDaemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Jalankan scheduler loop di foreground",
	RunE: func(cmd *cobra.Command, args []string) error {
		if scheduleDaemonInterval < time.Minute {
			scheduleDaemonInterval = time.Minute
		}
		fmt.Printf("Scheduler daemon aktif. Interval: %s\n", scheduleDaemonInterval)
		ticker := time.NewTicker(scheduleDaemonInterval)
		defer ticker.Stop()
		for {
			if err := runDueSchedules(); err != nil {
				fmt.Printf("scheduler error: %v\n", err)
			}
			<-ticker.C
		}
	},
}

var scheduleServiceCmd = &cobra.Command{
	Use:   "service [install|status|uninstall]",
	Short: "Kelola Systemd background service untuk Smara Scheduler",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		action := args[0]
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		serviceDir := filepath.Join(homeDir, ".config", "systemd", "user")
		serviceFile := filepath.Join(serviceDir, "smara-scheduler.service")

		switch action {
		case "install":
			execPath, err := os.Executable()
			if err != nil {
				return err
			}
			_ = os.MkdirAll(serviceDir, 0755)
			content := fmt.Sprintf(`[Unit]
Description=Smara Autonomous Scheduler Daemon
After=network.target

[Service]
Type=simple
ExecStart=%s schedule daemon --interval 1m
Restart=always
RestartSec=10

[Install]
WantedBy=default.target
`, execPath)
			if err := os.WriteFile(serviceFile, []byte(content), 0644); err != nil {
				return fmt.Errorf("gagal buat service file: %w", err)
			}
			fmt.Println("✓ File systemd service dibuat:", serviceFile)
			fmt.Println("Aktifkan service dengan perintah:")
			fmt.Println("  systemctl --user daemon-reload")
			fmt.Println("  systemctl --user enable --now smara-scheduler")
			return nil

		case "status":
			if _, err := os.Stat(serviceFile); os.IsNotExist(err) {
				fmt.Println("Service smara-scheduler belum ter-install.")
				return nil
			}
			fmt.Println("✓ Service file ditemukan:", serviceFile)
			fmt.Println("Jalankan 'systemctl --user status smara-scheduler' untuk detail rincian.")
			return nil

		case "uninstall":
			_ = os.Remove(serviceFile)
			fmt.Println("✓ Systemd service smara-scheduler dihapus.")
			return nil
		default:
			return fmt.Errorf("aksi tidak valid: %s (gunakan install/status/uninstall)", action)
		}
	},
}

func runDueSchedules() error {
	jobs, err := scheduler.Due(time.Now())
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		fmt.Println("Tidak ada schedule jatuh tempo.")
		return nil
	}

	allJobs, _ := scheduler.List()
	jobMap := make(map[string]*scheduler.Job)
	for _, j := range allJobs {
		jobMap[j.ID] = j
	}

	var failed int
	for _, job := range jobs {
		// Check DAG / DependsOn dependency chaining
		if job.DependsOn != "" {
			parent, exists := jobMap[job.DependsOn]
			if exists && parent.LastStatus != "success" {
				fmt.Printf("Skipping schedule %s: parent job %s status is '%s'\n", job.ID, parent.ID, parent.LastStatus)
				continue
			}
		}

		fmt.Printf("Running schedule %s workflow=%s\n", job.ID, job.Workflow)
		metadata := map[string]string{"schedule_id": job.ID, "schedule_spec": job.Spec}
		status := "success"
		if err := runWorkflowCommand(job.Workflow, "", metadata); err != nil {
			status = "failed"
			failed++
			fmt.Printf("schedule %s failed: %v\n", job.ID, err)
		}
		if err := scheduler.MarkRun(job.ID, status, time.Now()); err != nil {
			return err
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d schedule gagal", failed)
	}
	return nil
}

func init() {
	scheduleAddCmd.Flags().IntVar(&scheduleRetriesFlag, "retries", 3, "Maksimal percobaan ulang jika gagal (default 3)")
	scheduleAddCmd.Flags().IntVar(&scheduleRetryIntervalFlag, "retry-interval", 10, "Jeda awal percobaan ulang dalam detik")
	scheduleAddCmd.Flags().StringVar(&scheduleDependsOnFlag, "after", "", "ID schedule induk yang harus sukses lebih dahulu")

	scheduleDaemonCmd.Flags().DurationVar(&scheduleDaemonInterval, "interval", time.Minute, "Interval cek schedule")
	scheduleCmd.AddCommand(scheduleAddCmd, scheduleListCmd, scheduleRemoveCmd, scheduleRunDueCmd, scheduleDaemonCmd, scheduleServiceCmd)
	rootCmd.AddCommand(scheduleCmd)
}
