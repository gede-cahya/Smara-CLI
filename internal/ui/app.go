package ui

import (
	"context"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/lipgloss"
	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/agent/workflow"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/gede-cahya/Smara-CLI/internal/nudge"
	"github.com/gede-cahya/Smara-CLI/internal/skill"
	smarassh "github.com/gede-cahya/Smara-CLI/internal/ssh"
	"github.com/gede-cahya/Smara-CLI/internal/ui/clipboard"
	"github.com/gede-cahya/Smara-CLI/internal/ui/components"
)

// ═══════════════════════════════════════════════════════════════
// Smara CLI TUI App — Interactive Multi-Panel Terminal UI
// ═══════════════════════════════════════════════════════════════

// AppVersion is set by the main package at startup.
var AppVersion = "dev"

// Style definitions — Crush Pastel Green palette
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#09090b")).
			Background(lipgloss.Color("#bef264")).
			PaddingLeft(2).
			PaddingRight(2)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#86efac"))

	warnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#fcd34d"))

	errStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#fda4af"))

	agentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#bef264")).
			Bold(true)

	userStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f4f4f5")).
			Bold(true)

	messageStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f4f4f5"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#71717a"))

	thinkingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#a1a1aa")).
			Italic(true).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(lipgloss.Color("#27272a")).
			PaddingLeft(1).
			MarginLeft(1)

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#27272a"))

	terminalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#bef264")).
			Bold(true)

	codingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f4f4f5")).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(lipgloss.Color("#27272a")).
			PaddingLeft(1).
			MarginLeft(1)

	codePrefixStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#bef264")).
			Italic(true)
)

// Global reference for programmatic messaging
var globalProgram *tea.Program

// ChatMessage represents a single message in the UI
type ChatMessage struct {
	Role          string // "System", "User", "Agent"
	Content       string
	Thinking      string
	Thoughts      []string
	ToolsExecuted []string
	Time          time.Time
	InputTokens   int
	OutputTokens  int
	Duration      time.Duration
	ModelName     string
	ExpandedCode  bool // Toggle to expand collapsed code blocks in this message
}

type viewMode int

const (
	viewChat viewMode = iota
	viewNodeGraph
	viewHelp
)

// AppSupervisor interface to avoid circular dependency
type AppSupervisor interface {
	ProcessPrompt(ctx context.Context, prompt string) (*agent.PromptResult, error)
	GetMode() agent.Mode
	SetMode(mode agent.Mode)
	GetModelInfo() (provider, model string)
	GetProviderName() string
	SkillExecutor() skill.StepExecutor
	GetMCPInfo() map[string]agent.MCPServerInfo
	ClearHistory()
	SaveSession() error
}

// AppModel is the Bubbletea model for our TUI
type AppModel struct {
	viewport   viewport.Model
	textarea   textarea.Model
	messages   []ChatMessage
	err        error
	width      int
	height     int
	supervisor AppSupervisor
	ctx        context.Context
	cancel     context.CancelFunc
	processing bool

	// View mode
	currentView viewMode
	nodeGraph   components.NodeGraphModel

	// Streaming state
	currentStream   string
	currentThinking string
	currentExplore  string

	// Confirmation state
	awaitingConfirmation bool
	confirmMessage       string
	confirmResponseCh    chan bool
	confirmSelection     int // 0: Ya, 1: Tidak

	// Interactive TUI state
	spinner    spinner.Model
	statusText string

	// History management
	cmdHistory []string
	historyIdx int

	// Command handler callback
	onCommand func(string, []string)

	// Sidebar state
	todoList        TodoList
	showSidebar     bool
	sidebarViewport viewport.Model
	sidebarWidth    int

	// ─── NEW: Interactive components ───────────────────────────
	theme         *components.Theme
	layout        components.Layout
	headerComp    *components.Header
	sidebarComp   *components.Sidebar
	statusBarComp *components.StatusBar
	msgRenderer   *components.MessageRenderer
	helpOverlay   *components.HelpOverlay
	palette       *components.CommandPalette
	phaseStepper  *components.PhaseStepper
	sessionPicker *components.SessionPicker
	showHelp      bool
	showPalette   bool
	showThinking  bool // toggle thinking visibility

	// Animation state
	sidebarSpring      harmonica.Spring
	sidebarWidthFloat  float64
	sidebarTargetWidth float64
	sidebarVelocity    float64
	animatingSidebar   bool

	// Live Generate Animation state
	streamStartTime time.Time
	dotFrame        int
	cursorVisible   bool

	// Phase stepper state (real-time pipeline phases)
	currentPhase  string            // active phase name
	phaseContents map[string]string // phase name → accumulated live content
	phaseDescs    map[string]string // phase name → description
	phaseSeen     []string          // ordered list of phases that became active
	phaseSeenSet  map[string]bool   // dedup set for phaseSeen
	fadeWave      *components.FadeWave

	// Cancellation guard — drop stale stream/phase/process messages after cancel
	cancelled bool

	// Copy / paste & selection state
	selectionMode    bool      // Ctrl+S message selection mode
	selectedMsgIndex int       // index of selected historical message
	toastText        string    // transient notification text
	toastExpiry      time.Time // when toast disappears

	// Memory store for persistence
	store memory.MemoryStore
}

// InitialModel creates a new model
func InitialModel(sup AppSupervisor, onCmd func(cmd string, args []string), store memory.MemoryStore) AppModel {
	ta := textarea.New()
	ta.Placeholder = "Ketik pesan atau /help..."
	ta.Focus()
	ta.Prompt = "┃ "
	ta.CharLimit = 2000
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(false) // Disable enter to newline, we'll use enter to submit

	vp := viewport.New(80, 20)
	vp.SetContent(bannerContent())

	sidebarVp := viewport.New(30, 20)
	sidebarVp.SetContent("  Belum ada edit.")

	ctx, cancel := context.WithCancel(context.Background())

	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#bef264"))

	theme := components.GetTheme()

	return AppModel{
		textarea:        ta,
		viewport:        vp,
		messages:        []ChatMessage{},
		supervisor:      sup,
		ctx:             ctx,
		cancel:          cancel,
		spinner:         s,
		cmdHistory:      []string{},
		historyIdx:      -1,
		onCommand:       onCmd,
		sidebarViewport: sidebarVp,
		showSidebar:     false, // Default: sidebar hidden
		sidebarWidth:    0,
		// ─── NEW ─────────────────────────────────────────────────
		currentView:   viewChat,
		nodeGraph:     components.NewNodeGraph(),
		theme:         theme,
		headerComp:    components.NewHeader(80),
		sidebarComp:   components.NewSidebar(28, 20),
		statusBarComp: components.NewStatusBar(80),
		msgRenderer:   components.NewMessageRenderer(80),
		helpOverlay:   components.NewHelpOverlay(60),
		palette:       components.NewCommandPalette(50),
		phaseStepper:  components.NewPhaseStepper(80),
		sessionPicker: components.NewSessionPicker(70, 20),
		showThinking:  true,
		sidebarSpring: harmonica.NewSpring(harmonica.FPS(60), 6.0, 0.4),
		store:         store,
		phaseContents: make(map[string]string),
		phaseDescs:    make(map[string]string),
		phaseSeenSet:  make(map[string]bool),
		fadeWave:      components.NewFadeWave(80),
	}
}

type animTickMsg time.Time

func animTickCmd() tea.Cmd {
	return tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
		return animTickMsg(t)
	})
}

func bannerContent() string {
	banner := `
  ███████╗███╗   ███╗ █████╗ ██████╗  █████╗ 
  ██╔════╝████╗ ████║██╔══██╗██╔══██╗██╔══██╗
  ███████╗██╔████╔██║███████║██████╔╝███████║
  ╚════██║██║╚██╔╝██║██╔══██║██╔══██╗██╔══██║
  ███████║██║ ╚═╝ ██║██║  ██║██║  ██║██║  ██║
  ╚══════╝╚═╝     ╚═╝╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═╝
`
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#bef264")).Bold(true).Render(banner) +
		"\n" + dimStyle.Render(fmt.Sprintf("  स्मृति — Autonomous Multi-Agent Terminal v%s\n  Ketik /help untuk daftar perintah.\n", AppVersion))
}

