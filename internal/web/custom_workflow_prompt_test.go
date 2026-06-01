package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/agent/workflow"
	"github.com/gede-cahya/Smara-CLI/internal/config"
)

func TestExtractCustomWorkflowRunName(t *testing.T) {
	cases := map[string]string{
		"jalankan github-release-agent":                  "github-release-agent",
		"jalankan custom workflow git2":                  "git2",
		"run workflow release-pipeline":                  "release-pipeline",
		"execute custom workflow `github-release-agent`": "github-release-agent",
	}
	for input, want := range cases {
		got, ok := extractCustomWorkflowRunName(input)
		if !ok || got != want {
			t.Fatalf("extractCustomWorkflowRunName(%q) = %q, %v; want %q, true", input, got, ok, want)
		}
	}
}

func TestExtractCustomWorkflowRunRequestParallelSuffix(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"jalankan custom workflow github release agent secara parallel", "github release agent"},
		{"jalankan custom workflow github release agent secara paralel", "github release agent"},
		{"run workflow release-pipeline in parallel", "release-pipeline"},
		{"execute custom workflow `github-release-agent` parallel", "github-release-agent"},
	}
	for _, tc := range cases {
		got, parallel, ok := extractCustomWorkflowRunRequest(tc.input)
		if !ok || !parallel || got != tc.want {
			t.Fatalf("extractCustomWorkflowRunRequest(%q) = %q, %v, %v; want %q, true, true", tc.input, got, parallel, ok, tc.want)
		}
	}
}

