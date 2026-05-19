package magicpointer

import (
	"context"
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	got := Redact("email saya user@example.com password: rahasia token=abc")
	if strings.Contains(got, "user@example.com") || strings.Contains(got, "rahasia") || strings.Contains(got, "abc") {
		t.Fatalf("sensitive data not redacted: %q", got)
	}
}

func TestInferElements(t *testing.T) {
	els := InferElements("Search\nSave\nhttps://example.com\nNama:")
	if len(els) < 4 {
		t.Fatalf("expected elements, got %d", len(els))
	}
	var action, link bool
	for _, e := range els {
		if e.Type == "action_candidate" && strings.EqualFold(e.Text, "Save") {
			action = true
		}
		if e.Type == "link" {
			link = true
		}
	}
	if !action || !link {
		t.Fatalf("expected action and link elements: %+v", els)
	}
}

func TestSummarizeNoOCR(t *testing.T) {
	got := Summarize("", nil, false)
	if !strings.Contains(got, "OCR belum tersedia") {
		t.Fatalf("unexpected summary: %s", got)
	}
}

func TestParseTesseractTSV(t *testing.T) {
	tsv := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"5\t1\t1\t1\t1\t1\t10\t20\t40\t12\t90\tLogin\n" +
		"5\t1\t1\t1\t2\t1\t10\t50\t50\t12\t85\tSearch\n"
	text, els := parseTesseractTSV(tsv)
	if !strings.Contains(text, "Login") || len(els) != 2 {
		t.Fatalf("bad parse text=%q els=%+v", text, els)
	}
	if els[0].Box == nil {
		t.Fatalf("expected box: %+v", els[0])
	}
}

func TestPlanInstructionClick(t *testing.T) {
	els := []Element{{Type: "action_candidate", Text: "Login", Confidence: 0.9, Source: "test", Box: &Box{X: 1, Y: 2, W: 3, H: 4}}}
	plan := PlanInstruction("klik tombol login", els)
	if len(plan.Actions) != 1 || plan.Actions[0].Type != "click" || plan.Actions[0].Target == nil || plan.Actions[0].Target.Text != "Login" {
		t.Fatalf("bad plan: %+v", plan)
	}
}

func TestPlanInstructionType(t *testing.T) {
	els := []Element{{Type: "input_or_label", Text: "Nama:", Confidence: 0.8, Source: "test"}}
	plan := PlanInstruction("isi Nama dengan Cahya", els)
	if len(plan.Actions) != 1 || plan.Actions[0].Type != "type" || plan.Actions[0].Value != "Cahya" || !plan.Actions[0].RequiresConfirmation {
		t.Fatalf("bad type plan: %+v", plan)
	}
}

func TestPlanInstructionScroll(t *testing.T) {
	plan := PlanInstruction("scroll ke atas", nil)
	if len(plan.Actions) != 1 || plan.Actions[0].Type != "scroll" || plan.Actions[0].Value != "up" {
		t.Fatalf("bad scroll plan: %+v", plan)
	}
}

type fakeExecutor struct{ calls []string }

func (f *fakeExecutor) Click(ctx context.Context, x, y int) (string, error) {
	f.calls = append(f.calls, "click")
	return "fake", nil
}
func (f *fakeExecutor) TypeText(ctx context.Context, text string) (string, error) {
	f.calls = append(f.calls, "type:"+text)
	return "fake", nil
}
func (f *fakeExecutor) Scroll(ctx context.Context, direction string) (string, error) {
	f.calls = append(f.calls, "scroll:"+direction)
	return "fake", nil
}
func (f *fakeExecutor) Key(ctx context.Context, key string) (string, error) {
	f.calls = append(f.calls, "key:"+key)
	return "fake", nil
}

func TestExecutePlanClick(t *testing.T) {
	fx := &fakeExecutor{}
	plan := ActionPlan{Actions: []PlannedAction{{Type: "click", Target: &Element{Text: "Login", Box: &Box{X: 10, Y: 20, W: 30, H: 10}}, Risk: "low"}}}
	execed, warnings := ExecutePlan(context.Background(), plan, ExecuteOptions{Executor: fx})
	if len(warnings) != 0 || len(execed) != 1 || !execed[0].Success || execed[0].X != 25 || execed[0].Y != 25 || len(fx.calls) != 1 {
		t.Fatalf("bad execute: execed=%+v warnings=%+v calls=%+v", execed, warnings, fx.calls)
	}
}

func TestExecutePlanRequiresConfirmation(t *testing.T) {
	fx := &fakeExecutor{}
	plan := ActionPlan{Actions: []PlannedAction{{Type: "type", Value: "secret", RequiresConfirmation: true, Risk: "medium"}}}
	execed, warnings := ExecutePlan(context.Background(), plan, ExecuteOptions{Executor: fx})
	if len(warnings) == 0 || len(execed) != 1 || execed[0].Success || len(fx.calls) != 0 {
		t.Fatalf("expected blocked action: execed=%+v warnings=%+v calls=%+v", execed, warnings, fx.calls)
	}
}