// Init initializes the app
func (m AppModel) Init() tea.Cmd {
	// Check for pending nudges on startup
	if dbStore, ok := m.store.(*memory.SQLiteStore); ok {
		if nudges, err := nudge.GetAllPending(dbStore.DB()); err == nil && len(nudges) > 0 {
			banner := nudge.FormatNudges(nudges)
			m.addMessage("System", banner)
		}
	}
	return tea.Batch(textarea.Blink, m.spinner.Tick, tea.EnableBracketedPaste)
}

// ProcessMsg is sent when the supervisor finishes processing
type ProcessMsg struct {
	Result *agent.PromptResult
	Err    error
}

// StreamMsg is received when a chunk of text is streamed from LLM
type StreamMsg struct {
	Chunk      string
	IsThinking bool
	Phase      string // e.g. "Thinking", "Analyzing", "Exploring", "Generating"
}

// PhaseMsg is sent when the backend transitions to a new pipeline phase.
type PhaseMsg struct {
	Phase       string
	Description string
}

// LogMsg allows external systems to inject messages into the UI
type LogMsg struct {
	Message ChatMessage
}

type ExploreMsg struct {
	Path    string
	Content string
}

type ConfirmRequestMsg struct {
	Message    string
	ResponseCh chan bool
}

// Live Generate Animation ticks
type dotTickMsg struct{}
type cursorBlinkMsg struct{}
type waveTickMsg struct{}

func dotTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return dotTickMsg{}
	})
}

func cursorBlinkCmd() tea.Cmd {
	return tea.Tick(530*time.Millisecond, func(t time.Time) tea.Msg {
		return cursorBlinkMsg{}
	})
}

func waveTickCmd() tea.Cmd {
	return tea.Tick(33*time.Millisecond, func(t time.Time) tea.Msg {
		return waveTickMsg{}
	})
}

