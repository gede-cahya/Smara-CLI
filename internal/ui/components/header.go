package components

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

// ═══════════════════════════════════════════════════════════════
// Header Component — Clean Top Bar (Hermes style)
// ═══════════════════════════════════════════════════════════════

// Header renders the top header bar.
type Header struct {
	theme *Theme
	width int
}

// NewHeader creates a new header component.
func NewHeader(width int) *Header {
	return &Header{
		theme: GetTheme(),
		width: width,
	}
}

// SetWidth updates the header width.
func (h *Header) SetWidth(width int) {
	h.width = width
}

// Render renders the full header bar — Hermes style.
func (h *Header) Render(mode, provider, model string, processing bool, spinnerView, statusText string) string {
	var sb strings.Builder

	// ── Left section: Brand badge + Mode badge ─────────────────
	brand := h.theme.BrandBadge.Render(" Smara ")
	modeBadge := h.theme.ModeBadge(mode).Render(fmt.Sprintf(" %s %s ", ModeIcon(mode), strings.ToUpper(mode)))

	// ── Center: Provider / Model (subtle) ─────────────────────
	providerInfo := lipgloss.NewStyle().
		Foreground(h.theme.FgSubtext).
		Render(fmt.Sprintf("%s / %s", provider, model))
	centerPadded := providerInfo

	// ── Right: Processing indicator ───────────────────────────
	var rightSection string
	if processing {
		procText := statusText
		if procText == "" {
			procText = "Processing..."
		}
		rightSection = h.theme.SpinnerStyle.Render(fmt.Sprintf("%s %s", spinnerView, procText))
	}

	// ── Layout: LEFT …… CENTER …… RIGHT ──────────────────────
	leftBlock := lipgloss.JoinHorizontal(lipgloss.Center, brand, " ", modeBadge)
	leftW := lipgloss.Width(leftBlock)
	centerW := lipgloss.Width(centerPadded)
	rightW := lipgloss.Width(rightSection)

	// Calculate available space for center
	spacing := 2 // minimum padding between sections
	availableForCenter := h.width - leftW - rightW - spacing*2

	if availableForCenter < centerW {
		// Truncate: show only provider
		centerPadded = lipgloss.NewStyle().
			Foreground(h.theme.FgSubtext).
			Render(provider)
		centerW = lipgloss.Width(centerPadded)
		availableForCenter = h.width - leftW - rightW - spacing*2
	}

	// Build the full line with auto-spacing
	var headerContent string
	if availableForCenter <= 0 {
		// Very narrow: just left + right
		midGap := h.width - leftW - rightW
		if midGap < 1 {
			midGap = 1
		}
		headerContent = leftBlock + strings.Repeat(" ", midGap) + rightSection
	} else {
		leftPad := spacing
		rightPad := h.width - leftW - centerW - rightW - leftPad
		if rightPad < spacing {
			rightPad = spacing
		}
		headerContent = leftBlock + strings.Repeat(" ", leftPad) + centerPadded + strings.Repeat(" ", rightPad) + rightSection
	}

	// Fill remaining width
	contentW := lipgloss.Width(headerContent)
	if contentW < h.width {
		headerContent += strings.Repeat(" ", h.width-contentW)
	}

	sb.WriteString(h.theme.HeaderStyle.Width(h.width).Render(headerContent))
	return sb.String()
}
