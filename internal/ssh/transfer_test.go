package ssh

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandTilde(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		input    string
		expected string
	}{
		{"~/test.txt", filepath.Join(home, "test.txt")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"~", "~"},
		{"", ""},
	}

	for _, tt := range tests {
		got := expandTilde(tt.input)
		assert.Equal(t, tt.expected, got)
	}
}

func TestShellEscape(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/simple/path", "/simple/path"},
		{"/path with spaces", "/path with spaces"},
		{"/path'with'quotes", "'/path'\"'\"'with'\"'\"'quotes'"},
		{"/path;special", "'/path;special'"},
		{"/path&char", "'/path&char'"},
	}

	for _, tt := range tests {
		got := shellescape(tt.input)
		assert.Equal(t, tt.expected, got)
	}
}

func TestStore_SaveAndListTransferLogs(t *testing.T) {
	dbPath := t.TempDir() + "/ssh.db"
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	entry := TransferLogEntry{
		HostName:   "web1",
		Address:    "192.168.1.1",
		LocalPath:  "/tmp/app.tar.gz",
		RemotePath: "/opt/app.tar.gz",
		Direction:  "upload",
		Bytes:      1024000,
		Method:     "sftp",
		Status:     "success",
		Duration:   1234,
	}
	require.NoError(t, store.SaveTransferLog(entry))

	logs, err := store.ListTransferLogs(10)
	require.NoError(t, err)
	require.Len(t, logs, 1)

	assert.Equal(t, "web1", logs[0].HostName)
	assert.Equal(t, "192.168.1.1", logs[0].Address)
	assert.Equal(t, "/tmp/app.tar.gz", logs[0].LocalPath)
	assert.Equal(t, "/opt/app.tar.gz", logs[0].RemotePath)
	assert.Equal(t, "upload", logs[0].Direction)
	assert.Equal(t, int64(1024000), logs[0].Bytes)
	assert.Equal(t, "sftp", logs[0].Method)
	assert.Equal(t, "success", logs[0].Status)
	assert.Equal(t, int64(1234), logs[0].Duration)
}

func TestStore_SaveTransferLog_Error(t *testing.T) {
	dbPath := t.TempDir() + "/ssh.db"
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	entry := TransferLogEntry{
		HostName:   "web1",
		Address:    "192.168.1.1",
		LocalPath:  "/tmp/missing",
		RemotePath: "/opt/missing",
		Direction:  "download",
		Bytes:      0,
		Method:     "scp",
		Status:     "error",
		ErrorMsg:   "file not found",
		Duration:   500,
	}
	require.NoError(t, store.SaveTransferLog(entry))

	logs, err := store.ListTransferLogs(10)
	require.NoError(t, err)
	require.Len(t, logs, 1)

	assert.Equal(t, "error", logs[0].Status)
	assert.Equal(t, "file not found", logs[0].ErrorMsg)
}

func TestStore_ListTransferLogs_DefaultLimit(t *testing.T) {
	dbPath := t.TempDir() + "/ssh.db"
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	for i := 0; i < 3; i++ {
		require.NoError(t, store.SaveTransferLog(TransferLogEntry{
			HostName: "h", Address: "a", Direction: "upload",
			LocalPath: "l", RemotePath: "r",
		}))
	}

	logs, err := store.ListTransferLogs(0) // should default to 50
	require.NoError(t, err)
	assert.Len(t, logs, 3)
}

func TestTransferResult_Struct(t *testing.T) {
	res := TransferResult{
		LocalPath:  "/tmp/file.txt",
		RemotePath: "/home/user/file.txt",
		Direction:  "upload",
		Bytes:      4096,
		Status:     "success",
	}
	assert.Equal(t, "upload", res.Direction)
	assert.Equal(t, int64(4096), res.Bytes)
	assert.Equal(t, "success", res.Status)
}

func TestTransferConfig_Defaults(t *testing.T) {
	cfg := TransferConfig{
		Method:    TransferMethodSFTP,
		Recursive: false,
	}
	assert.Equal(t, TransferMethodSFTP, cfg.Method)
	assert.False(t, cfg.Recursive)
	assert.False(t, cfg.PreservePerms)
}

func TestTransferMethod_Strings(t *testing.T) {
	assert.Equal(t, TransferMethod("auto"), TransferMethodAuto)
	assert.Equal(t, TransferMethod("sftp"), TransferMethodSFTP)
	assert.Equal(t, TransferMethod("scp"), TransferMethodSCP)
}
