package skill

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_AllStepsOk(t *testing.T) {
	s := Skill{
		Name: "test-skill",
		Steps: []Step{
			{Tool: "echo", Args: map[string]interface{}{"msg": "hello"}},
			{Tool: "echo", Args: map[string]interface{}{"msg": "world"}},
		},
	}

	executor := func(tool string, args map[string]interface{}) (string, error) {
		return args["msg"].(string), nil
	}

	result, err := s.Run(executor)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "test-skill", result.SkillName)
	assert.Len(t, result.StepResults, 2)
	assert.Equal(t, "hello", result.StepResults[0].Output)
	assert.Equal(t, "world", result.StepResults[1].Output)
	assert.Contains(t, result.Summary, "echo: hello")
	assert.Contains(t, result.Summary, "echo: world")
}

func TestRun_StepFails(t *testing.T) {
	s := Skill{
		Name: "fail-skill",
		Steps: []Step{
			{Tool: "echo", Args: map[string]interface{}{}},
			{Tool: "fail", Args: map[string]interface{}{}},
		},
	}

	executor := func(tool string, args map[string]interface{}) (string, error) {
		if tool == "fail" {
			return "", errors.New("boom")
		}
		return "ok", nil
	}

	result, err := s.Run(executor)
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Len(t, result.StepResults, 2)
	assert.Equal(t, "ok", result.StepResults[0].Output)
	assert.NotNil(t, result.StepResults[1].Error)
	assert.Contains(t, result.Summary, "boom")
}

func TestRun_NoSteps(t *testing.T) {
	s := Skill{Name: "empty"}
	executor := func(tool string, args map[string]interface{}) (string, error) {
		return "", nil
	}
	result, err := s.Run(executor)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Empty(t, result.StepResults)
	assert.Empty(t, result.Summary)
}

func TestSummary(t *testing.T) {
	s := Skill{
		Description: "My Skill",
		Steps: []Step{
			{Tool: "a"},
			{Tool: "b"},
			{Tool: "c"},
		},
	}
	assert.Equal(t, "My Skill (3 steps: a -> b -> c)", s.Summary())
}

func TestSummary_EmptySteps(t *testing.T) {
	s := Skill{Description: "Empty"}
	assert.Equal(t, "Empty (0 steps: )", s.Summary())
}

func TestRun_LongOutputTruncated(t *testing.T) {
	s := Skill{
		Name:  "long",
		Steps: []Step{{Tool: "echo"}},
	}
	longOut := strings.Repeat("a", 300)
	executor := func(tool string, args map[string]interface{}) (string, error) {
		return longOut, nil
	}

	result, err := s.Run(executor)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Summary, "...")
}
