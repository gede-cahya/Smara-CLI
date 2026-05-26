package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/cognitive"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/mcp"
	"github.com/gede-cahya/Smara-CLI/internal/safety"
	"github.com/gede-cahya/Smara-CLI/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestSupervisor_TruncatesLargeToolResultBeforeModelAndCallback(t *testing.T) {
	path := t.TempDir() + "/large.txt"
	largeOutput := strings.Repeat("A", toolResultHeadChars+1000) + "MIDDLE" + strings.Repeat("Z", maxToolResultChars)
	require.NoError(t, os.WriteFile(path, []byte(largeOutput), 0644))

	mock := &mockSSHProvider{
		returnToolCall: true,
		toolCalls: []llm.ToolCall{
			{
				ID:       "call_read",
				Function: "read_file",
				Args: map[string]interface{}{
					"path": path,
				},
			},
		},
		finalContent: "done",
	}

	s := NewSupervisor(mock, nil)
	s.SetMode(ModeRush)

	var callbackOutput string
	s.callback = AgenticCallback{
		OnToolResult:  func(output string) { callbackOutput = output },
		OnPhaseChange: func(phase, description string) {},
		OnConfirm:     func(message string) bool { return true },
	}

	_, err := s.ProcessPrompt(context.Background(), "read the large file")
	require.NoError(t, err)
	require.NotEmpty(t, callbackOutput)
	assert.Less(t, len(callbackOutput), len(largeOutput))
	assert.Contains(t, callbackOutput, "characters omitted from tool result")
	assert.NotContains(t, callbackOutput, "MIDDLE")
	assert.True(t, strings.HasPrefix(callbackOutput, strings.Repeat("A", 100)))
	assert.True(t, strings.HasSuffix(callbackOutput, strings.Repeat("Z", 100)))

	var toolMessage *llm.Message
	for i := range mock.lastMessages {
		if mock.lastMessages[i].Role == llm.RoleTool {
			toolMessage = &mock.lastMessages[i]
			break
		}
	}
	require.NotNil(t, toolMessage)
	assert.Equal(t, callbackOutput, toolMessage.Content)
}

func TestSupervisor_ContinuesToFinalResponseOnFileToolError(t *testing.T) {
	path := t.TempDir() + "/duplicate.txt"
	require.NoError(t, os.WriteFile(path, []byte("needle\nneedle\n"), 0644))

	mock := &mockSSHProvider{
		returnToolCall: true,
		toolCalls: []llm.ToolCall{
			{
				ID:       "call_edit",
				Function: "edit_file",
				Args: map[string]interface{}{
					"path":        path,
					"old_content": "needle",
					"new_content": "replacement",
				},
			},
		},
		finalContent: "done",
	}

	s := NewSupervisor(mock, nil)
	s.SetMode(ModeRush)
	s.callback = AgenticCallback{
		OnToolResult:  func(output string) {},
		OnPhaseChange: func(phase, description string) {},
		OnConfirm:     func(message string) bool { return true },
	}

	result, err := s.ProcessPrompt(context.Background(), "edit file duplicate content")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 2, mock.calls, "file tool errors should still allow the model to compose a final answer")
	assert.Equal(t, "done", result.Response)
	assert.NotContains(t, result.Response, "Perubahan file gagal")
	assert.NotContains(t, result.Response, "Response final LLM")
	assert.Contains(t, result.ToolsExecuted, "edit_file")
}

