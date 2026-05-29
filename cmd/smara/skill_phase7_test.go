package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

func TestPhase7SkillRecommendCLIText(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	oldDBPath := config.Get().DBPath
	oldFormat := skillRecommendFormat
	oldLimit := skillRecommendLimit
	oldNoHistory := skillRecommendNoHistory
	os.Setenv("HOME", home)
	config.Get().DBPath = filepath.Join(home, "skill-history.db")
	skillRecommendFormat = "text"
	skillRecommendLimit = 5
	skillRecommendNoHistory = true
	t.Cleanup(func() {
		os.Setenv("HOME", oldHome)
		config.Get().DBPath = oldDBPath
		skillRecommendFormat = oldFormat
		skillRecommendLimit = oldLimit
		skillRecommendNoHistory = oldNoHistory
	})

	if err := skill.Save(&skill.Skill{Name: "phase7-deploy-node", Description: "Deploy Node.js app ke VPS", Steps: []skill.Step{{Tool: "ssh_exec"}}}, nil); err != nil {
		t.Fatalf("save deploy skill: %v", err)
	}
	if err := skill.Save(&skill.Skill{Name: "phase7-database-backup", Description: "Backup database", Steps: []skill.Step{{Tool: "shell"}}}, nil); err != nil {
		t.Fatalf("save db skill: %v", err)
	}

	out := captureStdout(t, func() {
		if err := skillRecommendCmd.RunE(skillRecommendCmd, []string{"deploy", "node", "vps"}); err != nil {
			t.Fatalf("recommend run: %v", err)
		}
	})
	if !strings.Contains(out, "Rekomendasi skill") || !strings.Contains(out, "phase7-deploy-node") {
		t.Fatalf("unexpected recommend output:\n%s", out)
	}
}

func TestPhase7SkillSuggestCLIJSONAlias(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	oldDBPath := config.Get().DBPath
	oldFormat := skillRecommendFormat
	oldLimit := skillRecommendLimit
	oldNoHistory := skillRecommendNoHistory
	os.Setenv("HOME", home)
	config.Get().DBPath = filepath.Join(home, "skill-history.db")
	skillRecommendFormat = "json"
	skillRecommendLimit = 1
	skillRecommendNoHistory = true
	t.Cleanup(func() {
		os.Setenv("HOME", oldHome)
		config.Get().DBPath = oldDBPath
		skillRecommendFormat = oldFormat
		skillRecommendLimit = oldLimit
		skillRecommendNoHistory = oldNoHistory
	})

	if err := skill.Save(&skill.Skill{Name: "phase7-monitoring-linux", Description: "Monitor CPU memory disk linux", Tags: []string{"monitoring"}, Steps: []skill.Step{{Tool: "shell"}}}, nil); err != nil {
		t.Fatalf("save monitoring skill: %v", err)
	}

	out := captureStdout(t, func() {
		if err := skillSuggestCmd.RunE(skillSuggestCmd, []string{"monitoring", "linux"}); err != nil {
			t.Fatalf("suggest run: %v", err)
		}
	})
	if !strings.Contains(out, `"skill_name": "phase7-monitoring-linux"`) || !strings.Contains(out, `"reasons"`) {
		t.Fatalf("unexpected suggest json output:\n%s", out)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	return buf.String()
}
