package dashboard

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gede-cahya/Smara-CLI/internal/metrics"
)

const (
	panelPlatform = iota
	panelLLM
	panelMCP
	panelSessions
	panelMemory
	panelErrors
	panelCount
)

// tickMsg is sent on each refresh interval.
type tickMsg time.Time

// DashboardModel is the bubbletea model for the dashboard TUI.
type DashboardModel struct {
	metrics     *metrics.Metrics
	metricsPath string
	dbPath      string
	activePanel int
	width       int
	height      int
	interval    time.Duration
	stale       bool
	offline     bool // true when serve is not running
	err         error
	version     string
}

// NewDashboardModel creates a new dashboard model.
func NewDashboardModel(metricsPath, dbPath, version string, interval time.Duration) DashboardModel {
	return DashboardModel{
		metricsPath: metricsPath,
		dbPath:      dbPath,
		interval:    interval,
		version:     version,
	}
}

// Init initializes the model.
func (m DashboardModel) Init() tea.Cmd {
	return tea.Batch(m.loadMetrics(), m.tick())
}

func (m DashboardModel) tick() tea.Cmd {
	return tea.Tick(m.interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m DashboardModel) loadMetrics() tea.Cmd {
	return func() tea.Msg {
		return tickMsg(time.Now())
	}
}

// Update handles messages.
func (m DashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "r":
			return m, m.loadMetrics()
		case "tab":
			m.activePanel = (m.activePanel + 1) % panelCount
		case "shift+tab":
			m.activePanel = (m.activePanel - 1 + panelCount) % panelCount
		case "1":
			m.activePanel = panelPlatform
		case "2":
			m.activePanel = panelLLM
		case "3":
			m.activePanel = panelMCP
		case "4":
			m.activePanel = panelSessions
		case "5":
			m.activePanel = panelMemory
		case "6":
			m.activePanel = panelErrors
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		// Try to read live metrics first
		met, err := metrics.ReadMetrics(m.metricsPath)
		if err == nil {
			m.metrics = met
			m.stale = metrics.IsMetricsStale(met, 10*time.Second)
			m.offline = false
			m.err = nil
		} else {
			// Fallback to database
			dbMet, dbErr := metrics.ReadFromDB(m.dbPath)
			if dbErr == nil {
				m.metrics = dbMet
				m.offline = true
				m.stale = false
				m.err = nil
			} else {
				m.err = err
				m.offline = true
			}
		}
		return m, m.tick()
	}

	return m, nil
}

// View renders the dashboard.
func (m DashboardModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	var sections []string

	// Header
	sections = append(sections, m.renderHeader())

	if m.err != nil && m.metrics == nil {
		sections = append(sections, errorStyle.Render(fmt.Sprintf("\n  Tidak bisa membaca data: %v\n  Pastikan 'smara serve' berjalan atau database ada di %s", m.err, m.dbPath)))
		sections = append(sections, m.renderFooter())
		return strings.Join(sections, "\n")
	}

	if m.metrics == nil {
		sections = append(sections, dimStyle.Render("\n  Memuat data..."))
		sections = append(sections, m.renderFooter())
		return strings.Join(sections, "\n")
	}

	// Calculate panel widths
	fullWidth := m.width - 2
	if fullWidth < 40 {
		fullWidth = 40
	}
	halfWidth := fullWidth/2 - 1
	if halfWidth < 20 {
		halfWidth = 20
	}

	// Row 1: Platform Status + LLM Usage
	row1Left := m.renderPanel(panelPlatform, "Platform Status", m.renderPlatformContent(), halfWidth)
	row1Right := m.renderPanel(panelLLM, "LLM Usage", m.renderLLMContent(), halfWidth)
	row1 := lipgloss.JoinHorizontal(lipgloss.Top, row1Left, " ", row1Right)
	sections = append(sections, row1)

	// Row 2: MCP Servers (full width)
	row2 := m.renderPanel(panelMCP, "MCP Servers", m.renderMCPContent(), fullWidth)
	sections = append(sections, row2)

	// Row 3: Sessions (full width)
	row3 := m.renderPanel(panelSessions, "Active Sessions", m.renderSessionsContent(), fullWidth)
	sections = append(sections, row3)

	// Row 4: Memory & Sync + Recent Errors
	row4Left := m.renderPanel(panelMemory, "Memory & Sync", m.renderMemoryContent(), halfWidth)
	row4Right := m.renderPanel(panelErrors, "Recent Errors", m.renderErrorsContent(), halfWidth)
	row4 := lipgloss.JoinHorizontal(lipgloss.Top, row4Left, " ", row4Right)
	sections = append(sections, row4)

	// Footer
	sections = append(sections, m.renderFooter())

	return strings.Join(sections, "\n")
}