func TestSupervisor_ContinuesToFinalResponseOnFileToolSuccess(t *testing.T) {
	path := t.TempDir() + "/target.txt"
	require.NoError(t, os.WriteFile(path, []byte("needle\n"), 0644))

	mock := &mockSSHProvider{
		returnToolCall: true,
		toolCalls: []llm.ToolCall{
			{
				ID:       "call_edit",
				Function: "edit_file",
				Args: map[string]interface{}{
					"path":        path,
					"old_content": "needle",
					"new_content": "replacement",
				},
			},
		},
		finalContent: "done",
	}

	s := NewSupervisor(mock, nil)
	s.SetMode(ModeRush)
	s.callback = AgenticCallback{
		OnToolResult:  func(output string) {},
		OnPhaseChange: func(phase, description string) {},
		OnConfirm:     func(message string) bool { return true },
	}

	result, err := s.ProcessPrompt(context.Background(), "edit file target content")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 2, mock.calls, "file tool success should allow the model to compose a final answer")
	assert.Equal(t, "done", result.Response)
	assert.NotContains(t, result.Response, "Perubahan file selesai")
	assert.NotContains(t, result.Response, "Response final LLM")
	assert.Contains(t, result.ToolsExecuted, "edit_file")
}

type postToolWaitProvider struct {
	mu           sync.Mutex
	calls        int
	chatCalls    int
	path         string
	quickDelay   time.Duration
	quickContent string
	quickErr     error
	finalDelay   time.Duration
	finalContent string
	finalErr     error
}

func (p *postToolWaitProvider) Name() string { return "post-tool-wait" }

func (p *postToolWaitProvider) Chat(messages []llm.Message) (*llm.ChatResponse, error) {
	p.mu.Lock()
	p.chatCalls++
	delay := p.quickDelay
	content := p.quickContent
	err := p.quickErr
	p.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	if err != nil {
		return nil, err
	}
	if content == "" {
		content = needMoreToolsMarker
	}
	return &llm.ChatResponse{Content: content}, nil
}

func (p *postToolWaitProvider) ChatWithTools(messages []llm.Message, tools []llm.ToolFunction) (*llm.ChatResponse, []llm.ToolCall, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	delay := p.finalDelay
	content := p.finalContent
	finalErr := p.finalErr
	p.mu.Unlock()

	if call == 1 {
		return &llm.ChatResponse{Content: "Membaca file."}, []llm.ToolCall{{
			ID:       "call_view",
			Function: "view_file",
			Args:     map[string]interface{}{"path": p.path},
		}}, nil
	}

	if delay > 0 {
		time.Sleep(delay)
	}
	if finalErr != nil {
		return nil, nil, finalErr
	}
	if content == "" {
		content = "jawaban final selesai"
	}
	return &llm.ChatResponse{Content: content}, nil, nil
}

func (p *postToolWaitProvider) GenerateEmbedding(text string) ([]float32, error) {
	return nil, nil
}

func TestSupervisor_WaitsForPostToolFinalResponseByDefault(t *testing.T) {
	oldTimeout := postToolLLMTimeout
	oldQuickTimeout := postToolQuickFinishTimeout
	postToolLLMTimeout = 0
	postToolQuickFinishTimeout = 0
	defer func() {
		postToolLLMTimeout = oldTimeout
		postToolQuickFinishTimeout = oldQuickTimeout
	}()

	path := t.TempDir() + "/target.txt"
	require.NoError(t, os.WriteFile(path, []byte("hello\n"), 0644))

	provider := &postToolWaitProvider{path: path, finalDelay: 40 * time.Millisecond, finalContent: "final setelah tool"}
	s := NewSupervisor(provider, nil)
	s.SetMode(ModeRush)
	s.callback = AgenticCallback{
		OnToolResult:  func(output string) {},
		OnPhaseChange: func(phase, description string) {},
		OnLog:         func(role, content string) {},
	}

	start := time.Now()
	result, err := s.ProcessPrompt(context.Background(), "lihat file target")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.GreaterOrEqual(t, time.Since(start), 40*time.Millisecond)
	assert.Equal(t, "final setelah tool", result.Response)
	assert.Contains(t, result.ToolsExecuted, "view_file")
	assert.Equal(t, 2, provider.calls)
	assert.Equal(t, 0, provider.chatCalls, "post-tool path should not use a no-tool finalizer")
}

