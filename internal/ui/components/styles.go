package components

import (
	"image/color"

	lipgloss "charm.land/lipgloss/v2"
)

// ═══════════════════════════════════════════════════════════════
// Hermes Dark — Smara CLI TUI Design System v2
// Crush-style flat Styles struct with semantic naming
//
// Palette: soft saturated accents on deep neutral background
// ────────────────────────────────────────────────────────────────
//   Base:       #0D0D12  (near-black with blue hint)
//   Surface:    #16161F  (card surfaces)
//   Elevated:   #1C1C28  (hover / active)
//   Overlay:    #252533  (modal backgrounds)
//   Overlay2:   #3F3F46  (secondary overlay)
//   Border:     #2D2D3D  (subtle borders)
//   ──────────────────────────────────────────────────────────
//   Text:       #E1E1EA  (primary text)
//   Subtext:    #8B8BA0  (secondary text)
//   Muted:      #5C5C6E  (tertiary / muted)
//   ──────────────────────────────────────────────────────────
//   Accent:     #7C6FF7  (primary purple)
//   Blue:       #5E9EFF  (info, user)
//   Green:      #3ECF8E  (success, terminal)
//   Amber:      #F0B848  (warnings)
//   Rose:       #F2646E  (errors)
//   Cyan:       #36D4E7  (mode: ask)
//   Pink:       #F47BBD  (mode: rush)
//   Violet:     #A78BFA  (mode: plan)
// ═══════════════════════════════════════════════════════════════

// ─── Color Constants (lipgloss v2: Color is a function, returns color.Color) ──

var (
	// Backgrounds
	ClrBase     = lipgloss.Color("#0D0D12")
	ClrSurface  = lipgloss.Color("#16161F")
	ClrElevated = lipgloss.Color("#1C1C28")
	ClrOverlay  = lipgloss.Color("#252533")
	ClrOverlay2 = lipgloss.Color("#3F3F46")
	ClrBorder   = lipgloss.Color("#2D2D3D")

	// Foregrounds
	ClrText    = lipgloss.Color("#E1E1EA")
	ClrSubtext = lipgloss.Color("#8B8BA0")
	ClrMuted   = lipgloss.Color("#5C5C6E")

	// Accents
	ClrAccent = lipgloss.Color("#7C6FF7")
	ClrBlue   = lipgloss.Color("#5E9EFF")
	ClrGreen  = lipgloss.Color("#3ECF8E")
	ClrAmber  = lipgloss.Color("#F0B848")
	ClrRose   = lipgloss.Color("#F2646E")
	ClrCyan   = lipgloss.Color("#36D4E7")
	ClrPink   = lipgloss.Color("#F47BBD")
	ClrViolet = lipgloss.Color("#A78BFA")
)

// Legacy color name aliases — map old HrmXxx to new ClrXxx
var (
	HrmBase     = ClrBase
	HrmSurface  = ClrSurface
	HrmElevated = ClrElevated
	HrmOverlay  = ClrOverlay
	HrmOverlay2 = ClrOverlay2
	HrmBorder   = ClrBorder
	HrmText     = ClrText
	HrmSubtext  = ClrSubtext
	HrmMuted    = ClrMuted
	HrmAccent   = ClrAccent
	HrmBlue     = ClrBlue
	HrmGreen    = ClrGreen
	HrmAmber    = ClrAmber
	HrmRose     = ClrRose
	HrmCyan     = ClrCyan
	HrmPink     = ClrPink
	HrmViolet   = ClrViolet
)

// ─── Mode Metadata ───────────────────────────────────────────────

type ModeMeta struct {
	Color color.Color
	Icon  string
	Label string
}

var ModeMetaMap = map[string]ModeMeta{
	"ask":  {Color: ClrCyan, Icon: "💬", Label: "ASK"},
	"rush": {Color: ClrPink, Icon: "⚡", Label: "RUSH"},
	"plan": {Color: ClrViolet, Icon: "📋", Label: "PLAN"},
	"test": {Color: ClrGreen, Icon: "🧪", Label: "TEST"},
}

// ModeColor returns the accent color for a mode.
func ModeColor(mode string) color.Color {
	if m, ok := ModeMetaMap[mode]; ok {
		return m.Color
	}
	return ClrAccent
}

// ModeIcon returns the emoji for a mode.
func ModeIcon(mode string) string {
	if m, ok := ModeMetaMap[mode]; ok {
		return m.Icon
	}
	return "🌀"
}

// ModeLabel returns the uppercase label for a mode.
func ModeLabel(mode string) string {
	if m, ok := ModeMetaMap[mode]; ok {
		return m.Label
	}
	return mode
}

