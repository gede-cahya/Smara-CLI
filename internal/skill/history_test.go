package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func withTempHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	old := os.Getenv("HOME")
	t.Setenv("HOME", dir)
	if old == "" {
		_ = old
	}
}

func TestHistoryCompareRollback(t *testing.T) {
	withTempHome(t)
	base := &Skill{Name: "phase3", Description: "base", Version: 1, Tags: []string{"a"}, Steps: []Step{{Tool: "shell", Args: map[string]interface{}{"cmd": "echo 1"}}}}
	if err := Save(base, nil); err != nil {
		t.Fatal(err)
	}

	v2 := &Skill{Name: "phase3", Description: "second", Version: 2, Tags: []string{"b"}, Steps: []Step{{Tool: "shell", Args: map[string]interface{}{"cmd": "echo 2"}}}}
	AttachLineage(v2, base, "test")
	if err := Save(v2, nil); err != nil {
		t.Fatal(err)
	}

	entries, current, err := History("phase3")
	if err != nil {
		t.Fatal(err)
	}
	if current.Version != 2 || len(entries) != 1 || entries[0].Version != 1 {
		t.Fatalf("unexpected history: current=%d entries=%v", current.Version, entries)
	}

	cmp, err := CompareVersions("phase3", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmp.Changes) == 0 {
		t.Fatal("expected compare changes")
	}

	rolled, err := Rollback("phase3", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rolled.Version != 3 {
		t.Fatalf("rollback should create v3, got v%d", rolled.Version)
	}
	if rolled.Description != "base" {
		t.Fatalf("expected base description, got %q", rolled.Description)
	}
	if len(rolled.Lineage) < 2 {
		t.Fatalf("expected lineage preserved plus rollback entry, got %d", len(rolled.Lineage))
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), skillsDir, "phase3.json")); err != nil {
		t.Fatal(err)
	}
}

func TestRollbackMissingVersion(t *testing.T) {
	withTempHome(t)
	base := &Skill{Name: "missing", Description: "base", Version: 1, Steps: []Step{{Tool: "shell", Args: map[string]interface{}{"cmd": "echo"}}}}
	if err := Save(base, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Rollback("missing", 9, nil); err == nil {
		t.Fatal("expected missing version error")
	}
}

func TestResolveVersion(t *testing.T) {
	for _, raw := range []string{"1", "v1", "V2"} {
		if _, err := ResolveVersion(raw); err != nil {
			t.Fatalf("%s should resolve: %v", raw, err)
		}
	}
	if _, err := ResolveVersion("vx"); err == nil {
		t.Fatal("expected invalid version")
	}
}
