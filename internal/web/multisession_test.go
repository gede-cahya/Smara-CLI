package web

import (
	"path/filepath"
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/mcp"
	"github.com/stretchr/testify/require"
)

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
