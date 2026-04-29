package sandbox

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_Defaults(t *testing.T) {
	s := New()
	require.NotNil(t, s)
	assert.Equal(t, LevelNormal, s.level)
	assert.Equal(t, 30*time.Second, s.timeout)
}

func TestSetLevel(t *testing.T) {
	s := New()
	s.SetLevel(LevelStrict)
	assert.Equal(t, LevelStrict, s.level)
}

func TestExecute_SafeCommand(t *testing.T) {
	s := New()
	result := s.Execute(context.Background(), "echo", "hello")
	assert.False(t, result.Blocked)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "hello")
}

func TestExecute_BlockedDangerousPattern(t *testing.T) {
	s := New()
	result := s.Execute(context.Background(), "rm", "-rf", "/")
	assert.True(t, result.Blocked)
	assert.NotEmpty(t, result.Reason)
	assert.Equal(t, -1, result.ExitCode)
}

func TestExecute_StrictMode_Whitelist(t *testing.T) {
	s := New()
	s.SetLevel(LevelStrict)
	result := s.Execute(context.Background(), "echo", "hello")
	assert.True(t, result.Blocked)
	assert.Contains(t, result.Reason, "whitelist")

	s.AllowCommand("echo")
	result = s.Execute(context.Background(), "echo", "hello")
	assert.False(t, result.Blocked)
	assert.Equal(t, 0, result.ExitCode)
}

func TestExecute_Timeout(t *testing.T) {
	s := New()
	s.SetTimeout(100 * time.Millisecond)
	result := s.Execute(context.Background(), "sleep", "2")
	assert.False(t, result.Blocked)
	assert.Equal(t, -1, result.ExitCode)
	assert.Contains(t, result.Stderr, "timeout")
}

func TestExecute_NonExistentCommand(t *testing.T) {
	s := New()
	result := s.Execute(context.Background(), "cmd_nonexistent_12345")
	assert.False(t, result.Blocked)
	assert.Equal(t, -1, result.ExitCode)
}

func TestExecuteScript_Safe(t *testing.T) {
	s := New()
	result := s.ExecuteScript(context.Background(), "echo safe_script")
	assert.False(t, result.Blocked)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "safe_script")
}

func TestExecuteScript_Blocked(t *testing.T) {
	s := New()
	result := s.ExecuteScript(context.Background(), "rm -rf /")
	assert.True(t, result.Blocked)
	assert.Contains(t, result.Reason, "blocked")
}

func TestIsSafePath_Inside(t *testing.T) {
	assert.True(t, IsSafePath("/tmp/file.txt", []string{"/tmp"}))
}

func TestIsSafePath_Outside(t *testing.T) {
	assert.False(t, IsSafePath("/etc/passwd", []string{"/tmp"}))
}

func TestWrapCommand(t *testing.T) {
	clean := WrapCommand("ls; rm -rf /")
	assert.NotContains(t, clean, ";")
	assert.NotContains(t, clean, "&&")
	assert.NotContains(t, clean, "|")
}