func TestSupervisor_QuickFinishAvoidsSlowPostToolLoop(t *testing.T) {
	oldTimeout := postToolLLMTimeout
	oldQuickTimeout := postToolQuickFinishTimeout
	postToolLLMTimeout = 0
	postToolQuickFinishTimeout = 100 * time.Millisecond
	defer func() {
		postToolLLMTimeout = oldTimeout
		postToolQuickFinishTimeout = oldQuickTimeout
	}()

	path := t.TempDir() + "/target.txt"
	require.NoError(t, os.WriteFile(path, []byte("hello\n"), 0644))

	provider := &postToolWaitProvider{
		path:         path,
		quickContent: "final cepat dari hasil tool",
		finalDelay:   200 * time.Millisecond,
		finalContent: "final lambat",
	}
	s := NewSupervisor(provider, nil)
	s.SetMode(ModeRush)
	s.callback = AgenticCallback{
		OnToolResult:  func(output string) {},
		OnPhaseChange: func(phase, description string) {},
		OnLog:         func(role, content string) {},
	}

	start := time.Now()
	result, err := s.ProcessPrompt(context.Background(), "lihat file target")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Less(t, time.Since(start), 150*time.Millisecond)
	assert.Equal(t, "final cepat dari hasil tool", result.Response)
	assert.Equal(t, 1, provider.calls)
	assert.Equal(t, 1, provider.chatCalls)
}