func TestExecutePlanTypeWithYesRedactsAuditValue(t *testing.T) {
	fx := &fakeExecutor{}
	plan := ActionPlan{Actions: []PlannedAction{{Type: "type", Value: "Cahya", RequiresConfirmation: true, Risk: "medium"}}}
	execed, warnings := ExecutePlan(context.Background(), plan, ExecuteOptions{Executor: fx, AssumeYes: true})
	if len(warnings) != 0 || len(execed) != 1 || !execed[0].Success || execed[0].Value != "[TYPED_TEXT_REDACTED]" || len(fx.calls) != 1 {
		t.Fatalf("bad type execute: execed=%+v warnings=%+v calls=%+v", execed, warnings, fx.calls)
	}
}

func TestEnrichVisualElementsPhase4(t *testing.T) {
	els := []Element{{Type: "text", Text: "Settings", Confidence: 0.8, Source: "test", Box: &Box{X: 10, Y: 10, W: 60, H: 20}}, {Type: "input_or_label", Text: "Nama:", Confidence: 0.7, Source: "test", Box: &Box{X: 10, Y: 50, W: 50, H: 18}}, {Type: "text", Text: "☐ Agree", Confidence: 0.7, Source: "test", Box: &Box{X: 10, Y: 90, W: 70, H: 18}}}
	out := EnrichVisualElements(els)
	var icon, input, check bool
	for _, e := range out {
		if e.Type == "icon_candidate" {
			icon = true
		}
		if e.Type == "input_field_candidate" {
			input = true
		}
		if e.Type == "checkbox_or_radio_candidate" {
			check = true
		}
	}
	if !icon || !input || !check {
		t.Fatalf("missing phase4 enrichments: %+v", out)
	}
}

func TestPlanWorkflowPhase5(t *testing.T) {
	els := []Element{{Type: "action_candidate", Text: "Search", Confidence: 0.9, Source: "test", Box: &Box{X: 1, Y: 2, W: 30, H: 10}}, {Type: "action_candidate", Text: "Login", Confidence: 0.9, Source: "test", Box: &Box{X: 50, Y: 2, W: 30, H: 10}}}
	plan := PlanInstruction("klik search lalu klik login", els)
	if len(plan.Actions) != 2 || !strings.Contains(plan.Summary, "workflow") {
		t.Fatalf("bad workflow plan: %+v", plan)
	}
}

func TestPhase6VoiceFileFallbackError(t *testing.T) {
	vc := ResolveVoiceInstruction(context.Background(), Options{VoiceFile: "/tmp/not-exist-audio.wav"})
	if !vc.Enabled || vc.Error == "" {
		t.Fatalf("expected voice error, got %+v", vc)
	}
}

func TestPhase7AppContextManualAndBoost(t *testing.T) {
	app := DetectAppContext(context.Background(), "browser")
	if app.Profile != "browser" {
		t.Fatalf("expected browser profile: %+v", app)
	}
	els := []Element{{Type: "text", Text: "Search", Confidence: 0.5, Source: "test"}}
	boosted := ApplyAppAwareBoosts(els, app)
	if boosted[0].Confidence <= els[0].Confidence || boosted[0].Attributes["app_profile_boost"] != "browser" {
		t.Fatalf("expected boost: %+v", boosted)
	}
}

func TestPlanInstructionWithAppNormalizesBrowserURL(t *testing.T) {
	els := []Element{{Type: "action_candidate", Text: "Address Bar", Confidence: 0.8, Source: "test", Box: &Box{X: 1, Y: 1, W: 100, H: 20}}}
	plan := PlanInstructionWithApp("klik alamat", els, AppContext{Profile: "browser"})
	if len(plan.Actions) != 1 || plan.Actions[0].Target == nil || !strings.Contains(strings.ToLower(plan.Actions[0].Target.Text), "address") {
		t.Fatalf("bad app-aware plan: %+v", plan)
	}
	if len(plan.Warnings) == 0 {
		t.Fatalf("expected normalization warning: %+v", plan)
	}
}

func TestPhase8PrivacyBlocksApp(t *testing.T) {
	cfg := DefaultPrivacyConfig()
	cfg.BlockedApps = []string{"secretapp"}
	rep := EvaluatePrivacy(cfg, AppContext{AppName: "SecretApp"}, "")
	if !rep.AppBlocked || rep.Reason == "" {
		t.Fatalf("expected blocked app: %+v", rep)
	}
}

