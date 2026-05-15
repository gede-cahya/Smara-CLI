package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	smarassh "github.com/gede-cahya/Smara-CLI/internal/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRealSupervisor(t *testing.T, mode Mode) *Supervisor {
	if os.Getenv("SMARA_REAL_LLM") != "1" {
		t.Skip("Set SMARA_REAL_LLM=1 untuk test LLM real")
	}

	provider, err := llm.NewProvider(llm.ProviderConfig{
		Name:   "custom",
		Model:  "deepseek-v4-flash",
		APIKey: "your-api-key-1",
		Host:   "http://localhost:8317/v1",
	})
	require.NoError(t, err, "gagal membuat provider")

	dbPath := filepath.Join(t.TempDir(), "test.db")
	memStore, err := memory.NewSQLiteStore(dbPath)
	require.NoError(t, err, "gagal membuat memory store")
	require.NoError(t, memStore.Init())

	s := NewSupervisorWithConfig(provider, llm.ProviderConfig{
		Name:   "custom",
		Model:  "deepseek-v4-flash",
		APIKey: "your-api-key-1",
		Host:   "http://localhost:8317/v1",
	}, memStore)
	s.SetMode(mode)
	return s
}

// ==================== SSH Tools ====================

func TestRealLLM_SSHExec(t *testing.T) {
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

	s := newRealSupervisor(t, ModeRush)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := s.ProcessPrompt(ctx, "cek status vps ubuntu pakai ssh")
	require.NoError(t, err)
	require.NotEmpty(t, result.Response)
	assert.Contains(t, result.ToolsExecuted, "ssh_exec")
	t.Logf("Response: %s", result.Response)
	t.Logf("Tools: %v", result.ToolsExecuted)
}

func TestRealLLM_SSHViewFile(t *testing.T) {
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

	s := newRealSupervisor(t, ModeAsk)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := s.ProcessPrompt(ctx, "lihat isi file /etc/os-release di vps cahya")
	require.NoError(t, err)
	require.NotEmpty(t, result.Response)
	assert.Contains(t, result.ToolsExecuted, "ssh_view_file")
	t.Logf("Response: %s", result.Response)
	t.Logf("Tools: %v", result.ToolsExecuted)
}

func TestRealLLM_SSHListDir(t *testing.T) {
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

	s := newRealSupervisor(t, ModeAsk)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := s.ProcessPrompt(ctx, "list direktori /var/log di vps")
	require.NoError(t, err)
	require.NotEmpty(t, result.Response)
	assert.Contains(t, result.ToolsExecuted, "ssh_list_dir")
	t.Logf("Response: %s", result.Response)
	t.Logf("Tools: %v", result.ToolsExecuted)
}

// ==================== File/Terminal Tools ====================

func TestRealLLM_RunCommand(t *testing.T) {
	s := newRealSupervisor(t, ModeRush)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := s.ProcessPrompt(ctx, "jalankan perintah echo hello world")
	require.NoError(t, err)
	require.NotEmpty(t, result.Response)
	assert.Contains(t, result.ToolsExecuted, "run_command")
	t.Logf("Response: %s", result.Response)
	t.Logf("Tools: %v", result.ToolsExecuted)
}

func TestRealLLM_ReadFile(t *testing.T) {
	s := newRealSupervisor(t, ModeAsk)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := s.ProcessPrompt(ctx, "baca file go.mod di project ini")
	require.NoError(t, err)
	require.NotEmpty(t, result.Response)
	assert.Contains(t, result.ToolsExecuted, "read_file")
	t.Logf("Response: %s", result.Response)
	t.Logf("Tools: %v", result.ToolsExecuted)
}

func TestRealLLM_ViewFile(t *testing.T) {
	s := newRealSupervisor(t, ModeAsk)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := s.ProcessPrompt(ctx, "lihat isi file internal/llm/types.go baris 1 sampai 20")
	require.NoError(t, err)
	require.NotEmpty(t, result.Response)
	assert.Contains(t, result.ToolsExecuted, "view_file")
	t.Logf("Response: %s", result.Response)
	t.Logf("Tools: %v", result.ToolsExecuted)
}

func TestRealLLM_ListDir(t *testing.T) {
	s := newRealSupervisor(t, ModeAsk)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := s.ProcessPrompt(ctx, "tampilkan isi folder cmd/")
	require.NoError(t, err)
	require.NotEmpty(t, result.Response)
	assert.Contains(t, result.ToolsExecuted, "list_dir")
	t.Logf("Response: %s", result.Response)
	t.Logf("Tools: %v", result.ToolsExecuted)
}

func TestRealLLM_GrepSearch(t *testing.T) {
	s := newRealSupervisor(t, ModeAsk)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := s.ProcessPrompt(ctx, "cari file yang ada fungsi Connect di folder internal/")
	require.NoError(t, err)
	require.NotEmpty(t, result.Response)
	assert.Contains(t, result.ToolsExecuted, "grep_search")
	t.Logf("Response: %s", result.Response)
	t.Logf("Tools: %v", result.ToolsExecuted)
}

