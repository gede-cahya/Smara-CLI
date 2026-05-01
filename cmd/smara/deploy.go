package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/deploy"
	"github.com/gede-cahya/Smara-CLI/internal/ssh"
)

var (
	deployPlatformFlag string
	deployServiceUser  string
	deployWithConfig   bool
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy Smara sebagai systemd service di remote VPS",
	Long: `Kelola deployment Smara Bot Server sebagai systemd service di remote VPS
untuk operasi 24/7 dengan auto-restart.

Subcommands:
  install <host>      Generate systemd unit, upload via SSH, enable & start
  status <host>       Cek status service smara
  logs <host>         Lihat log service (journalctl)
  stop <host>         Hentikan service
  uninstall <host>    Hapus service dari VPS`,
}

var deployInstallCmd = &cobra.Command{
	Use:   "install <ssh-host>",
	Short: "Install Smara sebagai systemd service di remote host",
	Args:  cobra.ExactArgs(1),
	RunE:  runDeployInstall,
}

var deployStatusCmd = &cobra.Command{
	Use:   "status <ssh-host>",
	Short: "Cek status service Smara di remote host",
	Args:  cobra.ExactArgs(1),
	RunE:  runDeployStatus,
}

var deployLogsCmd = &cobra.Command{
	Use:   "logs <ssh-host>",
	Short: "Lihat log service Smara di remote host",
	Args:  cobra.ExactArgs(1),
	RunE:  runDeployLogs,
}

var deployStopCmd = &cobra.Command{
	Use:   "stop <ssh-host>",
	Short: "Hentikan service Smara di remote host",
	Args:  cobra.ExactArgs(1),
	RunE:  runDeployStop,
}

var deployUninstallCmd = &cobra.Command{
	Use:   "uninstall <ssh-host>",
	Short: "Hapus service Smara dari remote host",
	Args:  cobra.ExactArgs(1),
	RunE:  runDeployUninstall,
}

func resolveHost(name string) (*ssh.Host, error) {
	host, err := ssh.GetHost(name)
	if err != nil {
		if strings.Contains(name, "@") {
			parts := strings.SplitN(name, "@", 2)
			return &ssh.Host{
				Name:    name,
				User:    parts[0],
				Address: parts[1],
				Port:    "22",
			}, nil
		}
		return nil, fmt.Errorf("host '%s' tidak ditemukan. Gunakan 'smara ssh add-host' terlebih dahulu", name)
	}
	return host, nil
}

func connectAndRun(hostName string, action func(*ssh.Client) error) error {
	host, err := resolveHost(hostName)
	if err != nil {
		return err
	}

	fmt.Printf("Menghubungkan ke %s@%s:%s ...\n", host.User, host.Address, host.Port)
	client, err := ssh.Connect(host)
	if err != nil {
		return fmt.Errorf("gagal koneksi: %w", err)
	}
	defer client.Close()

	return action(client)
}

