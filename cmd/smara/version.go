package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

const version = "1.5.0"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Tampilkan versi Smara",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("🌀 Smara v%s\n", version)
	},
}