func TestExtractCustomWorkflowRunRequestsMultipleNames(t *testing.T) {
	got, parallel, ok := extractCustomWorkflowRunRequests("jalankan workflow smara release agent dan smara-docs-site-agent secara parallel")
	if !ok || !parallel {
		t.Fatalf("extractCustomWorkflowRunRequests() ok=%v parallel=%v; want true true", ok, parallel)
	}
	want := []string{"smara release agent", "smara-docs-site-agent"}
	if len(got) != len(want) {
		t.Fatalf("extractCustomWorkflowRunRequests() = %#v; want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("extractCustomWorkflowRunRequests()[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

func TestCustomWorkflowExecutionPlanIgnoresParallelRequest(t *testing.T) {
	cw := &workflow.CustomWorkflow{
		Name:        "serial-check",
		Description: "check serial workflow execution",
		Agents: []workflow.CustomAgent{
			{Role: "alpha", Description: "first"},
			{Role: "beta", Description: "second"},
		},
	}

	plan := customWorkflowExecutionPlan(cw, "serial-check", true)
	if len(plan.Batches) != 2 {
		t.Fatalf("customWorkflowExecutionPlan() batches = %d; want 2 serial batches", len(plan.Batches))
	}
	for _, subtask := range plan.Subtasks {
		if subtask.CanParallel {
			t.Fatalf("subtask %q CanParallel = true; want false", subtask.ID)
		}
	}
	for _, batch := range plan.Batches {
		if batch.Mode != workflow.BatchModeSerial || batch.MaxConcurrency != 1 || len(batch.SubtaskIDs) != 1 {
			t.Fatalf("batch %#v; want one serial subtask with max concurrency 1", batch)
		}
	}
}

func TestFindCustomWorkflowTaskMatchesRunnerPrefixedTaskID(t *testing.T) {
	cw := &workflow.CustomWorkflow{
		Name: "prefixed-task",
		Agents: []workflow.CustomAgent{{
			Role: "release",
			Tasks: []workflow.Task{{
				ID:        "upload",
				Type:      "mcp",
				MCPServer: "github",
				ToolName:  "create_release",
			}},
		}},
	}

	task, ok := findCustomWorkflowTask(cw, "release", "release-upload")
	if !ok {
		t.Fatal("findCustomWorkflowTask() ok=false; want true")
	}
	if task.ToolName != "create_release" {
		t.Fatalf("findCustomWorkflowTask() ToolName = %q; want create_release", task.ToolName)
	}
}

func TestExtractCustomWorkflowCreateName(t *testing.T) {
	cases := map[string]string{
		"buat custom workflow spamload discover logic nanti kau rangkum range":        "spamload-discover-logic",
		"aku sekarang buat custom workflow spamload discover logic nanti kau rangkum": "spamload-discover-logic",
		"oke sekarang buatkan custom workflow spesialisasi desainer logo nanti":       "spesialisasi-desainer-logo",
		"buatkan custom workflow untuk testing website profesional":                   "testing-website-profesional",
		"create custom workflow release pipeline dengan QA":                           "release-pipeline",
	}
	for input, want := range cases {
		got, ok := extractCustomWorkflowCreateName(input)
		if !ok || got != want {
			t.Fatalf("extractCustomWorkflowCreateName(%q) = %q, %v; want %q, true", input, got, ok, want)
		}
	}
}

func TestCustomWorkflowQuestionDoesNotCreateWorkflow(t *testing.T) {
	prompt := "untuk custom workflow apakah ada fitur mengulangannya atau loop nanti saya mau buat custom workflow yang bisa melakukan secara loop pengulangan"
	if !isCustomWorkflowQuestion(prompt) {
		t.Fatalf("isCustomWorkflowQuestion(%q) = false; want true", prompt)
	}
	if _, ok := extractCustomWorkflowCreateName(prompt); !ok {
		t.Fatalf("extractCustomWorkflowCreateName(%q) = false; sanity check expected raw marker to match", prompt)
	}
}

func TestBuildPromptCustomWorkflowForLogoUsesGenerateImageTool(t *testing.T) {
	cw := buildPromptCustomWorkflow("spesialisasi-desainer-logo", "oke sekarang buatkan custom workflow spesialisasi desainer logo nanti bisa mengenerate image")
	var found bool
	for _, a := range cw.Agents {
		if a.Role == "image-generator" && len(a.Tasks) == 1 {
			found = a.Tasks[0].MCPServer == "builtin" && a.Tasks[0].ToolName == "generate_image" && a.Tasks[0].ToolArgs["prompt"] != ""
		}
	}
	if !found {
		t.Fatalf("logo workflow did not include image-generator builtin generate_image task: %#v", cw.Agents)
	}
}

func TestFindCustomWorkflowByAgentRole(t *testing.T) {
	cfg := config.Get()
	oldDBPath := cfg.DBPath
	cfg.DBPath = t.TempDir() + "/memory.db"
	t.Cleanup(func() { cfg.DBPath = oldDBPath })

	cw := &workflow.CustomWorkflow{
		Name:        "git2",
		Description: "release workflow",
		Agents: []workflow.CustomAgent{{
			Role:        "github-release-agent",
			Description: "release agent",
			Tasks:       []workflow.Task{{ID: "main", Description: "run release"}},
		}},
	}
	if err := workflow.SaveCustomWorkflow(cw); err != nil {
		t.Fatalf("SaveCustomWorkflow() error = %v", err)
	}

	for _, candidate := range []string{"github-release-agent", "github release agent"} {
		got, matched, err := findCustomWorkflowByNameOrAgent(candidate)
		if err != nil {
			t.Fatalf("findCustomWorkflowByNameOrAgent(%q) error = %v", candidate, err)
		}
		if got == nil || got.Name != "git2" || matched != "git2" {
			t.Fatalf("findCustomWorkflowByNameOrAgent(%q) = %#v, %q; want git2", candidate, got, matched)
		}
	}
}

func TestFindCustomWorkflowBySpokenWorkflowName(t *testing.T) {
	cfg := config.Get()
	oldDBPath := cfg.DBPath
	cfg.DBPath = t.TempDir() + "/memory.db"
	t.Cleanup(func() { cfg.DBPath = oldDBPath })

	cw := &workflow.CustomWorkflow{
		Name:        "smara-release-agent",
		Description: "release workflow",
		Agents: []workflow.CustomAgent{{
			Role:        "release-runner",
			Description: "release agent",
			Tasks:       []workflow.Task{{ID: "main", Description: "run release"}},
		}},
	}
	if err := workflow.SaveCustomWorkflow(cw); err != nil {
		t.Fatalf("SaveCustomWorkflow() error = %v", err)
	}

	for _, candidate := range []string{"smara release agent", "smara realease agent"} {
		got, matched, err := findCustomWorkflowByNameOrAgent(candidate)
		if err != nil {
			t.Fatalf("findCustomWorkflowByNameOrAgent(%q) error = %v", candidate, err)
		}
		if got == nil || got.Name != "smara-release-agent" || matched != "smara-release-agent" {
			t.Fatalf("findCustomWorkflowByNameOrAgent(%q) = %#v, %q; want smara-release-agent", candidate, got, matched)
		}
	}
}

func TestFindCustomWorkflowByWorkflowAgentAlias(t *testing.T) {
	cfg := config.Get()
	oldDBPath := cfg.DBPath
	cfg.DBPath = t.TempDir() + "/memory.db"
	t.Cleanup(func() { cfg.DBPath = oldDBPath })

	cw := &workflow.CustomWorkflow{
		Name:        "smara-docs-site",
		Description: "docs workflow",
		Agents: []workflow.CustomAgent{{
			Role:        "agent",
			Description: "docs site agent",
			Tasks:       []workflow.Task{{ID: "main", Description: "run docs site"}},
		}},
	}
	if err := workflow.SaveCustomWorkflow(cw); err != nil {
		t.Fatalf("SaveCustomWorkflow() error = %v", err)
	}

	for _, candidate := range []string{"smara-docs-site agent", "smara docs site agent"} {
		got, matched, err := findCustomWorkflowByNameOrAgent(candidate)
		if err != nil {
			t.Fatalf("findCustomWorkflowByNameOrAgent(%q) error = %v", candidate, err)
		}
		if got == nil || got.Name != "smara-docs-site" || matched != "smara-docs-site" {
			t.Fatalf("findCustomWorkflowByNameOrAgent(%q) = %#v, %q; want smara-docs-site", candidate, got, matched)
		}
	}
}

func TestFindCustomWorkflowBySuffixedAlias(t *testing.T) {
	cfg := config.Get()
	oldDBPath := cfg.DBPath
	cfg.DBPath = t.TempDir() + "/memory.db"
	t.Cleanup(func() { cfg.DBPath = oldDBPath })

	cw := &workflow.CustomWorkflow{
		Name:        "github-release-agent",
		Description: "release workflow",
		Agents: []workflow.CustomAgent{{
			Role:        "release-agent",
			Description: "release agent",
			Tasks:       []workflow.Task{{ID: "main", Description: "run release"}},
		}},
	}
	if err := workflow.SaveCustomWorkflow(cw); err != nil {
		t.Fatalf("SaveCustomWorkflow() error = %v", err)
	}

	got, matched, err := findCustomWorkflowByNameOrAgent("smara release agent")
	if err != nil {
		t.Fatalf("findCustomWorkflowByNameOrAgent() error = %v", err)
	}
	if got == nil || got.Name != "github-release-agent" || matched != "github-release-agent" {
		t.Fatalf("findCustomWorkflowByNameOrAgent(smara release agent) = %#v, %q; want github-release-agent", got, matched)
	}
}

func TestInferCustomWorkflowRunRequestsWithoutPrefix(t *testing.T) {
	cfg := config.Get()
	oldDBPath := cfg.DBPath
	cfg.DBPath = t.TempDir() + "/memory.db"
	t.Cleanup(func() { cfg.DBPath = oldDBPath })

	cw := &workflow.CustomWorkflow{
		Name:        "github-release-agent",
		Description: "release workflow",
		Agents: []workflow.CustomAgent{{
			Role:        "release-agent",
			Description: "release agent",
			Tasks:       []workflow.Task{{ID: "main", Description: "run release"}},
		}},
	}
	if err := workflow.SaveCustomWorkflow(cw); err != nil {
		t.Fatalf("SaveCustomWorkflow() error = %v", err)
	}

	got, parallel, ok := inferCustomWorkflowRunRequests("halankan github-release-agent")
	if !ok || parallel || len(got) != 1 || got[0] != "github-release-agent" {
		t.Fatalf("inferCustomWorkflowRunRequests() = %#v, %v, %v; want github-release-agent false true", got, parallel, ok)
	}
}

func TestCustomWorkflowRunPromptMissingAllIsHandled(t *testing.T) {
	cfg := config.Get()
	oldDBPath := cfg.DBPath
	cfg.DBPath = t.TempDir() + "/memory.db"
	t.Cleanup(func() { cfg.DBPath = oldDBPath })

	_, handled, err := (&Server{}).tryRunCustomWorkflowPrompt("jalankan workflow smara release agent")
	if !handled || err == nil {
		t.Fatalf("tryRunCustomWorkflowPrompt missing workflow handled=%v err=%v; want handled error", handled, err)
	}
}

func TestCustomWorkflowImportMergeRequiresExistingTarget(t *testing.T) {
	cfg := config.Get()
	oldDBPath := cfg.DBPath
	cfg.DBPath = t.TempDir() + "/memory.db"
	t.Cleanup(func() { cfg.DBPath = oldDBPath })

	body := []byte(`{
		"name":"missing-target",
		"mode":"merge",
		"json":"{\"name\":\"imported\",\"description\":\"x\",\"agents\":[{\"role\":\"agent\",\"description\":\"x\",\"tasks\":[{\"id\":\"main\",\"description\":\"x\"}]}]}"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/custom-workflow/import", bytes.NewReader(body))
	res := httptest.NewRecorder()

	(&Server{}).handleCustomWorkflowImport(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("handleCustomWorkflowImport status = %d; want %d", res.Code, http.StatusBadRequest)
	}
	if _, err := workflow.LoadCustomWorkflow("missing-target"); err == nil {
		t.Fatalf("merge import created missing target workflow")
	}
}
