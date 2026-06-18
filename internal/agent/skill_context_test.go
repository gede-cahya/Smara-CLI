package agent

import (
	"context"
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

func TestSelectAutoRunnableSkillChoosesClearSafeRecommendation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	requireNoError(t, os.MkdirAll(filepath.Join(home, ".smara", "skills"), 0755))
	requireNoError(t, skill.Save(&skill.Skill{
		Name:        "planning-test-plan",
		Description: "Membuat test plan lengkap untuk pengujian aplikasi dan quality assurance.",
		Version:     1,
		Tags:        []string{"planning", "test", "quality"},
		Steps:       []skill.Step{{Tool: "planning_template", Args: map[string]interface{}{"kind": "test-plan"}}},
	}, nil))

	selected := selectAutoRunnableSkill("buat planning test quality aplikasi", ModeAsk)
	if selected == nil || selected.Skill.Name != "planning-test-plan" {
		t.Fatalf("expected planning-test-plan auto selection, got: %#v", selected)
	}
}

func TestSelectAutoRunnableSkillRejectsRiskAndRequiredParams(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	requireNoError(t, os.MkdirAll(filepath.Join(home, ".smara", "skills"), 0755))
	requireNoError(t, skill.Save(&skill.Skill{
		Name:        "deploy-production-server",
		Description: "Deploy production server application.",
		Version:     1,
		Tags:        []string{"deploy", "production", "server"},
		Params:      []skill.ParamDef{{Name: "host", Required: true}},
		Steps:       []skill.Step{{Tool: "ssh_exec", Args: map[string]interface{}{"host": "__PARAM__host", "command": "deploy"}}},
	}, nil))

	if selected := selectAutoRunnableSkill("deploy production server sekarang", ModeRush); selected != nil {
		t.Fatalf("risky parameterized skill must not auto-run: %#v", selected)
	}
}

func TestSelectAutoRunnableSkillRejectsSkillManagementPrompt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	requireNoError(t, os.MkdirAll(filepath.Join(home, ".smara", "skills"), 0755))
	requireNoError(t, skill.Save(&skill.Skill{
		Name:        "skill-list-helper",
		Description: "Membantu melihat daftar skill yang tersedia.",
		Version:     1,
		Tags:        []string{"skill", "list"},
		Steps:       []skill.Step{{Tool: "skill_list", Args: map[string]interface{}{}}},
	}, nil))

	if selected := selectAutoRunnableSkill("tolong ngelist skill yang tersedia", ModeAsk); selected != nil {
		t.Fatalf("skill management prompt must not auto-run a recommended skill: %#v", selected)
	}
}

func TestSupervisorAutomaticallyRunsAndReportsSelectedSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	requireNoError(t, os.MkdirAll(filepath.Join(home, ".smara", "skills"), 0755))
	requireNoError(t, skill.Save(&skill.Skill{
		Name:        "planning-test-plan",
		Description: "Membuat test plan lengkap untuk pengujian aplikasi dan quality assurance.",
		Version:     1,
		Tags:        []string{"planning", "test", "quality"},
		Steps:       []skill.Step{{Tool: "planning_template", Args: map[string]interface{}{"kind": "test-plan", "goal": "aplikasi"}}},
	}, nil))

	provider := &mockSSHProvider{finalContent: "Test plan selesai."}
	supervisor := NewSupervisor(provider, nil)
	supervisor.SetMode(ModeAsk)
	var calledTool string
	var calledArgs map[string]interface{}
	var toolResult string
	supervisor.SetCallback(AgenticCallback{
		OnToolCall: func(_ string, tool string, args map[string]interface{}) {
			calledTool = tool
			calledArgs = args
		},
		OnToolResult: func(output string) {
			toolResult = output
		},
	})

	result, err := supervisor.ProcessPrompt(context.Background(), "buat planning test quality aplikasi")
	requireNoError(t, err)
	if calledTool != "skill_run" || calledArgs["skill_name"] != "planning-test-plan" || calledArgs["automatic"] != true {
		t.Fatalf("automatic skill callback missing or incorrect: tool=%q args=%v", calledTool, calledArgs)
	}
	if !strings.Contains(toolResult, "planning-test-plan") || !strings.Contains(toolResult, "dijalankan otomatis") {
		t.Fatalf("automatic skill result was not reported: %q", toolResult)
	}
	if len(result.ToolsExecuted) == 0 || result.ToolsExecuted[0] != "skill_run" {
		t.Fatalf("skill_run missing from prompt result tools: %v", result.ToolsExecuted)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
