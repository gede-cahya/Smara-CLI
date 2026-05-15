package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/safety"
)

var policyAction string
var policyTarget string
var policyRisk string
var policyReason string

var policyCmd = &cobra.Command{
	Use:   "policy",
	Short: "Kelola policy eksekusi tool dan automation",
}

var policyListCmd = &cobra.Command{
	Use:   "list",
	Short: "Tampilkan policy rules",
	RunE: func(cmd *cobra.Command, args []string) error {
		policy, err := safety.LoadPolicy()
		if err != nil {
			return err
		}
		fmt.Printf("Policy: %s\n", safety.PolicyPath())
		if len(policy.Rules) == 0 {
			fmt.Println("Belum ada policy rule. Default: allow.")
			return nil
		}
		fmt.Printf("%-22s %-10s %-13s %-8s %s\n", "TOOL", "ACTION", "RISK", "DECISION", "TARGET")
		for _, rule := range policy.Rules {
			fmt.Printf("%-22s %-10s %-13s %-8s %s\n", rule.Tool, rule.Action, rule.Risk, rule.Decision, rule.Target)
		}
		return nil
	},
}

var policySetCmd = &cobra.Command{
	Use:   "set [tool] [allow|ask|deny]",
	Short: "Tambah atau ubah policy rule",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		decision := safety.Decision(strings.ToLower(args[1]))
		if decision != safety.DecisionAllow && decision != safety.DecisionAsk && decision != safety.DecisionDeny {
			return fmt.Errorf("decision harus allow, ask, atau deny")
		}
		risk := safety.RiskLevel(strings.ToLower(policyRisk))
		if risk == "" {
			risk = safety.RiskMedium
		}
		action := safety.ActionType(strings.ToLower(policyAction))
		policy, err := safety.LoadPolicy()
		if err != nil {
			return err
		}
		policy.UpsertRule(safety.PolicyRule{
			Tool:     args[0],
			Action:   action,
			Target:   policyTarget,
			Risk:     risk,
			Decision: decision,
			Reason:   policyReason,
		})
		if err := safety.SavePolicy(policy); err != nil {
			return err
		}
		fmt.Printf("Policy rule tersimpan: %s %s %s\n", args[0], action, decision)
		return nil
	},
}

var policyCheckCmd = &cobra.Command{
	Use:   "check [tool]",
	Short: "Cek keputusan policy untuk tool",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		policy, err := safety.LoadPolicy()
		if err != nil {
			return err
		}
		result := policy.Evaluate(safety.PolicyRequest{
			Tool:   args[0],
			Action: safety.ActionType(strings.ToLower(policyAction)),
			Target: policyTarget,
		})
		fmt.Printf("Decision: %s\nRisk: %s\n", result.Decision, result.Risk)
		if result.Reason != "" {
			fmt.Printf("Reason: %s\n", result.Reason)
		}
		return nil
	},
}

func init() {
	policySetCmd.Flags().StringVar(&policyAction, "action", "", "Action type: read/write/execute/delete")
	policySetCmd.Flags().StringVar(&policyTarget, "target", "", "Target substring opsional")
	policySetCmd.Flags().StringVar(&policyRisk, "risk", "medium", "Risk: low/medium/high/destructive")
	policySetCmd.Flags().StringVar(&policyReason, "reason", "", "Alasan policy")
	policyCheckCmd.Flags().StringVar(&policyAction, "action", "", "Action type: read/write/execute/delete")
	policyCheckCmd.Flags().StringVar(&policyTarget, "target", "", "Target yang dicek")
	policyCmd.AddCommand(policyListCmd, policySetCmd, policyCheckCmd)
	rootCmd.AddCommand(policyCmd)
}
