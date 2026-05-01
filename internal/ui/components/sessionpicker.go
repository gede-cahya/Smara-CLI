package components

import (
	"fmt"
	"strings"

	"github.com/gede-cahya/Smara-CLI/internal/session"
)

// ═══════════════════════════════════════════════════════════════
// Session Picker Overlay — Keyboard-Driven Session Switcher
// ═══════════════════════════════════════════════════════════════

// SessionPickerItem represents a selectable session.
type SessionPickerItem struct {
	Session *session.Session
}

// SessionPicker renders a searchable list of sessions for quick switching.
type SessionPicker struct {
	theme    *Theme
	width    int
	height   int
	items    []SessionPickerItem
	filtered []SessionPickerItem
	selected int
	filter   string
	active   bool
	deleting bool       // confirmation mode for deletion
	deleteID string    // session ID pending deletion
}

// NewSessionPicker creates a new session picker.
func NewSessionPicker(width, height int) *SessionPicker {
	return &SessionPicker{
		theme:  GetTheme(),
		width:  width,
		height: height,
	}
}

// SetSize updates the picker dimensions.
func (p *SessionPicker) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// IsActive returns whether the picker is currently open.
func (p *SessionPicker) IsActive() bool {
	return p.active
}

// Open opens the picker with the given sessions.
func (p *SessionPicker) Open(sessions []*session.Session) {
	p.active = true
	p.filter = ""
	p.selected = 0
	p.deleting = false
	p.deleteID = ""
	p.items = make([]SessionPickerItem, 0, len(sessions))
	for _, s := range sessions {
		p.items = append(p.items, SessionPickerItem{Session: s})
	}
	p.filtered = p.items
}

// Close closes the picker.
func (p *SessionPicker) Close() {
	p.active = false
	p.deleting = false
	p.deleteID = ""
}

// Toggle toggles the picker open/closed.
func (p *SessionPicker) Toggle(sessions []*session.Session) {
	if p.active {
		p.Close()
	} else {
		p.Open(sessions)
	}
}

// SetFilter updates the filter text.
func (p *SessionPicker) SetFilter(text string) {
	p.filter = text
	p.selected = 0
	p.applyFilter()
}

// FilterText returns the current filter text.
func (p *SessionPicker) FilterText() string {
	return p.filter
}

// MoveSelection moves the selection up or down.
func (p *SessionPicker) MoveSelection(delta int) {
	if len(p.filtered) == 0 {
		return
	}
	p.selected += delta
	if p.selected < 0 {
		p.selected = len(p.filtered) - 1
	}
	if p.selected >= len(p.filtered) {
		p.selected = 0
	}
}

// SelectedItem returns the currently selected session item.
func (p *SessionPicker) SelectedItem() (SessionPickerItem, bool) {
	if p.selected < 0 || p.selected >= len(p.filtered) {
		return SessionPickerItem{}, false
	}
	return p.filtered[p.selected], true
}

// SetDeleteConfirm enters deletion confirmation mode for the selected session.
func (p *SessionPicker) SetDeleteConfirm() (string, bool) {
	if len(p.filtered) == 0 {
		return "", false
	}
	item := p.filtered[p.selected]
	p.deleting = true
	p.deleteID = item.Session.ID
	return item.Session.ID, true
}

// ConfirmDelete confirms deletion and returns the ID to delete.
func (p *SessionPicker) ConfirmDelete() string {
	id := p.deleteID
	p.deleting = false
	p.deleteID = ""
	return id
}

// CancelDelete cancels deletion confirmation.
func (p *SessionPicker) CancelDelete() {
	p.deleting = false
	p.deleteID = ""
}

// IsDeleting returns true if in deletion confirmation mode.
func (p *SessionPicker) IsDeleting() bool {
	return p.deleting
}

// RefreshItems reloads the session list while preserving selection state.
func (p *SessionPicker) RefreshItems(sessions []*session.Session) {
	p.items = make([]SessionPickerItem, 0, len(sessions))
	for _, s := range sessions {
		p.items = append(p.items, SessionPickerItem{Session: s})
	}
	p.applyFilter()
	if p.selected >= len(p.filtered) {
		p.selected = 0
		if len(p.filtered) > 0 {
			p.selected = len(p.filtered) - 1
		}
	}
}

