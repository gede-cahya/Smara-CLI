package main

import (
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// WorkspaceEvent represents an event in the virtual workspace visualization.
type WorkspaceEvent struct {
	Type       string `json:"type"`
	AgentID    string `json:"agent_id,omitempty"`
	AgentRole  string `json:"agent_role,omitempty"`
	FromX      int    `json:"from_x,omitempty"`
	FromY      int    `json:"from_y,omitempty"`
	ToX        int    `json:"to_x,omitempty"`
	ToY        int    `json:"to_y,omitempty"`
	ToAgent    string `json:"to_agent,omitempty"`
	Message    string `json:"message,omitempty"`
	Wave       int    `json:"wave,omitempty"`
	TotalWaves int    `json:"total_waves,omitempty"`
}

// emitWorkspaceEvent sends an event to the frontend workspace visualization.
func (a *App) emitWorkspaceEvent(ev WorkspaceEvent) {
	if a.ctx == nil {
		return
	}
	emitter := a.eventEmitter
	if emitter == nil {
		emitter = runtime.EventsEmit
	}
	func() {
		defer func() { _ = recover() }()
		emitter(a.ctx, "workspace:event", ev)
	}()
}

// EmitWorkflowStart signals the beginning of a workflow with total wave count.
func (a *App) EmitWorkflowStart(totalWaves int) {
	a.emitWorkspaceEvent(WorkspaceEvent{
		Type:       "workflow_start",
		TotalWaves: totalWaves,
	})
}

// EmitWaveStart signals the start of a specific wave.
func (a *App) EmitWaveStart(wave, totalWaves int) {
	a.emitWorkspaceEvent(WorkspaceEvent{
		Type:       "wave_start",
		Wave:       wave,
		TotalWaves: totalWaves,
	})
}

// EmitAgentSpawn signals a new agent entering the workspace.
func (a *App) EmitAgentSpawn(agentID, role string, x, y int) {
	a.emitWorkspaceEvent(WorkspaceEvent{
		Type:      "agent_spawn",
		AgentID:   agentID,
		AgentRole: role,
		ToX:       x,
		ToY:       y,
	})
}

// EmitAgentMove signals an agent moving to a new position.
func (a *App) EmitAgentMove(agentID string, fromX, fromY, toX, toY int) {
	a.emitWorkspaceEvent(WorkspaceEvent{
		Type:    "agent_move",
		AgentID: agentID,
		FromX:   fromX,
		FromY:   fromY,
		ToX:     toX,
		ToY:     toY,
	})
}

// EmitAgentWork signals an agent starting a task.
func (a *App) EmitAgentWork(agentID, message string) {
	a.emitWorkspaceEvent(WorkspaceEvent{
		Type:    "agent_work",
		AgentID: agentID,
		Message: message,
	})
}

// EmitAgentHandoff signals a handoff between agents.
func (a *App) EmitAgentHandoff(agentID, toAgent, message string) {
	a.emitWorkspaceEvent(WorkspaceEvent{
		Type:    "agent_handoff",
		AgentID: agentID,
		ToAgent: toAgent,
		Message: message,
	})
}

// EmitAgentReview signals QA review starting.
func (a *App) EmitAgentReview(agentID, toAgent, message string) {
	a.emitWorkspaceEvent(WorkspaceEvent{
		Type:    "agent_review",
		AgentID: agentID,
		ToAgent: toAgent,
		Message: message,
	})
}

// EmitWaveComplete signals a wave completion.
func (a *App) EmitWaveComplete(wave, totalWaves int) {
	a.emitWorkspaceEvent(WorkspaceEvent{
		Type:       "wave_complete",
		Wave:       wave,
		TotalWaves: totalWaves,
	})
}

// EmitWorkflowComplete signals the end of the entire workflow.
func (a *App) EmitWorkflowComplete() {
	a.emitWorkspaceEvent(WorkspaceEvent{
		Type: "workflow_complete",
	})
}

// TriggerWorkspaceDemo emits a full sequence of demo workspace events for testing.
func (a *App) TriggerWorkspaceDemo() {
	go func() {
		a.EmitWorkflowStart(3)
		a.EmitWaveStart(1, 3)
		a.EmitAgentSpawn("orch-1", "orchestrator", 500, 300)
		a.EmitAgentSpawn("fe-1", "frontend", -60, 200)
		a.EmitAgentSpawn("be-1", "backend", 1060, 200)
		a.EmitAgentSpawn("db-1", "database", 500, 860)
		a.EmitAgentSpawn("qa-1", "qa", -60, 650)

		time.Sleep(300 * time.Millisecond)
		a.EmitAgentMove("fe-1", -60, 200, 200, 200)
		a.EmitAgentMove("be-1", 1060, 200, 800, 200)
		a.EmitAgentMove("db-1", 500, 860, 500, 500)
		a.EmitAgentMove("qa-1", -60, 650, 300, 650)

		time.Sleep(800 * time.Millisecond)
		a.EmitAgentWork("be-1", "Designing REST API schema...")
		a.EmitAgentWork("fe-1", "Building React components...")
		a.EmitAgentWork("db-1", "Creating migration files...")

		time.Sleep(3 * time.Second)
		a.EmitAgentHandoff("be-1", "fe-1", "API spec ready")
		a.EmitAgentWork("fe-1", "Integrating API into UI...")

		time.Sleep(3 * time.Second)
		a.EmitAgentReview("qa-1", "fe-1", "Reviewing frontend code...")
		a.EmitAgentMove("qa-1", 300, 650, 200, 200)

		time.Sleep(2 * time.Second)
		a.EmitAgentMove("qa-1", 200, 200, 800, 200)

		time.Sleep(2 * time.Second)
		a.EmitAgentMove("qa-1", 800, 200, 500, 500)

		time.Sleep(2 * time.Second)
		a.EmitAgentMove("qa-1", 500, 500, 300, 650)

		time.Sleep(1 * time.Second)
		a.EmitWaveComplete(1, 3)
		a.EmitWaveStart(2, 3)

		time.Sleep(1 * time.Second)
		a.EmitWaveComplete(2, 3)
		a.EmitWaveStart(3, 3)

		time.Sleep(1 * time.Second)
		a.EmitWaveComplete(3, 3)
		a.EmitWorkflowComplete()
	}()
}