// Update handles messages and state changes
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
		cmds  []tea.Cmd
	)

	switch msg.(type) {
	case animTickMsg:
		if m.animatingSidebar {
			m.sidebarWidthFloat, m.sidebarVelocity = m.sidebarSpring.Update(m.sidebarWidthFloat, m.sidebarVelocity, m.sidebarTargetWidth)

			if math.Abs(m.sidebarWidthFloat-m.sidebarTargetWidth) < 0.5 && math.Abs(m.sidebarVelocity) < 0.5 {
				m.sidebarWidthFloat = m.sidebarTargetWidth
				m.animatingSidebar = false
			} else {
				cmds = append(cmds, animTickCmd())
			}
			m.sidebarWidth = int(m.sidebarWidthFloat)
			m.updateLayout()
			m.renderMessages()
		}
		return m, tea.Batch(cmds...)
	case components.NodeGraphTickMsg:
		var ngCmd tea.Cmd
		m.nodeGraph, ngCmd = m.nodeGraph.Update(msg)
		cmds = append(cmds, ngCmd)
		return m, tea.Batch(cmds...)
	}

	// ─── Handle overlays first ─────────────────────────────────
	// If palette is active, handle palette input
	if m.palette.IsActive() {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc", "ctrl+c":
				m.palette.Close()
				return m, nil
			case "enter":
				if item, ok := m.palette.SelectedItem(); ok {
					m.palette.Close()
					m.textarea.SetValue(item.Command)
					// Auto-submit
					v := strings.TrimSpace(item.Command)
					if v != "" {
						m.addMessage("User", v)
						if IsCommand(v) {
							cmdName, cmdArgs := ParseCommand(v)
							m.handleCommand(cmdName, cmdArgs)
						} else {
							m.cancelled = false
							m.processing = true
							m.statusText = "Memproses..."
							m.currentStream = ""
							m.currentThinking = ""
							sup := m.supervisor
							ctx := m.ctx
							cmds = append(cmds, m.spinner.Tick)
							cmds = append(cmds, func() tea.Msg {
								result, err := sup.ProcessPrompt(ctx, v)
								return ProcessMsg{Result: result, Err: err}
							})
						}
					}
					m.textarea.Reset()
				}
				return m, tea.Batch(cmds...)
			case "up":
				m.palette.MoveSelection(-1)
				return m, nil
			case "down":
				m.palette.MoveSelection(1)
				return m, nil
			case "backspace":
				if len(m.palette.FilterText()) > 0 {
					m.palette.SetFilter(m.palette.FilterText()[:len(m.palette.FilterText())-1])
				}
				return m, nil
			default:
				if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
					m.palette.SetFilter(m.palette.FilterText() + string(msg.Runes))
				}
				return m, nil
			}
		}
		// Still update textarea/viewport in background but don't process their input
		m.textarea, tiCmd = m.textarea.Update(msg)
		m.viewport, vpCmd = m.viewport.Update(msg)
		cmds = append(cmds, tiCmd, vpCmd)
		return m, tea.Batch(cmds...)
	}

	// If session picker is active, handle picker input
	if m.sessionPicker.IsActive() {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc", "ctrl+c":
				if m.sessionPicker.IsDeleting() {
					m.sessionPicker.CancelDelete()
					return m, nil
				}
				m.sessionPicker.Close()
				return m, nil
			case "enter":
				if m.sessionPicker.IsDeleting() {
					deleteID := m.sessionPicker.ConfirmDelete()
					m.sessionPicker.Close()
					if agentSup, ok := m.supervisor.(*agent.Supervisor); ok && deleteID != "" {
						if err := agentSup.DeleteSession(deleteID); err != nil {
							m.addMessage("System", fmt.Sprintf("Gagal menghapus session: %v", err))
						} else {
							m.addMessage("System", fmt.Sprintf("Session dihapus: %s", deleteID[:8]))
						}
					}
					return m, nil
				}
				if item, ok := m.sessionPicker.SelectedItem(); ok {
					m.sessionPicker.Close()
					if agentSup, ok := m.supervisor.(*agent.Supervisor); ok {
						if err := agentSup.SwitchSession(item.Session.ID); err != nil {
							m.addMessage("System", fmt.Sprintf("Gagal switch session: %v", err))
						} else {
							m.messages = []ChatMessage{}
							history, _ := agentSup.GetSessionHistory(item.Session.ID)
							for _, msg := range history {
								role := "User"
								if msg.Role == llm.RoleAssistant {
									role = "Agent"
								}
								m.messages = append(m.messages, ChatMessage{
									Role:    role,
									Content: msg.Content,
									Time:    time.Now(),
								})
							}
							m.renderMessages()
							m.showToast(fmt.Sprintf("Switched to: %s", item.Session.Name))
						}
					}
					return m, nil
				}
			case "up":
				m.sessionPicker.MoveSelection(-1)
				return m, nil
			case "down":
				m.sessionPicker.MoveSelection(1)
				return m, nil
			case "d":
				if !m.sessionPicker.IsDeleting() {
					if _, ok := m.sessionPicker.SelectedItem(); ok {
						m.sessionPicker.SetDeleteConfirm()
						return m, nil
					}
				}
			case "backspace":
				if len(m.sessionPicker.FilterText()) > 0 {
					m.sessionPicker.SetFilter(m.sessionPicker.FilterText()[:len(m.sessionPicker.FilterText())-1])
				}
				return m, nil
			default:
				if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
					m.sessionPicker.SetFilter(m.sessionPicker.FilterText() + string(msg.Runes))
				}
				return m, nil
			}
		}
		m.textarea, tiCmd = m.textarea.Update(msg)
		m.viewport, vpCmd = m.viewport.Update(msg)
		cmds = append(cmds, tiCmd, vpCmd)
		return m, tea.Batch(cmds...)
	}

	// If help overlay is showing
	if m.showHelp {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc", "ctrl+?", "ctrl+c":
				m.showHelp = false
				return m, nil
			}
		}
		m.textarea, tiCmd = m.textarea.Update(msg)
		m.viewport, vpCmd = m.viewport.Update(msg)
		cmds = append(cmds, tiCmd, vpCmd)
		return m, tea.Batch(cmds...)
	}

	m.textarea, tiCmd = m.textarea.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, tiCmd, vpCmd)

	switch msg := msg.(type) {
	case ConfirmRequestMsg:
		m.awaitingConfirmation = true
		m.confirmMessage = msg.Message
		m.confirmResponseCh = msg.ResponseCh
		m.confirmSelection = 0
		m.addMessage("System", m.confirmMessage)
		return m, nil

	case tea.KeyMsg:
		if msg.Paste {
			m.textarea.InsertString(string(msg.Runes))
			return m, nil
		}
		if m.awaitingConfirmation {
			switch msg.String() {
			case "left", "right":
				if m.confirmSelection == 0 {
					m.confirmSelection = 1
				} else {
					m.confirmSelection = 0
				}
				return m, nil
			case "enter":
				m.awaitingConfirmation = false
				m.confirmResponseCh <- (m.confirmSelection == 0)

				answer := "ya"
				if m.confirmSelection == 1 {
					answer = "tidak"
				}
				m.addMessage("User", answer)
				return m, nil
			case "esc", "ctrl+c":
				m.awaitingConfirmation = false
				m.confirmResponseCh <- false
				m.addMessage("User", "tidak")
				return m, nil
			}
			// Block other keys while confirming
			return m, nil
		}

		// Selection mode: copy from chat history
		if m.selectionMode {
			switch msg.Type {
			case tea.KeyUp:
				if m.selectedMsgIndex > 0 {
					m.selectedMsgIndex--
					m.renderMessages()
				}
				return m, nil
			case tea.KeyDown:
				if m.selectedMsgIndex < len(m.messages)-1 {
					m.selectedMsgIndex++
					m.renderMessages()
				}
				return m, nil
			case tea.KeyEnter:
				if m.selectedMsgIndex >= 0 && m.selectedMsgIndex < len(m.messages) {
					content := m.messages[m.selectedMsgIndex].Content
					clipboard.Write(content)
					m.showToast(fmt.Sprintf("Pesan #%d disalin ke clipboard", m.selectedMsgIndex+1))
					m.selectionMode = false
					m.renderMessages()
				}
				return m, nil
			case tea.KeyRunes:
				if len(msg.Runes) == 1 && (msg.Runes[0] == 'c' || msg.Runes[0] == 'C') {
					if m.selectedMsgIndex >= 0 && m.selectedMsgIndex < len(m.messages) {
						content := m.messages[m.selectedMsgIndex].Content
						clipboard.Write(content)
						m.showToast(fmt.Sprintf("Pesan #%d disalin ke clipboard", m.selectedMsgIndex+1))
						m.selectionMode = false
						m.renderMessages()
					}
					return m, nil
				}
			case tea.KeyEsc:
				m.selectionMode = false
				m.renderMessages()
				return m, nil
			}
			// Block other keys while in selection mode
			return m, nil
		}

		// ─── Node Graph view key handling ────────────────────────
		if m.currentView == viewNodeGraph {
			switch msg.Type {
			case tea.KeyUp:
				m.nodeGraph.FocusPrev()
				return m, nil
			case tea.KeyDown:
				m.nodeGraph.FocusNext()
				return m, nil
			case tea.KeyLeft:
				m.nodeGraph.FocusPrev()
				return m, nil
			case tea.KeyRight:
				m.nodeGraph.FocusNext()
				return m, nil
			case tea.KeyEnter:
				m.nodeGraph.TogglePopup()
				return m, nil
			case tea.KeyEsc:
				if m.nodeGraph.IsPopupOpen() {
					m.nodeGraph.ClosePopup()
				} else {
					m.currentView = viewChat
				}
				return m, nil
			}
			switch msg.String() {
			case "q":
				if m.nodeGraph.IsPopupOpen() {
					m.nodeGraph.ClosePopup()
				} else {
					m.currentView = viewChat
				}
				return m, nil
			case "r", "R":
				m.nodeGraph.ResetLayout()
				return m, nil
			}
			// Block other keys in node graph view
			return m, nil
		}

		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			if m.processing {
				m.cancel() // Cancel the current process
				m.ctx, m.cancel = context.WithCancel(context.Background())
				m.processing = false
				m.cancelled = true
				m.addMessage("System", "Proses dibatalkan.")
			} else {
				_ = m.supervisor.SaveSession()
				return m, tea.Quit
			}

		case tea.KeyCtrlQ:
			_ = m.supervisor.SaveSession()
			return m, tea.Quit

		case tea.KeyCtrlD:
			_ = m.supervisor.SaveSession()
			return m, tea.Quit

		case tea.KeyCtrlV:
			if msg.Alt { // Ctrl+Shift+V via bracketed paste
				// Handled by msg.Paste above
			} else {
				// 1. Try image clipboard first.
				if img, err := clipboard.ReadImage(); err == nil && img != nil {
					ref := fmt.Sprintf("[image:%s]", img.Path)
					m.textarea.InsertString(ref + " ")
					sizeKB := img.Size / 1024
					m.showToast(fmt.Sprintf("📎 Image attached (%dKB · %s) → %s", sizeKB, img.Source, img.Path))
					return m, nil
				}
				// 2. Fall back to text clipboard.
				if text, err := clipboard.Read(); err == nil && text != "" {
					m.textarea.InsertString(text)
				} else {
					m.showToast("Clipboard tidak tersedia di terminal ini (text & image kosong)")
				}
			}
			return m, nil

		case tea.KeyCtrlS:
			if !m.processing && len(m.messages) > 0 {
				m.selectionMode = true
				m.selectedMsgIndex = len(m.messages) - 1
				m.showToast("Mode seleksi aktif: ↑/↓ pilih, Enter/C salin, Esc batal")
				m.renderMessages()
			}
			return m, nil

		case tea.KeyCtrlB:
			// Toggle sidebar
			m.showSidebar = !m.showSidebar
			if m.showSidebar {
				m.sidebarTargetWidth = 28.0
			} else {
				m.sidebarTargetWidth = 0.0
			}
			if !m.animatingSidebar {
				m.animatingSidebar = true
				cmds = append(cmds, animTickCmd())
			}
			return m, tea.Batch(cmds...)

		case tea.KeyCtrlT:
			// Toggle thinking visibility
			m.showThinking = !m.showThinking
			m.renderMessages()
			return m, nil

		case tea.KeyCtrlP:
			// Toggle command palette
			m.palette.Toggle()
			return m, nil

		case tea.KeyF3:
			// Toggle session picker
			if agentSup, ok := m.supervisor.(*agent.Supervisor); ok {
				m.sessionPicker.Toggle(agentSup.ListSessions())
				if m.sessionPicker.IsActive() {
					m.sessionPicker.SetSize(m.layout.ContentW-10, m.layout.ChatH-4)
				}
			}
			return m, nil

		case tea.KeyRunes:
			// Handle '?' key to toggle help overlay
			if len(msg.Runes) == 1 && msg.Runes[0] == '?' && !m.processing {
				m.showHelp = !m.showHelp
				return m, nil
			}
		case tea.KeyF1:
			// Alternative: F1 for help
			m.showHelp = !m.showHelp
			return m, nil

		case tea.KeyTab:
			// Cycle agent modes: ask -> rush -> plan -> test
			if m.processing {
				m.showToast("Tunggu proses selesai sebelum ganti mode.")
				return m, nil
			}
			current := string(m.supervisor.GetMode())
			idx := -1
			for i, mode := range ModeOrder {
				if mode == current {
					idx = i
					break
				}
			}
			nextIdx := 0
			if idx >= 0 {
				nextIdx = (idx + 1) % len(ModeOrder)
			}
			nextMode := agent.Mode(ModeOrder[nextIdx])
			m.supervisor.SetMode(nextMode)
			info := agent.GetModeInfo(nextMode)
			m.showToast(fmt.Sprintf("Mode: %s %s", info.Emoji, info.Label))
			m.renderMessages()
			return m, nil

		case tea.KeyF2:
			// Cycle views: Chat -> NodeGraph -> Help -> Chat
			switch m.currentView {
			case viewChat:
				m.currentView = viewNodeGraph
				m.nodeGraph.SetSize(m.layout.ContentW, m.layout.Height-m.layout.HeaderH-m.layout.StatusH)
				return m, components.NodeGraphTickCmd()
			case viewNodeGraph:
				m.currentView = viewHelp
				return m, nil
			case viewHelp:
				m.currentView = viewChat
				return m, nil
			}

		case tea.KeyUp:
			if len(m.cmdHistory) > 0 {
				if m.historyIdx == -1 {
					m.historyIdx = len(m.cmdHistory) - 1
				} else if m.historyIdx > 0 {
					m.historyIdx--
				}
				m.textarea.SetValue(m.cmdHistory[m.historyIdx])
			}

		case tea.KeyDown:
			if m.historyIdx != -1 {
				if m.historyIdx < len(m.cmdHistory)-1 {
					m.historyIdx++
					m.textarea.SetValue(m.cmdHistory[m.historyIdx])
				} else {
					m.historyIdx = -1
					m.textarea.SetValue("")
				}
			}

		case tea.KeyEnter:
			v := strings.TrimSpace(m.textarea.Value())
			if v == "" || m.processing {
				return m, nil
			}

			// Add to history
			if len(m.cmdHistory) == 0 || m.cmdHistory[len(m.cmdHistory)-1] != v {
				m.cmdHistory = append(m.cmdHistory, v)
			}
			m.historyIdx = -1
			m.textarea.Reset()

			if IsExitCommand(v) {
				_ = m.supervisor.SaveSession()
				return m, tea.Quit
			}

			m.addMessage("User", v)

			if IsCommand(v) {
				// Handle command immediately and add to view
				cmdName, cmdArgs := ParseCommand(v)
				m.handleCommand(cmdName, cmdArgs)
			} else if isDirectSSHCommand(v) {
				// Intercept raw SSH commands like "ssh -i key.pem user@host"
				m.addMessage("System", fmt.Sprintf("Eksekusi SSH langsung: %s", v))
				m.cancelled = false
				m.processing = true
				m.statusText = "SSH..."
				cmds = append(cmds, m.spinner.Tick)
				cmds = append(cmds, func() tea.Msg {
					result, err := m.handleDirectSSH(v)
					return ProcessMsg{Result: &agent.PromptResult{Response: result}, Err: err}
				})
			} else {
				// Process @mentions
				processedPrompt := m.processFileMentions(v)

				m.cancelled = false
				m.processing = true
				m.statusText = "Memproses..."
				m.currentStream = ""
				m.currentThinking = ""
				m.streamStartTime = time.Now()
				m.dotFrame = 0
				m.cursorVisible = true
				m.currentPhase = ""
				m.phaseSeen = nil
				m.phaseContents = make(map[string]string)
				m.phaseDescs = make(map[string]string)
				m.fadeWave.Reset()
				sup := m.supervisor
				ctx := m.ctx

				cmds = append(cmds, m.spinner.Tick)
				cmds = append(cmds, dotTickCmd())

				// If in workflow mode, run the multi-agent workflow engine
				if sup.GetMode() == agent.ModeWorkflow {
					if agentSup, ok := sup.(*agent.Supervisor); ok {
						cmds = append(cmds, func() tea.Msg {
							result, err := workflow.RunWorkflow(agentSup, agentSup.GetProvider(), processedPrompt)
							if err != nil {
								return ProcessMsg{Result: nil, Err: err}
							}
							var sb strings.Builder
							sb.WriteString(fmt.Sprintf("# Workflow Complete: %s\n\n", result.FinalSummary))
							sb.WriteString(fmt.Sprintf("**Domain:** %s\n\n", result.Domain))
							sb.WriteString("## PRD\n\n")
							sb.WriteString(result.PRD)
							sb.WriteString("\n\n## Architecture / Workflow Design\n\n")
							sb.WriteString(result.Architecture)
							sb.WriteString("\n\n## QA Result\n\n")
							sb.WriteString(fmt.Sprintf("- Status: %s\n", result.QAResult.Status))
							sb.WriteString(fmt.Sprintf("- Score: %d/100\n", result.QAResult.Score))
							if len(result.QAResult.Issues) > 0 {
								sb.WriteString("- Issues:\n")
								for _, issue := range result.QAResult.Issues {
									sb.WriteString(fmt.Sprintf("  - %s\n", issue))
								}
							}
							sb.WriteString(fmt.Sprintf("\n**Project Directory:** %s\n", result.ProjectPath))
							thinking := fmt.Sprintf("Domain: %s | Agents: %d | QA: %s", result.Domain, len(result.AgentOutputs), result.QAResult.Status)
							return ProcessMsg{Result: &agent.PromptResult{
								Response:     sb.String(),
								Thinking:     thinking,
								InputTokens:  0,
								OutputTokens: 0,
								TotalTokens:  0,
								Duration:     0,
							}, Err: nil}
						})
					} else {
						// Fallback to normal processing if type assertion fails
						cmds = append(cmds, cursorBlinkCmd())
						cmds = append(cmds, waveTickCmd())
						cmds = append(cmds, func() tea.Msg {
							result, err := sup.ProcessPrompt(ctx, processedPrompt)
							return ProcessMsg{Result: result, Err: err}
						})
					}
				} else {
					// Normal supervisor processing
					cmds = append(cmds, cursorBlinkCmd())
					cmds = append(cmds, waveTickCmd())
					cmds = append(cmds, func() tea.Msg {
						result, err := sup.ProcessPrompt(ctx, processedPrompt)
						return ProcessMsg{Result: result, Err: err}
					})
				}
			}
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case dotTickMsg:
		if m.processing {
			m.dotFrame++
			m.renderMessages()
			cmds = append(cmds, dotTickCmd())
		}

	case cursorBlinkMsg:
		if m.processing {
			m.cursorVisible = !m.cursorVisible
			m.renderMessages()
			cmds = append(cmds, cursorBlinkCmd())
		}

	case waveTickMsg:
		if m.processing && m.currentPhase == "Generating" {
			// Re-render to update fade-wave gradient
			m.renderMessages()
			cmds = append(cmds, waveTickCmd())
		}

	case animTickMsg:
		// Periodic timer to clear expired toasts and refresh UI
		if m.toastText != "" && time.Now().After(m.toastExpiry) {
			m.toastText = ""
		}
		cmds = append(cmds, animTickCmd())

	case PhaseMsg:
		if m.cancelled {
			return m, nil
		}
		// Track phases in order (deduped so each phase appears only once)
		if m.currentPhase != msg.Phase {
			m.currentPhase = msg.Phase
			if msg.Phase != "" && !m.phaseSeenSet[msg.Phase] {
				m.phaseSeenSet[msg.Phase] = true
				m.phaseSeen = append(m.phaseSeen, msg.Phase)
				m.phaseDescs[msg.Phase] = msg.Description
			}
		}
		m.renderMessages()

	case StreamMsg:
		if m.cancelled {
			return m, nil
		}
		// Route content to the correct phase bucket
		if msg.IsThinking {
			m.currentThinking += msg.Chunk
			if msg.Phase != "" {
				m.phaseContents[msg.Phase] += msg.Chunk
			}
		} else {
			m.currentStream += msg.Chunk
			if msg.Phase != "" {
				m.phaseContents[msg.Phase] += msg.Chunk
				if msg.Phase == "Generating" {
					m.fadeWave.Append(msg.Chunk)
				}
			}
		}
		if msg.Phase != "" && m.currentPhase != msg.Phase {
			m.currentPhase = msg.Phase
			if !m.phaseSeenSet[msg.Phase] {
				m.phaseSeenSet[msg.Phase] = true
				m.phaseSeen = append(m.phaseSeen, msg.Phase)
			}
		}
		m.renderMessages()

	case ExploreMsg:
		m.currentExplore = msg.Content
		m.renderMessages()

	case ProcessMsg:
		if m.cancelled {
			m.cancelled = false
			m.processing = false
			m.statusText = ""
			m.currentStream = ""
			m.currentThinking = ""
			m.currentExplore = ""
			m.currentPhase = ""
			m.phaseSeen = nil
			m.phaseSeenSet = make(map[string]bool)
			m.phaseContents = make(map[string]string)
			m.phaseDescs = make(map[string]string)
			m.fadeWave.Reset()
			return m, nil
		}
		m.processing = false
		m.statusText = ""
		m.currentStream = ""
		m.currentThinking = ""
		m.currentExplore = ""
		m.currentPhase = ""
		m.phaseSeen = nil
		m.phaseSeenSet = make(map[string]bool)
		m.phaseContents = make(map[string]string)
		m.phaseDescs = make(map[string]string)
		m.fadeWave.Reset()
		if msg.Err != nil {
			if msg.Err.Error() == "context canceled" {
				// Already handled in KeyCtrlC
			} else {
				m.addMessage("System", fmt.Sprintf("Error: %v", msg.Err))
			}
		} else {
			_, modelName := m.supervisor.GetModelInfo()

			// Intercept the "Lanjutkan eksekusi? (ya/tidak)" message
			if strings.Contains(msg.Result.Response, "Lanjutkan eksekusi? (ya/tidak)") {
				// Extract everything before the prompt, if any
				cleanResp := strings.ReplaceAll(msg.Result.Response, "Lanjutkan eksekusi? (ya/tidak)", "")
				cleanResp = strings.TrimSpace(cleanResp)

				if cleanResp != "" {
					m.addMessageFull("Agent", cleanResp, msg.Result.Thinking, msg.Result.Thoughts, msg.Result.ToolsExecuted, msg.Result.InputTokens, msg.Result.OutputTokens, msg.Result.Duration, modelName)
				} else if msg.Result.Thinking != "" {
					m.addMessageFull("Agent", "", msg.Result.Thinking, msg.Result.Thoughts, msg.Result.ToolsExecuted, msg.Result.InputTokens, msg.Result.OutputTokens, msg.Result.Duration, modelName)
				}

				m.awaitingConfirmation = true
				m.confirmSelection = 0 // Default "Ya"
			} else {
				m.addMessageFull("Agent", msg.Result.Response, msg.Result.Thinking, msg.Result.Thoughts, msg.Result.ToolsExecuted, msg.Result.InputTokens, msg.Result.OutputTokens, msg.Result.Duration, modelName)
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()

	case LogMsg:
		m.messages = append(m.messages, msg.Message)
		m.renderMessages()
	}

	return m, tea.Batch(cmds...)
}

// updateLayout recalculates panel dimensions.
func (m *AppModel) updateLayout() {
	m.layout = components.ComputeLayout(m.width, m.height, m.showSidebar, m.sidebarWidth)

	// Update component widths
	m.headerComp.SetWidth(m.layout.ContentW)
	m.statusBarComp.SetWidth(m.layout.ContentW)
	m.msgRenderer.SetWidth(m.layout.ContentW)
	if m.phaseStepper != nil {
		m.phaseStepper.SetWidth(m.layout.ContentW)
	}

	if m.showSidebar {
		m.sidebarComp.SetSize(m.layout.SidebarW, m.layout.Height-m.layout.HeaderH-m.layout.StatusH)
	}

	// Update session picker size
	if m.sessionPicker != nil && m.sessionPicker.IsActive() {
		m.sessionPicker.SetSize(m.layout.ContentW-10, m.layout.ChatH-4)
	}

	// Update textarea
	m.textarea.SetWidth(m.layout.ContentW - 4)

	// Update viewport
	vpHeight := m.layout.ChatH
	if vpHeight < 5 {
		vpHeight = 5
	}
	m.viewport.Width = m.layout.ContentW - 4
	m.viewport.Height = vpHeight

	m.renderMessages()
}

func (m *AppModel) showToast(text string) {
	m.toastText = text
	m.toastExpiry = time.Now().Add(2 * time.Second)
}

func (m *AppModel) addMessage(role, content string) {
	m.addMessageFull(role, content, "", nil, nil, 0, 0, 0, "")
}

func (m *AppModel) addMessageWithThinking(role, content, thinking string) {
	m.addMessageFull(role, content, thinking, nil, nil, 0, 0, 0, "")
}

func (m *AppModel) addMessageFull(role, content, thinking string, thoughts, tools []string, inTokens, outTokens int, duration time.Duration, modelName string) {
	m.messages = append(m.messages, ChatMessage{
		Role:          role,
		Content:       content,
		Thinking:      thinking,
		Thoughts:      thoughts,
		ToolsExecuted: tools,
		Time:          time.Now(),
		InputTokens:   inTokens,
		OutputTokens:  outTokens,
		Duration:      duration,
		ModelName:     modelName,
	})
	m.renderMessages()
}

func (m *AppModel) renderMessages() {
	var sb strings.Builder
	sb.WriteString(bannerContent())

	mode := "ask"
	_, modelName := "", ""
	if m.supervisor != nil {
		mode = string(m.supervisor.GetMode())
		_, modelName = m.supervisor.GetModelInfo()
	}

	// Render historical messages using new message renderer
	for _, msg := range m.messages {
		thinking := msg.Thinking
		if !m.showThinking {
			thinking = ""
		}
		rendered := m.msgRenderer.RenderMessage(
			msg.Role, msg.Content, thinking,
			msg.Thoughts, msg.ToolsExecuted,
			msg.InputTokens, msg.OutputTokens, msg.Duration,
			mode, msg.ModelName, msg.ExpandedCode,
		)
		sb.WriteString(rendered)
		sb.WriteString("\n")
	}

	// Render current stream with live animation
	if m.processing || m.currentStream != "" || m.currentThinking != "" || m.currentExplore != "" {
		thinking := m.currentThinking
		if !m.showThinking {
			thinking = ""
		}
		elapsed := time.Since(m.streamStartTime)

		// Build phase info list for stepper from observed phases
		var phaseInfos []components.PhaseInfo
		seenActive := false
		for _, p := range m.phaseSeen {
			completed := false
			active := false
			if m.currentPhase == p {
				active = true
				seenActive = true
			} else if seenActive {
				completed = true
			}
			phaseInfos = append(phaseInfos, components.PhaseInfo{
				Name:        p,
				Description: m.phaseDescs[p],
				Active:      active,
				Completed:   completed,
				Content:     m.phaseContents[p],
				HasContent:  m.phaseContents[p] != "" || active || completed,
			})
		}

		// Fade-wave text for Generating phase
		var fadeText string
		if m.currentPhase == "Generating" && m.fadeWave != nil {
			fadeText = m.fadeWave.Render()
		}

		rendered := m.msgRenderer.RenderStream(m.currentStream, thinking, mode, elapsed, m.dotFrame, m.cursorVisible, modelName, phaseInfos, fadeText)
		sb.WriteString(rendered)
	}

	m.viewport.SetContent(sb.String())
	m.viewport.GotoBottom()
}

func (m *AppModel) handleCommand(cmd string, args []string) {
	switch cmd {
	case "help":
		m.addMessage("System", `Perintah tersedia:
  [Tab]              — Ganti mode agen (cycle: ask → rush → plan → test)
  [F2]               — Ganti tampilan (Chat → Node Graph → Help)
  [F3]               — Buka session picker
  /mode [ask|rush|plan|test] — Ganti mode agen
  /model [provider] [model] — Ganti LLM provider/model
  /skill [run <nama>] — Lihat atau jalankan skill tersimpan
  /help              — Tampilkan bantuan ini
  /expand [n]        — Toggle code blocks pesan Agent ke-n dari bawah (default: 1)
  /memory            — Lihat memori tersimpan
  /mcp               — Lihat MCP servers dan tools
  /mcp add <name> local <cmd> [args...] — Hubungkan MCP local
  /mcp add <name> remote <url> — Hubungkan MCP remote
  /mcp remove <name> — Putuskan dan hapus MCP server
  /session [list|new|info|switch|end|delete|search] — Kelola sessions
  /session new <nama> [--carry-over=N] — Buat session baru (bawa N turn terakhir)
  /session search <query> — Cari session dengan AI embedding
  /clear             — Bersihkan layar
  /ssh               — Lihat perintah SSH management
  /nudge             — Lihat nudge/reminder tertunda
  /remind <teks>     — Buat reminder nudge manual
  ssh user@host [cmd] — Eksekusi SSH langsung dari prompt
  exit               — Keluar dari Smara`)
	case "expand":
		// Toggle code block expansion for a specific agent message (default: latest)
		n := 1
		if len(args) > 0 {
			if parsed, err := strconv.Atoi(args[0]); err == nil && parsed > 0 {
				n = parsed
			}
		}
		found := 0
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].Role == "Agent" {
				found++
				if found == n {
					m.messages[i].ExpandedCode = !m.messages[i].ExpandedCode
					status := "collapsed"
					if m.messages[i].ExpandedCode {
						status = "expanded"
					}
					m.addMessage("System", fmt.Sprintf("Code blocks %s for agent message #%d.", status, n))
					break
				}
			}
		}
		if found == 0 {
			m.addMessage("System", "No agent messages to expand.")
		} else if found < n {
			m.addMessage("System", fmt.Sprintf("Hanya ditemukan %d pesan Agent (requested #%d).", found, n))
		}
		m.renderMessages()
	case "clear":
		m.messages = []ChatMessage{}
		m.renderMessages()
	case "workflow":
		m.addMessage("System", "Workflow management tersedia via CLI: smara workflow <list|resume>")
	case "ssh":
		if len(args) == 0 {
			m.addMessage("System", `Perintah SSH:
  /ssh list              — Daftar host tersimpan
  /ssh add <name>        — Tambah host (gunakan UI config)
  ssh user@host [cmd]    — Eksekusi SSH langsung dari prompt`)
		} else {
			m.addMessage("System", fmt.Sprintf("SSH command: %s %s", args[0], strings.Join(args[1:], " ")))
		}
	case "nudge":
		if dbStore, ok := m.store.(*memory.SQLiteStore); ok {
			if nudges, err := nudge.GetAllPending(dbStore.DB()); err == nil && len(nudges) > 0 {
				m.addMessage("System", nudge.FormatNudges(nudges))
			} else {
				m.addMessage("System", "Tidak ada nudge/reminder tertunda.")
			}
		} else {
			m.addMessage("System", "Nudge tidak tersedia (DB belum siap).")
		}
	case "remind":
		if len(args) == 0 {
			m.addMessage("System", "Gunakan: /remind <teks reminder> [when: hourly/daily at HH:MM/every N minutes]")
			return
		}
		text := strings.Join(args, " ")
		when := ""
		if idx := strings.Index(text, " when:"); idx >= 0 {
			when = strings.TrimSpace(text[idx+6:])
			text = strings.TrimSpace(text[:idx])
		}
		if when == "" {
			when = "hourly"
		}
		if dbStore, ok := m.store.(*memory.SQLiteStore); ok {
			_, err := nudge.CreateSchedule(dbStore.DB(), text, when, nil)
			if err == nil {
				m.addMessage("System", fmt.Sprintf("Reminder ditambahkan: '%s' (jadwal: %s)", text, when))
			} else {
				m.addMessage("System", fmt.Sprintf("Gagal menambahkan reminder: %v", err))
			}
		} else {
			m.addMessage("System", "Nudge tidak tersedia (DB belum siap).")
		}
	case "skill":
		if len(args) == 0 {
			names, err := skill.List()
			if err != nil {
				m.addMessage("System", fmt.Sprintf("Gagal list skill: %v", err))
				return
			}
			if len(names) == 0 {
				m.addMessage("System", "Belum ada skill tersimpan.\nGunakan: smara skill create <nama> untuk membuat skill baru.")
				return
			}
			var sb strings.Builder
			sb.WriteString("Skill tersimpan:\n")
			for _, n := range names {
				sk, _ := skill.Load(n)
				if sk != nil {
					sb.WriteString(fmt.Sprintf("  - %s: %s (%d steps)\n", n, sk.Description, len(sk.Steps)))
				} else {
					sb.WriteString(fmt.Sprintf("  - %s\n", n))
				}
			}
			sb.WriteString("\nGunakan: /skill run <nama> untuk menjalankan.")
			m.addMessage("System", sb.String())
			return
		}
		subcmd := args[0]
		switch subcmd {
		case "run":
			if len(args) < 2 {
				m.addMessage("System", "Gunakan: /skill run <nama-skill>")
				return
			}
			name := args[1]
			sk, err := skill.Load(name)
			if err != nil {
				m.addMessage("System", fmt.Sprintf("Skill '%s' tidak ditemukan: %v", name, err))
				return
			}
			m.addMessage("System", fmt.Sprintf("Menjalankan skill: %s (%d steps)", sk.Name, len(sk.Steps)))
			if m.supervisor == nil {
				m.addMessage("System", "Supervisor tidak tersedia (LLM belum diinisialisasi).")
				return
			}
			executor := m.supervisor.SkillExecutor()
			result, err := sk.Run(executor)
			if err != nil {
				m.addMessage("System", fmt.Sprintf("Skill gagal: %v", err))
				return
			}
			var sb strings.Builder
			if result.Success {
				sb.WriteString(fmt.Sprintf("Skill '%s' berhasil!\n", result.SkillName))
			} else {
				sb.WriteString(fmt.Sprintf("Skill '%s' gagal pada step:\n", result.SkillName))
			}
			for i, sr := range result.StepResults {
				status := "OK"
				if sr.Error != nil {
					status = fmt.Sprintf("ERROR: %v", sr.Error)
				}
				out := sr.Output
				if len(out) > 100 {
					out = out[:100] + "..."
				}
				sb.WriteString(fmt.Sprintf("  Step %d: %s → %s\n    Output: %s\n", i+1, sr.Tool, status, out))
			}
			m.addMessage("System", sb.String())
		default:
			m.addMessage("System", fmt.Sprintf("Subcommand tidak dikenal: %s. Gunakan 'run <nama>'.", subcmd))
		}
	case "mcp":
		if len(args) == 0 {
			// List connected servers
			mcpInfo := m.supervisor.GetMCPInfo()
			if len(mcpInfo) == 0 {
				m.addMessage("System", "Belum ada MCP server yang terhubung.")
				return
			}
			var msgParts []string
			for name, info := range mcpInfo {
				status := "connected"
				if !info.Connected {
					status = "error"
				}
				msgParts = append(msgParts, fmt.Sprintf("%s — %s", name, status))
				if len(info.Tools) > 0 {
					for _, tool := range info.Tools {
						desc := tool.Description
						if len(desc) > 60 {
							desc = desc[:60] + "..."
						}
						msgParts = append(msgParts, fmt.Sprintf("  ├── %s: %s", tool.Name, desc))
					}
				} else if info.Error != "" {
					msgParts = append(msgParts, fmt.Sprintf("  └── Error: %s", info.Error))
				}
			}
			m.addMessage("System", "MCP Servers:\n"+strings.Join(msgParts, "\n"))
			return
		}
		subcmd := args[0]
		switch subcmd {
		case "add":
			if len(args) < 3 {
				m.addMessage("System", "Gunakan: /mcp add <name> local <command> [args...]\n        /mcp add <name> remote <url>")
				return
			}
			name := args[1]
			mcpType := args[2]
			if mcpType != "local" && mcpType != "remote" {
				m.addMessage("System", "Type harus 'local' atau 'remote'")
				return
			}
			if m.supervisor == nil {
				m.addMessage("System", "Supervisor tidak tersedia.")
				return
			}
			// Use connect_mcp tool through supervisor executor
			toolArgs := map[string]interface{}{
				"name": name,
				"type": mcpType,
			}
			if mcpType == "local" {
				if len(args) < 4 {
					m.addMessage("System", "Gunakan: /mcp add <name> local <command> [args...]")
					return
				}
				toolArgs["command"] = args[3]
				if len(args) > 4 {
					toolArgs["args"] = args[4:]
				}
			} else {
				if len(args) < 4 {
					m.addMessage("System", "Gunakan: /mcp add <name> remote <url>")
					return
				}
				toolArgs["url"] = args[3]
			}
			result, err := m.supervisor.SkillExecutor()("connect_mcp", toolArgs)
			if err != nil {
				m.addMessage("System", fmt.Sprintf("Gagal menghubungkan MCP '%s': %v", name, err))
				return
			}
			m.addMessage("System", result)
		case "remove":
			if len(args) < 2 {
				m.addMessage("System", "Gunakan: /mcp remove <name>")
				return
			}
			name := args[1]
			if m.supervisor == nil {
				m.addMessage("System", "Supervisor tidak tersedia.")
				return
			}
			result, err := m.supervisor.SkillExecutor()("disconnect_mcp", map[string]interface{}{"name": name})
			if err != nil {
				m.addMessage("System", fmt.Sprintf("Gagal memutuskan MCP '%s': %v", name, err))
				return
			}
			m.addMessage("System", result)
		default:
			m.addMessage("System", fmt.Sprintf("Subcommand tidak dikenal: %s. Gunakan 'add' atau 'remove'.", subcmd))
		}
	default:
		if m.onCommand != nil {
			m.onCommand(cmd, args)
		} else {
			m.addMessage("System", fmt.Sprintf("Mengeksekusi perintah: /%s %s", cmd, strings.Join(args, " ")))
		}
	}
}

