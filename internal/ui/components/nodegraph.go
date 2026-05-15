package components

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	tea "github.com/charmbracelet/bubbletea"
)

type NodeStatus string

const (
	StatusIdle     NodeStatus = "idle"
	StatusWaiting  NodeStatus = "wait"
	StatusWorking  NodeStatus = "work"
	StatusThinking NodeStatus = "think"
	StatusDone     NodeStatus = "done"
	StatusError    NodeStatus = "error"
	StatusReview   NodeStatus = "review"
)

type NodeState struct {
	ID       string
	Role     string
	Label    string
	Status   NodeStatus
	Task     string
	Progress float64
	Duration time.Duration
}

type EdgeState struct {
	From   string
	To     string
	Label  string
	Active bool
	Error  bool
}

type NodePosition struct {
	Role   string
	GridX  int
	GridY  int
	Width  int
	Height int
}

var DefaultLayout = map[string]NodePosition{
	"orchestrator": {Role: "orchestrator", GridX: 50, GridY: 5, Width: 18, Height: 4},
	"frontend":     {Role: "frontend", GridX: 20, GridY: 22, Width: 16, Height: 4},
	"backend":      {Role: "backend", GridX: 50, GridY: 22, Width: 16, Height: 4},
	"database":     {Role: "database", GridX: 80, GridY: 22, Width: 16, Height: 4},
	"qa":           {Role: "qa", GridX: 50, GridY: 40, Width: 18, Height: 4},
}

var defaultEdges = []EdgeState{
	{From: "orchestrator", To: "frontend", Label: "task"},
	{From: "orchestrator", To: "backend", Label: "task"},
	{From: "orchestrator", To: "database", Label: "task"},
	{From: "frontend", To: "qa", Label: "code"},
	{From: "backend", To: "qa", Label: "api"},
	{From: "database", To: "qa", Label: "schema"},
}

type NodeGraphModel struct {
	width          int
	height         int
	nodes          map[string]NodeState
	edges          []EdgeState
	layout         map[string]NodePosition
	focusedNode    int
	nodeKeys       []string
	animationFrame int
	showPopup      bool
	popupNode      string
	theme          *Theme
	asciiMode      bool
}

func NewNodeGraph() NodeGraphModel {
	nodes := map[string]NodeState{
		"orchestrator": {ID: "orchestrator-1", Role: "orchestrator", Label: "Orchestrator", Status: StatusThinking, Task: "Planning workflow...", Progress: 0.3},
		"frontend":     {ID: "frontend-1", Role: "frontend", Label: "Frontend", Status: StatusWaiting, Task: "Awaiting design"},
		"backend":      {ID: "backend-1", Role: "backend", Label: "Backend", Status: StatusIdle},
		"database":     {ID: "database-1", Role: "database", Label: "Database", Status: StatusWaiting, Task: "Awaiting schema"},
		"qa":           {ID: "qa-1", Role: "qa", Label: "QA/Pentest", Status: StatusIdle},
	}
	return NodeGraphModel{
		nodes:     nodes,
		edges:     defaultEdges,
		layout:    copyLayout(DefaultLayout),
		nodeKeys:  []string{"orchestrator", "frontend", "backend", "database", "qa"},
		theme:     GetTheme(),
		asciiMode: false,
	}
}

func copyLayout(src map[string]NodePosition) map[string]NodePosition {
	dst := make(map[string]NodePosition, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func (m NodeGraphModel) Init() tea.Cmd { return NodeGraphTickCmd() }

// NodeGraphTickMsg is the periodic animation tick message.
type NodeGraphTickMsg struct{}

// NodeGraphTickCmd creates a command that sends a NodeGraphTickMsg after 200ms.
func NodeGraphTickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(_ time.Time) tea.Msg { return NodeGraphTickMsg{} })
}

func (m NodeGraphModel) Update(msg tea.Msg) (NodeGraphModel, tea.Cmd) {
	switch msg.(type) {
	case NodeGraphTickMsg:
		m.animationFrame = (m.animationFrame + 1) % 4
		return m, NodeGraphTickCmd()
	}
	return m, nil
}

func (m *NodeGraphModel) SetSize(width, height int) { m.width = width; m.height = height }

func (m *NodeGraphModel) UpdateAgent(id, role string, status NodeStatus, task string) {
	if n, ok := m.nodes[role]; ok {
		n.ID = id
		n.Status = status
		n.Task = task
		m.nodes[role] = n
	}
}

