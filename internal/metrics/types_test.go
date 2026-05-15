package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMetrics_Struct(t *testing.T) {
	now := time.Now()
	m := Metrics{
		StartedAt:      now,
		UpdatedAt:      now,
		Platforms:      map[string]*PlatformMetrics{"whatsapp": {Name: "whatsapp"}},
		LLM:            LLMMetrics{Provider: "ollama", TotalRequests: 100},
		MCP:            []MCPMetrics{{Name: "test", Connected: true}},
		Memory:         MemoryMetrics{TotalMemories: 50},
		Sync:           SyncMetrics{Enabled: true, Status: "idle"},
		RecentErrors:   []ErrorEntry{{Source: "llm", Message: "error"}},
		ActiveSessions: 3,
	}
	assert.Equal(t, now, m.StartedAt)
	assert.Equal(t, now, m.UpdatedAt)
	assert.Len(t, m.Platforms, 1)
	assert.Equal(t, "whatsapp", m.Platforms["whatsapp"].Name)
	assert.Equal(t, "ollama", m.LLM.Provider)
	assert.Equal(t, int64(100), m.LLM.TotalRequests)
	assert.Len(t, m.MCP, 1)
	assert.True(t, m.MCP[0].Connected)
	assert.Equal(t, 50, m.Memory.TotalMemories)
	assert.True(t, m.Sync.Enabled)
	assert.Equal(t, "idle", m.Sync.Status)
	assert.Len(t, m.RecentErrors, 1)
	assert.Equal(t, 3, m.ActiveSessions)
}

func TestPlatformMetrics_Struct(t *testing.T) {
	pm := PlatformMetrics{
		Name:         "discord",
		Status:       "online",
		MessagesIn:   100,
		MessagesOut:  50,
		ActiveUsers:  10,
		ErrorCount:   2,
		TopUsers:     []UserActivity{{UserID: "u1", Requests: 5}},
		AvgLatencyMs: 150,
	}
	assert.Equal(t, "discord", pm.Name)
	assert.Equal(t, "online", pm.Status)
	assert.Equal(t, int64(100), pm.MessagesIn)
	assert.Equal(t, int64(50), pm.MessagesOut)
	assert.Equal(t, 10, pm.ActiveUsers)
	assert.Equal(t, int64(2), pm.ErrorCount)
	assert.Len(t, pm.TopUsers, 1)
	assert.Equal(t, int64(150), pm.AvgLatencyMs)
}

func TestLLMMetrics_Struct(t *testing.T) {
	lm := LLMMetrics{
		Provider:         "openai",
		Model:            "gpt-4o",
		TotalRequests:    200,
		InputTokens:      10000,
		OutputTokens:     5000,
		EstimatedCostUSD: 1.5,
		AvgLatencyMs:     300,
	}
	assert.Equal(t, "openai", lm.Provider)
	assert.Equal(t, "gpt-4o", lm.Model)
	assert.Equal(t, int64(200), lm.TotalRequests)
	assert.Equal(t, int64(10000), lm.InputTokens)
	assert.Equal(t, int64(5000), lm.OutputTokens)
	assert.Equal(t, 1.5, lm.EstimatedCostUSD)
	assert.Equal(t, int64(300), lm.AvgLatencyMs)
}

func TestMCPMetrics_Struct(t *testing.T) {
	mm := MCPMetrics{
		Name:       "filesystem",
		Connected:  true,
		ToolCount:  10,
		CallCount:  50,
		ErrorCount: 1,
	}
	assert.Equal(t, "filesystem", mm.Name)
	assert.True(t, mm.Connected)
	assert.Equal(t, 10, mm.ToolCount)
	assert.Equal(t, int64(50), mm.CallCount)
	assert.Equal(t, int64(1), mm.ErrorCount)
}

func TestMemoryMetrics_Struct(t *testing.T) {
	mem := MemoryMetrics{
		TotalMemories: 100,
		UnsyncedCount: 5,
		DBSizeBytes:   1024000,
	}
	assert.Equal(t, 100, mem.TotalMemories)
	assert.Equal(t, 5, mem.UnsyncedCount)
	assert.Equal(t, int64(1024000), mem.DBSizeBytes)
}

func TestSyncMetrics_Struct(t *testing.T) {
	now := time.Now()
	sm := SyncMetrics{
		Enabled:       true,
		LastSyncAt:    now,
		PendingDeltas: 3,
		Status:        "syncing",
	}
	assert.True(t, sm.Enabled)
	assert.Equal(t, now, sm.LastSyncAt)
	assert.Equal(t, 3, sm.PendingDeltas)
	assert.Equal(t, "syncing", sm.Status)
}

func TestErrorEntry_Struct(t *testing.T) {
	now := time.Now()
	ee := ErrorEntry{
		Timestamp: now,
		Source:    "telegram",
		Message:   "connection failed",
	}
	assert.Equal(t, now, ee.Timestamp)
	assert.Equal(t, "telegram", ee.Source)
	assert.Equal(t, "connection failed", ee.Message)
}

func TestUserActivity_Struct(t *testing.T) {
	ua := UserActivity{
		UserID:   "user123",
		Username: "testuser",
		Platform: "discord",
		Requests: 42,
	}
	assert.Equal(t, "user123", ua.UserID)
	assert.Equal(t, "testuser", ua.Username)
	assert.Equal(t, "discord", ua.Platform)
	assert.Equal(t, int64(42), ua.Requests)
}
