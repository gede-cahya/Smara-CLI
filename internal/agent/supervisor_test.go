package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/gede-cahya/Smara-CLI/internal/cognitive"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/safety"
)

func TestSupervisor_PlanModeBlocksWriteTools(t *testing.T) {
	s := NewSupervisor(nil, nil)
	require.NotNil(t, s)

	se := safety.NewEngine()
	se.SetMode(safety.ModePlan)
	s.SetSafetyEngine(se)

	// Set mode to Plan
	s.SetMode(ModePlan)

	// Test that write tools are blocked
	result, err := s.executeToolCall(llm.ToolCall{
		Function: "write_file",
		Args: map[string]interface{}{
			"path":    "/tmp/test.txt",
			"content": "hello",
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "safety block")
	assert.Empty(t, result)

	// Verify draft was recorded
	drafts := se.GetDrafts()
	require.Len(t, drafts, 1)
	assert.Equal(t, "write_file", drafts[0].Tool)
}

func TestSupervisor_PlanModeAllowsReadTools(t *testing.T) {
	s := NewSupervisor(nil, nil)
	require.NotNil(t, s)

	se := safety.NewEngine()
	se.SetMode(safety.ModePlan)
	s.SetSafetyEngine(se)
	s.SetMode(ModePlan)

	// Test that read tools are allowed through safety (but will fail due to nil provider)
	_, err := s.executeToolCall(llm.ToolCall{
		Function: "search_memories",
		Args: map[string]interface{}{
			"query": "test",
		},
	})
	// search_memories is a read tool; safety allows it, but execution fails because
	// memStore and provider are nil. The key is that it is NOT a safety block error.
	if err != nil {
		assert.NotContains(t, err.Error(), "safety block")
	}
}

func TestSupervisor_BuildModeAllowsAllTools(t *testing.T) {
	s := NewSupervisor(nil, nil)
	require.NotNil(t, s)

	se := safety.NewEngine()
	se.SetMode(safety.ModeBuild)
	s.SetSafetyEngine(se)
	s.SetMode(ModeRush)

	// In Build Mode, write tools are allowed (execution may fail for other reasons)
	_, err := s.executeToolCall(llm.ToolCall{
		Function: "write_file",
		Args: map[string]interface{}{
			"path":    "/tmp/test.txt",
			"content": "hello",
		},
	})
	// Should not be a safety block error
	if err != nil {
		assert.NotContains(t, err.Error(), "safety block")
	}
}

func TestSupervisor_PlanModeFiltersToolList(t *testing.T) {
	s := NewSupervisor(nil, nil)
	require.NotNil(t, s)

	se := safety.NewEngine()
	se.SetMode(safety.ModePlan)
	s.SetSafetyEngine(se)
	s.SetMode(ModePlan)

	tools := s.ConvertMCPToolsToToolFunctions()
	require.NotEmpty(t, tools)

	// In Plan Mode, only read-only tools should be present
	for _, tool := range tools {
		assert.True(t, safety.IsReadOnlyTool(tool.Name),
			"tool %s should not be available in Plan Mode", tool.Name)
	}

	// Verify write tools are excluded
	for _, tool := range tools {
		assert.NotEqual(t, "write_file", tool.Name)
		assert.NotEqual(t, "edit_file", tool.Name)
		assert.NotEqual(t, "run_command", tool.Name)
		assert.NotEqual(t, "delete_file", tool.Name)
		assert.NotEqual(t, "remember", tool.Name)
	}
}

func TestSupervisor_BuildModeIncludesAllTools(t *testing.T) {
	s := NewSupervisor(nil, nil)
	require.NotNil(t, s)

	se := safety.NewEngine()
	se.SetMode(safety.ModeBuild)
	s.SetSafetyEngine(se)
	s.SetMode(ModeRush)

	tools := s.ConvertMCPToolsToToolFunctions()
	require.NotEmpty(t, tools)

	// In Build Mode, write tools should be present
	hasWriteFile := false
	for _, tool := range tools {
		if tool.Name == "write_file" {
			hasWriteFile = true
			break
		}
	}
	assert.True(t, hasWriteFile, "write_file should be available in Build Mode")
}

func TestSupervisor_CognitiveValidation(t *testing.T) {
	s := NewSupervisor(nil, nil)
	require.NotNil(t, s)

	// Create a validator with a strict schema
	validator := cognitive.NewValidator()
	validator.RegisterTool(cognitive.ToolSchema{
		Name:     "write_file",
		Type:     cognitive.TypeObject,
		Required: []string{"path", "content"},
		Properties: map[string]cognitive.PropertySchema{
			"path":    {Type: cognitive.TypeString},
			"content": {Type: cognitive.TypeString},
		},
	})
	s.SetCognitiveValidator(validator)

	// Missing required field should fail validation
	_, err := s.executeToolCall(llm.ToolCall{
		Function: "write_file",
		Args: map[string]interface{}{
			"path": "/tmp/test.txt",
			// missing content
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cognitive validation failed")
	assert.Contains(t, err.Error(), "missing required field: content")

	// Valid args should pass validation (and then fail for other reasons like nil memStore)
	_, err = s.executeToolCall(llm.ToolCall{
		Function: "write_file",
		Args: map[string]interface{}{
			"path":    "/tmp/test.txt",
			"content": "hello",
		},
	})
	// Should not be a cognitive validation error
	if err != nil {
		assert.NotContains(t, err.Error(), "cognitive validation failed")
	}
}
