package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallFromPluginSourceLocalManifest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	manifest := `{
		"name":"demo-plugin",
		"skills":[{
			"name":"demo-plugin-skill",
			"description":"Demo plugin skill",
			"version":1,
			"steps":[{"tool":"run_command","args":{"command":"echo hello"}}]
		}]
	}`
	if err := os.WriteFile(filepath.Join(dir, "smara-skill.json"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	installed, err := InstallFromPluginSource(PluginInstallOptions{Source: dir})
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if len(installed) != 1 || installed[0].Name != "demo-plugin-skill" {
		t.Fatalf("unexpected installed skills: %#v", installed)
	}
	loaded, err := Load("demo-plugin-skill")
	if err != nil || loaded == nil {
		t.Fatalf("skill was not persisted: %v", err)
	}
}

func TestInstallFromPluginSourceAliasRejectsMultiSkill(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	file := filepath.Join(t.TempDir(), "skills.json")
	manifest := `{"skills":[
		{"name":"a","description":"A","steps":[{"tool":"run_command","args":{"command":"echo a"}}]},
		{"name":"b","description":"B","steps":[{"tool":"run_command","args":{"command":"echo b"}}]}
	]}`
	if err := os.WriteFile(file, []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallFromPluginSource(PluginInstallOptions{Source: file, Alias: "x"}); err == nil {
		t.Fatal("expected alias + multi-skill manifest to fail")
	}
}

func TestNormalizePluginSourceNpxSkillsAdd(t *testing.T) {
	source, err := NormalizePluginSource([]string{"npx", "skills", "add", "pbakaus/impeccable"})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if source != "pbakaus/impeccable" {
		t.Fatalf("unexpected source: %s", source)
	}
}

func TestNormalizePluginSourceQuotedNpxSkillsAdd(t *testing.T) {
	source, err := NormalizePluginSource([]string{"npx skills add pbakaus/impeccable"})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if source != "pbakaus/impeccable" {
		t.Fatalf("unexpected source: %s", source)
	}
}

func TestNormalizePluginSourceNpxWithFlags(t *testing.T) {
	source, err := NormalizePluginSource([]string{"npx", "--yes", "skills", "add", "pbakaus/impeccable"})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if source != "pbakaus/impeccable" {
		t.Fatalf("unexpected source: %s", source)
	}
}