func (m *NodeGraphModel) SetNodeProgress(role string, progress float64) {
	if n, ok := m.nodes[role]; ok {
		n.Progress = progress
		m.nodes[role] = n
	}
}

func (m *NodeGraphModel) AddEdge(from, to, label string, active bool) {
	for i, e := range m.edges {
		if e.From == from && e.To == to {
			m.edges[i].Active = active
			m.edges[i].Label = label
			return
		}
	}
	m.edges = append(m.edges, EdgeState{From: from, To: to, Label: label, Active: active})
}

func (m *NodeGraphModel) FocusNext() {
	if len(m.nodeKeys) == 0 {
		return
	}
	m.focusedNode = (m.focusedNode + 1) % len(m.nodeKeys)
}
func (m *NodeGraphModel) FocusPrev() {
	if len(m.nodeKeys) == 0 {
		return
	}
	m.focusedNode--
	if m.focusedNode < 0 {
		m.focusedNode = len(m.nodeKeys) - 1
	}
}
func (m *NodeGraphModel) FocusedRole() string {
	if m.focusedNode < 0 || m.focusedNode >= len(m.nodeKeys) {
		return ""
	}
	return m.nodeKeys[m.focusedNode]
}
func (m *NodeGraphModel) TogglePopup() {
	if m.showPopup {
		m.showPopup = false
		m.popupNode = ""
	} else {
		m.showPopup = true
		m.popupNode = m.FocusedRole()
	}
}
func (m *NodeGraphModel) ClosePopup()       { m.showPopup = false; m.popupNode = "" }
func (m *NodeGraphModel) IsPopupOpen() bool { return m.showPopup }
func (m *NodeGraphModel) ResetLayout()      { m.layout = copyLayout(DefaultLayout) }

// View renders the node graph
func (m NodeGraphModel) View() string {
	if m.width < 40 || m.height < 10 {
		return "Terminal too small"
	}
	var sb strings.Builder

	title := m.theme.HeaderStyle.Render(fmt.Sprintf(" SMARA NODE GRAPH — %d agents ", len(m.nodes)))
	pad := (m.width - lipgloss.Width(title)) / 2
	if pad < 0 {
		pad = 0
	}
	sb.WriteString(strings.Repeat(" ", pad))
	sb.WriteString(title)
	sb.WriteString("\n\n")

	type cell struct {
		char  rune
		style lipgloss.Style
		z     int
	}
	canvas := make([][]cell, m.height)
	for y := range canvas {
		canvas[y] = make([]cell, m.width)
		for x := range canvas[y] {
			canvas[y][x] = cell{char: ' '}
		}
	}
	place := func(x, y int, s string, style lipgloss.Style, z int) {
		runes := []rune(s)
		for i, r := range runes {
			cx := x + i
			if cx >= 0 && cx < m.width && y >= 0 && y < m.height && z >= canvas[y][cx].z {
				canvas[y][cx] = cell{char: r, style: style, z: z}
			}
		}
	}
	placeRune := func(x, y int, r rune, style lipgloss.Style, z int) {
		if x >= 0 && x < m.width && y >= 0 && y < m.height && z >= canvas[y][x].z {
			canvas[y][x] = cell{char: r, style: style, z: z}
		}
	}

	for _, edge := range m.edges {
		m.drawEdge(edge, placeRune, 1)
	}
	for i, role := range m.nodeKeys {
		m.drawNode(m.nodes[role], m.layout[role], i == m.focusedNode, place, placeRune, 2)
	}

	for y := 0; y < m.height; y++ {
		var line strings.Builder
		for x := 0; x < m.width; x++ {
			c := canvas[y][x]
			if c.z > 0 {
				line.WriteString(c.style.Render(string(c.char)))
			} else {
				line.WriteRune(c.char)
			}
		}
		sb.WriteString(line.String())
		sb.WriteString("\n")
	}

	hint := " [Tab] Chat/Node  [↑↓←→] Navigate  [Enter] Detail  [r] Reset  [q/Esc] Back "
	hw := len(hint)
	hp := (m.width - hw) / 2
	if hp < 0 {
		hp = 0
	}
	sb.WriteString(strings.Repeat(" ", hp))
	sb.WriteString(lipgloss.NewStyle().Foreground(ClrMuted).Render(hint))

	if m.showPopup && m.popupNode != "" {
		if n, ok := m.nodes[m.popupNode]; ok {
			return m.overlayPopup(sb.String(), m.renderPopup(n))
		}
	}
	return sb.String()
}