func TestPhase8AllowlistBlocksUnknown(t *testing.T) {
	cfg := DefaultPrivacyConfig()
	cfg.PrivacyMode = "allowlist"
	cfg.AllowedApps = []string{"firefox"}
	rep := EvaluatePrivacy(cfg, AppContext{AppName: "Terminal"}, "")
	if !rep.AppBlocked {
		t.Fatalf("expected allowlist block: %+v", rep)
	}
}

func TestPhase9RecordLearning(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/learning.json"
	plan := &ActionPlan{Actions: []PlannedAction{{Type: "click"}, {Type: "scroll"}}}
	lp, err := RecordLearning(path, AppContext{Profile: "browser", AppName: "Firefox"}, "klik email user@example.com", plan)
	if err != nil || lp.TotalEvents != 1 || lp.AppUsage["browser"] != 1 || lp.ActionUsage["click"] != 1 {
		t.Fatalf("bad learning profile err=%v lp=%+v", err, lp)
	}
	if strings.Contains(lp.RecentEvents[0].Instruction, "user@example.com") {
		t.Fatalf("instruction not redacted: %+v", lp.RecentEvents[0])
	}
}

func (f *fakeExecutor) OpenApp(ctx context.Context, name string) (string, error) {
	f.calls = append(f.calls, "open:"+name)
	return "fake", nil
}

func TestPlanDesktopLaunchOrBrowserTask(t *testing.T) {
	plan, ok := PlanDesktopLaunchOrBrowserTask("Buka browser lalu cari dokumentasi Go terbaru")
	if !ok || len(plan.Actions) != 4 || plan.Actions[0].Type != "open_app" || plan.Actions[1].Type != "key" || plan.Actions[2].Type != "type" || plan.Actions[3].Type != "key" {
		t.Fatalf("bad browser plan ok=%v plan=%+v", ok, plan)
	}
	if !strings.Contains(strings.ToLower(plan.Actions[2].Value), "dokumentasi go") {
		t.Fatalf("missing query: %+v", plan.Actions[2])
	}
}

func TestPlanInstructionOpenApp(t *testing.T) {
	plan := PlanInstruction("buka terminal", nil)
	if len(plan.Actions) != 1 || plan.Actions[0].Type != "open_app" || plan.Actions[0].Value != "terminal" {
		t.Fatalf("bad open app plan: %+v", plan)
	}
}

func TestExecutePlanOpenApp(t *testing.T) {
	fx := &fakeExecutor{}
	plan := ActionPlan{Actions: []PlannedAction{{Type: "open_app", Value: "terminal", Risk: "low"}}}
	execed, warnings := ExecutePlan(context.Background(), plan, ExecuteOptions{Executor: fx})
	if len(warnings) != 0 || len(execed) != 1 || !execed[0].Success || len(fx.calls) != 1 || fx.calls[0] != "open:terminal" {
		t.Fatalf("bad open execute: execed=%+v warnings=%+v calls=%+v", execed, warnings, fx.calls)
	}
}

func TestRunAutopilotStopsSafelyOnMissingTarget(t *testing.T) {
	run, err := RunAutopilot(context.Background(), AutopilotOptions{
		Options:  Options{Instruction: "klik tombol save", Executor: &fakeExecutor{}},
		MaxSteps: 3,
		Observer: func(ctx context.Context, opts Options) (ScreenContext, error) {
			plan := ActionPlan{Instruction: opts.Instruction, Mode: ModeExecute, Actions: []PlannedAction{{Type: "click", Reason: "test"}}}
			return ScreenContext{Plan: &plan}, nil
		},
	})
	if err != nil || run.Completed || !strings.Contains(run.StopReason, "target UI tidak ditemukan") || len(run.Iterations) != 1 {
		t.Fatalf("bad autopilot stop err=%v run=%+v", err, run)
	}
}

func TestRunAutopilotCompletesBrowserPlan(t *testing.T) {
	fx := &fakeExecutor{}
	run, err := RunAutopilot(context.Background(), AutopilotOptions{
		Options:  Options{Instruction: "Buka browser cari docs Go", Executor: fx, AssumeYes: true},
		MaxSteps: 10,
		Observer: func(ctx context.Context, opts Options) (ScreenContext, error) {
			plan, _ := PlanDesktopLaunchOrBrowserTask(opts.Instruction)
			plan.Mode = ModeExecute
			ex, w := ExecutePlan(ctx, plan, ExecuteOptions{Executor: opts.Executor, AssumeYes: opts.AssumeYes})
			plan.Executed = ex
			plan.Warnings = w
			return ScreenContext{Plan: &plan, RawText: "results"}, nil
		},
	})
	if err != nil || !run.Completed || len(run.Iterations) != 1 || len(fx.calls) != 4 {
		t.Fatalf("bad autopilot complete err=%v run=%+v calls=%+v", err, run, fx.calls)
	}
}
