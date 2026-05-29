package skill

import (
	"strings"
	"testing"
)

func TestReviewSkillInstallRemoteRequiresApproval(t *testing.T) {
	sk := &Skill{
		Name:        "remote-review-test",
		Description: "safe skill from remote",
		Version:     1,
		Steps: []Step{{
			Tool: "view_file",
			Args: map[string]interface{}{"path": "README.md"},
		}},
	}

	report := ReviewSkillInstall(InstallReviewOptions{
		Source: "https://example.com/skill.json",
		Skill:  sk,
	})

	if !report.RequiresApproval {
		t.Fatalf("expected remote source to require approval")
	}
	if report.CanInstall {
		t.Fatalf("expected install to be blocked without approval")
	}
	joined := strings.Join(report.BlockingReasons, ";")
	if !strings.Contains(joined, "explicit approval required") {
		t.Fatalf("expected approval blocking reason, got %q", joined)
	}
}

func TestReviewSkillInstallApprovedRemoteCanInstall(t *testing.T) {
	sk := &Skill{
		Name:        "remote-approved-test",
		Description: "safe skill from remote",
		Version:     1,
		Steps: []Step{{
			Tool: "view_file",
			Args: map[string]interface{}{"path": "README.md"},
		}},
	}

	report := ReviewSkillInstall(InstallReviewOptions{
		Source:  "https://example.com/skill.json",
		Skill:   sk,
		Approve: true,
	})

	if !report.RequiresApproval {
		t.Fatalf("expected remote source to require approval")
	}
	if !report.CanInstall {
		t.Fatalf("expected approved safe remote skill to be installable, blockers: %v", report.BlockingReasons)
	}
}

func TestReviewSkillInstallBlockedToolPolicy(t *testing.T) {
	sk := &Skill{
		Name:        "blocked-tool-test",
		Description: "uses blocked shell tool",
		Version:     1,
		Steps: []Step{{
			Tool: "run_command",
			Args: map[string]interface{}{"command": "rm -rf /tmp/demo"},
		}},
	}

	report := ReviewSkillInstall(InstallReviewOptions{
		Source:       "local-test",
		Skill:        sk,
		Approve:      true,
		BlockedTools: []string{"run_command"},
	})

	if report.CanInstall {
		t.Fatalf("expected blocked tool policy to prevent install")
	}
	joined := strings.Join(report.BlockingReasons, ";")
	if !strings.Contains(joined, "blocked tool used: run_command") {
		t.Fatalf("expected blocked tool warning, got %q", joined)
	}
	if len(report.ShellCommands) == 0 {
		t.Fatalf("expected shell command inventory in report")
	}
}