func (m NodeGraphModel) drawNode(node NodeState, pos NodePosition, focused bool, place func(int, int, string, lipgloss.Style, int), placeRune func(int, int, rune, lipgloss.Style, int), z int) {
	x := pos.GridX * m.width / 100
	y := pos.GridY * m.height / 100
	w := pos.Width
	h := pos.Height
	if x < 1 {
		x = 1
	}
	if y < 2 {
		y = 2
	}
	if x+w >= m.width {
		x = m.width - w - 1
	}
	if y+h >= m.height {
		y = m.height - h - 1
	}

	borderStyle := m.nodeBorderStyle(node.Role)
	if focused {
		borderStyle = borderStyle.Bold(true)
	}
	contentStyle := lipgloss.NewStyle().Foreground(ClrText)

	tc, hc, vc := "┌", "─", "│"
	if m.asciiMode {
		tc, hc, vc = "+", "-", "|"
	}

	place(x, y, tc+strings.Repeat(hc, w-2)+"┐", borderStyle, z)
	avatar := avatarForRole(node.Role)
	statusStr := statusIndicator(node.Status)
	label := truncate(node.Label, 10)
	line1 := fmt.Sprintf(" %s %-10s %s ", avatar, label, statusStr)
	l1n := utf8.RuneCountInString(line1)
	if l1n > w-2 {
		line1 = string([]rune(line1)[:w-2])
	}
	if l1n < w-2 {
		line1 += strings.Repeat(" ", w-2-l1n)
	}
	place(x, y+1, vc+line1+vc, contentStyle, z)

	if node.Status == StatusWorking || node.Status == StatusThinking {
		progress := renderProgressBar(node.Progress, w-4)
		line2 := " " + progress + " "
		l2n := utf8.RuneCountInString(line2)
		if l2n > w-2 {
			line2 = string([]rune(line2)[:w-2])
		}
		if l2n < w-2 {
			line2 += strings.Repeat(" ", w-2-l2n)
		}
		place(x, y+2, vc+line2+vc, contentStyle, z)
	} else {
		statusText := string(node.Status)
		padLen := w - 4 - utf8.RuneCountInString(statusText)
		if padLen < 0 {
			padLen = 0
		}
		line2 := fmt.Sprintf(" %s%s", statusText, strings.Repeat(" ", padLen))
		l2n := utf8.RuneCountInString(line2)
		if l2n > w-2 {
			line2 = string([]rune(line2)[:w-2])
		}
		if l2n < w-2 {
			line2 += strings.Repeat(" ", w-2-l2n)
		}
		place(x, y+2, vc+line2+vc, contentStyle, z)
	}
	place(x, y+h-1, "└"+strings.Repeat(hc, w-2)+"┘", borderStyle, z)
}

func (m NodeGraphModel) drawEdge(edge EdgeState, placeRune func(int, int, rune, lipgloss.Style, int), z int) {
	fromPos, ok1 := m.layout[edge.From]
	toPos, ok2 := m.layout[edge.To]
	if !ok1 || !ok2 {
		return
	}
	fx := fromPos.GridX*m.width/100 + fromPos.Width/2
	fy := fromPos.GridY*m.height/100 + fromPos.Height - 1
	tx := toPos.GridX*m.width/100 + toPos.Width/2
	ty := toPos.GridY * m.height / 100

	style := lipgloss.NewStyle().Foreground(ClrMuted)
	if edge.Active {
		style = lipgloss.NewStyle().Foreground(ClrAccent).Bold(true)
	}
	if edge.Error {
		style = lipgloss.NewStyle().Foreground(ClrRose).Bold(true)
	}

	// Simple Manhattan routing: down then horizontal then down/up
	midY := (fy + ty) / 2
	for y := fy; y <= midY && y < m.height; y++ {
		placeRune(fx, y, '│', style, z)
	}
	if fx < tx {
		for x := fx; x <= tx && x < m.width; x++ {
			placeRune(x, midY, '─', style, z)
		}
	}
	if fx > tx {
		for x := fx; x >= tx && x >= 0; x-- {
			placeRune(x, midY, '─', style, z)
		}
	}
	for y := midY; y <= ty && y < m.height; y++ {
		placeRune(tx, y, '│', style, z)
	}

	// Animated arrow on active edge
	if edge.Active {
		animPos := (m.animationFrame + fx + fy) % (midY - fy + 1)
		if animPos >= 0 && animPos < midY-fy {
			placeRune(fx, fy+animPos, '▸', style, z+1)
		}
	}
}