// buildSidebarData collects data for the sidebar.
func (m *AppModel) buildSidebarData() components.SidebarData {
	data := components.SidebarData{
		Messages: len(m.messages),
		Mode:     "ask",
	}

	if m.supervisor != nil {
		data.Mode = string(m.supervisor.GetMode())
		provider, model := m.supervisor.GetModelInfo()
		data.Provider = provider
		data.Model = model
	}

	// Count tokens from last agent message
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Role == "Agent" {
			data.InTokens += m.messages[i].InputTokens
			data.OutTokens += m.messages[i].OutputTokens
			if m.messages[i].Duration > 0 {
				data.Elapsed = m.messages[i].Duration
			}
			break
		}
	}

	return data
}

// View renders the UI
func (m AppModel) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	mode := "ask"
	provider, modelName := "", ""
	if m.supervisor != nil {
		mode = string(m.supervisor.GetMode())
		provider, modelName = m.supervisor.GetModelInfo()
	}

	// ─── Node Graph View ──────────────────────────────────────
	if m.currentView == viewNodeGraph {
		header := m.headerComp.Render(mode, provider, modelName, m.processing, m.spinner.View(), m.statusText)
		ngContent := m.nodeGraph.View()
		statusBar := m.statusBarComp.Render(components.StatusContext{
			Mode:       mode,
			Provider:   provider,
			Model:      modelName,
			Processing: m.processing,
		})
		var sidebar string
		if m.layout.SidebarW > 0 {
			m.sidebarComp.SetSize(m.layout.SidebarW, m.layout.Height-m.layout.HeaderH-m.layout.StatusH)
			sidebar = m.sidebarComp.Render(m.buildSidebarData())
		}
		mainColumn := fmt.Sprintf("%s\n%s\n%s", header, ngContent, statusBar)
		if m.layout.SidebarW > 0 {
			return components.JoinHorizontal(mainColumn, sidebar, m.layout.ContentW)
		}
		return mainColumn
	}

	// ─── Help View ────────────────────────────────────────────
	if m.currentView == viewHelp {
		header := m.headerComp.Render(mode, provider, modelName, m.processing, m.spinner.View(), m.statusText)
		helpContent := m.helpOverlay.Render()
		helpCentered := m.helpOverlay.Center(helpContent, m.width, m.height-m.layout.HeaderH-m.layout.StatusH)
		statusBar := m.statusBarComp.Render(components.StatusContext{
			Mode:       mode,
			Provider:   provider,
			Model:      modelName,
			Processing: m.processing,
		})
		var sidebar string
		if m.layout.SidebarW > 0 {
			m.sidebarComp.SetSize(m.layout.SidebarW, m.layout.Height-m.layout.HeaderH-m.layout.StatusH)
			sidebar = m.sidebarComp.Render(m.buildSidebarData())
		}
		mainColumn := fmt.Sprintf("%s\n%s\n%s", header, helpCentered, statusBar)
		if m.layout.SidebarW > 0 {
			return components.JoinHorizontal(mainColumn, sidebar, m.layout.ContentW)
		}
		return mainColumn
	}

	// ─── Header ────────────────────────────────────────────────
	header := m.headerComp.Render(mode, provider, modelName, m.processing, m.spinner.View(), m.statusText)

	// ─── Chat Area ─────────────────────────────────────────────
	chatContent := m.viewport.View()

	// ─── Input Area ────────────────────────────────────────────
	var inputArea string
	if m.awaitingConfirmation {
		yaStyle := lipgloss.NewStyle().Padding(0, 1)
		tidakStyle := lipgloss.NewStyle().Padding(0, 1)

		if m.confirmSelection == 0 {
			yaStyle = yaStyle.Background(lipgloss.Color("#bef264")).Foreground(lipgloss.Color("#09090b")).Bold(true)
			tidakStyle = tidakStyle.Foreground(lipgloss.Color("#71717a"))
		} else {
			yaStyle = yaStyle.Foreground(lipgloss.Color("#71717a"))
			tidakStyle = tidakStyle.Background(lipgloss.Color("#fda4af")).Foreground(lipgloss.Color("#09090b")).Bold(true)
		}

		confirmPrompt := warnStyle.Render("➤ Lanjutkan eksekusi?")
		inputArea = fmt.Sprintf("\n  %s\n  %s    %s\n  %s",
			confirmPrompt,
			yaStyle.Render("[ Ya ]"),
			tidakStyle.Render("[ Tidak ]"),
			dimStyle.Render("(Gunakan panah Kiri/Kanan dan tekan Enter)"),
		)
	} else {
		inputArea = m.textarea.View()
	}

	// ─── Status Bar ────────────────────────────────────────────
	totalTokens := 0
	inputTokens := 0
	outputTokens := 0
	for _, msg := range m.messages {
		inputTokens += msg.InputTokens
		outputTokens += msg.OutputTokens
		totalTokens += msg.InputTokens + msg.OutputTokens
	}
	statusBar := m.statusBarComp.Render(components.StatusContext{
		Mode:         mode,
		Provider:     provider,
		Model:        modelName,
		TokenCount:   totalTokens,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Processing:   m.processing,
	})
	// ─── Sidebar ───────────────────────────────────────────────
	var sidebar string
	if m.layout.SidebarW > 0 {
		m.sidebarComp.SetSize(m.layout.SidebarW, m.layout.Height-m.layout.HeaderH-m.layout.StatusH)
		sidebarData := m.buildSidebarData()
		sidebar = m.sidebarComp.Render(sidebarData)
	}

	// ─── Combine Main Column ───────────────────────────────────
	mainColumn := fmt.Sprintf("%s\n%s\n%s\n%s",
		header,
		chatContent,
		inputArea,
		statusBar,
	)

	// ─── Toast Notification ────────────────────────────────────
	if m.toastText != "" && time.Now().Before(m.toastExpiry) {
		toastStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#bef264")).
			Background(lipgloss.Color("#1a1a1a")).
			Bold(true).
			Padding(0, 1).
			MarginLeft(m.layout.ContentW - len(m.toastText) - 4)
		toast := toastStyle.Render("📋 " + m.toastText)
		mainColumn = mainColumn + "\n" + toast
	}

	// ─── Session Picker Overlay ────────────────────────────────
	if m.sessionPicker.IsActive() {
		pickerContent := m.sessionPicker.Render()
		pickerOverlay := m.sessionPicker.Center(pickerContent, m.width, m.height)
		// Overlay picker on top of main column by replacing lines in the center
		mainLines := strings.Split(mainColumn, "\n")
		pickerLines := strings.Split(pickerOverlay, "\n")
		pickerH := len(pickerLines)
		startY := (len(mainLines) - pickerH) / 2
		if startY < 0 {
			startY = 0
		}
		for i, pLine := range pickerLines {
			idx := startY + i
			if idx < len(mainLines) {
				mainLines[idx] = pLine
			} else {
				mainLines = append(mainLines, pLine)
			}
		}
		mainColumn = strings.Join(mainLines, "\n")
	}

	// ─── Final Layout ──────────────────────────────────────────
	if m.layout.SidebarW > 0 {
		// Join main column + sidebar horizontally
		return components.JoinHorizontal(mainColumn, sidebar, m.layout.ContentW)
	}

	return mainColumn
}

