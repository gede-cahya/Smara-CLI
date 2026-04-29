package components

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

// ═══════════════════════════════════════════════════════════════
// Phase Stepper — Real-time pipeline progress indicator
// ═══════════════════════════════════════════════════════════════

// PhaseInfo tracks the state of one pipeline phase.
type PhaseInfo struct {
	Name        string // e.g. "Thinking", "Analyzing", "Exploring", "Generating"
	Description string // e.g. "Reasoning about the problem..."
	Active      bool
	Completed   bool
	Content     string // live accumulated text for this phase
	HasContent  bool   // true if this phase ever received content
}

// PhaseStepper renders a vertical list of pipeline phases.
//   ✓ Completed  → dimmed with checkmark
//   → Active     → highlighted with spinner arrow
//   ○ Pending    → muted

type PhaseStepper struct {
	theme  *Theme
	width  int
	active string // current active phase name
}

// NewPhaseStepper creates a new stepper component.
func NewPhaseStepper(width int) *PhaseStepper {
	return &PhaseStepper{
		theme: GetTheme(),
		width: width,
	}
}

// SetWidth updates the component width.
func (ps *PhaseStepper) SetWidth(width int) {
	ps.width = width
}

// SetActive sets the currently active phase name.
func (ps *PhaseStepper) SetActive(phase string) {
	ps.active = phase
}

// Render builds the stepper string from a list of PhaseInfo.
func (ps *PhaseStepper) Render(phases []PhaseInfo) string {
	if len(phases) == 0 {
		return ""
	}

	var sb strings.Builder
	innerW := ps.width - 4
	if innerW < 20 {
		innerW = 20
	}

	for i, p := range phases {
		if !p.HasContent && !p.Active {
			// Skip pending phases that never produced anything
			continue
		}
		var line string
		if p.Completed {
			line = ps.renderCompleted(p, innerW)
		} else if p.Active {
			line = ps.renderActive(p, innerW)
		} else {
			line = ps.renderPending(p, innerW)
		}
		sb.WriteString("  ")
		sb.WriteString(line)
		// Show live content under the active phase
		if p.Active && p.Content != "" {
			sb.WriteString("\n")
			summary := ps.truncate(p.Content, innerW-4)
			contentStyle := lipgloss.NewStyle().Foreground(ClrSubtext).Faint(true)
			sb.WriteString("    ")
			sb.WriteString(contentStyle.Render(summary))
		}
		if i < len(phases)-1 {
			sb.WriteString("\n")
		}
	}

	return lipgloss.NewStyle().
		Width(ps.width - 2).
		PaddingLeft(1).
		PaddingRight(1).
		Render(sb.String())
}

func (ps *PhaseStepper) renderCompleted(p PhaseInfo, w int) string {
	done := lipgloss.NewStyle().Foreground(ClrGreen).Bold(true).Render("✓")
	label := lipgloss.NewStyle().Foreground(ClrMuted).Render(p.Name)
	var extra string
	if p.Content != "" {
		extra = lipgloss.NewStyle().Foreground(ClrMuted).Faint(true).Render(fmt.Sprintf(" (%d chars)", len(p.Content)))
	}
	return fmt.Sprintf("%s %s%s", done, label, extra)
}

func (ps *PhaseStepper) renderActive(p PhaseInfo, w int) string {
	arrow := lipgloss.NewStyle().Foreground(ClrAccent).Bold(true).Render("→")
	label := lipgloss.NewStyle().Foreground(ClrText).Bold(true).Render(p.Name)
	var desc string
	if p.Description != "" {
		desc = lipgloss.NewStyle().Foreground(ClrSubtext).Render(" " + p.Description)
	}
	return fmt.Sprintf("%s %s%s", arrow, label, desc)
}

func (ps *PhaseStepper) truncate(s string, maxLen int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > 0 {
		s = lines[len(lines)-1] // last line
	}
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}

func (ps *PhaseStepper) renderPending(p PhaseInfo, w int) string {
	bullet := lipgloss.NewStyle().Foreground(ClrMuted).Render("○")
	label := lipgloss.NewStyle().Foreground(ClrMuted).Render(p.Name)
	return fmt.Sprintf("%s %s", bullet, label)
}