func (m DashboardModel) renderHeader() string {
	title := headerStyle.Render(" 🌀 Smara Dashboard ")

	var rightParts []string
	rightParts = append(rightParts, dimStyle.Render("v"+m.version))

	if m.metrics != nil && !m.metrics.StartedAt.IsZero() {
		uptime := time.Since(m.metrics.StartedAt).Round(time.Second)
		rightParts = append(rightParts, dimStyle.Render(fmt.Sprintf("⏱ Uptime: %s", formatDuration(uptime))))
	}

	if m.offline {
		rightParts = append(rightParts, warnStyle.Render("⚠ Serve offline"))
	} else if m.stale {
		rightParts = append(rightParts, warnStyle.Render("⚠ Data stale"))
	}

	right := strings.Join(rightParts, "  ")

	gap := m.width - lipgloss.Width(title) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}

	return title + strings.Repeat(" ", gap) + right
}

func (m DashboardModel) renderPanel(id int, title, content string, width int) string {
	style := panelStyle
	if id == m.activePanel {
		style = panelActiveStyle
	}

	titleRendered := panelTitleStyle.Render(title)
	body := titleRendered + "\n" + content

	return style.Width(width - 4).Render(body)
}

func (m DashboardModel) renderPlatformContent() string {
	if m.metrics == nil || len(m.metrics.Platforms) == 0 {
		if m.offline {
			return dimStyle.Render("Serve tidak aktif")
		}
		return dimStyle.Render("Tidak ada platform aktif")
	}

	var lines []string
	var totalIn, totalOut int64

	for name, pm := range m.metrics.Platforms {
		status := statusOnline.String()
		if pm.Status != "online" {
			status = statusOffline.String()
		}
		line := fmt.Sprintf("%s %-12s %s  %s",
			status,
			name,
			labelStyle.Render(pm.Status),
			valueStyle.Render(fmt.Sprintf("%d", pm.MessagesIn)),
		)
		lines = append(lines, line)
		totalIn += pm.MessagesIn
		totalOut += pm.MessagesOut
	}

	lines = append(lines, "")
	lines = append(lines, dimStyle.Render(fmt.Sprintf("Total: %d in / %d out", totalIn, totalOut)))

	return strings.Join(lines, "\n")
}

func (m DashboardModel) renderLLMContent() string {
	if m.metrics == nil {
		return dimStyle.Render("No data")
	}

	llm := m.metrics.LLM
	var lines []string

	lines = append(lines, fmt.Sprintf("%s %s", labelStyle.Render("Provider:"), valueStyle.Render(llm.Provider)))
	lines = append(lines, fmt.Sprintf("%s %s", labelStyle.Render("Model:  "), valueStyle.Render(llm.Model)))
	lines = append(lines, fmt.Sprintf("%s %s", labelStyle.Render("Requests:"), valueStyle.Render(fmt.Sprintf("%d", llm.TotalRequests))))

	inK := float64(llm.InputTokens) / 1000
	outK := float64(llm.OutputTokens) / 1000
	lines = append(lines, fmt.Sprintf("%s %s",
		labelStyle.Render("Tokens: "),
		valueStyle.Render(fmt.Sprintf("%.1fK in / %.1fK out", inK, outK)),
	))

	lines = append(lines, fmt.Sprintf("%s %s",
		labelStyle.Render("Cost:   "),
		greenStyle.Render(fmt.Sprintf("~$%.4f", llm.EstimatedCostUSD)),
	))

	if llm.AvgLatencyMs > 0 {
		lines = append(lines, fmt.Sprintf("%s %s",
			labelStyle.Render("Latency:"),
			valueStyle.Render(fmt.Sprintf("%dms avg", llm.AvgLatencyMs)),
		))
	}

	return strings.Join(lines, "\n")
}

