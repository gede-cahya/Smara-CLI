package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditFileFallsBackToUniqueMatchWhenLineRangeDrifts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.txt")
	initial := "alpha\nbeta\nneedle\nomega\n"
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ExecuteBuiltinTool("edit_file", map[string]interface{}{
		"path":        path,
		"old_content": "needle",
		"new_content": "replacement",
		"start_line":  float64(1),
		"end_line":    float64(2),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "range baris 1-2 tidak cocok") {
		t.Fatalf("expected drift note in result, got %q", result)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "alpha\nbeta\nreplacement\nomega\n"; got != want {
		t.Fatalf("file content = %q, want %q", got, want)
	}
}

func TestEditFileTreatsZeroEndLineAsOpenEnded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.txt")
	initial := "alpha\nbeta\nneedle\nomega\n"
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ExecuteBuiltinTool("edit_file", map[string]interface{}{
		"path":        path,
		"old_content": "needle",
		"new_content": "replacement",
		"start_line":  float64(1),
		"end_line":    float64(0),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "berhasil diperbarui") {
		t.Fatalf("expected success result, got %q", result)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "alpha\nbeta\nreplacement\nomega\n"; got != want {
		t.Fatalf("file content = %q, want %q", got, want)
	}
}

func TestEditFileRejectsEmptyOldContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.txt")
	if err := os.WriteFile(path, []byte("alpha\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ExecuteBuiltinTool("edit_file", map[string]interface{}{
		"path":        path,
		"old_content": "",
		"new_content": "replacement",
	}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "old_content tidak boleh kosong") {
		t.Fatalf("unexpected error: %v", err)
	}
}
