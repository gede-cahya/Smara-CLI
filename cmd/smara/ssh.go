package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/ssh"
)

var (
	sshHost     string
	sshUser     string
	sshKey      string
	sshPassword string
	sshPort     string
	sshKeyName  string
	sshKeyType  string
	sshKeyBits  int
	sshLogLimit int
)

var sshCmd = &cobra.Command{
	Use:   "ssh",
	Short: "Manajemen VPS via SSH",
	Long: `Kelola host SSH, generate key pair, eksekusi perintah remote,
dan lihat log eksekusi langsung dari Smara.

Subcommands:
  add-host <nama>   Tambah host baru
  list              Daftar host tersimpan
  exec <nama> <cmd> Eksekusi perintah ke remote host
  connect <nama>    Buka sesi SSH interaktif
  keygen            Generate SSH key pair baru
  logs              Lihat riwayat eksekusi`,
}

var sshAddHostCmd = &cobra.Command{
	Use:   "add-host <nama>",
	Short: "Tambah host SSH baru",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if sshHost == "" || sshUser == "" {
			return fmt.Errorf("--host dan --user wajib diisi")
		}
		if sshPort == "" {
			sshPort = "22"
		}

		host := ssh.Host{
			Name:     args[0],
			Address:  sshHost,
			Port:     sshPort,
			User:     sshUser,
			KeyPath:  sshKey,
			Password: sshPassword,
		}

		if err := ssh.SaveHost(host); err != nil {
			return fmt.Errorf("gagal menyimpan host: %w", err)
		}

		fmt.Printf("Host '%s' berhasil disimpan: %s@%s:%s\n", host.Name, host.User, host.Address, host.Port)
		if host.KeyPath != "" {
			fmt.Printf("  Key: %s\n", host.KeyPath)
		}
		return nil
	},
}

var sshListCmd = &cobra.Command{
	Use:   "list",
	Short: "Daftar host SSH tersimpan",
	RunE: func(cmd *cobra.Command, args []string) error {
		hosts, err := ssh.LoadHosts()
		if err != nil {
			return fmt.Errorf("gagal membaca host: %w", err)
		}

		if len(hosts) == 0 {
			fmt.Println("Belum ada host SSH tersimpan.")
			fmt.Println("Gunakan: smara ssh add-host <nama> --host <ip> --user <user>")
			return nil
		}

		fmt.Printf("%-12s %-20s %-8s %-15s %-20s\n", "NAME", "ADDRESS", "PORT", "USER", "KEY")
		fmt.Println(strings.Repeat("-", 85))
		for _, h := range hosts {
			key := h.KeyPath
			if key == "" && h.Password != "" {
				key = "(password)"
			} else if key == "" {
				key = "(none)"
			}
			fmt.Printf("%-12s %-20s %-8s %-15s %-20s\n", h.Name, h.Address, h.Port, h.User, key)
		}
		return nil
	},
}

var sshExecCmd = &cobra.Command{
	Use:   "exec <nama> <command...>",
	Short: "Eksekusi perintah di remote host via SSH",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		hostName := args[0]
		command := strings.Join(args[1:], " ")

		var host *ssh.Host
		var err error

		// Coba ambil dari config
		host, err = ssh.GetHost(hostName)
		if err != nil {
			// Mungkin user memberikan user@host langsung
			if strings.Contains(hostName, "@") {
				parts := strings.SplitN(hostName, "@", 2)
				host = &ssh.Host{
					Name:    hostName,
					User:    parts[0],
					Address: parts[1],
					Port:    "22",
					KeyPath: sshKey,
				}
			} else {
				return fmt.Errorf("host '%s' tidak ditemukan. Gunakan 'smara ssh add-host' terlebih dahulu.", hostName)
			}
		}

		if sshKey != "" {
			host.KeyPath = sshKey
		}

		fmt.Printf("Menghubungkan ke %s@%s:%s ...\n", host.User, host.Address, host.Port)
		start := time.Now()

		client, err := ssh.Connect(host)
		if err != nil {
			return fmt.Errorf("gagal koneksi: %w", err)
		}
		defer client.Close()

		fmt.Printf("Menjalankan: %s\n", command)
		stdout, stderr, err := client.Exec(command)
		duration := time.Since(start).Milliseconds()

		// Log ke database
		cfg := config.Get()
		store, err := ssh.NewStore(cfg.DBPath)
		if err == nil {
			defer store.Close()
			status := "success"
			if err != nil {
				status = "error"
			}
			_ = store.SaveLog(ssh.LogEntry{
				HostName: host.Name,
				Address:  host.Address,
				Command:  command,
				Stdout:   stdout,
				Stderr:   stderr,
				Status:   status,
				Duration: duration,
			})
		}

		if stdout != "" {
			fmt.Println("\n--- stdout ---")
			fmt.Println(stdout)
		}
		if stderr != "" {
			fmt.Println("\n--- stderr ---")
			fmt.Println(stderr)
		}

		if err != nil {
			return fmt.Errorf("eksekusi gagal: %w", err)
		}

		fmt.Printf("\nSelesai dalam %d ms\n", duration)
		return nil
	},
}

