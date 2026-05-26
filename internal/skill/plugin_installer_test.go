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

func TestInstallFromPluginSourceCodexSkillFolder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(`---
name: graphify
description: Graphify skill
trigger: /graphify
---

Run graphify and summarize the generated report.
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".graphify_version"), []byte("1"), 0644); err != nil {
		t.Fatal(err)
	}
	installed, err := InstallFromPluginSource(PluginInstallOptions{Source: dir})
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if len(installed) != 1 || installed[0].Name != "graphify" {
		t.Fatalf("unexpected installed skills: %#v", installed)
	}
	loaded, err := Load("graphify")
	if err != nil {
		t.Fatalf("skill was not persisted: %v", err)
	}
	if loaded.Steps[0].Tool != "skill_instructions" {
		t.Fatalf("expected instruction wrapper, got %#v", loaded.Steps)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".smara", "skills", "graphify", ".graphify_version")); err != nil {
		t.Fatalf("expected sidecar asset copied: %v", err)
	}
}

func TestInstallFromPluginSourceClaudeMarkdownTree(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	agents := filepath.Join(root, ".claude", "agents")
	if err := os.MkdirAll(agents, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agents, "reviewer.md"), []byte(`# Reviewer

Review code and report concrete risks.
`), 0644); err != nil {
		t.Fatal(err)
	}
	installed, err := InstallFromPluginSource(PluginInstallOptions{Source: root})
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if len(installed) != 1 || installed[0].Name != "reviewer" {
		t.Fatalf("unexpected installed skills: %#v", installed)
	}
	if _, err := Load("reviewer"); err != nil {
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