// ─── Crush-Style Flat Styles Struct ─────────────────────────────

// Styles holds every TUI style definition in one flat struct.
// This is the Crush pattern: single source of truth, no nesting.
type Styles struct {
	// App shell
	App lipgloss.Style

	// Header
	Header        lipgloss.Style
	BrandBadge    lipgloss.Style
	ModeBadge     lipgloss.Style
	ModeBadgeAsk  lipgloss.Style
	ModeBadgeRush lipgloss.Style
	ModeBadgePlan lipgloss.Style
	ModeBadgeTest lipgloss.Style

	// Chat messages
	ChatUser     lipgloss.Style
	ChatAgent    lipgloss.Style
	ChatSystem   lipgloss.Style
	ChatTerminal lipgloss.Style
	ChatTime     lipgloss.Style
	ChatDivider  lipgloss.Style

	// Thinking block
	ThinkingBlock    lipgloss.Style
	ThinkingGradient [8]lipgloss.Style // 8 gradient steps for streaming

	// Code blocks
	CodeBlock  lipgloss.Style
	CodeInline lipgloss.Style
	CodeLang   lipgloss.Style
	CodeBorder lipgloss.Style

	// Stats (token counts, etc.)
	StatsLabel lipgloss.Style
	StatsValue lipgloss.Style

	// Spinner & live indicator
	Spinner lipgloss.Style
	LiveDot lipgloss.Style

	// Status bar
	StatusBar     lipgloss.Style
	StatusBarKey  lipgloss.Style
	StatusBarHint lipgloss.Style
	StatusBarSep  lipgloss.Style

	// Sidebar
	Sidebar        lipgloss.Style
	SidebarTitle   lipgloss.Style
	SidebarItem    lipgloss.Style
	SidebarItemSel lipgloss.Style
	SidebarDivider lipgloss.Style

	// Input
	Input            lipgloss.Style
	InputPlaceholder lipgloss.Style
	InputPrompt      lipgloss.Style

	// Overlays
	HelpOverlay      lipgloss.Style
	HelpOverlayTitle lipgloss.Style
	HelpOverlayKey   lipgloss.Style
	HelpOverlayDesc  lipgloss.Style
	Palette          lipgloss.Style
	PaletteItem      lipgloss.Style
	PaletteItemSel   lipgloss.Style
	PaletteFilter    lipgloss.Style

	// Dialog & toast
	DialogOverlay   lipgloss.Style
	DialogBox       lipgloss.Style
	DialogTitle     lipgloss.Style
	DialogBody      lipgloss.Style
	DialogButton    lipgloss.Style
	DialogButtonSel lipgloss.Style
	ToastInfo       lipgloss.Style
	ToastSuccess    lipgloss.Style
	ToastWarn       lipgloss.Style
	ToastError      lipgloss.Style

	// Borders & dividers
	BorderSubtle lipgloss.Style
	Divider      lipgloss.Style

	// Diff view
	DiffAdd    lipgloss.Style
	DiffDel    lipgloss.Style
	DiffHeader lipgloss.Style
	DiffHunk   lipgloss.Style
}