func runDeployInstall(cmd *cobra.Command, args []string) error {
	hostName := args[0]
	host, err := resolveHost(hostName)
	if err != nil {
		return err
	}

	fmt.Printf("Menghubungkan ke %s@%s:%s ...\n", host.User, host.Address, host.Port)
	client, err := ssh.Connect(host)
	if err != nil {
		return fmt.Errorf("gagal koneksi: %w", err)
	}
	defer client.Close()

	// 1. Detect or install smara binary
	fmt.Println("Mendeteksi binary smara di remote...")
	binPath, err := deploy.DetectSmaraBinary(client)
	if err != nil {
		return err
	}
	fmt.Printf("Binary ditemukan: %s\n", binPath)

	// 2. Determine platforms
	cfg := config.Get()
	var platforms []string
	if deployPlatformFlag != "" {
		for _, p := range strings.Split(deployPlatformFlag, ",") {
			p = strings.TrimSpace(strings.ToLower(p))
			if p != "" {
				platforms = append(platforms, p)
			}
		}
	} else {
		platforms = deploy.DeterminePlatforms(cfg)
	}
	if len(platforms) == 0 {
		return fmt.Errorf("tidak ada platform yang diaktifkan. Gunakan --platform atau atur di config")
	}
	fmt.Printf("Platform aktif: %s\n", strings.Join(platforms, ", "))

	// 3. Build ExecStart
	execStart := fmt.Sprintf("%s serve --platform %s", binPath, strings.Join(platforms, ","))

	// 4. Determine service user
	svcUser := deployServiceUser
	if svcUser == "" {
		svcUser = host.User
	}

	// 5. Determine working directory
	workingDir := fmt.Sprintf("/home/%s", svcUser)

	// 6. Generate and install unit
	serviceCfg := &deploy.ServiceConfig{
		User:       svcUser,
		WorkingDir: workingDir,
		ExecStart:  execStart,
	}

	fmt.Println("Menginstall systemd service...")
	var uploadConfigPath string
	if deployWithConfig {
		cfgFile := config.Get().DBPath
		// derive config path from DBPath: ~/.smara/memory.db -> ~/.smara/config.yaml
		cfgDir := filepath.Dir(cfgFile)
		localCfg := filepath.Join(cfgDir, "config.yaml")
		if _, err := os.Stat(localCfg); err == nil {
			uploadConfigPath = localCfg
			fmt.Printf("Mengupload config: %s\n", localCfg)
		} else {
			fmt.Printf("Peringatan: config file tidak ditemukan di %s\n", localCfg)
		}
	}

	if err := deploy.Install(client, serviceCfg, uploadConfigPath); err != nil {
		return fmt.Errorf("gagal install service: %w", err)
	}

	fmt.Println("Service smara berhasil diinstall dan diaktifkan.")
	fmt.Println("Gunakan 'smara deploy status", hostName, "' untuk memantau.")
	return nil
}

func runDeployStatus(cmd *cobra.Command, args []string) error {
	return connectAndRun(args[0], func(client *ssh.Client) error {
		fmt.Println("Mengecek status service...")
		stdout, stderr, err := deploy.Status(client)
		if stdout != "" {
			fmt.Println(stdout)
		}
		if stderr != "" {
			fmt.Fprintln(os.Stderr, stderr)
		}
		return err
	})
}

func runDeployLogs(cmd *cobra.Command, args []string) error {
	return connectAndRun(args[0], func(client *ssh.Client) error {
		fmt.Println("Mengambil log service...")
		stdout, stderr, err := deploy.Logs(client, 50)
		if stdout != "" {
			fmt.Println(stdout)
		}
		if stderr != "" {
			fmt.Fprintln(os.Stderr, stderr)
		}
		return err
	})
}

func runDeployStop(cmd *cobra.Command, args []string) error {
	return connectAndRun(args[0], func(client *ssh.Client) error {
		fmt.Println("Menghentikan service...")
		stdout, stderr, err := deploy.Stop(client)
		if stdout != "" {
			fmt.Println(stdout)
		}
		if stderr != "" {
			fmt.Fprintln(os.Stderr, stderr)
		}
		if err != nil {
			return err
		}
		fmt.Println("Service smara berhasil dihentikan.")
		return nil
	})
}

func runDeployUninstall(cmd *cobra.Command, args []string) error {
	return connectAndRun(args[0], func(client *ssh.Client) error {
		fmt.Println("Menghapus service...")
		stdout, stderr, err := deploy.Uninstall(client)
		if stdout != "" {
			fmt.Println(stdout)
		}
		if stderr != "" {
			fmt.Fprintln(os.Stderr, stderr)
		}
		return err
	})
}

func init() {
	// install flags
	deployInstallCmd.Flags().StringVar(&deployPlatformFlag, "platform", "", "Platform yang dijalankan (comma-separated: telegram,discord,whatsapp). Default: auto-detect dari config")
	deployInstallCmd.Flags().StringVar(&deployServiceUser, "user", "", "User untuk menjalankan service. Default: SSH user")
	deployInstallCmd.Flags().BoolVar(&deployWithConfig, "with-config", false, "Upload config lokal ke remote VPS")

	// logs flags
	deployLogsCmd.Flags().IntVar(&sshLogLimit, "lines", 50, "Jumlah baris log yang ditampilkan")

	// Register subcommands
	deployCmd.AddCommand(deployInstallCmd)
	deployCmd.AddCommand(deployStatusCmd)
	deployCmd.AddCommand(deployLogsCmd)
	deployCmd.AddCommand(deployStopCmd)
	deployCmd.AddCommand(deployUninstallCmd)
}