func (m DashboardModel) renderMCPContent() string {
	if m.metrics == nil || len(m.metrics.MCP) == 0 {
		return dimStyle.Render("Tidak ada MCP server")
	}

	var lines []string
	for _, mc := range m.metrics.MCP {
		status := statusOnline.String()
		if !mc.Connected {
			status = statusOffline.String()
		}
		errStr := ""
		if mc.ErrorCount > 0 {
			errStr = errorStyle.Render(fmt.Sprintf("  %d errors", mc.ErrorCount))
		}
		line := fmt.Sprintf("%s %-16s %s  %s  %s%s",
			status,
			mc.Name,
			labelStyle.Render(boolStatus(mc.Connected)),
			valueStyle.Render(fmt.Sprintf("%d tools", mc.ToolCount)),
			dimStyle.Render(fmt.Sprintf("%d calls", mc.CallCount)),
			errStr,
		)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (m DashboardModel) renderSessionsContent() string {
	if m.metrics == nil {
		return dimStyle.Render("No data")
	}

	if m.metrics.ActiveSessions == 0 {
		return dimStyle.Render("Tidak ada session aktif")
	}

	return valueStyle.Render(fmt.Sprintf("%d session aktif", m.metrics.ActiveSessions))
}

func (m DashboardModel) renderMemoryContent() string {
	if m.metrics == nil {
		return dimStyle.Render("No data")
	}

	mem := m.metrics.Memory
	syn := m.metrics.Sync

	var lines []string
	lines = append(lines, fmt.Sprintf("%s  %s", labelStyle.Render("Memories:"), valueStyle.Render(fmt.Sprintf("%d", mem.TotalMemories))))
	lines = append(lines, fmt.Sprintf("%s  %s", labelStyle.Render("Unsynced:"), valueStyle.Render(fmt.Sprintf("%d", mem.UnsyncedCount))))
	lines = append(lines, fmt.Sprintf("%s  %s", labelStyle.Render("DB Size: "), valueStyle.Render(formatBytes(mem.DBSizeBytes))))

	if syn.Enabled {
		syncStatusIcon := statusOnline.String()
		if syn.Status == "error" {
			syncStatusIcon = statusOffline.String()
		} else if syn.Status == "syncing" {
			syncStatusIcon = statusWarning.String()
		}

		lastSync := "never"
		if !syn.LastSyncAt.IsZero() {
			lastSync = timeAgo(syn.LastSyncAt)
		}

		lines = append(lines, fmt.Sprintf("%s  %s %s", labelStyle.Render("Sync:    "), syncStatusIcon, dimStyle.Render(syn.Status)))
		lines = append(lines, fmt.Sprintf("%s  %s", labelStyle.Render("Last:    "), dimStyle.Render(lastSync)))
	} else {
		lines = append(lines, fmt.Sprintf("%s  %s", labelStyle.Render("Sync:    "), dimStyle.Render("disabled")))
	}

	return strings.Join(lines, "\n")
}

func (m DashboardModel) renderErrorsContent() string {
	if m.metrics == nil || len(m.metrics.RecentErrors) == 0 {
		return greenStyle.Render("Tidak ada error")
	}

	var lines []string
	max := 8
	errors := m.metrics.RecentErrors
	if len(errors) > max {
		errors = errors[len(errors)-max:]
	}

	for _, e := range errors {
		ts := e.Timestamp.Format("15:04")
		source := fmt.Sprintf("[%s]", e.Source)
		msg := e.Message
		if len(msg) > 40 {
			msg = msg[:40] + "..."
		}
		lines = append(lines, fmt.Sprintf("%s %s %s",
			dimStyle.Render(ts),
			warnStyle.Render(source),
			errorStyle.Render(msg),
		))
	}

	return strings.Join(lines, "\n")
}

func (m DashboardModel) renderFooter() string {
	keys := []struct{ key, desc string }{
		{"q", "Quit"},
		{"r", "Refresh"},
		{"tab", "Navigate"},
		{"1-6", "Jump Panel"},
	}

	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %s", footerKeyStyle.Render("["+k.key+"]"), footerStyle.Render(k.desc)))
	}

	return "\n" + strings.Join(parts, "  ")
}

// RenderOnce renders a non-interactive snapshot to stdout.
func RenderOnce(metricsPath, dbPath, version string) string {
	var m *metrics.Metrics
	var offline bool

	met, err := metrics.ReadMetrics(metricsPath)
	if err == nil {
		m = met
		offline = false
	} else {
		dbMet, dbErr := metrics.ReadFromDB(dbPath)
		if dbErr == nil {
			m = dbMet
			offline = true
		} else {
			return fmt.Sprintf("Error: tidak bisa membaca data (%v)", err)
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🌀 Smara Dashboard — Snapshot at %s\n\n", time.Now().Format("2006-01-02 15:04:05")))

	if offline {
		sb.WriteString("⚠  Serve tidak aktif — menampilkan data tersimpan\n\n")
	}

	// Platforms
	sb.WriteString("Platforms:\n")
	if len(m.Platforms) > 0 {
		for name, pm := range m.Platforms {
			icon := "●"
			if pm.Status != "online" {
				icon = "○"
			}
			sb.WriteString(fmt.Sprintf("  %s %-12s %-8s %d msgs   %d active users\n",
				icon, name, pm.Status, pm.MessagesIn, pm.ActiveUsers))
		}
	} else {
		sb.WriteString("  (tidak ada platform aktif)\n")
	}

	// LLM
	sb.WriteString(fmt.Sprintf("\nLLM: %s %s | %d requests | %dK tokens | ~$%.4f\n",
		m.LLM.Provider, m.LLM.Model, m.LLM.TotalRequests,
		(m.LLM.InputTokens+m.LLM.OutputTokens)/1000, m.LLM.EstimatedCostUSD))

	// MCP
	if len(m.MCP) > 0 {
		sb.WriteString("\nMCP: ")
		var mcpParts []string
		for _, mc := range m.MCP {
			mcpParts = append(mcpParts, fmt.Sprintf("%s (%d tools, %d calls)", mc.Name, mc.ToolCount, mc.CallCount))
		}
		sb.WriteString(strings.Join(mcpParts, " | "))
		sb.WriteString("\n")
	}

	// Sessions + Memory
	sb.WriteString(fmt.Sprintf("\nSessions: %d active | Memory: %d entries (%s) | Sync: %s\n",
		m.ActiveSessions, m.Memory.TotalMemories, formatBytes(m.Memory.DBSizeBytes), m.Sync.Status))

	// Errors
	if len(m.RecentErrors) > 0 {
		sb.WriteString(fmt.Sprintf("\nRecent Errors (%d):\n", len(m.RecentErrors)))
		max := 5
		errors := m.RecentErrors
		if len(errors) > max {
			errors = errors[len(errors)-max:]
		}
		for _, e := range errors {
			sb.WriteString(fmt.Sprintf("  %s [%s] %s\n", e.Timestamp.Format("15:04"), e.Source, e.Message))
		}
	}

	return sb.String()
}

// --- helpers ---

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return t.Format("02 Jan 15:04")
	}
}

func boolStatus(b bool) string {
	if b {
		return "connected"
	}
	return "disconnected"
}
