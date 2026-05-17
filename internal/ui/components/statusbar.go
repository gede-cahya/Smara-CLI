package components

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

// ═══════════════════════════════════════════════════════════════
// Status Bar Component — Hermes-style Bottom Bar
// ═══════════════════════════════════════════════════════════════

// StatusBar renders the bottom status bar with keyboard hints and context.
type StatusBar struct {
	theme *Theme
	width int
}

// NewStatusBar creates a new status bar component.
func NewStatusBar(width int) *StatusBar {
	return &StatusBar{
		theme: GetTheme(),
		width: width,
	}
}

// SetWidth updates the status bar width.
func (sb *StatusBar) SetWidth(width int) {
	sb.width = width
}

// StatusContext holds runtime context for the status bar.
type StatusContext struct {
	Mode         string
	Provider     string
	Model        string
	TokenCount   int
	InputTokens  int
	OutputTokens int
	Processing   bool
}

func (sb *StatusBar) Render(ctx StatusContext) string {
	var leftParts []string
	var rightParts []string

	// ── LEFT: Keyboard shortcuts ─────────────────────────────────
	leftParts = append(leftParts, sb.renderKey("Tab", "Mode"))
	leftParts = append(leftParts, sb.renderKey("↑↓", "History"))
	leftParts = append(leftParts, sb.renderKey("Ctrl+B", "Sidebar"))
	leftParts = append(leftParts, sb.renderKey("Ctrl+K", "Palette"))
	leftParts = append(leftParts, sb.renderKey("Ctrl+?", "Help"))

	// ── RIGHT: State / context ───────────────────────────────────
	if ctx.Processing {
		rightParts = append(rightParts, sb.theme.SpinnerStyle.Render("● Processing"))
	} else {
		modeStyle := sb.theme.ModeBadge(ctx.Mode)
		rightParts = append(rightParts, modeStyle.Render(fmt.Sprintf(" %s %s ", ModeIcon(ctx.Mode), strings.ToUpper(ctx.Mode))))
	}

	// Provider info
	if ctx.Provider != "" {
		provInfo := sb.theme.StatsLabel.Render(ctx.Provider)
		if ctx.Model != "" {
			provInfo += sb.theme.StatsLabel.Render("/" + ctx.Model)
		}
		rightParts = append(rightParts, provInfo)
	}

	// Token counts
	if ctx.TokenCount > 0 {
		tokenLabel := fmt.Sprintf("∑%d tk", ctx.TokenCount)
		if ctx.InputTokens > 0 || ctx.OutputTokens > 0 {
			tokenLabel = fmt.Sprintf("∑%d tk ⇣%d ⇡%d", ctx.TokenCount, ctx.InputTokens, ctx.OutputTokens)
		}
		rightParts = append(rightParts, sb.theme.StatsValue.Render(tokenLabel))
	}

	// ── Layout: LEFT …… RIGHT ───────────────────────────────────
	leftStr := strings.Join(leftParts, "  ")
	rightStr := strings.Join(rightParts, "  ")

	leftW := lipgloss.Width(leftStr)
	rightW := lipgloss.Width(rightStr)
	spacing := 2

	available := sb.width - leftW - rightW - spacing
	if available < 1 {
		available = 1
	}

	bar := leftStr + strings.Repeat(" ", available) + rightStr

	// Fill to full width
	if lipgloss.Width(bar) < sb.width {
		bar += strings.Repeat(" ", sb.width-lipgloss.Width(bar))
	}

	return sb.theme.StatusBarStyle.Width(sb.width).Render(bar)
}

func (sb *StatusBar) renderKey(key, desc string) string {
	return sb.theme.StatusBarKey.Render(key) + sb.theme.StatusBarHint.Render(" "+desc)
}
