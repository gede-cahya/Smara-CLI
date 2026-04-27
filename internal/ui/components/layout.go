package components

import (
	"image/color"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

// ═══════════════════════════════════════════════════════════════
// Layout Helpers — Flexbox & Panel Management
// ═══════════════════════════════════════════════════════════════

// Layout holds computed dimensions for the TUI layout.
type Layout struct {
	Width       int
	Height      int
	SidebarW    int
	ContentW    int
	HeaderH     int
	StatusH     int
	InputH      int
	ChatH       int
	ShowSidebar bool
}

// ComputeLayout calculates panel dimensions based on terminal size.
func ComputeLayout(width, height int, showSidebar bool) Layout {
	const (
		minWidth     = 60
		minHeight    = 15
		sidebarWidth = 28
		headerHeight = 1
		statusHeight = 1
		inputHeight  = 3
	)

	if width < minWidth {
		width = minWidth
	}
	if height < minHeight {
		height = minHeight
	}

	l := Layout{
		Width:       width,
		Height:      height,
		ShowSidebar: showSidebar,
		HeaderH:     headerHeight,
		StatusH:     statusHeight,
		InputH:      inputHeight,
	}

	if showSidebar && width >= 100 {
		l.SidebarW = sidebarWidth
		l.ContentW = width - sidebarWidth
	} else {
		l.SidebarW = 0
		l.ContentW = width
	}

	// Account for borders/padding
	l.ChatH = height - l.HeaderH - l.StatusH - l.InputH - 2 // 2 for borders
	if l.ChatH < 5 {
		l.ChatH = 5
	}

	return l
}

// JoinHorizontal joins strings horizontally with a separator.
func JoinHorizontal(left, right string, leftWidth int) string {
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")

	maxLines := len(leftLines)
	if len(rightLines) > maxLines {
		maxLines = len(rightLines)
	}

	var sb strings.Builder
	for i := 0; i < maxLines; i++ {
		l := ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		// Pad left to exact width
		l = lipgloss.NewStyle().Width(leftWidth).Render(l)

		r := ""
		if i < len(rightLines) {
			r = rightLines[i]
		}

		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(l)
		sb.WriteString(r)
	}

	return sb.String()
}

// FillRight fills empty space on the right of content.
func FillRight(content string, totalWidth int) string {
	lines := strings.Split(content, "\n")
	var sb strings.Builder
	for i, line := range lines {
		w := lipgloss.Width(line)
		if w < totalWidth {
			line += strings.Repeat(" ", totalWidth-w)
		}
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(line)
	}
	return sb.String()
}

// Box draws a simple box around content.
func Box(content string, width int, borderColor color.Color) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(width-2).
		Padding(0, 1)
	return style.Render(content)
}

// VerticalDivider creates a vertical divider line.
func VerticalDivider(height int, c color.Color) string {
	style := lipgloss.NewStyle().
		Foreground(c).
		Height(height)
	return style.Render("│")
}

// HorizontalDivider creates a horizontal divider line.
func HorizontalDivider(width int, c color.Color) string {
	style := lipgloss.NewStyle().
		Foreground(c).
		Width(width)
	return style.Render(strings.Repeat("─", width))
}
