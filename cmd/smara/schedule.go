package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/scheduler"
)

var scheduleDaemonInterval time.Duration

var scheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Jadwalkan workflow Smara",
}

var scheduleAddCmd = &cobra.Command{
	Use:   "add [spec] [workflow]",
	Short: "Tambahkan jadwal workflow",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		job, err := scheduler.Add(args[0], args[1])
		if err != nil {
			return err
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
		fmt.Printf("%-14s %-16s %-20s %-22s %s\n", "ID", "SPEC", "WORKFLOW", "NEXT", "LAST")
		for _, job := range jobs {
			last := "-"
			if job.LastRunAt != nil {
				last = job.LastRunAt.Format("2006-01-02 15:04") + " " + job.LastStatus
			}
			fmt.Printf("%-14s %-16s %-20s %-22s %s\n", job.ID, job.Spec, job.Workflow, job.NextRunAt.Format("2006-01-02 15:04:05"), last)
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

func runDueSchedules() error {
	jobs, err := scheduler.Due(time.Now())
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		fmt.Println("Tidak ada schedule jatuh tempo.")
		return nil
	}
	var failed int
	for _, job := range jobs {
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
	scheduleDaemonCmd.Flags().DurationVar(&scheduleDaemonInterval, "interval", time.Minute, "Interval cek schedule")
	scheduleCmd.AddCommand(scheduleAddCmd, scheduleListCmd, scheduleRemoveCmd, scheduleRunDueCmd, scheduleDaemonCmd)
	rootCmd.AddCommand(scheduleCmd)
}