// NewStyles returns the complete Smara design system.
func NewStyles() Styles {
	// ── Reusable primitives ──────────────────────────────────
	rounded := lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	normalBottom := lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, true, false)

	s := Styles{
		// ── App shell ─────────────────────────────────────────
		App: lipgloss.NewStyle().
			Background(ClrBase),

		// ── Header ────────────────────────────────────────────
		Header: lipgloss.NewStyle().
			Background(ClrSurface).
			Foreground(ClrText).
			Padding(0, 1).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(ClrAccent),

		BrandBadge: lipgloss.NewStyle().
			Background(ClrAccent).
			Foreground(ClrBase).
			Bold(true).
			Padding(0, 2),

		ModeBadge: lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			Foreground(ClrBase),

		ModeBadgeAsk: lipgloss.NewStyle().
			Bold(true).Padding(0, 1).
			Background(ClrCyan).Foreground(ClrBase),

		ModeBadgeRush: lipgloss.NewStyle().
			Bold(true).Padding(0, 1).
			Background(ClrPink).Foreground(ClrBase),

		ModeBadgePlan: lipgloss.NewStyle().
			Bold(true).Padding(0, 1).
			Background(ClrViolet).Foreground(ClrBase),

		ModeBadgeTest: lipgloss.NewStyle().
			Bold(true).Padding(0, 1).
			Background(ClrGreen).Foreground(ClrBase),

		// ── Chat messages ────────────────────────────────────
		ChatUser: lipgloss.NewStyle().
			Foreground(ClrText).
			Background(ClrSurface).
			Padding(1, 2),

		ChatAgent: lipgloss.NewStyle().
			Foreground(ClrText).
			Padding(1, 2),

		ChatSystem: lipgloss.NewStyle().
			Foreground(ClrSubtext).
			Padding(0, 2),

		ChatTerminal: lipgloss.NewStyle().
			Foreground(ClrGreen).
			Background(ClrSurface).
			Padding(1, 2),

		ChatTime: lipgloss.NewStyle().
			Foreground(ClrMuted).
			Faint(true),

		ChatDivider: lipgloss.NewStyle().
			Foreground(ClrBorder),

		// ── Thinking ─────────────────────────────────────────
		ThinkingBlock: lipgloss.NewStyle().
			Background(ClrSurface).
			Padding(1, 2).
			Margin(0, 2),

		// ThinkingGradient: 8-step opacity for streaming text
		ThinkingGradient: [8]lipgloss.Style{
			lipgloss.NewStyle().Foreground(ClrMuted).Faint(true),
			lipgloss.NewStyle().Foreground(ClrMuted),
			lipgloss.NewStyle().Foreground(ClrSubtext).Faint(true),
			lipgloss.NewStyle().Foreground(ClrSubtext),
			lipgloss.NewStyle().Foreground(ClrAccent).Faint(true),
			lipgloss.NewStyle().Foreground(ClrAccent),
			lipgloss.NewStyle().Foreground(ClrText),
			lipgloss.NewStyle().Foreground(ClrText).Bold(true),
		},

		// ── Code ─────────────────────────────────────────────
		CodeBlock: lipgloss.NewStyle().
			Background(ClrElevated).
			Foreground(ClrText).
			Padding(1, 2).
			Margin(0, 2),

		CodeInline: lipgloss.NewStyle().
			Background(ClrElevated).
			Foreground(ClrGreen).
			Padding(0, 1),

		CodeLang: lipgloss.NewStyle().
			Foreground(ClrAccent).
			Bold(true).
			Padding(0, 1),

		CodeBorder: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(ClrAccent).
			PaddingLeft(1).
			MarginLeft(1),

		// ── Stats ────────────────────────────────────────────
		StatsLabel: lipgloss.NewStyle().
			Foreground(ClrMuted).
			Faint(true),

		StatsValue: lipgloss.NewStyle().
			Foreground(ClrSubtext),

		// ── Spinner ──────────────────────────────────────────
		Spinner: lipgloss.NewStyle().
			Foreground(ClrAccent).
			Bold(true).
			Padding(0, 1),

		LiveDot: lipgloss.NewStyle().
			Foreground(ClrGreen).
			Bold(true),

		// ── Status bar ───────────────────────────────────────
		StatusBar: lipgloss.NewStyle().
			Background(ClrSurface).
			Foreground(ClrMuted).
			Padding(0, 1),

		StatusBarKey: lipgloss.NewStyle().
			Background(ClrElevated).
			Foreground(ClrAccent).
			Bold(true).
			Padding(0, 1),

		StatusBarHint: lipgloss.NewStyle().
			Foreground(ClrMuted).
			Padding(0, 1),

		StatusBarSep: lipgloss.NewStyle().
			Foreground(ClrBorder).
			Padding(0, 1),

		// ── Sidebar ─────────────────────────────────────────
		Sidebar: lipgloss.NewStyle().
			Background(ClrSurface).
			Width(28).
			Padding(1, 1).
			Border(lipgloss.NormalBorder(), false, false, false, false).
			BorderForeground(ClrBorder),

		SidebarTitle: lipgloss.NewStyle().
			Foreground(ClrAccent).
			Bold(true).
			Padding(0, 1),

		SidebarItem: lipgloss.NewStyle().
			Foreground(ClrSubtext).
			Padding(0, 1),

		SidebarItemSel: lipgloss.NewStyle().
			Foreground(ClrAccent).
			Bold(true).
			Padding(0, 1),

		SidebarDivider: lipgloss.NewStyle().
			Foreground(ClrBorder),

		// ── Input ────────────────────────────────────────────
		Input: lipgloss.NewStyle().
			Foreground(ClrText).
			Padding(0, 1),

		InputPlaceholder: lipgloss.NewStyle().
			Foreground(ClrMuted),

		InputPrompt: lipgloss.NewStyle().
			Foreground(ClrAccent).
			Bold(true),

		// ── Help overlay ─────────────────────────────────────
		HelpOverlay: lipgloss.NewStyle().
			Background(ClrOverlay).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ClrAccent).
			Padding(1, 2),

		HelpOverlayTitle: lipgloss.NewStyle().
			Foreground(ClrAccent).
			Bold(true).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(ClrBorder),

		HelpOverlayKey: lipgloss.NewStyle().
			Background(ClrElevated).
			Foreground(ClrAccent).
			Bold(true).
			Padding(0, 1),

		HelpOverlayDesc: lipgloss.NewStyle().
			Foreground(ClrSubtext),

		// ── Palette ──────────────────────────────────────────
		Palette: lipgloss.NewStyle().
			Background(ClrOverlay).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ClrAccent).
			Padding(1, 2),

		PaletteItem: lipgloss.NewStyle().
			Foreground(ClrSubtext).
			Padding(0, 1),

		PaletteItemSel: lipgloss.NewStyle().
			Background(ClrAccent).
			Foreground(ClrBase).
			Bold(true).
			Padding(0, 1),

		PaletteFilter: lipgloss.NewStyle().
			Foreground(ClrText).
			Padding(0, 1),

		// ── Dialog ───────────────────────────────────────────
		DialogOverlay: lipgloss.NewStyle().
			Background(ClrOverlay),

		DialogBox: lipgloss.NewStyle().
			Background(ClrOverlay).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ClrAccent).
			Padding(2, 3),

		DialogTitle: lipgloss.NewStyle().
			Foreground(ClrAccent).
			Bold(true),

		DialogBody: lipgloss.NewStyle().
			Foreground(ClrText),

		DialogButton: lipgloss.NewStyle().
			Foreground(ClrSubtext).
			Padding(0, 2),

		DialogButtonSel: lipgloss.NewStyle().
			Background(ClrAccent).
			Foreground(ClrBase).
			Bold(true).
			Padding(0, 2),

		// ── Toast ────────────────────────────────────────────
		ToastInfo: lipgloss.NewStyle().
			Background(ClrBlue).
			Foreground(ClrBase).
			Padding(1, 2),

		ToastSuccess: lipgloss.NewStyle().
			Background(ClrGreen).
			Foreground(ClrBase).
			Padding(1, 2),

		ToastWarn: lipgloss.NewStyle().
			Background(ClrAmber).
			Foreground(ClrBase).
			Padding(1, 2),

		ToastError: lipgloss.NewStyle().
			Background(ClrRose).
			Foreground(ClrBase).
			Padding(1, 2),

		// ── Borders ──────────────────────────────────────────
		BorderSubtle: rounded.Copy().
			BorderForeground(ClrBorder),

		Divider: lipgloss.NewStyle().
			Foreground(ClrBorder),

		// ── Diff ─────────────────────────────────────────────
		DiffAdd: lipgloss.NewStyle().
			Background(lipgloss.Color("#1a3a1a")).
			Foreground(ClrGreen),

		DiffDel: lipgloss.NewStyle().
			Background(lipgloss.Color("#3a1a1a")).
			Foreground(ClrRose),

		DiffHeader: lipgloss.NewStyle().
			Foreground(ClrAccent).
			Bold(true),

		DiffHunk: lipgloss.NewStyle().
			Foreground(ClrCyan),
	}

	_ = normalBottom // used pattern
	return s
}

