package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
)

// Version is set during release builds.
var version = "1.20.34"

func init() {
	// Inject version into agent system prompts so the LLM knows which version it runs.
	agent.AgentVersion = version
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print Smara version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("🌀 Smara v%s\n", version)
	},
}
