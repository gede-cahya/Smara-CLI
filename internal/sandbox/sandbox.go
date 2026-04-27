// Package sandbox provides isolated command execution with safety guards
// for Smara CLI's terminal operations.
package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Level represents the sandbox restriction level.
type Level string

const (
	LevelStrict     Level = "strict"     // Whitelist only, no file system write
	LevelNormal     Level = "normal"     // Blacklist dangerous commands, allow writes
	LevelPermissive Level = "permissive" // Minimal restrictions
)

// Result contains the output of a sandboxed command.
type Result struct {
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
	Blocked  bool          `json:"blocked"`
	Reason   string        `json:"reason,omitempty"`
}

// Sandbox provides isolated command execution.
type Sandbox struct {
	level           Level
	workingDir      string
	timeout         time.Duration
	allowedCommands []string
	blockedCommands []string
	envVars         map[string]string
	mu              sync.RWMutex
}

// Default blocked/dangerous commands.
var defaultBlockedCommands = []string{
	"rm -rf /", "rm -rf /*", ":(){ :|:& };:", "> /dev/sda",
	"mkfs", "dd if=/dev/zero", "chmod -R 777 /",
	"curl | sh", "wget | sh", "bash <(curl",
}

// New creates a new sandbox with default settings.
func New() *Sandbox {
	wd, _ := os.Getwd()
	return &Sandbox{
		level:           LevelNormal,
		workingDir:      wd,
		timeout:         30 * time.Second,
		blockedCommands: defaultBlockedCommands,
		allowedCommands: []string{},
		envVars:         make(map[string]string),
	}
}

// SetLevel sets the sandbox restriction level.
func (s *Sandbox) SetLevel(level Level) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.level = level
}

// SetTimeout sets the maximum execution time.
func (s *Sandbox) SetTimeout(timeout time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timeout = timeout
}

// SetWorkingDir sets the working directory for commands.
func (s *Sandbox) SetWorkingDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workingDir = dir
}

// AllowCommand adds a command to the whitelist (strict mode).
func (s *Sandbox) AllowCommand(cmd string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allowedCommands = append(s.allowedCommands, cmd)
}

// BlockCommand adds a command pattern to the blacklist.
func (s *Sandbox) BlockCommand(pattern string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blockedCommands = append(s.blockedCommands, pattern)
}

// Execute runs a command within the sandbox.
func (s *Sandbox) Execute(ctx context.Context, command string, args ...string) *Result {
	start := time.Now()

	// Validate command
	if blockReason := s.validateCommand(command, args); blockReason != "" {
		return &Result{
			Blocked:  true,
			Reason:   blockReason,
			ExitCode: -1,
			Duration: time.Since(start),
		}
	}

	// Prepare context with timeout
	s.mu.RLock()
	timeout := s.timeout
	wd := s.workingDir
	s.mu.RUnlock()

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build command
	cmd := exec.CommandContext(execCtx, command, args...)
	cmd.Dir = wd

	// Set environment
	cmd.Env = os.Environ()
	s.mu.RLock()
	for k, v := range s.envVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	s.mu.RUnlock()

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run
	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if execCtx.Err() == context.DeadlineExceeded {
			exitCode = -1
			return &Result{
				Stdout:   stdout.String(),
				Stderr:   "Error: command exceeded timeout",
				ExitCode: exitCode,
				Duration: duration,
			}
		} else {
			exitCode = -1
		}
	}

	return &Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Duration: duration,
	}
}

// ExecuteScript runs a shell script within the sandbox.
func (s *Sandbox) ExecuteScript(ctx context.Context, script string) *Result {
	// Validate script content
	if blockReason := s.validateScript(script); blockReason != "" {
		return &Result{
			Blocked:  true,
			Reason:   blockReason,
			ExitCode: -1,
		}
	}

	return s.Execute(ctx, "sh", "-c", script)
}

func (s *Sandbox) validateCommand(command string, args []string) string {
	s.mu.RLock()
	level := s.level
	blocked := make([]string, len(s.blockedCommands))
	copy(blocked, s.blockedCommands)
	allowed := make([]string, len(s.allowedCommands))
	copy(allowed, s.allowedCommands)
	s.mu.RUnlock()

	// In strict mode, only whitelisted commands allowed
	if level == LevelStrict {
		found := false
		for _, a := range allowed {
			if a == command {
				found = true
				break
			}
		}
		if !found {
			return fmt.Sprintf("command '%s' not in whitelist (strict mode)", command)
		}
	}

	// Check blocked patterns
	fullCmd := command + " " + strings.Join(args, " ")
	for _, pattern := range blocked {
		if strings.Contains(fullCmd, pattern) {
			return fmt.Sprintf("blocked dangerous pattern: '%s'", pattern)
		}
	}

	return ""
}

func (s *Sandbox) validateScript(script string) string {
	s.mu.RLock()
	blocked := make([]string, len(s.blockedCommands))
	copy(blocked, s.blockedCommands)
	s.mu.RUnlock()

	for _, pattern := range blocked {
		if strings.Contains(script, pattern) {
			return fmt.Sprintf("blocked dangerous pattern in script: '%s'", pattern)
		}
	}

	return ""
}

// IsSafePath checks if a path is within allowed boundaries.
func IsSafePath(path string, allowedDirs []string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	for _, dir := range allowedDirs {
		allowedAbs, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		if strings.HasPrefix(abs, allowedAbs) {
			return true
		}
	}
	return false
}

// WrapCommand wraps a command string for safe execution.
func WrapCommand(command string) string {
	// Escape common dangerous characters
	dangerous := []string{";", "&&", "||", "|", "`", "$", "<", ">"}
	for _, d := range dangerous {
		command = strings.ReplaceAll(command, d, "")
	}
	return strings.TrimSpace(command)
}