func (p *SessionPicker) applyFilter() {
	if p.filter == "" {
		p.filtered = p.items
		return
	}
	lowerFilter := strings.ToLower(p.filter)
	p.filtered = make([]SessionPickerItem, 0, len(p.items))
	for _, item := range p.items {
		s := item.Session
		if strings.Contains(strings.ToLower(s.Name), lowerFilter) ||
			strings.Contains(strings.ToLower(s.ID), lowerFilter) ||
			strings.Contains(strings.ToLower(s.Context), lowerFilter) {
			p.filtered = append(p.filtered, item)
		}
	}
}

// Render renders the session picker overlay.
func (p *SessionPicker) Render() string {
	if !p.active {
		return ""
	}

	var sb strings.Builder
	innerW := p.width - 6
	if innerW < 30 {
		innerW = 30
	}

	// Title
	sb.WriteString(p.theme.HelpOverlayTitle.Render("🗂️  Session Picker"))
	sb.WriteString("\n\n")

	if p.deleting {
		// Deletion confirmation dialog
		for _, item := range p.filtered {
			if item.Session.ID == p.deleteID {
				name := item.Session.Name
				idShort := item.Session.ID
				if len(idShort) > 8 {
					idShort = idShort[:8]
				}
				sb.WriteString(p.theme.HelpOverlayTitle.Render(fmt.Sprintf("Hapus session '%s' [%s]?", name, idShort)))
				sb.WriteString("\n\n")
				sb.WriteString(p.theme.PaletteItemSelected.Render(" [ Ya ] "))
				sb.WriteString(" ")
				sb.WriteString(p.theme.PaletteItem.Render(" [ Tidak ] "))
				break
			}
		}
		sb.WriteString("\n\n")
		sb.WriteString(p.theme.HelpOverlayDesc.Render("  Enter: Konfirmasi  •  Esc: Batal"))
		return p.theme.PaletteStyle.Width(p.width).Render(sb.String())
	}

	// Filter input
	filterDisplay := p.filter
	if filterDisplay == "" {
		filterDisplay = p.theme.InputPlaceholder.Render("Type to filter sessions...")
	}
	sb.WriteString(p.theme.PaletteFilter.Width(innerW).Render("> " + filterDisplay))
	sb.WriteString("\n\n")

	// Items
	if len(p.filtered) == 0 {
		sb.WriteString(p.theme.HelpOverlayDesc.Render("  No sessions found."))
	} else {
		maxVisible := p.height - 8
		if maxVisible < 3 {
			maxVisible = 3
		}
		startIdx := 0
		if p.selected >= maxVisible {
			startIdx = p.selected - maxVisible + 1
		}
		endIdx := startIdx + maxVisible
		if endIdx > len(p.filtered) {
			endIdx = len(p.filtered)
		}

		for i := startIdx; i < endIdx; i++ {
			item := p.filtered[i]
			s := item.Session
			idShort := s.ID
			if len(idShort) > 8 {
				idShort = idShort[:8]
			}

			stateIcon := "🟢"
			if s.State == session.StateEnded {
				stateIcon = "⚫"
			} else if s.State == session.StatePaused {
				stateIcon = "🟡"
			}

			msgCount := len(s.History) / 2
			timeStr := s.UpdatedAt.Format("02 Jan 15:04")

			line := fmt.Sprintf(" %s %s [%s] %d msgs · %s", stateIcon, s.Name, idShort, msgCount, timeStr)
			if i == p.selected {
				line = p.theme.PaletteItemSelected.Render(line)
			} else {
				line = p.theme.PaletteItem.Render(line)
			}
			sb.WriteString(line)
			sb.WriteString("\n")
		}

		if len(p.filtered) > maxVisible {
			sb.WriteString(p.theme.HelpOverlayDesc.Render(fmt.Sprintf("  ... %d more sessions ...", len(p.filtered)-maxVisible)))
			sb.WriteString("\n")
		}
	}

	// Footer
	sb.WriteString("\n")
	sb.WriteString(p.theme.HelpOverlayDesc.Render("  ↑↓ Navigate  •  Enter: Switch  •  d: Delete  •  Esc: Close"))

	return p.theme.PaletteStyle.Width(p.width).Render(sb.String())
}

// Center centers the overlay within given dimensions.
func (p *SessionPicker) Center(content string, termWidth, termHeight int) string {
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
