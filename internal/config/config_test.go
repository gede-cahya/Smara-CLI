package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, "ollama", cfg.Provider)
	assert.Equal(t, "minimax-m2.5:cloud", cfg.Model)
	assert.Equal(t, "http://localhost:11434", cfg.OllamaHost)
	assert.Equal(t, "", cfg.OpenAIAPIKey)
	assert.Equal(t, "gpt-4o", cfg.OpenAIModel)
	assert.Equal(t, "", cfg.OpenRouterAPIKey)
	assert.Equal(t, "anthropic/claude-sonnet-4", cfg.OpenRouterModel)
	assert.Equal(t, "", cfg.AnthropicAPIKey)
	assert.Equal(t, "claude-sonnet-4-20250514", cfg.AnthropicModel)
	assert.Equal(t, "", cfg.CustomAPIKey)
	assert.Equal(t, "https://api.openai.com/v1", cfg.CustomBaseURL)
	assert.Equal(t, 15, cfg.SyncInterval)
	assert.Empty(t, cfg.MCPServers)
	assert.False(t, cfg.Verbose)
	assert.Equal(t, "default", cfg.ActiveWorkspace)
	assert.Equal(t, int64(0), cfg.ActiveWorkspaceID)
}

func TestDefaultConfig_DirPath(t *testing.T) {
	cfg := DefaultConfig()
	assert.Contains(t, cfg.SyncDir, ".smara")
	assert.Contains(t, cfg.DBPath, ".smara")
	assert.Contains(t, cfg.DBPath, "memory.db")
}

func TestDefaultConfig_Platforms(t *testing.T) {
	cfg := DefaultConfig()
	assert.False(t, cfg.Platforms.WhatsApp.Enabled)
	assert.Contains(t, cfg.Platforms.WhatsApp.SessionDir, "wa-session")
	assert.Equal(t, 10, cfg.Platforms.WhatsApp.RateLimit)
	assert.Equal(t, 3, cfg.Platforms.WhatsApp.RateBurst)
	assert.Equal(t, 4000, cfg.Platforms.MaxResponseLen)
	assert.True(t, cfg.Platforms.TypingIndicator)
}

func TestSmaraConfig_Struct(t *testing.T) {
	cfg := &SmaraConfig{
		Provider:          "openai",
		Model:             "gpt-4o",
		Verbose:           true,
		OpenAIAPIKey:      "test-key",
		ActiveWorkspace:   "test-workspace",
		ActiveWorkspaceID: 42,
	}
	assert.Equal(t, "openai", cfg.Provider)
	assert.Equal(t, "gpt-4o", cfg.Model)
	assert.True(t, cfg.Verbose)
	assert.Equal(t, "test-key", cfg.OpenAIAPIKey)
	assert.Equal(t, "test-workspace", cfg.ActiveWorkspace)
	assert.Equal(t, int64(42), cfg.ActiveWorkspaceID)
}

func TestMCPServer_Struct(t *testing.T) {
	server := MCPServer{
		Name:    "test-server",
		Command: "npx",
		Args:    []string{"-y", "server-filesystem"},
		Env:     map[string]string{"NODE_ENV": "production"},
	}
	assert.Equal(t, "test-server", server.Name)
	assert.Equal(t, "npx", server.Command)
	assert.Equal(t, []string{"-y", "server-filesystem"}, server.Args)
	assert.Equal(t, "production", server.Env["NODE_ENV"])
}

func TestPlatformConfig_Struct(t *testing.T) {
	pc := PlatformConfig{
		WhatsApp: WhatsAppConfig{
			Enabled:    true,
			SessionDir: "/tmp/wa",
			RateLimit:  20,
			RateBurst:  5,
		},
		Discord: PlatformBotConfig{
			Token:     "test-token",
			Enabled:   true,
			GuildIDs:  []string{"12345"},
		},
	}
	assert.True(t, pc.WhatsApp.Enabled)
	assert.Equal(t, "/tmp/wa", pc.WhatsApp.SessionDir)
	assert.Equal(t, "test-token", pc.Discord.Token)
	assert.True(t, pc.Discord.Enabled)
	assert.Equal(t, []string{"12345"}, pc.Discord.GuildIDs)
}

func TestWhatsAppConfig_Struct(t *testing.T) {
	w := WhatsAppConfig{
		Enabled:    true,
		SessionDir: "/tmp/wa",
		RateLimit:  10,
		RateBurst:  3,
	}
	assert.True(t, w.Enabled)
	assert.Equal(t, "/tmp/wa", w.SessionDir)
	assert.Equal(t, 10, w.RateLimit)
	assert.Equal(t, 3, w.RateBurst)
}

func TestPlatformBotConfig_Struct(t *testing.T) {
	pb := PlatformBotConfig{
		Token:        "token123",
		Enabled:      true,
		AllowedUsers: []string{"user1"},
		GuildIDs:     []string{"guild1"},
		RateLimit:    10,
		RateBurst:    3,
	}
	assert.Equal(t, "token123", pb.Token)
	assert.True(t, pb.Enabled)
	assert.Equal(t, []string{"user1"}, pb.AllowedUsers)
	assert.Equal(t, []string{"guild1"}, pb.GuildIDs)
	assert.Equal(t, 10, pb.RateLimit)
	assert.Equal(t, 3, pb.RateBurst)
}
