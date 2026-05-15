package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/runlog"
)

var runLimit int

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Lihat dan replay run Smara",
}

var runHistoryCmd = &cobra.Command{
	Use:     "history",
	Aliases: []string{"list", "ls"},
	Short:   "Tampilkan riwayat run",
	RunE: func(cmd *cobra.Command, args []string) error {
		runs, err := runlog.List(runLimit)
		if err != nil {
			return err
		}
		if len(runs) == 0 {
			fmt.Println("Belum ada run tersimpan.")
			return nil
		}
		fmt.Printf("%-18s %-10s %-10s %-22s %s\n", "ID", "STATUS", "KIND", "STARTED", "NAME")
		for _, r := range runs {
			fmt.Printf("%-18s %-10s %-10s %-22s %s\n", r.ID, r.Status, r.Kind, r.StartedAt.Format("2006-01-02 15:04:05"), r.Name)
		}
		return nil
	},
}

var runShowCmd = &cobra.Command{
	Use:   "show [id]",
	Short: "Tampilkan detail run",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := runlog.Load(args[0])
		if err != nil {
			return err
		}
		data, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	},
}

var runReplayCmd = &cobra.Command{
	Use:   "replay [id]",
	Short: "Jalankan ulang run yang didukung",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := runlog.Load(args[0])
		if err != nil {
			return err
		}
		switch r.Kind {
		case "workflow":
			metadata := map[string]string{"replay_of": r.ID, "replayed_at": time.Now().Format(time.RFC3339)}
			return runWorkflowCommand(r.Name, r.Project, metadata)
		default:
			return fmt.Errorf("run kind '%s' belum mendukung replay", r.Kind)
		}
	},
}

var runTimelineCmd = &cobra.Command{
	Use:   "timeline [id]",
	Short: "Tampilkan timeline event run",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := runlog.Load(args[0])
		if err != nil {
			return err
		}
		fmt.Printf("Run %s (%s/%s)\n", r.ID, r.Kind, r.Name)
		for _, event := range r.Events {
			line := fmt.Sprintf("%s  %-16s %s", event.Time.Format("15:04:05"), event.Type, event.Message)
			if len(event.Data) > 0 {
				var parts []string
				for k, v := range event.Data {
					parts = append(parts, fmt.Sprintf("%s=%s", k, v))
				}
				line += "  " + strings.Join(parts, " ")
			}
			fmt.Println(line)
		}
		return nil
	},
}

func init() {
	runHistoryCmd.Flags().IntVarP(&runLimit, "limit", "n", 20, "Jumlah run yang ditampilkan")
	runCmd.AddCommand(runHistoryCmd, runShowCmd, runReplayCmd, runTimelineCmd)
	rootCmd.AddCommand(runCmd)
}
