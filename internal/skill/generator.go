package skill

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/gede-cahya/Smara-CLI/internal/llm"
)

// WorkflowCapture holds tool calls from a successful workflow run.
type WorkflowCapture struct {
	ProjectName string
	Steps       []CapturedStep
}

// CapturedStep represents one executed tool call from a workflow.
type CapturedStep struct {
	Tool          string
	Args          map[string]interface{}
	OutputSummary string
	Success       bool
}

// GenerateFromWorkflow uses LLM to convert captured steps into a Skill.
func GenerateFromWorkflow(capture WorkflowCapture, provider llm.Provider) (*Skill, error) {
	if provider == nil {
		return nil, fmt.Errorf("no LLM provider available for skill generation")
	}

	prompt := buildGeneratePrompt(capture)

	resp, err := provider.Chat([]llm.Message{
		{Role: llm.RoleSystem, Content: "Kamu adalah Skill Generator untuk Smara CLI. Output HANYA JSON skill valid, tanpa markdown fences atau penjelasan tambahan."},
		{Role: llm.RoleUser, Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	// Try to extract JSON from response (handle potential markdown fences)
	content := resp.Content
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	sk, err := FromJSON([]byte(content))
	if err != nil {
		return nil, fmt.Errorf("LLM returned invalid skill JSON: %w\nRaw: %s", err, resp.Content)
	}

	if err := sk.Validate(); err != nil {
		return nil, fmt.Errorf("generated skill validation failed: %w", err)
	}

	return sk, nil
}

// buildGeneratePrompt creates the LLM prompt from captured workflow steps.
func buildGeneratePrompt(capture WorkflowCapture) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Buat skill JSON dari workflow '%s' berikut ini.\n\n", capture.ProjectName))
	sb.WriteString("Tool calls yang sukses dieksekusi:\n")

	for i, step := range capture.Steps {
		if !step.Success {
			continue
		}
		argsJSON, _ := json.Marshal(step.Args)
		sb.WriteString(fmt.Sprintf("%d. Tool: %s\n", i+1, step.Tool))
		sb.WriteString(fmt.Sprintf("   Args: %s\n", string(argsJSON)))
		if step.OutputSummary != "" {
			sb.WriteString(fmt.Sprintf("   Output: %s\n", step.OutputSummary))
		}
	}

	sb.WriteString("\nBuat skill JSON dengan field berikut:\n")
	sb.WriteString("- \"name\": snake_case nama deskriptif (max 30 chars)\n")
	sb.WriteString("- \"description\": 1-2 kalimat apa yang dilakukan skill ini\n")
	sb.WriteString("- \"steps\": array tool calls dalam urutan yang sama\n")
	sb.WriteString("- \"tags\": array 1-3 tag kategori (misal: [\"deploy\", \"frontend\", \"git\"])\n")
	sb.WriteString("- \"version\": 1\n")
	sb.WriteString("\nHANYA output JSON. Tidak ada penjelasan tambahan.")

	return sb.String()
}

// PromptUserForCapture displays a preview and asks user for confirmation (CLI mode).
// Returns (confirmed, name, error). Name is the skill name user provided.
func PromptUserForCapture(capture WorkflowCapture) (bool, string, error) {
	fmt.Println("\n🔧 Workflow berhasil! Tangkap sebagai skill baru?")
	fmt.Printf("   Project: %s\n", capture.ProjectName)
	fmt.Printf("   Steps sukses: %d/%d\n", countSuccessful(capture.Steps), len(capture.Steps))
	fmt.Println("\n   Steps:")
	for _, step := range capture.Steps {
		if step.Success {
			fmt.Printf("     - %s\n", step.Tool)
		}
	}
	fmt.Print("\nSimpan sebagai skill? [nama-skill/n]: ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, "", err
	}
	input = strings.TrimSpace(input)

	if input == "" || strings.ToLower(input) == "n" || strings.ToLower(input) == "no" {
		return false, "", nil
	}

	return true, input, nil
}

func countSuccessful(steps []CapturedStep) int {
	count := 0
	for _, s := range steps {
		if s.Success {
			count++
		}
	}
	return count
}
