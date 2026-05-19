package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is set during release builds.
var version = "1.20.8"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print Smara version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("🌀 Smara v%s\n", version)
	},
}
