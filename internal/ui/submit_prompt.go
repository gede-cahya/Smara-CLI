package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/agent/workflow"
	"github.com/gede-cahya/Smara-CLI/internal/orchestration"
)

// submitPrompt sends a prompt to the current supervisor, mirroring the normal
// Enter flow. It is used by interactive Plan-mode buttons/options.
func (m *AppModel) submitPrompt(prompt string) tea.Cmd {
	processedPrompt := m.processFileMentions(prompt)
	m.cancelled = false
	m.processing = true
	m.statusText = "Memproses..."
	m.currentStream = ""
	m.currentThinking = ""
	m.streamStartTime = time.Now()
	m.dotFrame = 0
	m.cursorVisible = true
	m.currentPhase = ""
	m.phaseSeen = nil
	m.phaseSeenSet = make(map[string]bool)
	m.phaseContents = make(map[string]string)
	m.phaseDescs = make(map[string]string)
	m.fadeWave.Reset()

	sup := m.supervisor
	ctx := m.ctx
	cmds := []tea.Cmd{m.spinner.Tick, dotTickCmd()}

	if sup.GetMode() == agent.ModeParallel && (orchestration.IsAgentSwarmWorkflowPrompt(processedPrompt) || orchestration.ShouldAutoParallelOrchestrate(processedPrompt, sup.GetMode())) {
		if agentSup, ok := sup.(*agent.Supervisor); ok {
			cmds = append(cmds, func() tea.Msg {
				result, err := workflow.RunWorkflow(agentSup, agentSup.GetProvider(), processedPrompt)
				if err != nil {
					return ProcessMsg{Result: nil, Err: err}
				}
				label := "✅ Auto parallel orchestration selesai"
				if orchestration.IsAgentSwarmWorkflowPrompt(processedPrompt) {
					label = "✅ Agent Swarm Workflow selesai"
				}
				return ProcessMsg{Result: &agent.PromptResult{Response: label + "\n\n" + result.FinalSummary}, Err: nil}
			})
			return tea.Batch(cmds...)
		}
	}

	cmds = append(cmds, cursorBlinkCmd(), waveTickCmd(), func() tea.Msg {
		result, err := sup.ProcessPrompt(context.WithoutCancel(ctx), processedPrompt)
		return ProcessMsg{Result: result, Err: err}
	})
	return tea.Batch(cmds...)
}
