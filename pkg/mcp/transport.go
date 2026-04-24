package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

// StdioTransport manages JSON-RPC communication over stdin/stdout of a subprocess.
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex
}

// NewStdioTransport creates a new transport by spawning the given command.
func NewStdioTransport(command string, args []string, env map[string]string) (*StdioTransport, error) {
	cmd := exec.Command(command, args...)

	// Inherit system environment and overlay custom env vars
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("gagal membuat stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("gagal membuat stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("gagal menjalankan MCP server '%s': %w", command, err)
	}

	return &StdioTransport{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
	}, nil
}

// Send writes a JSON-RPC request to the subprocess stdin.
func (t *StdioTransport) Send(req *Request) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("gagal marshal request: %w", err)
	}

	// Write JSON followed by newline
	if _, err := t.stdin.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("gagal menulis ke stdin: %w", err)
	}

	return nil
}

// Receive reads a JSON-RPC response from the subprocess stdout.
func (t *StdioTransport) Receive() (*Response, error) {
	line, err := t.stdout.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("gagal membaca dari stdout: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("gagal parse response: %w", err)
	}

	return &resp, nil
}

// Close terminates the subprocess gracefully.
func (t *StdioTransport) Close() error {
	t.stdin.Close()
	return t.cmd.Wait()
}
