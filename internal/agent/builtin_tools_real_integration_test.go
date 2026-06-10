//go:build integration

package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/memory"
)

func requireBuiltinSuccess(
	t *testing.T,
	name string,
	args map[string]interface{},
	contains string,
) string {
	t.Helper()
	output, err := ExecuteBuiltinTool(name, args, nil)
	if err != nil {
		t.Fatalf("%s failed: %v", name, err)
	}
	if contains != "" && !strings.Contains(output, contains) {
		t.Fatalf("%s output %q does not contain %q", name, output, contains)
	}
	return output
}

func TestBuiltinToolsRealCatalogExposure(t *testing.T) {
	realTested := []string{
		"run_command",
		"write_file",
		"read_file",
		"view_file",
		"edit_file",
		"grep_search",
		"search_path",
		"list_dir",
		"get_cwd",
		"analyze_workspace",
		"export_data",
		"planning_template",
		"delete_file",
		"graphify_init",
		"graphify_query",
		"schedule_reminder",
		"read_document",
		"extract_strings",
		"scan_signature",
		"analyze_dependencies",
		"generate_call_graph",
		"analyze_binary",
	}

	exposed := make(map[string]bool)
	for _, tool := range GetBuiltinTools() {
		exposed[tool.Name] = true
	}
	for _, name := range realTested {
		if !exposed[name] {
			t.Errorf("real-tested builtin tool %q is not exposed to the agent", name)
		}
	}

	filtered := GetBuiltinToolsFiltered([]string{"binary", "graphify"})
	for _, tool := range filtered {
		if toolGroup[tool.Name] == "binary" || toolGroup[tool.Name] == "graphify" {
			t.Errorf("disabled tool group still exposes %q", tool.Name)
		}
	}
}

func TestBuiltinToolsRealCoreWorkflow(t *testing.T) {
	root := t.TempDir()
	notePath := filepath.Join(root, "notes", "sample.txt")

	requireBuiltinSuccess(t, "run_command", map[string]interface{}{
		"command": "printf builtin-real-ok",
	}, "builtin-real-ok")
	requireBuiltinSuccess(t, "write_file", map[string]interface{}{
		"path":    notePath,
		"content": "alpha\nneedle\nomega\n",
	}, "berhasil ditulis")
	requireBuiltinSuccess(t, "read_file", map[string]interface{}{
		"path": notePath,
	}, "needle")
	requireBuiltinSuccess(t, "view_file", map[string]interface{}{
		"path":       notePath,
		"start_line": float64(2),
		"end_line":   float64(2),
	}, "2 | needle")
	requireBuiltinSuccess(t, "edit_file", map[string]interface{}{
		"path":        notePath,
		"old_content": "needle",
		"new_content": "replacement",
	}, "berhasil diperbarui")
	requireBuiltinSuccess(t, "grep_search", map[string]interface{}{
		"query": "replacement",
		"path":  root,
	}, "sample.txt")
	requireBuiltinSuccess(t, "search_path", map[string]interface{}{
		"query": "sample.txt",
		"root":  root,
	}, "sample.txt")
	requireBuiltinSuccess(t, "list_dir", map[string]interface{}{
		"path": filepath.Dir(notePath),
	}, "sample.txt")
	requireBuiltinSuccess(t, "get_cwd", map[string]interface{}{}, "")

	originalCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalCWD) })
	requireBuiltinSuccess(t, "analyze_workspace", map[string]interface{}{
		"depth": float64(3),
	}, "sample.txt")

	exportPath := filepath.Join(root, "report.json")
	requireBuiltinSuccess(t, "export_data", map[string]interface{}{
		"format": "json",
		"path":   exportPath,
		"data": []interface{}{
			map[string]interface{}{"status": "ok", "tool": "builtin"},
		},
	}, "report.json")
	requireBuiltinSuccess(t, "planning_template", map[string]interface{}{
		"kind": "test-plan",
		"goal": "Verify real built-in workflow",
	}, "Golden path")
	requireBuiltinSuccess(t, "delete_file", map[string]interface{}{
		"path": notePath,
	}, "berhasil dihapus")

	if _, err := os.Stat(notePath); !os.IsNotExist(err) {
		t.Fatalf("delete_file did not remove %s", notePath)
	}
}

func TestBuiltinToolsRealGraphifyWorkflow(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "sample.go")
	source := "package sample\n\nfunc Hello() string { return \"hello\" }\n"
	if err := os.WriteFile(sourcePath, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := memory.NewSQLiteStore(filepath.Join(root, "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	previousDB := BuiltinDB
	BuiltinDB = store.DB()
	t.Cleanup(func() { BuiltinDB = previousDB })

	requireBuiltinSuccess(t, "graphify_init", map[string]interface{}{
		"path": root,
		"name": "real-builtins",
	}, "Graph 'real-builtins' dibuat")
	requireBuiltinSuccess(t, "graphify_query", map[string]interface{}{
		"graph_name": "real-builtins",
		"query":      "Hello",
		"depth":      float64(1),
	}, "Hello")
	requireBuiltinSuccess(t, "schedule_reminder", map[string]interface{}{
		"prompt_text": "Verify real built-in workflow",
		"when":        "tomorrow",
	}, "Reminder tersimpan")
}

func TestBuiltinToolsRealAnalysisWorkflow(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "sample.go")
	source := `package sample

import "fmt"

func helper() string { return "builtin-analysis-marker" }
func Hello() { fmt.Println(helper()) }
`
	if err := os.WriteFile(sourcePath, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	requireBuiltinSuccess(t, "read_document", map[string]interface{}{
		"path":      sourcePath,
		"max_chars": float64(5000),
	}, "builtin-analysis-marker")
	requireBuiltinSuccess(t, "extract_strings", map[string]interface{}{
		"file_path":   sourcePath,
		"min_length":  float64(8),
		"max_results": float64(100),
	}, "builtin-analysis-marker")
	requireBuiltinSuccess(t, "scan_signature", map[string]interface{}{
		"file_path": sourcePath,
		"patterns": []interface{}{
			"builtin-analysis-marker",
			"regex:fmt\\.Println",
		},
	}, "builtin-analysis-marker")
	requireBuiltinSuccess(t, "analyze_dependencies", map[string]interface{}{
		"source_path": root,
		"language":    "go",
	}, "fmt")
	requireBuiltinSuccess(t, "generate_call_graph", map[string]interface{}{
		"source_path": root,
		"language":    "go",
		"max_depth":   float64(3),
	}, "Hello")
	requireBuiltinSuccess(t, "analyze_binary", map[string]interface{}{
		"file_path": "/bin/sh",
	}, "Binary Analysis Report")
}
