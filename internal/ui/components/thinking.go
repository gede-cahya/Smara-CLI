package components

import (
	lipgloss "charm.land/lipgloss/v2"
)

// ═══════════════════════════════════════════════════════════════
// Thinking Toggle Component — Collapsible Reasoning Block
// ═══════════════════════════════════════════════════════════════

// ThinkingToggle manages a collapsible thinking/reasoning block.
type ThinkingToggle struct {
	theme    *Theme
	width    int
	expanded bool
	content  string
}

// NewThinkingToggle creates a new thinking toggle component.
func NewThinkingToggle(width int) *ThinkingToggle {
	return &ThinkingToggle{
		theme:    GetTheme(),
		width:    width,
		expanded: false,
	}
}

// SetWidth updates the component width.
func (t *ThinkingToggle) SetWidth(width int) {
	t.width = width
}

// SetContent sets the thinking content.
func (t *ThinkingToggle) SetContent(content string) {
	t.content = content
}

// IsExpanded returns whether the thinking block is expanded.
func (t *ThinkingToggle) IsExpanded() bool {
	return t.expanded
}

// Toggle toggles the expanded state.
func (t *ThinkingToggle) Toggle() {
	t.expanded = !t.expanded
}

// Expand expands the thinking block.
func (t *ThinkingToggle) Expand() {
	t.expanded = true
}

// Collapse collapses the thinking block.
func (t *ThinkingToggle) Collapse() {
	t.expanded = false
}

// Render renders the thinking block.
func (t *ThinkingToggle) Render() string {
	if t.content == "" {
		return ""
	}

	innerW := t.width - 4
	if innerW < 20 {
		innerW = 20
	}

	// Header line with toggle indicator
	var header string
	if t.expanded {
		header = t.theme.ThinkingHeader.Render("💭 Thinking ▲ (click to collapse)")
	} else {
		header = t.theme.ThinkingHeader.Render("💭 Thinking... ▼ (click to expand)")
	}

	if !t.expanded {
		// Collapsed — just show header
		return t.theme.ThinkingBlock.Width(t.width - 2).Render(header)
	}

	// Expanded — show header + content
	content := t.theme.ThinkingContent.Width(innerW - 2).Render(t.content)

	return t.theme.ThinkingBlock.Width(t.width - 2).Render(
		lipgloss.JoinVertical(lipgloss.Left, header, content),
	)
}

// QuickToggle returns a compact toggle indicator for inline use.
func QuickToggle(expanded bool, theme *Theme) string {
	if expanded {
		return theme.ThinkingHeader.Render("▲")
	}
	return theme.ThinkingHeader.Render("▼")
}
