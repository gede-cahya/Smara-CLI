package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorkflowState_Struct(t *testing.T) {
	ws := WorkflowState{
		ProjectDir:    "/tmp/test",
		Status:        "running",
		AgentOutputs:  make(map[string][]Result),
		BlueprintJSON: []byte(`{"name":"test"}`),
	}
	assert.Equal(t, "/tmp/test", ws.ProjectDir)
	assert.Equal(t, "running", ws.Status)
	assert.NotNil(t, ws.AgentOutputs)
	assert.Equal(t, []byte(`{"name":"test"}`), ws.BlueprintJSON)
}

func TestWorkflowState_NilQAResult(t *testing.T) {
	ws := WorkflowState{
		Status: "completed",
	}
	assert.Nil(t, ws.QAResult)
	assert.Empty(t, ws.AgentOutputs)
}
