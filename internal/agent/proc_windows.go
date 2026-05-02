//go:build windows

package agent

import (
	"os"
	"os/exec"
)

func setProcessGroup(cmd *exec.Cmd) {
	// No process group support on Windows in this way
}

func killProcessGroup(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}
