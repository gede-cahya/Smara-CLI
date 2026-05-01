package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeBinary_ELF(t *testing.T) {
	// Test against the built smara binary (likely ELF on Linux)
	binPath := filepath.Join("..", "..", "bin", "smara")
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		t.Skip("no binary found at ../../bin/smara")
	}

	res, err := analyzeBinaryFile(binPath)
	if err != nil {
		t.Fatalf("analyzeBinaryFile failed: %v", err)
	}
	if !strings.Contains(res, "Binary Analysis Report") {
		t.Error("expected report header")
	}
	if !strings.Contains(res, "ELF") && !strings.Contains(res, "PE") && !strings.Contains(res, "Mach-O") {
		t.Logf("format detection may have missed (output: %s)", res)
	}
	if !strings.Contains(res, "Entropy") {
		t.Error("expected entropy section")
	}
}

func TestExtractStrings_Self(t *testing.T) {
	// Test on a text file (README)
	readme := filepath.Join("..", "..", "README.md")
	res, err := extractStringsFromFile(readme, 4, 20)
	if err != nil {
		t.Fatalf("extractStringsFromFile failed: %v", err)
	}
	if !strings.Contains(res, "Extracted Strings") {
		t.Error("expected extracted strings header")
	}
	if strings.Count(res, "\n") < 3 {
		t.Error("expected at least a few strings extracted")
	}
}

func TestScanSignature_TextFile(t *testing.T) {
	readme := filepath.Join("..", "..", "README.md")
	res, err := scanSignature(readme, []string{"Smara", "regex:https?://[a-zA-Z0-9.-]+"})
	if err != nil {
		t.Fatalf("scanSignature failed: %v", err)
	}
	if !strings.Contains(res, "Signature Scan Report") {
		t.Error("expected scan report header")
	}
	if !strings.Contains(res, "Occurrences:") {
		t.Error("expected occurrences count")
	}
}

func TestAnalyzeDependencies_Self(t *testing.T) {
	agentDir := filepath.Join("..", "..", "internal", "agent")
	res, err := analyzeDependencies(agentDir, "go")
	if err != nil {
		t.Fatalf("analyzeDependencies failed: %v", err)
	}
	if !strings.Contains(res, "Dependency Analysis") {
		t.Error("expected dependency analysis header")
	}
	if !strings.Contains(res, "Summary:") {
		t.Error("expected summary section")
	}
}

func TestGenerateCallGraph_Self(t *testing.T) {
	agentDir := filepath.Join("..", "..", "internal", "agent")
	res, err := generateCallGraph(agentDir, "go", 3)
	if err != nil {
		t.Fatalf("generateCallGraph failed: %v", err)
	}
	if !strings.Contains(res, "Static Call Graph Outline") {
		t.Error("expected call graph header")
	}
	if !strings.Contains(res, "Summary:") {
		t.Error("expected summary section")
	}
}

func TestAnalyzeBinary_DirectoryError(t *testing.T) {
	_, err := analyzeBinaryFile("..")
	if err == nil {
		t.Error("expected error for directory")
	}
}

func TestExtractStrings_DirectoryError(t *testing.T) {
	_, err := extractStringsFromFile("..", 4, 10)
	if err == nil {
		t.Error("expected error for directory")
	}
}

func TestScanSignature_DirectoryError(t *testing.T) {
	_, err := scanSignature("..", []string{"test"})
	if err == nil {
		t.Error("expected error for directory")
	}
}

func TestAnalyzeDependencies_SingleFile(t *testing.T) {
	f := filepath.Join("builtin_tools.go")
	res, err := analyzeDependencies(f, "go")
	if err != nil {
		t.Fatalf("analyzeDependencies on single file failed: %v", err)
	}
	if !strings.Contains(res, "Dependency Analysis") {
		t.Error("expected header")
	}
}
