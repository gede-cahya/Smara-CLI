package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var version = "1.8.7"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Tampilkan versi Smara",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("🌀 Smara v%s\n", version)
	},
}
