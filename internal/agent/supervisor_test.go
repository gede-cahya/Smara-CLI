package agent

import (
	"sync"
	"testing"
	"time"

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

	// Create a validator with a strict schema for a non-builtin tool
	validator := cognitive.NewValidator()
	validator.RegisterTool(cognitive.ToolSchema{
		Name:     "custom_api_call",
		Type:     cognitive.TypeObject,
		Required: []string{"url", "method"},
		Properties: map[string]cognitive.PropertySchema{
			"url":    {Type: cognitive.TypeString},
			"method": {Type: cognitive.TypeString},
		},
	})
	s.SetCognitiveValidator(validator)

	// Missing required field should fail validation
	_, err := s.executeToolCall(llm.ToolCall{
		Function: "custom_api_call",
		Args: map[string]interface{}{
			"url": "https://example.com",
			// missing method
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cognitive validation failed")
	assert.Contains(t, err.Error(), "missing required field: method")

	// Valid args should pass validation (and then fail for other reasons like no route)
	_, err = s.executeToolCall(llm.ToolCall{
		Function: "custom_api_call",
		Args: map[string]interface{}{
			"url":    "https://example.com",
			"method": "GET",
		},
	})
	// Should not be a cognitive validation error
	if err != nil {
		assert.NotContains(t, err.Error(), "cognitive validation failed")
	}
}

func TestPromptResult_Struct(t *testing.T) {
	r := PromptResult{
		Response:      "hello",
		Thinking:      "<thinking>reasoning</thinking>",
		Thoughts:      []string{"step 1", "step 2"},
		ToolsExecuted: []string{"read_file", "search_memories"},
		InputTokens:   100,
		OutputTokens:  50,
		TotalTokens:   150,
		Duration:      2 * time.Second,
	}
	assert.Equal(t, "hello", r.Response)
	assert.Equal(t, "<thinking>reasoning</thinking>", r.Thinking)
	assert.Equal(t, []string{"step 1", "step 2"}, r.Thoughts)
	assert.Equal(t, []string{"read_file", "search_memories"}, r.ToolsExecuted)
	assert.Equal(t, 100, r.InputTokens)
	assert.Equal(t, 50, r.OutputTokens)
	assert.Equal(t, 150, r.TotalTokens)
	assert.Equal(t, 2*time.Second, r.Duration)
}

func TestPromptResult_Defaults(t *testing.T) {
	r := PromptResult{}
	assert.Empty(t, r.Response)
	assert.Empty(t, r.Thinking)
	assert.Empty(t, r.Thoughts)
	assert.Empty(t, r.ToolsExecuted)
	assert.Equal(t, 0, r.InputTokens)
	assert.Equal(t, 0, r.OutputTokens)
	assert.Equal(t, 0, r.TotalTokens)
	assert.Equal(t, time.Duration(0), r.Duration)
}

func TestAgenticCallback_Struct(t *testing.T) {
	cb := AgenticCallback{
		OnToolCall:    func(server, tool string, args map[string]interface{}) {},
		OnToolResult:  func(output string) {},
		OnIteration:   func(current, max int) {},
		OnStream:      func(chunk string, isThinking bool) {},
		OnPhaseChange: func(phase, description string) {},
		OnLog:         func(role, content string) {},
		OnConfirm:     func(message string) bool { return true },
		OnExplore:     func(path string, results string) {},
	}
	assert.NotNil(t, cb.OnToolCall)
	assert.NotNil(t, cb.OnToolResult)
	assert.NotNil(t, cb.OnIteration)
	assert.NotNil(t, cb.OnStream)
	assert.NotNil(t, cb.OnPhaseChange)
	assert.NotNil(t, cb.OnLog)
	assert.NotNil(t, cb.OnConfirm)
	assert.NotNil(t, cb.OnExplore)
}

func TestNewSupervisor_Defaults(t *testing.T) {
	s := NewSupervisor(nil, nil)
	require.NotNil(t, s)
	assert.Nil(t, s.provider)
	assert.NotNil(t, s.sessionRegistry)
	assert.NotNil(t, s.mcpClients)
	assert.NotNil(t, s.mcpInfo)
	assert.NotNil(t, s.toolRoute)
	assert.NotNil(t, s.taskCh)
	assert.NotNil(t, s.resultCh)
	assert.Equal(t, 4, s.maxWorkers)
	assert.Equal(t, 10, s.maxIterations)
	assert.Equal(t, ModeAsk, s.mode)
	assert.NotNil(t, s.history)
	assert.Empty(t, s.history)
	assert.Equal(t, int64(0), s.workspaceID)
}

func TestSupervisor_SetMode_Ask(t *testing.T) {
	s := NewSupervisor(nil, nil)
	s.SetMode(ModeAsk)
	assert.Equal(t, ModeAsk, s.mode)
}

func TestSupervisor_SetMode_Rush(t *testing.T) {
	s := NewSupervisor(nil, nil)
	s.SetMode(ModeRush)
	assert.Equal(t, ModeRush, s.mode)
}

func TestSupervisor_SetMode_Plan(t *testing.T) {
	s := NewSupervisor(nil, nil)
	se := safety.NewEngine()
	s.SetSafetyEngine(se)
	s.SetMode(ModePlan)
	assert.Equal(t, ModePlan, s.mode)
	assert.Equal(t, safety.ModePlan, se.GetMode())
}

func TestSupervisor_SetMode_PlanToRush_SafetySwitches(t *testing.T) {
	s := NewSupervisor(nil, nil)
	se := safety.NewEngine()
	s.SetSafetyEngine(se)

	s.SetMode(ModePlan)
	assert.Equal(t, safety.ModePlan, se.GetMode())

	s.SetMode(ModeRush)
	assert.Equal(t, safety.ModeBuild, se.GetMode())
}

func TestSupervisor_WorkspaceID(t *testing.T) {
	s := NewSupervisor(nil, nil)
	assert.Equal(t, int64(0), s.GetWorkspaceID())

	s.SetWorkspaceID(42)
	assert.Equal(t, int64(42), s.GetWorkspaceID())
}

func TestSupervisor_GetProviderName_Unknown(t *testing.T) {
	s := NewSupervisor(nil, nil)
	assert.Equal(t, "unknown", s.GetProviderName())
}

func TestSupervisor_AddContext(t *testing.T) {
	s := NewSupervisor(nil, nil)
	s.AddContext("system instruction")
	assert.Len(t, s.history, 1)
	assert.Equal(t, llm.RoleSystem, s.history[0].Role)
	assert.Equal(t, "system instruction", s.history[0].Content)

	s.AddContext("another context")
	assert.Len(t, s.history, 2)
}

func TestSupervisor_GetStats_Initial(t *testing.T) {
	s := NewSupervisor(nil, nil)
	stats := s.GetStats()
	assert.Equal(t, 0, stats.PromptCount)
	assert.Equal(t, 0, stats.TotalTokens)
	assert.Equal(t, 0.0, stats.TotalCost)
	assert.Equal(t, 0, stats.AvgTokensPerReq)
	assert.Equal(t, time.Duration(0), stats.TotalDuration)
	assert.Equal(t, 0, stats.InputTokens)
	assert.Equal(t, 0, stats.OutputTokens)
	assert.Equal(t, time.Duration(0), stats.LastDuration)
	assert.False(t, stats.SessionStart.IsZero())
}

func TestSupervisor_updateStats(t *testing.T) {
	s := NewSupervisor(nil, nil)

	s.updateStats(100, 0.001, 2*time.Second)
	stats := s.GetStats()
	assert.Equal(t, 1, stats.PromptCount)
	assert.Equal(t, 100, stats.TotalTokens)
	assert.Equal(t, 0.001, stats.TotalCost)
	assert.Equal(t, 100, stats.AvgTokensPerReq)
	assert.Equal(t, 2*time.Second, stats.TotalDuration)

	s.updateStats(200, 0.002, 3*time.Second)
	stats = s.GetStats()
	assert.Equal(t, 2, stats.PromptCount)
	assert.Equal(t, 300, stats.TotalTokens)
	assert.Equal(t, 0.003, stats.TotalCost)
	assert.Equal(t, 150, stats.AvgTokensPerReq)
	assert.Equal(t, 5*time.Second, stats.TotalDuration)
}

func TestSupervisor_GetStats_ThreadSafety(t *testing.T) {
	s := NewSupervisor(nil, nil)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.updateStats(10, 0.0001, time.Millisecond)
			_ = s.GetStats()
		}()
	}
	wg.Wait()
	stats := s.GetStats()
	assert.Equal(t, 100, stats.PromptCount)
	assert.Equal(t, 1000, stats.TotalTokens)
}

func TestSupervisor_SetSafetyEngine_SyncsMode(t *testing.T) {
	s := NewSupervisor(nil, nil)
	s.SetMode(ModeRush)

	se := safety.NewEngine()
	s.SetSafetyEngine(se)
	assert.Equal(t, safety.ModeBuild, se.GetMode())
}

func TestSupervisor_SetSafetyEngine_PlanModeSyncs(t *testing.T) {
	s := NewSupervisor(nil, nil)
	s.SetMode(ModePlan)

	se := safety.NewEngine()
	s.SetSafetyEngine(se)
	assert.Equal(t, safety.ModePlan, se.GetMode())
}

func TestSupervisor_SetCognitiveValidator(t *testing.T) {
	s := NewSupervisor(nil, nil)
	v := cognitive.NewValidator()
	s.SetCognitiveValidator(v)
	assert.NotNil(t, s.cognitiveValidator)
}

func TestStats_Struct(t *testing.T) {
	stats := Stats{
		PromptCount:     5,
		TotalTokens:     1000,
		TotalCost:       0.05,
		TotalDuration:   10 * time.Second,
		AvgTokensPerReq: 200,
		InputTokens:     600,
		OutputTokens:    400,
		LastDuration:    2 * time.Second,
	}
	assert.Equal(t, 5, stats.PromptCount)
	assert.Equal(t, 1000, stats.TotalTokens)
	assert.Equal(t, 0.05, stats.TotalCost)
	assert.Equal(t, 10*time.Second, stats.TotalDuration)
	assert.Equal(t, 200, stats.AvgTokensPerReq)
	assert.Equal(t, 600, stats.InputTokens)
	assert.Equal(t, 400, stats.OutputTokens)
	assert.Equal(t, 2*time.Second, stats.LastDuration)
}

func TestMCPServerInfo_Struct(t *testing.T) {
	info := MCPServerInfo{
		Name:      "test-server",
		Connected: true,
		Tools:     nil,
		Error:     "",
	}
	assert.Equal(t, "test-server", info.Name)
	assert.True(t, info.Connected)
	assert.Empty(t, info.Error)
}
