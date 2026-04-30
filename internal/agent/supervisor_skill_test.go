package agent

import (
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/safety"
	"github.com/gede-cahya/Smara-CLI/internal/skill"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSupervisor_SkillExecutor(t *testing.T) {
	s := NewSupervisor(nil, nil)
	require.NotNil(t, s)

	// Test that SkillExecutor returns a non-nil function
	executor := s.SkillExecutor()
	require.NotNil(t, executor)

	// Test with a built-in read-only tool (should work in Ask mode)
	result, err := executor("get_cwd", nil)
	assert.NoError(t, err)
	assert.NotEmpty(t, result)
}

func TestSupervisor_SkillExecutor_ModeFiltering(t *testing.T) {
	// Test in Plan mode — write tools should be blocked
	s := NewSupervisor(nil, nil)
	s.mode = ModePlan

	se := safety.NewEngine()
	s.SetSafetyEngine(se)

	executor := s.SkillExecutor()
	require.NotNil(t, executor)

	// Read-only tool should work
	result, err := executor("get_cwd", nil)
	assert.NoError(t, err)
	assert.NotEmpty(t, result)

	// Write tool should be blocked in Plan mode
	_, err = executor("write_file", map[string]interface{}{
		"path":    "/tmp/test.txt",
		"content": "test",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "safety")
}

func TestSkill_Run_WithSupervisorExecutor(t *testing.T) {
	s := NewSupervisor(nil, nil)

	// Create a simple skill that gets cwd
	sk := &skill.Skill{
		Name:        "test-cwd",
		Description: "Test getting current directory",
		Steps: []skill.Step{
			{Tool: "get_cwd", Args: nil},
		},
		Version: 1,
	}

	result, err := sk.Run(s.SkillExecutor())
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Len(t, result.StepResults, 1)
	assert.Equal(t, "get_cwd", result.StepResults[0].Tool)
	assert.NotEmpty(t, result.StepResults[0].Output)
}
