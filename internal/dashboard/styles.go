package dashboard

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	colorPrimary   = lipgloss.Color("#7D56F4")
	colorGreen     = lipgloss.Color("#04B575")
	colorRed       = lipgloss.Color("#FF3366")
	colorYellow    = lipgloss.Color("#F3C623")
	colorCyan      = lipgloss.Color("#36C5F0")
	colorDim       = lipgloss.Color("#767676")
	colorBorder    = lipgloss.Color("#3C3C3C")
	colorWhite     = lipgloss.Color("#FAFAFA")
	colorBorderAct = lipgloss.Color("#7D56F4")

	// Header
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorWhite).
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
