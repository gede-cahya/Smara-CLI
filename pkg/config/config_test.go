package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMCPServer_Struct(t *testing.T) {
	server := MCPServer{
		Name:    "test-server",
		Command: "node",
		Args:    []string{"server.js"},
		Env:     map[string]string{"NODE_ENV": "test"},
	}
	assert.Equal(t, "test-server", server.Name)
	assert.Equal(t, "node", server.Command)
	assert.Equal(t, []string{"server.js"}, server.Args)
	assert.Equal(t, "test", server.Env["NODE_ENV"])
}

func TestPlatformBotConfig_Struct(t *testing.T) {
	cfg := PlatformBotConfig{
		Enabled:      true,
		Token:        "bot_token_123",
		AllowedUsers: []string{"user1", "user2"},
		BlockedUsers: []string{"spammer"},
		RateLimit:    60,
		RateBurst:    5,
	}
	assert.True(t, cfg.Enabled)
	assert.Equal(t, "bot_token_123", cfg.Token)
	assert.Len(t, cfg.AllowedUsers, 2)
	assert.Equal(t, 60, cfg.RateLimit)
	assert.Equal(t, 5, cfg.RateBurst)
}

func TestWhatsAppConfig_Struct(t *testing.T) {
	cfg := WhatsAppConfig{
		Enabled:        true,
		SessionDir:     "/tmp/whatsapp",
		AllowedNumbers: []string{"+628123456789"},
		RateLimit:      30,
		RateBurst:      3,
	}
	assert.True(t, cfg.Enabled)
	assert.Equal(t, "/tmp/whatsapp", cfg.SessionDir)
	assert.Len(t, cfg.AllowedNumbers, 1)
}

func TestSmaraConfig_Struct(t *testing.T) {
	cfg := SmaraConfig{
		Provider:        "ollama",
		Model:           "llama3.1",
		OllamaHost:      "http://localhost:11434",
		OpenAIAPIKey:    "",
		SyncDir:         "/tmp/sync",
		SyncInterval:    5,
		Verbose:         false,
		DBPath:          "/tmp/smara.db",
		ActiveWorkspace: "default",
		MCPServers: []MCPServer{
			{Name: "figma", Command: "npx", Args: []string{"-y", "@figma/mcp"}},
		},
	}
	assert.Equal(t, "ollama", cfg.Provider)
	assert.Equal(t, "llama3.1", cfg.Model)
	assert.Equal(t, "http://localhost:11434", cfg.OllamaHost)
	assert.Equal(t, "/tmp/smara.db", cfg.DBPath)
	assert.Len(t, cfg.MCPServers, 1)
	assert.Equal(t, "figma", cfg.MCPServers[0].Name)
}

func TestPlatformConfig_Struct(t *testing.T) {
	cfg := PlatformConfig{
		Telegram: PlatformBotConfig{
			Enabled: true,
			Token:   "tg_token",
		},
		Discord: PlatformBotConfig{
			Enabled: false,
		},
		WhatsApp: WhatsAppConfig{
			Enabled: true,
		},
		MaxResponseLen:   4096,
		TypingIndicator:  true,
		LogConversations: false,
	}
	assert.True(t, cfg.Telegram.Enabled)
	assert.False(t, cfg.Discord.Enabled)
	assert.True(t, cfg.WhatsApp.Enabled)
	assert.Equal(t, 4096, cfg.MaxResponseLen)
	assert.True(t, cfg.TypingIndicator)
}

func TestSmaraConfig_Defaults(t *testing.T) {
	cfg := SmaraConfig{}
	assert.Empty(t, cfg.Provider)
	assert.Empty(t, cfg.Model)
	assert.Equal(t, int64(0), cfg.ActiveWorkspaceID)
	assert.Empty(t, cfg.MCPServers)
	assert.False(t, cfg.SmaraMCPEnabled)
}
