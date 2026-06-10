package web

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/mcp"
	"github.com/stretchr/testify/require"
)

type blockingWebSessionProvider struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *blockingWebSessionProvider) Name() string { return "blocking-web-session" }

func (p *blockingWebSessionProvider) Chat(_ []llm.Message) (*llm.ChatResponse, error) {
	p.once.Do(func() { close(p.started) })
	<-p.release
	return &llm.ChatResponse{Content: "old response"}, nil
}

func (p *blockingWebSessionProvider) ChatWithTools(_ []llm.Message, _ []llm.ToolFunction) (*llm.ChatResponse, []llm.ToolCall, error) {
	resp, err := p.Chat(nil)
	return resp, nil, err
}

func (p *blockingWebSessionProvider) GenerateEmbedding(string) ([]float32, error) {
	return nil, nil
}

func TestWebSessionManagerCopiesMCPConnectionsToSessionSupervisor(t *testing.T) {
	manager := NewWebSessionManager(nil, llm.ProviderConfig{}, nil, "default", 1, 0, filepath.Join(t.TempDir(), "sessions.json"))
	manager.SetMCPConnections(
		map[string]*mcp.Client{"obsidian": nil},
		map[string]agent.MCPServerInfo{
			"obsidian": {
				Name:      "obsidian",
				Connected: true,
				Tools:     []mcp.Tool{{Name: "obsidian_update_note"}},
			},
		},
	)

	session := manager.Create("Test", string(agent.ModeAsk))
	supervisor := manager.ensureSupervisor(session)
	info := supervisor.GetMCPInfo()

	require.Contains(t, info, "obsidian")
	require.True(t, info["obsidian"].Connected)
	require.Len(t, info["obsidian"].Tools, 1)
	require.Equal(t, "obsidian_update_note", info["obsidian"].Tools[0].Name)
}

func TestWebSessionManagerRecordDirectResultPersistsWorkflowResponse(t *testing.T) {
	manager := NewWebSessionManager(nil, llm.ProviderConfig{}, nil, "default", 1, 0, filepath.Join(t.TempDir(), "sessions.json"))
	session := manager.Create("Workflow", string(agent.ModeWorkflow))

	err := manager.RecordDirectResult(session.ID, "jalankan workflow release", string(agent.ModeWorkflow), "workflow selesai", WebSessionCompleted, "")
	require.NoError(t, err)

	dto, ok := manager.GetCompact(session.ID, 10)
	require.True(t, ok)
	require.Equal(t, WebSessionCompleted, dto.Status)
	require.Len(t, dto.History, 3)
	require.Equal(t, "user", dto.History[1].Role)
	require.Equal(t, "jalankan workflow release", dto.History[1].Content)
	require.Equal(t, "assistant", dto.History[2].Role)
	require.Equal(t, "workflow selesai", dto.History[2].Content)
}

func TestWebSessionManagerStaleRunCannotOverwriteReplacementStatus(t *testing.T) {
	provider := &blockingWebSessionProvider{started: make(chan struct{}), release: make(chan struct{})}
	manager := NewWebSessionManager(provider, llm.ProviderConfig{}, nil, "default", 1, 0, filepath.Join(t.TempDir(), "sessions.json"))
	session := manager.Create("Race", string(agent.ModeAsk))

	done := make(chan error, 1)
	go func() {
		_, err := manager.Run(context.Background(), session.ID, "old prompt", string(agent.ModeAsk), agent.AgenticCallback{})
		done <- err
	}()

	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("old run did not start")
	}

	session.mu.Lock()
	oldCancel := session.cancel
	session.activeRun++
	session.Status = WebSessionRunning
	session.Error = ""
	session.cancel = nil
	session.mu.Unlock()

	require.NotNil(t, oldCancel)
	oldCancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("old run did not stop after cancellation")
	}
	close(provider.release)

	dto, ok := manager.GetCompact(session.ID, 10)
	require.True(t, ok)
	require.Equal(t, WebSessionRunning, dto.Status)
	require.Empty(t, dto.Error)
}
