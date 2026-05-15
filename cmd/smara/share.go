package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/agent/workflow"
	"github.com/gede-cahya/Smara-CLI/internal/sharing"
	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

var shareTeam string
var shareType string

var shareCmd = &cobra.Command{
	Use:   "share",
	Short: "Kelola metadata sharing resource Smara",
}

var shareSetCmd = &cobra.Command{
	Use:   "set [type] [name] [private|workspace|team]",
	Short: "Set visibility resource",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureShareTargetExists(args[0], args[1]); err != nil {
			return err
		}
		res, err := sharing.Set(args[0], args[1], sharing.Visibility(strings.ToLower(args[2])), shareTeam)
		if err != nil {
			return err
		}
		fmt.Printf("%s/%s visibility=%s", res.Type, res.Name, res.Visibility)
		if res.Team != "" {
			fmt.Printf(" team=%s", res.Team)
		}
		fmt.Println()
		return nil
	},
}

var shareShowCmd = &cobra.Command{
	Use:   "show [type] [name]",
	Short: "Tampilkan metadata sharing resource",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		res, err := sharing.Get(args[0], args[1])
		if err != nil {
			return err
		}
		fmt.Printf("Type: %s\nName: %s\nVisibility: %s\nWorkspace: %s\n", res.Type, res.Name, res.Visibility, res.Workspace)
		if res.Team != "" {
			fmt.Printf("Team: %s\n", res.Team)
		}
		return nil
	},
}

var shareListCmd = &cobra.Command{
	Use:   "list",
	Short: "Tampilkan semua metadata sharing",
	RunE: func(cmd *cobra.Command, args []string) error {
		resources, err := sharing.List(shareType)
		if err != nil {
			return err
		}
		fmt.Printf("Sharing metadata: %s\n", sharing.Path())
		if len(resources) == 0 {
			fmt.Println("Belum ada resource sharing metadata.")
			return nil
		}
		fmt.Printf("%-10s %-24s %-10s %-16s %s\n", "TYPE", "NAME", "VISIBILITY", "WORKSPACE", "TEAM")
		for _, res := range resources {
			fmt.Printf("%-10s %-24s %-10s %-16s %s\n", res.Type, res.Name, res.Visibility, res.Workspace, res.Team)
		}
		return nil
	},
}

func ensureShareTargetExists(resourceType, name string) error {
	switch strings.ToLower(resourceType) {
	case "skill":
		_, err := skill.Load(name)
		return err
	case "workflow":
		_, err := workflow.LoadCustomWorkflow(name)
		return err
	case "memory":
		return nil
	default:
		return fmt.Errorf("type harus skill, workflow, atau memory")
	}
}

func init() {
	shareSetCmd.Flags().StringVar(&shareTeam, "team", "", "Nama team untuk visibility=team")
	shareListCmd.Flags().StringVar(&shareType, "type", "", "Filter type: skill/workflow/memory")
	shareCmd.AddCommand(shareSetCmd, shareShowCmd, shareListCmd)
	rootCmd.AddCommand(shareCmd)
}