var sshConnectCmd = &cobra.Command{
	Use:   "connect <nama>",
	Short: "Buka sesi SSH interaktif (one-shot session)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		hostName := args[0]
		host, err := ssh.GetHost(hostName)
		if err != nil {
			return err
		}
		if sshKey != "" {
			host.KeyPath = sshKey
		}

		fmt.Printf("Menghubungkan ke %s@%s:%s ...\n", host.User, host.Address, host.Port)
		client, err := ssh.Connect(host)
		if err != nil {
			return fmt.Errorf("gagal koneksi: %w", err)
		}
		defer client.Close()

		fmt.Println("Sesi SSH aktif. Ketik 'exit' untuk keluar.")
		return client.InteractiveSession(os.Stdin, os.Stdout, os.Stderr, 80, 24)
	},
}

var sshKeygenCmd = &cobra.Command{
	Use:   "keygen --name <nama> [--type ed25519|rsa] [--bits 4096]",
	Short: "Generate SSH key pair baru",
	RunE: func(cmd *cobra.Command, args []string) error {
		if sshKeyName == "" {
			return fmt.Errorf("--name wajib diisi (nama file key)")
		}
		if sshKeyType == "" {
			sshKeyType = "ed25519"
		}

		pubPath, privPath, err := ssh.GenerateKeyPair(sshKeyName, sshKeyType, sshKeyBits)
		if err != nil {
			return fmt.Errorf("gagal generate key: %w", err)
		}

		fmt.Printf("Key pair berhasil dibuat:\n")
		fmt.Printf("  Private: %s\n", privPath)
		fmt.Printf("  Public:  %s\n", pubPath)
		fmt.Println("\nUntuk deploy ke VPS, salin isi .pub ke ~/.ssh/authorized_keys di server.")
		return nil
	},
}

var sshLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Lihat riwayat eksekusi SSH",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Get()
		store, err := ssh.NewStore(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("gagal membuka database: %w", err)
		}
		defer store.Close()

		logs, err := store.ListLogs(sshLogLimit)
		if err != nil {
			return fmt.Errorf("gagal membaca logs: %w", err)
		}

		if len(logs) == 0 {
			fmt.Println("Belum ada log eksekusi SSH.")
			return nil
		}

		fmt.Printf("%-5s %-12s %-20s %-20s %-8s %-10s\n", "ID", "HOST", "COMMAND", "WAKTU", "STATUS", "DUR(ms)")
		fmt.Println(strings.Repeat("-", 90))
		for _, l := range logs {
			cmdPreview := l.Command
			if len(cmdPreview) > 20 {
				cmdPreview = cmdPreview[:17] + "..."
			}
			fmt.Printf("%-5d %-12s %-20s %-20s %-8s %-10d\n",
				l.ID, l.HostName, cmdPreview, l.CreatedAt.Format("02-01 15:04:05"), l.Status, l.Duration)
		}
		return nil
	},
}

func init() {
	// Flags for add-host
	sshAddHostCmd.Flags().StringVar(&sshHost, "host", "", "Alamat IP atau hostname VPS (wajib)")
	sshAddHostCmd.Flags().StringVar(&sshUser, "user", "", "Username SSH (wajib)")
	sshAddHostCmd.Flags().StringVar(&sshKey, "key", "", "Path ke private key")
	sshAddHostCmd.Flags().StringVar(&sshPassword, "password", "", "Password SSH (fallback)")
	sshAddHostCmd.Flags().StringVar(&sshPort, "port", "22", "Port SSH (default: 22)")

	// Flags for exec/connect override
	sshExecCmd.Flags().StringVar(&sshKey, "key", "", "Override private key path")
	sshConnectCmd.Flags().StringVar(&sshKey, "key", "", "Override private key path")

	// Flags for keygen
	sshKeygenCmd.Flags().StringVar(&sshKeyName, "name", "", "Nama file key (wajib)")
	sshKeygenCmd.Flags().StringVar(&sshKeyType, "type", "ed25519", "Tipe key: ed25519 atau rsa")
	sshKeygenCmd.Flags().IntVar(&sshKeyBits, "bits", 4096, "Bit size untuk RSA (default: 4096)")

	// Flags for logs
	sshLogsCmd.Flags().IntVar(&sshLogLimit, "limit", 50, "Jumlah log yang ditampilkan")

	// Register subcommands
	sshCmd.AddCommand(sshAddHostCmd)
	sshCmd.AddCommand(sshListCmd)
	sshCmd.AddCommand(sshExecCmd)
	sshCmd.AddCommand(sshConnectCmd)
	sshCmd.AddCommand(sshKeygenCmd)
	sshCmd.AddCommand(sshLogsCmd)
}
