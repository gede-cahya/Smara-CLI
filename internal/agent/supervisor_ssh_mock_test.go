package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	smarassh "github.com/gede-cahya/Smara-CLI/internal/ssh"
)

type mockSSHProvider struct {
	calls           int
	lastMessages    []llm.Message
	toolCalls       []llm.ToolCall
	returnToolCall  bool
	dsmlContent     string // simulates LLM writing DSML inside content instead of native tool_calls
	finalContent    string
}

func (m *mockSSHProvider) Name() string { return "mock-ssh" }

func (m *mockSSHProvider) Chat(messages []llm.Message) (*llm.ChatResponse, error) {
	m.calls++
	m.lastMessages = messages
	return &llm.ChatResponse{Content: m.finalContent}, nil
}

func (m *mockSSHProvider) ChatWithTools(messages []llm.Message, tools []llm.ToolFunction) (*llm.ChatResponse, []llm.ToolCall, error) {
	m.calls++
	m.lastMessages = messages
	if m.dsmlContent != "" {
		content := m.dsmlContent
		m.dsmlContent = "" // consume so next call returns finalContent
		return &llm.ChatResponse{Content: content}, nil, nil
	}
	if m.returnToolCall {
		m.returnToolCall = false
		tc := m.toolCalls[0]
		m.toolCalls = m.toolCalls[1:]
		return &llm.ChatResponse{Content: "Mengeksekusi tool..."}, []llm.ToolCall{tc}, nil
	}
	return &llm.ChatResponse{Content: m.finalContent}, nil, nil
}

func (m *mockSSHProvider) GenerateEmbedding(text string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3}, nil
}

func TestSupervisor_MockLLM_SSHExecToolCalled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	require.NoError(t, smarassh.EnsureDir())

	host := smarassh.Host{
		Name:    "vps-cahya",
		Address: "129.226.222.242",
		User:    "ubuntu",
		Port:    "22",
		KeyPath: "/home/cahya/Downloads/vpsCahya.pem",
	}
	require.NoError(t, smarassh.SaveHost(host))

	mock := &mockSSHProvider{
		returnToolCall: true,
		toolCalls: []llm.ToolCall{
			{
				ID:       "call_1",
				Function: "ssh_exec",
				Args: map[string]interface{}{
					"host":    "ubuntu@129.226.222.242",
					"command": "uptime",
				},
			},
		},
		finalContent: "VPS uptime: 15 days.",
	}

	s := NewSupervisor(mock, nil)
	s.SetMode(ModeRush)

	var executedTools []string
	s.callback = AgenticCallback{
		OnToolCall: func(server, tool string, args map[string]interface{}) {
			executedTools = append(executedTools, fmt.Sprintf("%s: %v", tool, args))
		},
		OnToolResult: func(output string) {},
		OnPhaseChange: func(phase, description string) {},
		OnIteration:   func(current, max int) {},
		OnStream:      func(chunk string, isThinking bool) {},
		OnConfirm:     func(message string) bool { return true },
	}

	_, err := s.ProcessPrompt(context.Background(), "cek status vps ubuntu")
	require.NoError(t, err)

	require.Len(t, executedTools, 1)
	assert.Contains(t, executedTools[0], "ssh_exec")
	assert.Contains(t, executedTools[0], "ubuntu@129.226.222.242")
	assert.GreaterOrEqual(t, mock.calls, 1)

	hasUserPrompt := false
	for _, msg := range mock.lastMessages {
		if msg.Role == llm.RoleUser && msg.Content == "cek status vps ubuntu" {
			hasUserPrompt = true
		}
	}
	assert.True(t, hasUserPrompt, "mock should receive user prompt in messages")
}

func TestSupervisor_MockLLM_SSHViewFileToolCalled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	require.NoError(t, smarassh.EnsureDir())

	host := smarassh.Host{
		Name:    "vps-cahya",
		Address: "129.226.222.242",
		User:    "ubuntu",
		Port:    "22",
		KeyPath: "/home/cahya/Downloads/vpsCahya.pem",
	}
	require.NoError(t, smarassh.SaveHost(host))

	mock := &mockSSHProvider{
		returnToolCall: true,
		toolCalls: []llm.ToolCall{
			{
				ID:       "call_1",
				Function: "ssh_view_file",
				Args: map[string]interface{}{
					"host": "vps-cahya",
					"path": "/var/log/syslog",
				},
			},
		},
		finalContent: "Log diterima.",
	}

	s := NewSupervisor(mock, nil)
	s.SetMode(ModeAsk)

	var executedTools []string
	s.callback = AgenticCallback{
		OnToolCall: func(server, tool string, args map[string]interface{}) {
			executedTools = append(executedTools, tool)
		},
		OnToolResult:  func(output string) {},
		OnPhaseChange: func(phase, description string) {},
		OnIteration:   func(current, max int) {},
		OnStream:      func(chunk string, isThinking bool) {},
		OnConfirm:     func(message string) bool { return true },
	}

	_, err := s.ProcessPrompt(context.Background(), "lihat log vps cahya")
	require.NoError(t, err)

	require.Len(t, executedTools, 1)
	assert.Equal(t, "ssh_view_file", executedTools[0])
}

func TestSupervisor_MockLLM_DSMLToolCallExtracted(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	require.NoError(t, smarassh.EnsureDir())

	host := smarassh.Host{
		Name:    "vps-cahya",
		Address: "129.226.222.242",
		User:    "ubuntu",
		Port:    "22",
		KeyPath: "/dev/null/nonexistent", // forces fast failure, we only care about extraction
	}
	require.NoError(t, smarassh.SaveHost(host))

	dsml := `<| DSML | tool_calls>
<| DSML | invoke name="ssh_exec">
<| DSML | parameter name="host" string="true">vps-cahya</| DSML | parameter>
<| DSML | parameter name="command" string="true">uptime</| DSML | parameter>
</| DSML | invoke>
</| DSML | tool_calls>`

	mock := &mockSSHProvider{
		dsmlContent:  dsml,
		finalContent: "VPS uptime: 15 hari.",
	}

	s := NewSupervisor(mock, nil)
	s.SetMode(ModeRush)

	var executedTools []string
	var toolArgs []map[string]interface{}
	s.callback = AgenticCallback{
		OnToolCall: func(server, tool string, args map[string]interface{}) {
			executedTools = append(executedTools, tool)
			toolArgs = append(toolArgs, args)
		},
		OnToolResult:  func(output string) {},
		OnPhaseChange: func(phase, description string) {},
		OnIteration:   func(current, max int) {},
		OnStream:      func(chunk string, isThinking bool) {},
		OnConfirm:     func(message string) bool { return true },
	}

	result, err := s.ProcessPrompt(context.Background(), "cek status vps cahya")
	require.NoError(t, err)

	// DSML should have been extracted and ssh_exec attempted (will fail due to bad key, but tool is called)
	require.GreaterOrEqual(t, len(executedTools), 1, "DSML tool call should have been extracted and OnToolCall fired")
	assert.Equal(t, "ssh_exec", executedTools[0])
	assert.Equal(t, "vps-cahya", toolArgs[0]["host"])
	assert.Equal(t, "uptime", toolArgs[0]["command"])

	// At least 2 LLM calls: 1st returns DSML, 2nd returns final answer after tool result
	assert.GreaterOrEqual(t, mock.calls, 2, "should call LLM at least twice: DSML iteration + final answer")

	// Final content should contain the final answer, not raw DSML
	assert.Contains(t, result.Response, "VPS uptime: 15 hari.")
	assert.NotContains(t, result.Response, "<| DSML |")
}
