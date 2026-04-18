package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Kelola konfigurasi Smara",
	Long:  "Lihat, atur, dan daftar konfigurasi Smara CLI.",
}

var configSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Atur nilai konfigurasi",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, value := args[0], args[1]
		if err := config.Set(key, value); err != nil {
			return fmt.Errorf("gagal menyimpan config: %w", err)
		}
		fmt.Printf("  ✓ %s = %s\n", key, value)
		return nil
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Tampilkan nilai konfigurasi",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		value := config.GetValue(args[0])
		if value == nil {
			fmt.Printf("  ⚠ Key '%s' tidak ditemukan\n", args[0])
			return
		}
		fmt.Printf("  %s = %v\n", args[0], value)
	},
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "Tampilkan semua konfigurasi",
	Aliases: []string{"ls"},
	Run: func(cmd *cobra.Command, args []string) {
		settings := config.AllSettings()
		fmt.Println()
		for k, v := range settings {
			fmt.Printf("  %s = %v\n", k, v)
		}
		fmt.Println()
	},
}

func init() {
	configCmd.AddCommand(configSetCmd, configGetCmd, configListCmd)
}
