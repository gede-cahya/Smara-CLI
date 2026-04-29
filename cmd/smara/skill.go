package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Kelola reusable automation skill",
	Long:  `Buat, jalankan, edit, dan hapus skill (resep automation yang tersimpan).`,
}

var skillRunArgs string

var skillRunCmd = &cobra.Command{
	Use:   "run [nama-skill]",
	Short: "Jalankan skill yang tersimpan",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		sk, err := skill.Load(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Skill '%s' tidak ditemukan.\n", name)
			os.Exit(1)
		}
		fmt.Printf("Menjalankan skill: %s\n", sk.Summary())
		// Actual execution requires a StepExecutor; in CLI we just print.
		fmt.Println("Gunakan TUI atau agent untuk eksekusi penuh.")
	},
}

var skillListCmd = &cobra.Command{
	Use:   "list",
	Short: "Daftar skill yang tersimpan",
	Run: func(cmd *cobra.Command, args []string) {
		names, err := skill.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Gagal list skill: %v\n", err)
			os.Exit(1)
		}
		if len(names) == 0 {
			fmt.Println("Belum ada skill tersimpan.")
			return
		}
		fmt.Println("Skill tersimpan:")
		for _, n := range names {
			sk, _ := skill.Load(n)
			if sk != nil {
				fmt.Printf("  - %s: %s\n", n, sk.Description)
			} else {
				fmt.Printf("  - %s\n", n)
			}
		}
	},
}

var skillDeleteCmd = &cobra.Command{
	Use:   "delete [nama-skill]",
	Short: "Hapus skill",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		if err := skill.Delete(name, nil); err != nil {
			fmt.Fprintf(os.Stderr, "Gagal hapus skill: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Skill '%s' dihapus.\n", name)
	},
}

var skillCreateCmd = &cobra.Command{
	Use:   "create [nama-skill]",
	Short: "Buat skill baru dari file JSON",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		// Read JSON from stdin
		fmt.Println("Tempel JSON skill (Ctrl+D untuk selesai):")
		var buf strings.Builder
		var b [1024]byte
		for {
			n, err := os.Stdin.Read(b[:])
			if n > 0 {
				buf.Write(b[:n])
			}
			if err != nil {
				break
			}
		}
		sk, err := skill.FromJSON([]byte(buf.String()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "JSON tidak valid: %v\n", err)
			os.Exit(1)
		}
		sk.Name = name
		if err := skill.Save(sk, nil); err != nil {
			fmt.Fprintf(os.Stderr, "Gagal simpan skill: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Skill '%s' tersimpan.\n", name)
	},
}

func init() {
	skillCmd.AddCommand(skillRunCmd, skillListCmd, skillDeleteCmd, skillCreateCmd)
	rootCmd.AddCommand(skillCmd)
}
