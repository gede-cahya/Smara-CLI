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
