package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetBuiltinTools_ReturnsNonEmpty(t *testing.T) {
	tools := GetBuiltinTools()
	assert.NotEmpty(t, tools, "GetBuiltinTools should return at least one tool")
}

func TestGetBuiltinTools_HasRunCommand(t *testing.T) {
	tools := GetBuiltinTools()
	found := false
	for _, tool := range tools {
		if tool.Name == "run_command" {
			found = true
			assert.NotEmpty(t, tool.Description)
			break
		}
	}
	assert.True(t, found, "run_command tool should be present")
}

func TestGetBuiltinTools_HasViewFile(t *testing.T) {
	tools := GetBuiltinTools()
	found := false
	for _, tool := range tools {
		if tool.Name == "view_file" {
			found = true
			assert.NotEmpty(t, tool.Description)
			break
		}
	}
	assert.True(t, found, "view_file tool should be present")
}

func TestGetBuiltinTools_ToolParameters(t *testing.T) {
	tools := GetBuiltinTools()
	for _, tool := range tools {
		assert.NotEmpty(t, tool.Name, "every tool must have a name")
		assert.NotEmpty(t, tool.Description, "every tool must have a description")
		assert.NotNil(t, tool.Parameters, "every tool must have parameters")
	}
}