// ─── Programmatic message injection ───────────────────────────

// InjectLog sends a log message to the TUI.
func InjectLog(role, content string) {
	if globalProgram != nil {
		go globalProgram.Send(LogMsg{
			Message: ChatMessage{
				Role:    role,
				Content: content,
				Time:    time.Now(),
			},
		})
	} else {
		fmt.Printf("[%s] %s\n", role, content)
	}
}

// TUI-compatible Print overrides

// TUIPrintInfo replaces the standard PrintInfo when using TUI
func TUIPrintInfo(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	InjectLog("System", msg)
}

// TUIPrintSuccess replaces the standard PrintSuccess when using TUI
func TUIPrintSuccess(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	InjectLog("System", "✓ "+msg)
}

// TUIPrintWarning replaces the standard PrintWarning when using TUI
func TUIPrintWarning(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	InjectLog("System", "⚠ "+msg)
}

// TUIPrintError replaces the standard PrintError when using TUI
func TUIPrintError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	InjectLog("System", "Error: "+msg)
}

// processFileMentions searches for @filename in the prompt and injects file content
func (m *AppModel) processFileMentions(prompt string) string {
	// First pass: extract [image:/path] references and surface them as
	// system attachments so the agent knows an image is in scope.
	prompt = m.processImageRefs(prompt)

	re := regexp.MustCompile(`@([\w\.\/\-]+)`)
	matches := re.FindAllStringSubmatch(prompt, -1)
	if len(matches) == 0 {
		return prompt
	}

	var contextBuilder strings.Builder
	hasAddedFiles := false

	for _, match := range matches {
		filePath := match[1]
		content, err := os.ReadFile(filePath)
		if err != nil {
			m.messages = append(m.messages, ChatMessage{
				Role:    "System",
				Content: fmt.Sprintf("⚠ Gagal membaca file @%s: %v", filePath, err),
				Time:    time.Now(),
			})
			continue
		}

		if !hasAddedFiles {
			contextBuilder.WriteString("Konteks dari file yang direferensikan:\n\n")
			hasAddedFiles = true
		}

		m.messages = append(m.messages, ChatMessage{
			Role:    "System",
			Content: fmt.Sprintf("📎 Menyertakan isi file @%s (%d bytes)", filePath, len(content)),
			Time:    time.Now(),
		})

		contextBuilder.WriteString(fmt.Sprintf("--- FILE: %s ---\n", filePath))
		contextBuilder.WriteString(string(content))
		contextBuilder.WriteString("\n\n")
	}

	if !hasAddedFiles {
		return prompt
	}

	m.renderMessages()
	return contextBuilder.String() + "\nPrompt User:\n" + prompt
}

