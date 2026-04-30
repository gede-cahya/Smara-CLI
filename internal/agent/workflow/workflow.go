// Package workflow provides the agentic workflow engine for multi-agent orchestration.
package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
)

// extractProjectName generates a name from the user prompt.
func extractProjectName(prompt string) string {
	lower := strings.ToLower(prompt)
	keywords := []string{"web", "app", "saas", "platform", "system", "service", "api", "dashboard"}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			// Try to extract 2-3 words before the keyword
			idx := strings.Index(lower, kw)
			before := strings.TrimSpace(lower[:idx])
			words := strings.Fields(before)
			if len(words) >= 2 {
				return strings.Join(words[len(words)-2:], "-") + "-" + kw
			}
			return kw + "-project"
		}
	}
	return "workflow-" + fmt.Sprintf("%d", time.Now().Unix()%10000)
}

// RunWorkflow is the public API entrypoint for executing a workflow.
func RunWorkflow(supervisor *agent.Supervisor, provider llm.Provider, prompt string) (*WorkflowResult, error) {
	projectDir := filepath.Join(os.TempDir(), fmt.Sprintf("smara-workflow-%d", time.Now().Unix()))
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return nil, fmt.Errorf("gagal buat project dir: %w", err)
	}

	// Register in global registry
	reg, _ := LoadRegistry()
	name := extractProjectName(prompt)
	reg.Add(name, projectDir, prompt)
	_ = reg.Save()

	orch := NewOrchestrator(supervisor, provider, projectDir)
	result, err := orch.Run(context.Background(), prompt)
	if err != nil {
		reg.UpdateStatus(projectDir, "failed")
	} else {
		reg.UpdateStatus(projectDir, "completed")
	}
	_ = reg.Save()
	return result, err
}

// RunWorkflowWithDir executes a workflow with a specific project directory.
func RunWorkflowWithDir(supervisor *agent.Supervisor, provider llm.Provider, prompt, projectDir string) (*WorkflowResult, error) {
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return nil, fmt.Errorf("gagal buat project dir: %w", err)
	}

	orch := NewOrchestrator(supervisor, provider, projectDir)
	return orch.Run(context.Background(), prompt)
}