// ═══════════════════════════════════════════════════════════════
// Backward compatibility — Theme wrapper for existing code
// ═══════════════════════════════════════════════════════════════

// Theme is the legacy struct kept for backward compatibility.
// New code should use NewStyles() directly.
type Theme struct {
	BgBase     color.Color
	BgSurface  color.Color
	BgElevated color.Color
	BgOverlay  color.Color
	BgOverlay2 color.Color
	BgBorder   color.Color

	FgText    color.Color
	FgSubtext color.Color
	FgMuted   color.Color

	Accent       color.Color
	AccentBlue   color.Color
	AccentGreen  color.Color
	AccentAmber  color.Color
	AccentRose   color.Color
	AccentCyan   color.Color
	AccentPink   color.Color
	AccentViolet color.Color

	AccentRed    color.Color
	AccentYellow color.Color

	HeaderStyle lipgloss.Style
	HeaderBadge lipgloss.Style
	BrandBadge  lipgloss.Style

	MessageUser     lipgloss.Style
	MessageAgent    lipgloss.Style
	MessageSystem   lipgloss.Style
	MessageTerminal lipgloss.Style
	MessageTime     lipgloss.Style

	ThinkingBlock   lipgloss.Style
	ThinkingHeader  lipgloss.Style
	ThinkingContent lipgloss.Style

	CodeBlock  lipgloss.Style
	CodeInline lipgloss.Style
	CodeLang   lipgloss.Style

	StatsLabel lipgloss.Style
	StatsValue lipgloss.Style

	SpinnerStyle   lipgloss.Style
	LiveIndicator  lipgloss.Style
	StatusBarStyle lipgloss.Style
	StatusBarKey   lipgloss.Style
	StatusBarHint  lipgloss.Style

	SidebarStyle   lipgloss.Style
	SidebarTitle   lipgloss.Style
	SidebarItem    lipgloss.Style
	SidebarDivider lipgloss.Style

	InputStyle       lipgloss.Style
	InputPlaceholder lipgloss.Style
	InputPrompt      lipgloss.Style

	HelpOverlayStyle    lipgloss.Style
	HelpOverlayTitle    lipgloss.Style
	HelpOverlayKey      lipgloss.Style
	HelpOverlayDesc     lipgloss.Style
	PaletteStyle        lipgloss.Style
	PaletteItem         lipgloss.Style
	PaletteItemSelected lipgloss.Style
	PaletteFilter       lipgloss.Style

	BorderSubtle lipgloss.Style
	DividerStyle lipgloss.Style
}

