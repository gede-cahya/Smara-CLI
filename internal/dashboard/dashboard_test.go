package dashboard

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gede-cahya/Smara-CLI/internal/metrics"
	"github.com/stretchr/testify/assert"
)

func TestPanelConstants(t *testing.T) {
	assert.Equal(t, 0, panelPlatform)
	assert.Equal(t, 1, panelLLM)
	assert.Equal(t, 2, panelMCP)
	assert.Equal(t, 3, panelSessions)
	assert.Equal(t, 4, panelMemory)
	assert.Equal(t, 5, panelErrors)
}

func newTestModel() DashboardModel {
	return NewDashboardModel("/tmp/metrics.json", "/tmp/db", "1.0.0", 5*time.Second)
}

func TestNewDashboardModel(t *testing.T) {
	m := newTestModel()
	assert.Equal(t, "1.0.0", m.version)
	assert.Equal(t, "/tmp/metrics.json", m.metricsPath)
	assert.Equal(t, "/tmp/db", m.dbPath)
	assert.Equal(t, 0, m.activePanel)
	assert.Equal(t, 0, m.width)
	assert.False(t, m.offline)
}

func TestUpdate_KeyQuit(t *testing.T) {
	m := newTestModel()
	m.width = 120
	m.height = 40

	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	newM, cmd := m.Update(keyMsg)

	assert.Equal(t, m, newM.(DashboardModel)) // same model when quitting
	assert.NotNil(t, cmd)                     // tea.Quit
}

func TestUpdate_KeyCtrlC(t *testing.T) {
	m := newTestModel()
	m.width = 120

	keyMsg := tea.KeyMsg{Type: tea.KeyCtrlC}
	newM, cmd := m.Update(keyMsg)

	assert.Equal(t, m, newM.(DashboardModel))
	assert.NotNil(t, cmd)
}

func TestUpdate_KeyEscape(t *testing.T) {
	m := newTestModel()
	m.width = 120

	keyMsg := tea.KeyMsg{Type: tea.KeyEscape}
	newM, cmd := m.Update(keyMsg)

	assert.Equal(t, m, newM.(DashboardModel))
	assert.NotNil(t, cmd)
}

func TestUpdate_KeyRefresh(t *testing.T) {
	m := newTestModel()
	m.width = 120

	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	newM, cmd := m.Update(keyMsg)

	assert.NotNil(t, cmd)
	_ = newM.(DashboardModel)
}

func TestUpdate_KeyNavigate(t *testing.T) {
	m := newTestModel()
	m.width = 120

	// Tab forward
	keyMsg := tea.KeyMsg{Type: tea.KeyTab}
	newM, _ := m.Update(keyMsg)
	assert.Equal(t, 1, newM.(DashboardModel).activePanel)

	// Tab again
	newM2, _ := newM.(DashboardModel).Update(keyMsg)
	assert.Equal(t, 2, newM2.(DashboardModel).activePanel)

	// Shift+Tab backward
	shiftTab := tea.KeyMsg{Type: tea.KeyShiftTab}
	newM3, _ := newM2.(DashboardModel).Update(shiftTab)
	assert.Equal(t, 1, newM3.(DashboardModel).activePanel)
}

func TestUpdate_KeyJumpPanels(t *testing.T) {
	m := newTestModel()
	m.width = 120

	cases := []struct {
		key   rune
		panel int
	}{
		{'1', panelPlatform},
		{'2', panelLLM},
		{'3', panelMCP},
		{'4', panelSessions},
		{'5', panelMemory},
		{'6', panelErrors},
	}

	for _, c := range cases {
		keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{c.key}}
		newM, _ := m.Update(keyMsg)
		assert.Equal(t, c.panel, newM.(DashboardModel).activePanel, "panel %d", c.key)
		m = newM.(DashboardModel) // chain for next
	}
}

func TestUpdate_WindowSize(t *testing.T) {
	m := newTestModel()

	msg := tea.WindowSizeMsg{Width: 100, Height: 30}
	newM, cmd := m.Update(msg)

	assert.Equal(t, 100, newM.(DashboardModel).width)
	assert.Equal(t, 30, newM.(DashboardModel).height)
	assert.Nil(t, cmd) // no periodic cmd on resize
}

func TestView_Loading(t *testing.T) {
	m := newTestModel()
	m.width = 0

	view := m.View()
	assert.Equal(t, "Loading...", view)
}

func TestView_WithMetrics(t *testing.T) {
	m := newTestModel()
	m.width = 120
	m.height = 40
	m.metrics = &metrics.Metrics{
		StartedAt:      time.Now().Add(-time.Hour),
		ActiveSessions: 3,
		LLM:            metrics.LLMMetrics{Provider: "openai", Model: "gpt-4", TotalRequests: 10},
		Memory:         metrics.MemoryMetrics{TotalMemories: 100, UnsyncedCount: 5},
		Sync:           metrics.SyncMetrics{Enabled: true, Status: "ok", LastSyncAt: time.Now()},
	}

	view := m.View()
	assert.Contains(t, view, "Smara Dashboard")
	assert.Contains(t, view, "Platform Status")
	assert.Contains(t, view, "LLM Usage")
	assert.Contains(t, view, "MCP Servers")
	assert.Contains(t, view, "Active Sessions")
	assert.Contains(t, view, "Memory")
}

func TestView_ErrorState(t *testing.T) {
	m := newTestModel()
	m.width = 120
	m.height = 40
	m.err = assert.AnError

	view := m.View()
	assert.Contains(t, view, "Tidak bisa membaca data")
}

func TestView_NilMetrics(t *testing.T) {
	m := newTestModel()
	m.width = 120
	m.height = 40

	view := m.View()
	assert.Contains(t, view, "Memuat data")
}

func TestRenderOnce_Output(t *testing.T) {
	// ReadFromDB never returns error (it swallows query errors), so output is always produced
	result := RenderOnce("/nonexistent/metrics.json", "/nonexistent/db", "1.0.0")
	assert.Contains(t, result, "Smara Dashboard")
	assert.Contains(t, result, "Snapshot")
}
