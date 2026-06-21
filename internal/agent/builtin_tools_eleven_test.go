package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func requireToolOK(t *testing.T, name string, args map[string]interface{}, want string) string {
	t.Helper()
	out, err := ExecuteBuiltinTool(name, args, nil)
	if err != nil {
		t.Fatalf("%s returned error: %v", name, err)
	}
	if want != "" && !strings.Contains(out, want) {
		t.Fatalf("%s output %q does not contain %q", name, out, want)
	}
	return out
}

func TestBuiltinToolsElevenNewToolsCLI(t *testing.T) {
	root := t.TempDir()

	src := filepath.Join(root, "a.txt")
	if err := os.WriteFile(src, []byte("alpha\nbeta\n"), 0644); err != nil {
		t.Fatal(err)
	}

	requireToolOK(t, "glob", map[string]interface{}{"pattern": "*.txt", "root": root, "limit": float64(10)}, "a.txt")

	copied := filepath.Join(root, "nested", "b.txt")
	requireToolOK(t, "copy_file", map[string]interface{}{"source": src, "destination": copied}, "berhasil disalin")
	if got, _ := os.ReadFile(copied); string(got) != "alpha\nbeta\n" {
		t.Fatalf("copy_file wrote %q", got)
	}

	renamed := filepath.Join(root, "nested", "c.txt")
	requireToolOK(t, "rename_file", map[string]interface{}{"old_path": copied, "new_path": renamed}, "berhasil dipindahkan")
	if _, err := os.Stat(renamed); err != nil {
		t.Fatalf("rename_file destination missing: %v", err)
	}

	requireToolOK(t, "get_file_info", map[string]interface{}{"path": renamed}, "Informasi file")
	requireToolOK(t, "apply_diff", map[string]interface{}{
		"path": renamed,
		"diff": "@@ -1,2 +1,2 @@\n-alpha\n+ALPHA\n beta",
	}, "Diff berhasil diterapkan")
	if got, _ := os.ReadFile(renamed); !strings.Contains(string(got), "ALPHA") {
		t.Fatalf("apply_diff result = %q", got)
	}

	py := filepath.Join(root, "ok.py")
	if err := os.WriteFile(py, []byte("print('ok')\n"), 0644); err != nil {
		t.Fatal(err)
	}
	requireToolOK(t, "get_diagnostics", map[string]interface{}{"path": py}, "Tidak ada errors")

	termOut := requireToolOK(t, "create_terminal", map[string]interface{}{
		"command":     "sleep 30",
		"working_dir": root,
	}, "PID:")
	re := regexp.MustCompile(`PID: (\d+)`)
	match := re.FindStringSubmatch(termOut)
	if len(match) != 2 {
		t.Fatalf("could not parse pid from %q", termOut)
	}
	requireToolOK(t, "kill_process", map[string]interface{}{"pid": mustFloatPID(t, match[1])}, "berhasil dihentikan")
	time.Sleep(50 * time.Millisecond)
}

func TestBuiltinToolsElevenNewGitToolsCLI(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test User")

	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("one\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "file.txt")
	requireToolOK(t, "git_commit", map[string]interface{}{"path": root, "message": "initial commit"}, "Commit berhasil")

	if err := os.WriteFile(file, []byte("two\n"), 0644); err != nil {
		t.Fatal(err)
	}
	requireToolOK(t, "get_git_status", map[string]interface{}{"path": root}, "Modified")
	requireToolOK(t, "git_diff", map[string]interface{}{"path": root}, "-one")
}

func mustFloatPID(t *testing.T, pid string) float64 {
	t.Helper()
	var n int
	if _, err := fmt.Sscanf(pid, "%d", &n); err != nil {
		t.Fatalf("bad pid %q: %v", pid, err)
	}
	return float64(n)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
