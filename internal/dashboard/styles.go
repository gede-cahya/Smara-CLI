package dashboard

import lipgloss "charm.land/lipgloss/v2"

var (
	// Colors — Crush Pastel Green palette
	colorPrimary   = lipgloss.Color("#bef264")
	colorGreen     = lipgloss.Color("#86efac")
	colorRed       = lipgloss.Color("#fda4af")
	colorYellow    = lipgloss.Color("#fcd34d")
	colorCyan      = lipgloss.Color("#67e8f9")
	colorDim       = lipgloss.Color("#71717a")
	colorBorder    = lipgloss.Color("#27272a")
	colorWhite     = lipgloss.Color("#f4f4f5")
	colorBorderAct = lipgloss.Color("#bef264")

	// Header
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#09090b")).
			Background(colorPrimary).
			PaddingLeft(1).
			PaddingRight(1)

	// Panel styles
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	panelActiveStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorBorderAct).
				Padding(0, 1)

	panelTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorCyan)

	// Status indicators
	statusOnline  = lipgloss.NewStyle().Foreground(colorGreen).SetString("●")
	statusOffline = lipgloss.NewStyle().Foreground(colorRed).SetString("○")
	statusWarning = lipgloss.NewStyle().Foreground(colorYellow).SetString("◐")

	// Text styles
	labelStyle = lipgloss.NewStyle().Foreground(colorDim)
	valueStyle = lipgloss.NewStyle().Foreground(colorWhite)
	errorStyle = lipgloss.NewStyle().Foreground(colorRed)
	warnStyle  = lipgloss.NewStyle().Foreground(colorYellow)
	greenStyle = lipgloss.NewStyle().Foreground(colorGreen)
	dimStyle   = lipgloss.NewStyle().Foreground(colorDim)

	// Footer
	footerStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	footerKeyStyle = lipgloss.NewStyle().
			Foreground(colorYellow)
)