func (t *Theme) ModeColor(mode string) color.Color { return ModeColor(mode) }

func (t *Theme) ModeBadge(mode string) lipgloss.Style {
	base := lipgloss.NewStyle().Bold(true).Padding(0, 1).Foreground(ClrBase)
	switch mode {
	case "ask":
		return base.Background(ClrCyan)
	case "rush":
		return base.Background(ClrPink)
	case "plan":
		return base.Background(ClrViolet)
	case "test":
		return base.Background(ClrGreen)
	default:
		return base.Background(ClrAccent)
	}
}

// GetTheme returns the legacy Theme struct populated from the new Styles.
func GetTheme() *Theme {
	s := NewStyles()
	subtleBorder := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ClrBorder)

	return &Theme{
		BgBase:     ClrBase,
		BgSurface:  ClrSurface,
		BgElevated: ClrElevated,
		BgOverlay:  ClrOverlay,
		BgOverlay2: ClrOverlay2,
		BgBorder:   ClrBorder,

		FgText:    ClrText,
		FgSubtext: ClrSubtext,
		FgMuted:   ClrMuted,

		Accent:       ClrAccent,
		AccentBlue:   ClrBlue,
		AccentGreen:  ClrGreen,
		AccentAmber:  ClrAmber,
		AccentRose:   ClrRose,
		AccentCyan:   ClrCyan,
		AccentPink:   ClrPink,
		AccentViolet: ClrViolet,
		AccentRed:    ClrRose,
		AccentYellow: ClrAmber,

		HeaderStyle: s.Header,
		HeaderBadge: s.ModeBadge,
		BrandBadge:  s.BrandBadge,

		MessageUser:     s.ChatUser,
		MessageAgent:    s.ChatAgent,
		MessageSystem:   s.ChatSystem,
		MessageTerminal: s.ChatTerminal,
		MessageTime:     s.ChatTime,

		ThinkingBlock:   s.ThinkingBlock,
		ThinkingHeader:  lipgloss.NewStyle().Foreground(ClrAccent).Bold(true),
		ThinkingContent: lipgloss.NewStyle().Foreground(ClrSubtext).Italic(true),

		CodeBlock:  s.CodeBlock,
		CodeInline: s.CodeInline,
		CodeLang:   s.CodeLang,

		StatsLabel: s.StatsLabel,
		StatsValue: s.StatsValue,

		SpinnerStyle:   s.Spinner,
		LiveIndicator:  s.LiveDot,
		StatusBarStyle: s.StatusBar,
		StatusBarKey:   s.StatusBarKey,
		StatusBarHint:  s.StatusBarHint,

		SidebarStyle:   s.Sidebar,
		SidebarTitle:   s.SidebarTitle,
		SidebarItem:    s.SidebarItem,
		SidebarDivider: s.SidebarDivider,

		InputStyle:       s.Input,
		InputPlaceholder: s.InputPlaceholder,
		InputPrompt:      s.InputPrompt,

		HelpOverlayStyle:    s.HelpOverlay,
		HelpOverlayTitle:    s.HelpOverlayTitle,
		HelpOverlayKey:      s.HelpOverlayKey,
		HelpOverlayDesc:     s.HelpOverlayDesc,
		PaletteStyle:        s.Palette,
		PaletteItem:         s.PaletteItem,
		PaletteItemSelected: s.PaletteItemSel,
		PaletteFilter:       s.PaletteFilter,

		BorderSubtle: subtleBorder,
		DividerStyle: s.Divider,
	}
}