func TestRealLLM_WriteFile(t *testing.T) {
	s := newRealSupervisor(t, ModeRush)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := s.ProcessPrompt(ctx, "buat file /tmp/smara_test_prompt.txt dengan isi 'hello from prompt test'")
	require.NoError(t, err)
	require.NotEmpty(t, result.Response)
	assert.Contains(t, result.ToolsExecuted, "write_file")
	t.Logf("Response: %s", result.Response)
	t.Logf("Tools: %v", result.ToolsExecuted)

	// Verify file was actually written
	content, err := os.ReadFile("/tmp/smara_test_prompt.txt")
	require.NoError(t, err)
	assert.Contains(t, string(content), "hello from prompt test")
}

func TestRealLLM_AnalyzeWorkspace(t *testing.T) {
	s := newRealSupervisor(t, ModeAsk)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := s.ProcessPrompt(ctx, "analisis struktur project ini")
	require.NoError(t, err)
	require.NotEmpty(t, result.Response)
	assert.Contains(t, result.ToolsExecuted, "analyze_workspace")
	t.Logf("Response: %s", result.Response)
	t.Logf("Tools: %v", result.ToolsExecuted)
}

// ==================== Research/Memory Tools ====================

func TestRealLLM_WebSearch(t *testing.T) {
	s := newRealSupervisor(t, ModeAsk)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := s.ProcessPrompt(ctx, "cari info tentang golang context package di internet")
	require.NoError(t, err)
	require.NotEmpty(t, result.Response)
	assert.Contains(t, result.ToolsExecuted, "web_search")
	t.Logf("Response: %s", result.Response)
	t.Logf("Tools: %v", result.ToolsExecuted)
}

func TestRealLLM_Remember(t *testing.T) {
	s := newRealSupervisor(t, ModeRush)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := s.ProcessPrompt(ctx, "ingat bahwa saya suka tema gelap")
	require.NoError(t, err)
	require.NotEmpty(t, result.Response)
	assert.Contains(t, result.ToolsExecuted, "remember")
	t.Logf("Response: %s", result.Response)
	t.Logf("Tools: %v", result.ToolsExecuted)
}

func TestRealLLM_SearchMemories(t *testing.T) {
	s := newRealSupervisor(t, ModeAsk)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// First seed a memory
	_, err := s.ProcessPrompt(ctx, "ingat bahwa saya suka tema gelap")
	require.NoError(t, err)

	// Now search for it
	ctx2, cancel2 := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel2()
	result, err := s.ProcessPrompt(ctx2, "apa yang pernah saya minta simpan tentang tema?")
	require.NoError(t, err)
	require.NotEmpty(t, result.Response)
	assert.Contains(t, result.ToolsExecuted, "search_memories")
	t.Logf("Response: %s", result.Response)
	t.Logf("Tools: %v", result.ToolsExecuted)
}

// ==================== Management Tools ====================

func TestRealLLM_UserModel(t *testing.T) {
	s := newRealSupervisor(t, ModeRush)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := s.ProcessPrompt(ctx, "atur verbosity ke low")
	require.NoError(t, err)
	require.NotEmpty(t, result.Response)
	assert.Contains(t, result.ToolsExecuted, "user_model")
	t.Logf("Response: %s", result.Response)
	t.Logf("Tools: %v", result.ToolsExecuted)
}

func TestRealLLM_ScheduleReminder(t *testing.T) {
	s := newRealSupervisor(t, ModeRush)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := s.ProcessPrompt(ctx, "ingatkan saya cek wa tiap jam")
	require.NoError(t, err)
	require.NotEmpty(t, result.Response)
	assert.Contains(t, result.ToolsExecuted, "schedule_reminder")
	t.Logf("Response: %s", result.Response)
	t.Logf("Tools: %v", result.ToolsExecuted)
}

// ==================== Multi-step Test ====================

func TestRealLLM_MultiStep(t *testing.T) {
	s := newRealSupervisor(t, ModeRush)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	result, err := s.ProcessPrompt(ctx, "analisis project ini lalu simpan ke memori bahwa ini project Go CLI")
	require.NoError(t, err)
	require.NotEmpty(t, result.Response)

	// Should have called analyze_workspace and remember
	hasAnalyze := false
	hasRemember := false
	for _, tool := range result.ToolsExecuted {
		if tool == "analyze_workspace" {
			hasAnalyze = true
		}
		if tool == "remember" {
			hasRemember = true
		}
	}
	assert.True(t, hasAnalyze, "seharusnya memanggil analyze_workspace")
	assert.True(t, hasRemember, "seharusnya memanggil remember")
	t.Logf("Response: %s", result.Response)
	t.Logf("Tools: %v", result.ToolsExecuted)
}