func (m NodeGraphModel) renderPopup(node NodeState) string {
	var sb strings.Builder
	title := lipgloss.NewStyle().Foreground(ClrAccent).Bold(true).Render(fmt.Sprintf(" Agent: %s ", node.ID))
	content := fmt.Sprintf("\n Role:    %s\n Status:  %s %s\n Task:    %s\n Progress: %s %.0f%%\n Duration: %s\n\n [Esc] Close  [l] View Log ",
		node.Label, statusIndicator(node.Status), node.Status, node.Task,
		renderProgressBar(node.Progress, 20), node.Progress*100, node.Duration)
	sb.WriteString(title)
	sb.WriteString(content)
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ClrAccent).Padding(1, 2).Render(sb.String())
}

func (m NodeGraphModel) overlayPopup(base, popup string) string {
	baseLines := strings.Split(base, "\n")
	popupLines := strings.Split(popup, "\n")
	pw := 0
	for _, l := range popupLines {
		if len(l) > pw {
			pw = len(l)
		}
	}
	ph := len(popupLines)
	startY := (m.height - ph) / 2
	if startY < 0 {
		startY = 0
	}
	startX := (m.width - pw) / 2
	if startX < 0 {
		startX = 0
	}

	for i, pl := range popupLines {
		by := startY + i
		if by >= 0 && by < len(baseLines) {
			bl := baseLines[by]
			if startX < len(bl) {
				left := ""
				if startX > 0 {
					left = bl[:startX]
				}
				right := ""
				if startX+len(pl) < len(bl) {
					right = bl[startX+len(pl):]
				}
				baseLines[by] = left + pl + right
			}
		}
	}
	return strings.Join(baseLines, "\n")
}

func (m NodeGraphModel) nodeBorderStyle(role string) lipgloss.Style {
	switch role {
	case "orchestrator":
		return lipgloss.NewStyle().Foreground(ClrViolet)
	case "frontend":
		return lipgloss.NewStyle().Foreground(ClrCyan)
	case "backend":
		return lipgloss.NewStyle().Foreground(ClrGreen)
	case "database":
		return lipgloss.NewStyle().Foreground(ClrAmber)
	case "qa":
		return lipgloss.NewStyle().Foreground(ClrRose)
	default:
		return lipgloss.NewStyle().Foreground(ClrAccent)
	}
}

func (m NodeGraphModel) nodeBgStyle(role string) lipgloss.Style {
	switch role {
	case "orchestrator":
		return lipgloss.NewStyle().Background(ClrViolet).Foreground(ClrBase)
	case "frontend":
		return lipgloss.NewStyle().Background(ClrCyan).Foreground(ClrBase)
	case "backend":
		return lipgloss.NewStyle().Background(ClrGreen).Foreground(ClrBase)
	case "database":
		return lipgloss.NewStyle().Background(ClrAmber).Foreground(ClrBase)
	case "qa":
		return lipgloss.NewStyle().Background(ClrRose).Foreground(ClrBase)
	default:
		return lipgloss.NewStyle().Background(ClrElevated).Foreground(ClrText)
	}
}

func avatarForRole(role string) string {
	switch role {
	case "orchestrator":
		return "🧠"
	case "frontend":
		return "🎨"
	case "backend":
		return "⚙️"
	case "database":
		return "🗄️"
	case "qa":
		return "🔍"
	default:
		return "🤖"
	}
}

func statusIndicator(status NodeStatus) string {
	switch status {
	case StatusIdle:
		return "○"
	case StatusWaiting:
		return "○ ○"
	case StatusWorking:
		return "▓▓▓"
	case StatusThinking:
		return "◐"
	case StatusDone:
		return "✓"
	case StatusError:
		return "✕"
	case StatusReview:
		return "◯"
	default:
		return "○"
	}
}

func renderProgressBar(progress float64, width int) string {
	if width <= 0 {
		return ""
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	filled := int(progress * float64(width))
	if filled > width {
		filled = width
	}
	empty := width - filled
	return strings.Repeat("▓", filled) + strings.Repeat("░", empty)
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}
