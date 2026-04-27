package components

import (
	"fmt"
	"strings"
)

// ═══════════════════════════════════════════════════════════════
// Command Palette Component — Quick Command Search
// ═══════════════════════════════════════════════════════════════

// CommandPalette renders a searchable command palette.
type CommandPalette struct {
	theme    *Theme
	width    int
	items    []PaletteItem
	selected int
	filter   string
	active   bool
}

// PaletteItem represents a selectable command.
type PaletteItem struct {
	Command     string
	Description string
	Action      func()
}

// NewCommandPalette creates a new command palette.
func NewCommandPalette(width int) *CommandPalette {
	return &CommandPalette{
		theme: GetTheme(),
		width: width,
		items: defaultPaletteItems(),
	}
}

// SetWidth updates the palette width.
func (p *CommandPalette) SetWidth(width int) {
	p.width = width
}

// IsActive returns whether the palette is currently open.
func (p *CommandPalette) IsActive() bool {
	return p.active
}

// Toggle toggles the palette open/closed.
func (p *CommandPalette) Toggle() {
	p.active = !p.active
	if p.active {
		p.filter = ""
		p.selected = 0
	}
}

// Close closes the palette.
func (p *CommandPalette) Close() {
	p.active = false
}

// Open opens the palette.
func (p *CommandPalette) Open() {
	p.active = true
	p.filter = ""
	p.selected = 0
}

// FilterText returns the current filter text.
func (p *CommandPalette) FilterText() string {
	return p.filter
}

// SetFilter updates the filter text.
func (p *CommandPalette) SetFilter(text string) {
	p.filter = text
	p.selected = 0
}

// MoveSelection moves the selection up or down.
func (p *CommandPalette) MoveSelection(delta int) {
	filtered := p.filteredItems()
	if len(filtered) == 0 {
		return
	}
	p.selected += delta
	if p.selected < 0 {
		p.selected = len(filtered) - 1
	}
	if p.selected >= len(filtered) {
		p.selected = 0
	}
}

// SelectedItem returns the currently selected item.
func (p *CommandPalette) SelectedItem() (PaletteItem, bool) {
	filtered := p.filteredItems()
	if p.selected < 0 || p.selected >= len(filtered) {
		return PaletteItem{}, false
	}
	return filtered[p.selected], true
}

// filteredItems returns items matching the filter.
func (p *CommandPalette) filteredItems() []PaletteItem {
	if p.filter == "" {
		return p.items
	}
	var result []PaletteItem
	lowerFilter := strings.ToLower(p.filter)
	for _, item := range p.items {
		if strings.Contains(strings.ToLower(item.Command), lowerFilter) ||
			strings.Contains(strings.ToLower(item.Description), lowerFilter) {
			result = append(result, item)
		}
	}
	return result
}

// Render renders the command palette.
func (p *CommandPalette) Render() string {
	if !p.active {
		return ""
	}

	var sb strings.Builder
	innerW := p.width - 6
	if innerW < 30 {
		innerW = 30
	}

	// Title
	sb.WriteString(p.theme.HelpOverlayTitle.Render("🔍 Command Palette"))
	sb.WriteString("\n\n")

	// Filter input
	filterDisplay := p.filter
	if filterDisplay == "" {
		filterDisplay = p.theme.InputPlaceholder.Render("Type to search...")
	}
	sb.WriteString(p.theme.PaletteFilter.Width(innerW).Render("> " + filterDisplay))
	sb.WriteString("\n\n")

	// Items
	filtered := p.filteredItems()
	if len(filtered) == 0 {
		sb.WriteString(p.theme.HelpOverlayDesc.Render("  No commands found."))
	} else {
		for i, item := range filtered {
			if i >= 8 {
				break // Limit visible items
			}
			cmd := item.Command
			desc := item.Description
			if i == p.selected {
				cmd = p.theme.PaletteItemSelected.Render(fmt.Sprintf(" %-20s", cmd))
				desc = p.theme.PaletteItemSelected.Render(fmt.Sprintf(" %s", desc))
			} else {
				cmd = p.theme.PaletteItem.Render(fmt.Sprintf(" %-20s", cmd))
				desc = p.theme.HelpOverlayDesc.Render(desc)
			}
			sb.WriteString(fmt.Sprintf("  %s %s\n", cmd, desc))
		}
	}

	// Footer
	sb.WriteString("\n")
	sb.WriteString(p.theme.HelpOverlayDesc.Render("  ↑↓ Navigate  •  Enter Select  •  Esc Close"))

	return p.theme.PaletteStyle.Width(p.width).Render(sb.String())
}

func defaultPaletteItems() []PaletteItem {
	return []PaletteItem{
		{"/help", "Show available commands", nil},
		{"/mode ask", "Switch to Ask mode", nil},
		{"/mode rush", "Switch to Rush mode", nil},
		{"/mode plan", "Switch to Plan mode", nil},
		{"/mode test", "Switch to Test mode", nil},
		{"/memory", "View saved memories", nil},
		{"/mcp", "View MCP servers", nil},
		{"/session list", "List all sessions", nil},
		{"/session new", "Create new session", nil},
		{"/clear", "Clear chat screen", nil},
		{"exit", "Exit Smara", nil},
	}
}

// SetItems replaces the palette items.
func (p *CommandPalette) SetItems(items []PaletteItem) {
	p.items = items
}
