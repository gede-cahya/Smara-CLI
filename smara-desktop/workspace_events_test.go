package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mockEventCollector captures emitted events for testing.
type mockEventCollector struct {
	events []WorkspaceEvent
}

func (m *mockEventCollector) collect(ev WorkspaceEvent) {
	m.events = append(m.events, ev)
}

func TestWorkspaceEventSerialization(t *testing.T) {
	ev := WorkspaceEvent{
		Type:       "agent_spawn",
		AgentID:    "fe-1",
		AgentRole:  "frontend",
		ToX:        200,
		ToY:        200,
		Wave:       1,
		TotalWaves: 3,
		Message:    "Hello",
	}

	data, err := json.Marshal(ev)
	assert.NoError(t, err)

	var decoded WorkspaceEvent
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, "agent_spawn", decoded.Type)
	assert.Equal(t, "fe-1", decoded.AgentID)
	assert.Equal(t, "frontend", decoded.AgentRole)
	assert.Equal(t, 200, decoded.ToX)
	assert.Equal(t, 200, decoded.ToY)
	assert.Equal(t, 1, decoded.Wave)
	assert.Equal(t, 3, decoded.TotalWaves)
	assert.Equal(t, "Hello", decoded.Message)
}

func TestEmitMethods(t *testing.T) {
	var emitted []WorkspaceEvent
	mockEmitter := func(ctx context.Context, event string, data ...interface{}) {
		if len(data) > 0 {
			if ev, ok := data[0].(WorkspaceEvent); ok {
				emitted = append(emitted, ev)
			}
		}
	}

	app := &App{
		ctx:          context.Background(),
		eventEmitter: mockEmitter,
	}

	app.EmitWorkflowStart(3)
	app.EmitWaveStart(1, 3)
	app.EmitAgentSpawn("fe-1", "frontend", 200, 200)
	app.EmitAgentMove("fe-1", -60, 200, 200, 200)
	app.EmitAgentWork("be-1", "Designing API...")
	app.EmitAgentHandoff("be-1", "fe-1", "API ready")
	app.EmitAgentReview("qa-1", "fe-1", "Reviewing...")
	app.EmitWaveComplete(1, 3)
	app.EmitWorkflowComplete()

	assert.Len(t, emitted, 9)
	assert.Equal(t, "workflow_start", emitted[0].Type)
	assert.Equal(t, "agent_spawn", emitted[2].Type)
	assert.Equal(t, "fe-1", emitted[2].AgentID)
}

func TestTriggerWorkspaceDemo(t *testing.T) {
	mockEmitter := func(ctx context.Context, event string, data ...interface{}) {}
	app := &App{
		ctx:          context.Background(),
		eventEmitter: mockEmitter,
	}

	assert.NotPanics(t, func() {
		app.TriggerWorkspaceDemo()
	})

	// Give goroutine time to emit events
	time.Sleep(100 * time.Millisecond)
}

func TestWorkspaceEventTypes(t *testing.T) {
	tests := []struct {
		name string
		ev   WorkspaceEvent
	}{
		{"workflow_start", WorkspaceEvent{Type: "workflow_start", TotalWaves: 3}},
		{"wave_start", WorkspaceEvent{Type: "wave_start", Wave: 1, TotalWaves: 3}},
		{"agent_spawn", WorkspaceEvent{Type: "agent_spawn", AgentID: "orch-1", AgentRole: "orchestrator"}},
		{"agent_move", WorkspaceEvent{Type: "agent_move", AgentID: "fe-1", FromX: 0, FromY: 0, ToX: 100, ToY: 100}},
		{"agent_work", WorkspaceEvent{Type: "agent_work", AgentID: "be-1", Message: "Coding..."}},
		{"agent_handoff", WorkspaceEvent{Type: "agent_handoff", AgentID: "be-1", ToAgent: "fe-1"}},
		{"agent_review", WorkspaceEvent{Type: "agent_review", AgentID: "qa-1", ToAgent: "fe-1"}},
		{"wave_complete", WorkspaceEvent{Type: "wave_complete", Wave: 1, TotalWaves: 3}},
		{"workflow_complete", WorkspaceEvent{Type: "workflow_complete"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.ev)
			assert.NoError(t, err)

			var decoded WorkspaceEvent
			err = json.Unmarshal(data, &decoded)
			assert.NoError(t, err)
			assert.Equal(t, tt.ev.Type, decoded.Type)
		})
	}
}