// processImageRefs scans for [image:/path/to/file.png] tokens in the prompt.
// For each match, it surfaces a system-level attachment notice in the chat
// and appends a normalized hint to the prompt the agent receives so the
// model knows there is an image referenced — without trying to inline its
// bytes (TUI prompts go through the LLM in plain text only).
//
// If/when the active provider supports vision messages, the supervisor can
// upgrade the [image:...] tokens into multimodal content blocks. For now,
// the path stays in the prompt and built-in tools (e.g. read_file or an
// OCR tool) can pick it up.
func (m *AppModel) processImageRefs(prompt string) string {
	re := regexp.MustCompile(`\[image:([^\]]+)\]`)
	matches := re.FindAllStringSubmatch(prompt, -1)
	if len(matches) == 0 {
		return prompt
	}
	for _, match := range matches {
		path := strings.TrimSpace(match[1])
		st, err := os.Stat(path)
		if err != nil {
			m.messages = append(m.messages, ChatMessage{
				Role:    "System",
				Content: fmt.Sprintf("⚠ Image tidak ditemukan: %s (%v)", path, err),
				Time:    time.Now(),
			})
			continue
		}
		m.messages = append(m.messages, ChatMessage{
			Role:    "System",
			Content: fmt.Sprintf("🖼  Image attached: %s (%d KB)", path, st.Size()/1024),
			Time:    time.Now(),
		})
	}
	m.renderMessages()
	return prompt
}

