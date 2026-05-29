package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

func TestBuildOrchestrationRuleSkillContextUsesBundledFallback(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	ctx := buildOrchestrationRuleSkillContext()
	if !strings.Contains(ctx, orchestrationRuleSkillName) {
		t.Fatalf("context does not include rule skill name: %q", ctx)
	}
	if !strings.Contains(ctx, "bundled skill") {
		t.Fatalf("context does not use bundled fallback: %q", ctx)
	}
	if !strings.Contains(ctx, "sapaan/chat singkat") {
		t.Fatalf("context does not include routing guardrails: %q", ctx)
	}
}

func TestBuildOrchestrationRuleSkillContextPrefersUserSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	requireNoError(t, os.MkdirAll(filepath.Join(home, ".smara", "skills"), 0755))
	requireNoError(t, skill.Save(&skill.Skill{
		Name:        orchestrationRuleSkillName,
		Description: "USER CUSTOM RULE: pakai parallel hanya jika user minta atau task kompleks.",
		Version:     1,
		Steps: []skill.Step{{
			Tool: "skill_read",
			Args: map[string]interface{}{"skill_name": orchestrationRuleSkillName},
		}},
	}, nil))

	ctx := buildOrchestrationRuleSkillContext()
	if !strings.Contains(ctx, "user skill") || !strings.Contains(ctx, "USER CUSTOM RULE") {
		t.Fatalf("context did not prefer user skill: %q", ctx)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
