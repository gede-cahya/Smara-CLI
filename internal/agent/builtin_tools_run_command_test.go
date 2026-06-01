package agent

import (
	"strings"
	"testing"
)

func TestBuiltinRunCommandTimeoutSecAllowsLongerCommand(t *testing.T) {
	out, err := ExecuteBuiltinTool("run_command", map[string]interface{}{
		"command":     "sleep 1; echo done",
		"timeout_sec": 2,
	}, nil)
	if err != nil {
		t.Fatalf("ExecuteBuiltinTool(run_command) error = %v", err)
	}
	if !strings.Contains(out, "done") {
		t.Fatalf("ExecuteBuiltinTool(run_command) output = %q; want done", out)
	}
}

func TestBuiltinRunCommandTimeoutSecCanFailFast(t *testing.T) {
	_, err := ExecuteBuiltinTool("run_command", map[string]interface{}{
		"command":     "sleep 2; echo done",
		"timeout_sec": 1,
	}, nil)
	if err == nil {
		t.Fatal("ExecuteBuiltinTool(run_command) error = nil; want timeout")
	}
	if !strings.Contains(err.Error(), "timeout setelah 1 detik") {
		t.Fatalf("ExecuteBuiltinTool(run_command) error = %v; want 1 second timeout", err)
	}
}