func TestSupervisor_PostToolErrorReturnsFallbackSummary(t *testing.T) {
	oldTimeout := postToolLLMTimeout
	oldQuickTimeout := postToolQuickFinishTimeout
	postToolLLMTimeout = 0
	postToolQuickFinishTimeout = 0
	defer func() {
		postToolLLMTimeout = oldTimeout
		postToolQuickFinishTimeout = oldQuickTimeout
	}()

	path := t.TempDir() + "/target.txt"
	require.NoError(t, os.WriteFile(path, []byte("hello\n"), 0644))

	provider := &postToolWaitProvider{path: path, finalErr: fmt.Errorf("fetch failed")}
	s := NewSupervisor(provider, nil)
	s.SetMode(ModeRush)
	s.callback = AgenticCallback{
		OnToolResult:  func(output string) {},
		OnPhaseChange: func(phase, description string) {},
		OnLog:         func(role, content string) {},
	}

	result, err := s.ProcessPrompt(context.Background(), "lihat file target")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Contains(t, result.Response, "fetch failed")
	assert.Contains(t, result.Response, "Hasil tool terakhir")
	assert.Contains(t, result.Response, "view_file")
	assert.Equal(t, 0, provider.chatCalls, "post-tool errors should not be converted through a finalizer")
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
	assert.Equal(t, 30, s.maxIterations)
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

func TestSupervisor_UnregisterMCPClient(t *testing.T) {
	s := NewSupervisor(nil, nil)
	require.NotNil(t, s)

	// Register via public API and verify via GetMCPInfo
	s.RegisterMCPClient("test-server", nil) // nil client is fine for this test
	s.UpdateMCPInfo("test-server", []mcp.Tool{{Name: "tool-a", Description: "desc"}})

	info := s.GetMCPInfo()
	assert.Contains(t, info, "test-server")

	// Unregister
	s.UnregisterMCPClient("test-server")

	info = s.GetMCPInfo()
	assert.NotContains(t, info, "test-server")
}

func TestSupervisor_UnregisterMCPClient_NonExistent(t *testing.T) {
	s := NewSupervisor(nil, nil)
	require.NotNil(t, s)

	// Should not panic
	s.UnregisterMCPClient("missing-server")
	assert.Empty(t, s.GetMCPInfo())
}

func TestSupervisor_ClearHistory(t *testing.T) {
	s := NewSupervisor(nil, nil)

	// Add some history
	s.AddContext("system instruction")
	s.history = append(s.history, llm.Message{Role: llm.RoleUser, Content: "hello"})
	s.history = append(s.history, llm.Message{Role: llm.RoleAssistant, Content: "hi"})
	require.Len(t, s.history, 3)

	// Create a session so registry has a current session
	sess, err := s.CreateSession(SessionConfig{Name: "Test", Mode: "ask"})
	require.NoError(t, err)
	sess.History = append(sess.History, llm.Message{Role: llm.RoleUser, Content: "session msg"})
	require.Len(t, sess.History, 1)

	// Clear history
	s.ClearHistory()

	// Supervisor history cleared
	assert.Len(t, s.history, 0)
	// Session history cleared
	assert.Len(t, sess.History, 0)
}

func TestSupervisor_ClearHistory_NoSession(t *testing.T) {
	s := NewSupervisor(nil, nil)
	s.history = append(s.history, llm.Message{Role: llm.RoleUser, Content: "hello"})
	require.Len(t, s.history, 1)

	s.ClearHistory()
	assert.Len(t, s.history, 0)
}

// mockSessionStore is a minimal in-memory store for testing.
type mockSessionStore struct {
	sessions map[string]*session.Session
}

func (m *mockSessionStore) CreateSession(s *session.Session) error { m.sessions[s.ID] = s; return nil }
func (m *mockSessionStore) GetSession(id string) (*session.Session, error) {
	s, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return s, nil
}
func (m *mockSessionStore) UpdateSession(s *session.Session) error { m.sessions[s.ID] = s; return nil }
func (m *mockSessionStore) DeleteSession(id string) error          { delete(m.sessions, id); return nil }
func (m *mockSessionStore) ListSessions() ([]session.Session, error) {
	var out []session.Session
	for _, s := range m.sessions {
		out = append(out, *s)
	}
	return out, nil
}
func (m *mockSessionStore) ListSessionsByWorkspace(workspaceID int64) ([]session.Session, error) {
	var out []session.Session
	for _, s := range m.sessions {
		if s.WorkspaceID == workspaceID {
			out = append(out, *s)
		}
	}
	return out, nil
}
func (m *mockSessionStore) ListActiveSessions() ([]session.Session, error)  { return m.ListSessions() }
func (m *mockSessionStore) GetLastActiveSession() (*session.Session, error) { return nil, nil }
func (m *mockSessionStore) GetLastActiveSessionByWorkspace(workspaceID int64) (*session.Session, error) {
	return nil, nil
}

func TestSupervisor_SaveSession(t *testing.T) {
	s := NewSupervisor(nil, nil)
	store := &mockSessionStore{sessions: make(map[string]*session.Session)}
	s.SetSessionStore(store)

	sess, err := s.CreateSession(SessionConfig{Name: "Test", Mode: "ask"})
	require.NoError(t, err)
	sess.History = append(sess.History, llm.Message{Role: llm.RoleUser, Content: "msg"})

	err = s.SaveSession()
	require.NoError(t, err)

	stored, err := store.GetSession(sess.ID)
	require.NoError(t, err)
	assert.Len(t, stored.History, 1)
	assert.Equal(t, "msg", stored.History[0].Content)
}

func TestSupervisor_SaveSession_NoStore(t *testing.T) {
	s := NewSupervisor(nil, nil)
	err := s.SaveSession()
	assert.NoError(t, err)
}

func TestSupervisor_SaveSession_NoCurrentSession(t *testing.T) {
	s := NewSupervisor(nil, nil)
	store := &mockSessionStore{sessions: make(map[string]*session.Session)}
	s.SetSessionStore(store)

	err := s.SaveSession()
	assert.NoError(t, err)
}

func TestSessionRegistry_Create_SetsWorkspaceID(t *testing.T) {
	r := NewSessionRegistry()
	cfg := SessionConfig{Name: "Test", WorkspaceID: 42, Mode: "ask"}
	sess, err := r.Create(cfg)
	require.NoError(t, err)
	assert.Equal(t, int64(42), sess.WorkspaceID)
}
