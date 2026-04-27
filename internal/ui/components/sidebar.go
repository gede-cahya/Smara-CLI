package components

import (
	"fmt"
	"strings"
	"time"
)

// ═══════════════════════════════════════════════════════════════
// Sidebar Component — Right Panel with Session/Stats/Tools Info
// ═══════════════════════════════════════════════════════════════

// Sidebar renders the right-side information panel.
type Sidebar struct {
	theme  *Theme
	width  int
	height int
}

// NewSidebar creates a new sidebar component.
func NewSidebar(width, height int) *Sidebar {
	return &Sidebar{
		theme:  GetTheme(),
		width:  width,
		height: height,
	}
}

// SetSize updates sidebar dimensions.
func (s *Sidebar) SetSize(width, height int) {
	s.width = width
	s.height = height
}

// SidebarData holds the data to display in the sidebar.
type SidebarData struct {
	SessionName string
	SessionID   string
	Mode        string
	Provider    string
	Model       string
	Messages    int
	InTokens    int
	OutTokens   int
	Tools       []string
	MCPs        []string
	Elapsed     time.Duration
}

// Render renders the full sidebar panel.
func (s *Sidebar) Render(data SidebarData) string {
	var sb strings.Builder
	innerW := s.width - 4
	if innerW < 10 {
		innerW = 10
	}

	// ── Title ──────────────────────────────────────────
	sb.WriteString(s.theme.SidebarTitle.Render("📊 Info Panel"))
	sb.WriteString("\n")

	// ── Session ────────────────────────────────────────
	sb.WriteString(s.theme.SidebarTitle.Render("Session"))
	sb.WriteString("\n")
	if data.SessionName != "" {
		sb.WriteString(s.theme.SidebarItem.Render(fmt.Sprintf("%s", data.SessionName)))
		sb.WriteString("\n")
	}
	if data.SessionID != "" {
		idStr := data.SessionID
		if len(idStr) > 8 {
			idStr = idStr[:8]
		}
		sb.WriteString(s.theme.SidebarItem.Render(fmt.Sprintf("ID: %s", idStr)))
		sb.WriteString("\n")
	}
	sb.WriteString(s.theme.SidebarItem.Render(fmt.Sprintf("Msgs: %d", data.Messages)))
	sb.WriteString("\n\n")

	// ── Model ──────────────────────────────────────────
	sb.WriteString(s.theme.SidebarTitle.Render("Model"))
	sb.WriteString("\n")
	sb.WriteString(s.theme.SidebarItem.Render(fmt.Sprintf("%s", data.Provider)))
	sb.WriteString("\n")
	modelName := data.Model
	if len(modelName) > innerW-2 {
		modelName = modelName[:innerW-5] + "..."
	}
	sb.WriteString(s.theme.SidebarItem.Render(modelName))
	sb.WriteString("\n\n")

	// ── Stats ──────────────────────────────────────────
	sb.WriteString(s.theme.SidebarTitle.Render("Stats"))
	sb.WriteString("\n")
	sb.WriteString(s.renderStatLine("In", fmt.Sprintf("%d", data.InTokens)))
	sb.WriteString(s.renderStatLine("Out", fmt.Sprintf("%d", data.OutTokens)))
	sb.WriteString(s.renderStatLine("Total", fmt.Sprintf("%d", data.InTokens+data.OutTokens)))
	if data.Elapsed > 0 {
		sb.WriteString(s.renderStatLine("Time", data.Elapsed.Round(time.Millisecond).String()))
	}
	sb.WriteString("\n")

	// ── Tools ──────────────────────────────────────────
	if len(data.Tools) > 0 {
		sb.WriteString(s.theme.SidebarTitle.Render("Tools"))
		sb.WriteString("\n")
		for _, tool := range data.Tools {
			toolName := tool
			if len(toolName) > innerW-4 {
				toolName = toolName[:innerW-7] + "..."
			}
			sb.WriteString(s.theme.SidebarItem.Render(fmt.Sprintf("• %s", toolName)))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// ── MCPs ───────────────────────────────────────────
	if len(data.MCPs) > 0 {
		sb.WriteString(s.theme.SidebarTitle.Render("MCPs"))
		sb.WriteString("\n")
		for _, mcp := range data.MCPs {
			mcpName := mcp
			if len(mcpName) > innerW-4 {
				mcpName = mcpName[:innerW-7] + "..."
			}
			sb.WriteString(s.theme.SidebarItem.Render(fmt.Sprintf("• %s", mcpName)))
			sb.WriteString("\n")
		}
	}

	content := sb.String()

	// Fill height with background
	lines := strings.Split(content, "\n")
	if len(lines) < s.height {
		content += strings.Repeat("\n", s.height-len(lines))
	}

	return s.theme.SidebarStyle.Width(s.width).Height(s.height).Render(content)
}

func (s *Sidebar) renderStatLine(label, value string) string {
	return s.theme.SidebarItem.Render(
		s.theme.StatsLabel.Render(fmt.Sprintf("%-6s", label))+
			s.theme.StatsValue.Render(value),
	) + "\n"
}
