package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSplitToolList(t *testing.T) {
	got := splitToolList("alpha, beta,,gamma ")
	want := []string{"alpha", "beta", "gamma"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLoadSkillLintToolFileArrayAndObject(t *testing.T) {
	dir := t.TempDir()

	arrayPath := filepath.Join(dir, "tools-array.json")
	if err := os.WriteFile(arrayPath, []byte(`["alpha_tool","beta_tool"]`), 0644); err != nil {
		t.Fatal(err)
	}
	arrayTools, err := loadSkillLintToolFile(arrayPath)
	if err != nil {
		t.Fatalf("array registry error: %v", err)
	}
	if len(arrayTools) != 2 || arrayTools[0] != "alpha_tool" || arrayTools[1] != "beta_tool" {
		t.Fatalf("array tools = %#v", arrayTools)
	}

	objectPath := filepath.Join(dir, "tools-object.json")
	if err := os.WriteFile(objectPath, []byte(`{"tools":["gamma_tool"]}`), 0644); err != nil {
		t.Fatal(err)
	}
	objectTools, err := loadSkillLintToolFile(objectPath)
	if err != nil {
		t.Fatalf("object registry error: %v", err)
	}
	if len(objectTools) != 1 || objectTools[0] != "gamma_tool" {
		t.Fatalf("object tools = %#v", objectTools)
	}
}

func TestSkillKnownToolsIncludesDefaultsAndFlags(t *testing.T) {
	oldAllowTools := skillLintAllowTools
	oldToolFile := skillLintToolFile
	t.Cleanup(func() {
		skillLintAllowTools = oldAllowTools
		skillLintToolFile = oldToolFile
	})

	dir := t.TempDir()
	registryPath := filepath.Join(dir, "tools.json")
	if err := os.WriteFile(registryPath, []byte(`{"tools":["registry_tool"]}`), 0644); err != nil {
		t.Fatal(err)
	}

	skillLintAllowTools = []string{"flag_tool, second_flag_tool"}
	skillLintToolFile = registryPath

	known, err := skillKnownTools()
	if err != nil {
		t.Fatalf("skillKnownTools error: %v", err)
	}
	for _, tool := range []string{"get-library-documentation", "flag_tool", "second_flag_tool", "registry_tool"} {
		if !known[tool] {
			t.Fatalf("known[%q] = false", tool)
		}
	}
}