// SetGlobalProgram sets the global program for log injection
func SetGlobalProgram(p *tea.Program) {
	globalProgram = p
}

// GetGlobalProgram returns the global program
func GetGlobalProgram() *tea.Program {
	return globalProgram
}

// NewProgram creates a new bubbletea program
func NewProgram(m AppModel) *tea.Program {
	return tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
}

// LoadHistory injects previous history into the model
func (m *AppModel) LoadHistory(history []struct{ Role, Content string }) {
	for _, h := range history {
		role := "User"
		if h.Role == "assistant" {
			role = "Agent"
		}
		m.messages = append(m.messages, ChatMessage{
			Role:    role,
			Content: h.Content,
			Time:    time.Now(),
		})
	}
	m.renderMessages()
}

// isDirectSSHCommand detects raw SSH commands typed in the prompt.
func isDirectSSHCommand(input string) bool {
	input = strings.TrimSpace(input)
	// Match patterns like "ssh user@host", "ssh -i /path/key.pem user@host", "ssh -p 2222 user@host"
	if !strings.HasPrefix(input, "ssh ") {
		return false
	}
	// Ensure it contains user@host pattern somewhere
	parts := strings.Fields(input)
	for _, p := range parts {
		if strings.Contains(p, "@") {
			return true
		}
	}
	return false
}

