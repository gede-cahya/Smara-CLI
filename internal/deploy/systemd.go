package deploy

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/ssh"
)

const unitTemplate = `[Unit]
Description=Smara Bot Server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User={{.User}}
WorkingDirectory={{.WorkingDir}}
ExecStart={{.ExecStart}}
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=smara

[Install]
WantedBy=multi-user.target
`

// ServiceConfig holds parameters for the systemd unit.
type ServiceConfig struct {
	User       string
	WorkingDir string
	ExecStart  string
}

// GenerateUnit renders the systemd unit file content.
func GenerateUnit(cfg *ServiceConfig) (string, error) {
	t := template.Must(template.New("unit").Parse(unitTemplate))
	var buf bytes.Buffer
	if err := t.Execute(&buf, cfg); err != nil {
		return "", fmt.Errorf("gagal render template: %w", err)
	}
	return buf.String(), nil
}

// DeterminePlatforms returns enabled platforms from local config.
func DeterminePlatforms(cfg *config.SmaraConfig) []string {
	var platforms []string
	if cfg.Platforms.Telegram.Enabled {
		platforms = append(platforms, "telegram")
	}
	if cfg.Platforms.Discord.Enabled {
		platforms = append(platforms, "discord")
	}
	if cfg.Platforms.WhatsApp.Enabled {
		platforms = append(platforms, "whatsapp")
	}
	return platforms
}

// writeTempUnit writes unit content to a local temp file and returns the path.
func writeTempUnit(content string) (string, error) {
	tmpFile, err := os.CreateTemp("", "smara-*.service")
	if err != nil {
		return "", fmt.Errorf("gagal membuat temp file: %w", err)
	}
	defer tmpFile.Close()
	if _, err := tmpFile.WriteString(content); err != nil {
		return "", fmt.Errorf("gagal menulis temp file: %w", err)
	}
	return tmpFile.Name(), nil
}

// Install installs the systemd service on a remote host.
func Install(client *ssh.Client, cfg *ServiceConfig, uploadConfigPath string) error {
	unitContent, err := GenerateUnit(cfg)
	if err != nil {
		return err
	}

	// Upload unit file via temp file + SFTP
	tmpPath, err := writeTempUnit(unitContent)
	if err != nil {
		return err
	}
	defer os.Remove(tmpPath)

	remoteTmp := "/tmp/smara.service"
	if _, err := ssh.UploadFile(client, tmpPath, remoteTmp, false); err != nil {
		return fmt.Errorf("gagal upload unit file: %w", err)
	}

	_, stderr, err := client.Exec("sudo mv /tmp/smara.service /etc/systemd/system/smara.service && sudo chmod 644 /etc/systemd/system/smara.service")
	if err != nil {
		return fmt.Errorf("gagal install unit file: %w (stderr: %s)", err, stderr)
	}

	// Optionally upload config
	if uploadConfigPath != "" {
		remoteCfgDir := fmt.Sprintf("/home/%s/.smara", cfg.User)
		_, stderr, err = client.Exec("mkdir -p " + remoteCfgDir)
		if err != nil {
			return fmt.Errorf("gagal buat remote config dir: %w (stderr: %s)", err, stderr)
		}
		remoteCfgPath := filepath.Join(remoteCfgDir, "config.yaml")
		if _, err := ssh.UploadFile(client, uploadConfigPath, remoteCfgPath, false); err != nil {
			return fmt.Errorf("gagal upload config: %w", err)
		}
	}

	// Reload systemd
	_, stderr, err = client.Exec("sudo systemctl daemon-reload")
	if err != nil {
		return fmt.Errorf("gagal daemon-reload: %w (stderr: %s)", err, stderr)
	}

	// Enable and start
	_, stderr, err = client.Exec("sudo systemctl enable --now smara")
	if err != nil {
		return fmt.Errorf("gagal enable/start service: %w (stderr: %s)", err, stderr)
	}

	return nil
}

// Status returns the service status.
func Status(client *ssh.Client) (string, string, error) {
	return client.Exec("sudo systemctl status smara --no-pager")
}

// Logs returns recent service logs.
func Logs(client *ssh.Client, lines int) (string, string, error) {
	if lines <= 0 {
		lines = 50
	}
	return client.Exec(fmt.Sprintf("sudo journalctl -u smara -n %d --no-pager", lines))
}

// Stop stops the service.
func Stop(client *ssh.Client) (string, string, error) {
	return client.Exec("sudo systemctl stop smara")
}

// Uninstall stops, disables, and removes the service.
func Uninstall(client *ssh.Client) (string, string, error) {
	_, stderr, err := client.Exec("sudo systemctl stop smara")
	if err != nil && !strings.Contains(stderr, "not loaded") && !strings.Contains(stderr, "inactive") {
		// ignore if already stopped or not loaded
	}

	_, stderr, err = client.Exec("sudo systemctl disable smara")
	if err != nil && !strings.Contains(stderr, "No such file") && !strings.Contains(stderr, "not loaded") {
		// ignore if not enabled
	}

	_, stderr, err = client.Exec("sudo rm -f /etc/systemd/system/smara.service")
	if err != nil {
		return "", stderr, fmt.Errorf("gagal hapus unit file: %w", err)
	}

	_, stderr, err = client.Exec("sudo systemctl daemon-reload")
	if err != nil {
		return "", stderr, fmt.Errorf("gagal daemon-reload: %w", err)
	}

	return "Service smara berhasil di-uninstall", "", nil
}

// DetectSmaraBinary finds or installs the smara binary on a remote host.
func DetectSmaraBinary(client *ssh.Client) (string, error) {
	// Check if smara is in PATH
	stdout, stderr, err := client.Exec("which smara")
	if err == nil && strings.TrimSpace(stdout) != "" {
		return strings.TrimSpace(stdout), nil
	}

	// Try go install
	_, stderr, err = client.Exec("go install github.com/gede-cahya/Smara-CLI@latest")
	if err != nil {
		return "", fmt.Errorf("smara tidak ditemukan dan go install gagal: %w (stderr: %s)", err, stderr)
	}

	// Check again
	stdout, _, err = client.Exec("which smara")
	if err == nil && strings.TrimSpace(stdout) != "" {
		return strings.TrimSpace(stdout), nil
	}

	// Fallback to common GOPATH location
	home, _, err := client.Exec("echo $HOME")
	if err != nil {
		return "", fmt.Errorf("gagal mendeteksi HOME: %w", err)
	}
	gopathBin := filepath.Join(strings.TrimSpace(home), "go", "bin", "smara")
	_, _, err = client.Exec("test -x " + gopathBin)
	if err == nil {
		return gopathBin, nil
	}

	return "", fmt.Errorf("smara binary tidak ditemukan di remote")
}

// EncodeForShell encodes a multi-line string for safe shell transport via base64.
func EncodeForShell(content string) string {
	return base64.StdEncoding.EncodeToString([]byte(content))
}
