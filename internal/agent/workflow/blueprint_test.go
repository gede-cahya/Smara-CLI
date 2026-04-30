package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlueprint_ToJSON(t *testing.T) {
	bp := Blueprint{
		ProjectName:  "Restaurant SaaS",
		Description:  "A restaurant management SaaS platform",
		PRD:          "# PRD\n\nFeatures...",
		Architecture: "# Architecture\n\nLayers...",
		Agents: []AgentSpec{
			{
				Role:        "frontend",
				Description: "Build UI",
				Tasks: []Task{
					{ID: "t1", Description: "Create login page"},
				},
				DependsOn: []string{"backend"},
			},
			{
				Role:        "backend",
				Description: "Build API",
				Tasks: []Task{
					{ID: "t1", Description: "Create auth endpoints"},
				},
			},
		},
	}

	b, err := bp.ToJSON()
	require.NoError(t, err)
	assert.Contains(t, string(b), "Restaurant SaaS")
	assert.Contains(t, string(b), "frontend")
	assert.Contains(t, string(b), "backend")
}

func TestBlueprint_StructFields(t *testing.T) {
	bp := Blueprint{
		ProjectName:  "Test",
		Description:  "Desc",
		PRD:          "PRD",
		Architecture: "Arch",
		Agents:       []AgentSpec{},
	}
	assert.Equal(t, "Test", bp.ProjectName)
	assert.Equal(t, "Desc", bp.Description)
	assert.Equal(t, "PRD", bp.PRD)
	assert.Equal(t, "Arch", bp.Architecture)
	assert.Empty(t, bp.Agents)
}

func TestAgentSpec_Struct(t *testing.T) {
	spec := AgentSpec{
		Role:        "frontend",
		Description: "UI work",
		Skills:      []string{"react", "nextjs"},
		Tasks: []Task{
			{ID: "1", Description: "task", Type: "llm", MCPServer: "stitch", ToolName: "generate"},
		},
		DependsOn: []string{"backend"},
	}
	assert.Equal(t, "frontend", spec.Role)
	assert.Equal(t, "UI work", spec.Description)
	assert.Equal(t, []string{"react", "nextjs"}, spec.Skills)
	assert.Len(t, spec.Tasks, 1)
	assert.Equal(t, []string{"backend"}, spec.DependsOn)
}

func TestTask_Struct(t *testing.T) {
	task := Task{
		ID:        "t1",
		Description: "desc",
		Type:      "mcp",
		MCPServer: "figma",
		ToolName:  "create_design",
	}
	assert.Equal(t, "t1", task.ID)
	assert.Equal(t, "desc", task.Description)
	assert.Equal(t, "mcp", task.Type)
	assert.Equal(t, "figma", task.MCPServer)
	assert.Equal(t, "create_design", task.ToolName)
}