// handleDirectSSH parses and executes a raw SSH command from the prompt.
func (m *AppModel) handleDirectSSH(input string) (string, error) {
	// Parse: ssh [-i key] [-p port] [-o ...] user@host [command...]
	parts := strings.Fields(input)
	var keyPath, port, userAtHost string
	var remoteCommandParts []string
	var afterHost bool

	for i := 1; i < len(parts); i++ {
		p := parts[i]
		if afterHost {
			remoteCommandParts = append(remoteCommandParts, p)
			continue
		}
		if p == "-i" && i+1 < len(parts) {
			keyPath = parts[i+1]
			i++
			continue
		}
		if p == "-p" && i+1 < len(parts) {
			port = parts[i+1]
			i++
			continue
		}
		if strings.HasPrefix(p, "-") {
			// Skip other ssh options
			continue
		}
		if strings.Contains(p, "@") {
			userAtHost = p
			afterHost = true
			continue
		}
	}

	if userAtHost == "" {
		return "", fmt.Errorf("format SSH tidak valid. Gunakan: ssh [-i key] user@host [command]")
	}

	ua := strings.SplitN(userAtHost, "@", 2)
	user := ua[0]
	address := ua[1]

	host := &smarassh.Host{
		Name:    userAtHost,
		User:    user,
		Address: address,
		Port:    "22",
		KeyPath: keyPath,
	}
	if port != "" {
		host.Port = port
	}

	client, err := smarassh.Connect(host)
	if err != nil {
		return "", fmt.Errorf("gagal koneksi: %w", err)
	}
	defer client.Close()

	// Auto-save host config for future sessions
	_ = smarassh.SaveHost(*host)

	if len(remoteCommandParts) == 0 {
		return fmt.Sprintf("Berhasil terhubung ke %s@%s:%s (sesi interaktif belum tersedia dari prompt, gunakan /ssh connect %s)", user, address, host.Port, userAtHost), nil
	}

	remoteCmd := strings.Join(remoteCommandParts, " ")
	stdout, stderr, err := client.Exec(remoteCmd)
	var sb strings.Builder
	if stdout != "" {
		sb.WriteString(stdout)
	}
	if stderr != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("stderr: " + stderr)
	}
	if err != nil {
		if sb.Len() > 0 {
			return sb.String(), nil // return output even on error
		}
		return "", err
	}
	if sb.Len() == 0 {
		return "Perintah berhasil dieksekusi tanpa output.", nil
	}
	return sb.String(), nil
}
