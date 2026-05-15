package agent

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/llm"
	smarassh "github.com/gede-cahya/Smara-CLI/internal/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRealSSH_SaveHostAndConnect(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	require.NoError(t, smarassh.EnsureDir())

	host := smarassh.Host{
		Name:    "vps-cahya",
		Address: "129.226.222.242",
		User:    "ubuntu",
		Port:    "22",
		KeyPath: "/home/cahya/Downloads/vpsCahya.pem",
	}
	require.NoError(t, smarassh.SaveHost(host))

	loaded, err := smarassh.GetHost("vps-cahya")
	require.NoError(t, err)
	assert.Equal(t, "129.226.222.242", loaded.Address)
	assert.Equal(t, "ubuntu", loaded.User)
	assert.Equal(t, "/home/cahya/Downloads/vpsCahya.pem", loaded.KeyPath)
	t.Logf("Host tersimpan: %s@%s (key: %s)", loaded.User, loaded.Address, loaded.KeyPath)
}

func TestRealSSH_ConnectAndUptime(t *testing.T) {
	if os.Getenv("SMARA_REAL_SSH") != "1" {
		t.Skip("Set SMARA_REAL_SSH=1 untuk test koneksi SSH real ke VPS")
	}

	host := &smarassh.Host{
		Name:    "vps-cahya",
		Address: "129.226.222.242",
		User:    "ubuntu",
		Port:    "22",
		KeyPath: "/home/cahya/Downloads/vpsCahya.pem",
	}

	client, err := smarassh.Connect(host)
	require.NoError(t, err, "gagal koneksi SSH ke %s@%s", host.User, host.Address)
	defer client.Close()

	stdout, stderr, err := client.Exec("uptime")
	require.NoError(t, err, "gagal eksekusi uptime")
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "load")
	t.Logf("uptime output: %s", stdout)
}

func TestRealSSH_Whoami(t *testing.T) {
	if os.Getenv("SMARA_REAL_SSH") != "1" {
		t.Skip("Set SMARA_REAL_SSH=1 untuk test koneksi SSH real ke VPS")
	}

	host := &smarassh.Host{
		Name:    "vps-cahya",
		Address: "129.226.222.242",
		User:    "ubuntu",
		Port:    "22",
		KeyPath: "/home/cahya/Downloads/vpsCahya.pem",
	}

	client, err := smarassh.Connect(host)
	require.NoError(t, err)
	defer client.Close()

	stdout, stderr, err := client.Exec("whoami")
	require.NoError(t, err)
	assert.Empty(t, stderr)
	assert.Equal(t, "ubuntu\n", stdout)
	t.Logf("whoami output: %q", stdout)
}

func TestRealLLM_Deepseek_SSHPrompt(t *testing.T) {
	if os.Getenv("SMARA_REAL_LLM") != "1" {
		t.Skip("Set SMARA_REAL_LLM=1 untuk test LLM real dengan deepseek-v4-pro")
	}

	t.Setenv("HOME", t.TempDir())
	require.NoError(t, smarassh.EnsureDir())

	host := smarassh.Host{
		Name:    "vps-cahya",
		Address: "129.226.222.242",
		User:    "ubuntu",
		Port:    "22",
		KeyPath: "/home/cahya/Downloads/vpsCahya.pem",
	}
	require.NoError(t, smarassh.SaveHost(host))

	provider, err := llm.NewProvider(llm.ProviderConfig{
		Name:   "custom",
		Model:  "deepseek-v4-flash",
		APIKey: "your-api-key-1",
		Host:   "http://localhost:8317/v1",
	})
	require.NoError(t, err, "gagal membuat provider custom deepseek")

	s := NewSupervisor(provider, nil)
	s.SetMode(ModeRush)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := s.ProcessPrompt(ctx, "cek status vps ubuntu pakai ssh")
	require.NoError(t, err, "ProcessPrompt gagal")

	t.Logf("Response: %s", result.Response)
	t.Logf("Thinking: %s", result.Thinking)
	t.Logf("Tools executed: %v", result.ToolsExecuted)
	t.Logf("Duration: %s", result.Duration)

	require.NotEmpty(t, result.Response)
}
