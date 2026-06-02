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

func TestBuildSkillContextForWorkflowModeDisablesAutoCreate(t *testing.T) {
	ctx := buildSkillContextForMode(ModeWorkflow)

	if strings.Contains(ctx, "skill_create") {
		t.Fatalf("workflow mode skill context should not advertise skill_create: %q", ctx)
	}
	if !strings.Contains(ctx, "node builder") {
		t.Fatalf("workflow mode skill context should mention node builder execution: %q", ctx)
	}
}

func TestBuildSkillRecommendationContextRoutesRelevantSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	requireNoError(t, os.MkdirAll(filepath.Join(home, ".smara", "skills"), 0755))
	requireNoError(t, skill.Save(&skill.Skill{
		Name:        "vps-health-check",
		Description: "Cek kesehatan VPS/server remote: uptime, disk, memory, docker.",
		Version:     1,
		Tags:        []string{"vps", "server", "monitoring"},
		Steps:       []skill.Step{{Tool: "run_command", Args: map[string]interface{}{"command": "echo ok"}}},
	}, nil))

	ctx := buildSkillRecommendationContext("cek kesehatan vps saya", ModeAsk)
	if !strings.Contains(ctx, "vps-health-check") || !strings.Contains(ctx, "skill_run") {
		t.Fatalf("expected vps-health-check recommendation with skill_run policy, got: %q", ctx)
	}
}

func TestBuildSkillRecommendationContextDoesNotRouteGreeting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	requireNoError(t, os.MkdirAll(filepath.Join(home, ".smara", "skills"), 0755))
	requireNoError(t, skill.Save(&skill.Skill{
		Name:        "vps-health-check",
		Description: "Cek kesehatan VPS/server remote.",
		Version:     1,
		Steps:       []skill.Step{{Tool: "run_command", Args: map[string]interface{}{"command": "echo ok"}}},
	}, nil))

	ctx := buildSkillRecommendationContext("hallo", ModeAsk)
	if strings.Contains(ctx, "vps-health-check") || strings.Contains(ctx, "prioritaskan panggil `skill_run`") {
		t.Fatalf("greeting should not recommend skill: %q", ctx)
	}
	if !strings.Contains(ctx, "sapaan/chat singkat") {
		t.Fatalf("greeting guardrail missing: %q", ctx)
	}
}

func TestBuildSkillRecommendationContextDisabledInWorkflowMode(t *testing.T) {
	ctx := buildSkillRecommendationContext("cek vps", ModeWorkflow)
	if ctx != "" {
		t.Fatalf("workflow mode should disable auto recommendation, got: %q", ctx)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
