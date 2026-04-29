package components

import (
	"fmt"
	"strings"
)

// ═══════════════════════════════════════════════════════════════
// Help Overlay Component — Keyboard Shortcuts Dialog
// ═══════════════════════════════════════════════════════════════

// HelpOverlay renders a modal dialog with keyboard shortcuts.
type HelpOverlay struct {
	theme *Theme
	width int
}

// NewHelpOverlay creates a new help overlay component.
func NewHelpOverlay(width int) *HelpOverlay {
	return &HelpOverlay{
		theme: GetTheme(),
		width: width,
	}
}

// SetWidth updates the overlay width.
func (h *HelpOverlay) SetWidth(width int) {
	h.width = width
}

// shortcut represents a single keyboard shortcut entry.
type shortcut struct {
	Key  string
	Desc string
}

var shortcuts = []shortcut{
	{"Tab", "Cycle mode (ask → rush → plan → test)"},
	{"↑ / ↓", "Navigate command history"},
	{"Enter", "Send message / confirm / copy selected"},
	{"Ctrl+C", "Cancel processing / copy last response"},
	{"Ctrl+Q", "Exit Smara"},
	{"Ctrl+D", "Exit Smara"},
	{"Ctrl+V / Ctrl+Shift+V", "Paste from clipboard"},
	{"Ctrl+S", "Select message to copy"},
	{"Ctrl+B", "Toggle sidebar"},
	{"Ctrl+T", "Toggle thinking visibility"},
	{"Ctrl+U", "Clear current line"},
	{"Ctrl+W", "Delete last word"},
	{"Ctrl+?", "Show / hide this help"},
}

var commandShortcuts = []shortcut{
	{"/help", "Show available commands"},
	{"/mode [ask|rush|plan|test]", "Change agent mode"},
	{"/model [provider] [model]", "Change LLM provider/model"},
	{"/memory", "View saved memories"},
	{"/mcp", "View MCP servers and tools"},
	{"/session [list|new|switch|end]", "Manage sessions"},
	{"/clear", "Clear chat screen"},
	{"exit", "Exit Smara"},
}

// Render renders the help overlay dialog.
func (h *HelpOverlay) Render() string {
	var sb strings.Builder

	title := h.theme.HelpOverlayTitle.Render("⌨️  Keyboard Shortcuts")
	sb.WriteString(title)
	sb.WriteString("\n\n")

	// Navigation section
	sb.WriteString(h.theme.HelpOverlayTitle.Render("Navigation"))
	sb.WriteString("\n")
	for _, s := range shortcuts {
		key := h.theme.HelpOverlayKey.Render(fmt.Sprintf("%-12s", s.Key))
		desc := h.theme.HelpOverlayDesc.Render(s.Desc)
		sb.WriteString(fmt.Sprintf("  %s %s\n", key, desc))
	}

	sb.WriteString("\n")

	// Commands section
	sb.WriteString(h.theme.HelpOverlayTitle.Render("Commands"))
	sb.WriteString("\n")
	for _, s := range commandShortcuts {
		key := h.theme.HelpOverlayKey.Render(fmt.Sprintf("%-30s", s.Key))
		desc := h.theme.HelpOverlayDesc.Render(s.Desc)
		sb.WriteString(fmt.Sprintf("  %s %s\n", key, desc))
	}

	// Footer hint
	sb.WriteString("\n")
	sb.WriteString(h.theme.HelpOverlayDesc.Render("Press Ctrl+? or Esc to close this help."))

	content := sb.String()

	// Box the content
	innerW := h.width - 8
	if innerW < 40 {
		innerW = 40
	}

	return h.theme.HelpOverlayStyle.Width(innerW).Render(content)
}

// Center centers the overlay within given dimensions.
func (h *HelpOverlay) Center(content string, termWidth, termHeight int) string {
	lines := strings.Split(content, "\n")
	contentH := len(lines)
	contentW := 0
	for _, line := range lines {
		if len(line) > contentW {
			contentW = len(line)
		}
	}

	padTop := (termHeight - contentH) / 2
	if padTop < 0 {
		padTop = 0
	}
	padLeft := (termWidth - contentW) / 2
	if padLeft < 0 {
		padLeft = 0
	}

	var sb strings.Builder
	for i := 0; i < padTop; i++ {
		sb.WriteString("\n")
	}
	for _, line := range lines {
		sb.WriteString(strings.Repeat(" ", padLeft))
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String()
}
