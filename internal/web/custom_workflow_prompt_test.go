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

	got, matched, err := findCustomWorkflowByNameOrAgent("github-release-agent")
	if err != nil {
		t.Fatalf("findCustomWorkflowByNameOrAgent() error = %v", err)
	}
	if got == nil || got.Name != "git2" || matched != "git2" {
		t.Fatalf("findCustomWorkflowByNameOrAgent() = %#v, %q; want git2", got, matched)
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
