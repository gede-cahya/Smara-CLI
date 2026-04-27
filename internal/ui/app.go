package ui

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/ui/components"
)

// ═══════════════════════════════════════════════════════════════
// Smara CLI TUI App — Interactive Multi-Panel Terminal UI
// ═══════════════════════════════════════════════════════════════

// Style definitions (kept for backward compat, use theme now)
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			PaddingLeft(2).
			PaddingRight(2)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575"))

	warnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F3C623"))

	errStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF3366"))

	agentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#36C5F0")).
			Bold(true)

	userStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E2E2E2")).
			Bold(true)

	messageStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E2E2E2"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#767676"))

	thinkingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Italic(true).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(lipgloss.Color("#444444")).
			PaddingLeft(1).
			MarginLeft(1)

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#3C3C3C"))

	terminalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A6E22E")).
			Bold(true)

	codingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D1D1D1")).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(lipgloss.Color("#3A3A3A")).
			PaddingLeft(1).
			MarginLeft(1)

	codePrefixStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#36C5F0")).
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
}

// Supervisor interface to avoid circular dependency
type AppSupervisor interface {
	ProcessPrompt(ctx context.Context, prompt string) (*agent.PromptResult, error)
	GetMode() agent.Mode
	SetMode(mode agent.Mode)
	GetModelInfo() (string, string)
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
	showHelp      bool
	showPalette   bool
	showThinking  bool // toggle thinking visibility
}

// InitialModel creates a new model
func InitialModel(sup AppSupervisor, onCmd func(cmd string, args []string)) AppModel {
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
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

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
		sidebarWidth:    28,
		// ─── NEW ─────────────────────────────────────────────────
		theme:         theme,
		headerComp:    components.NewHeader(80),
		sidebarComp:   components.NewSidebar(28, 20),
		statusBarComp: components.NewStatusBar(80),
		msgRenderer:   components.NewMessageRenderer(80),
		helpOverlay:   components.NewHelpOverlay(60),
		palette:       components.NewCommandPalette(50),
		showThinking:  true,
	}
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
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true).Render(banner) +
		"\n" + dimStyle.Render("  स्मृति — Autonomous Multi-Agent Terminal v1.8.0\n  Ketik /help untuk daftar perintah.\n")
}

// Init initializes the app
func (m AppModel) Init() tea.Cmd {
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

// Update handles messages and state changes
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
		cmds  []tea.Cmd
	)

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

		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			if m.processing {
				m.cancel() // Cancel the current process
				m.ctx, m.cancel = context.WithCancel(context.Background())
				m.processing = false
				m.addMessage("System", "Proses dibatalkan.")
			} else {
				return m, tea.Quit
			}

		case tea.KeyCtrlD:
			return m, tea.Quit

		case tea.KeyCtrlB:
			// Toggle sidebar
			m.showSidebar = !m.showSidebar
			m.updateLayout()
			m.renderMessages()
			return m, nil

		case tea.KeyCtrlT:
			// Toggle thinking visibility
			m.showThinking = !m.showThinking
			m.renderMessages()
			return m, nil

		case tea.KeyCtrlP:
			// Toggle command palette
			m.palette.Toggle()
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
			// Cycle modes
			if m.supervisor != nil {
				currentMode := m.supervisor.GetMode()
				var nextMode agent.Mode
				switch currentMode {
				case "ask":
					nextMode = "rush"
				case "rush":
					nextMode = "plan"
				case "plan":
					nextMode = "test"
				case "test":
					nextMode = "ask"
				default:
					nextMode = "ask"
				}
				m.supervisor.SetMode(nextMode)
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
				return m, tea.Quit
			}

			m.addMessage("User", v)

			if IsCommand(v) {
				// Handle command immediately and add to view
				cmdName, cmdArgs := ParseCommand(v)
				m.handleCommand(cmdName, cmdArgs)
			} else {
				// Process @mentions
				processedPrompt := m.processFileMentions(v)

				// Send to supervisor
				m.processing = true
				m.statusText = "Memproses..."
				m.currentStream = ""
				m.currentThinking = ""
				sup := m.supervisor
				ctx := m.ctx

				cmds = append(cmds, m.spinner.Tick)
				cmds = append(cmds, func() tea.Msg {
					result, err := sup.ProcessPrompt(ctx, processedPrompt)
					return ProcessMsg{Result: result, Err: err}
				})
			}
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case StreamMsg:
		if msg.IsThinking {
			m.currentThinking += msg.Chunk
		} else {
			m.currentStream += msg.Chunk
		}
		m.renderMessages()

	case ExploreMsg:
		m.currentExplore = msg.Content
		m.renderMessages()

	case ProcessMsg:
		m.processing = false
		m.statusText = ""
		m.currentStream = ""
		m.currentThinking = ""
		m.currentExplore = ""
		if msg.Err != nil {
			if msg.Err.Error() == "context canceled" {
				// Already handled in KeyCtrlC
			} else {
				m.addMessage("System", fmt.Sprintf("Error: %v", msg.Err))
			}
		} else {
			// Intercept the "Lanjutkan eksekusi? (ya/tidak)" message
			if strings.Contains(msg.Result.Response, "Lanjutkan eksekusi? (ya/tidak)") {
				// Extract everything before the prompt, if any
				cleanResp := strings.ReplaceAll(msg.Result.Response, "Lanjutkan eksekusi? (ya/tidak)", "")
				cleanResp = strings.TrimSpace(cleanResp)

				if cleanResp != "" {
					m.addMessageFull("Agent", cleanResp, msg.Result.Thinking, msg.Result.Thoughts, msg.Result.ToolsExecuted, msg.Result.InputTokens, msg.Result.OutputTokens, msg.Result.Duration)
				} else if msg.Result.Thinking != "" {
					m.addMessageFull("Agent", "", msg.Result.Thinking, msg.Result.Thoughts, msg.Result.ToolsExecuted, msg.Result.InputTokens, msg.Result.OutputTokens, msg.Result.Duration)
				}

				m.awaitingConfirmation = true
				m.confirmSelection = 0 // Default "Ya"
			} else {
				m.addMessageFull("Agent", msg.Result.Response, msg.Result.Thinking, msg.Result.Thoughts, msg.Result.ToolsExecuted, msg.Result.InputTokens, msg.Result.OutputTokens, msg.Result.Duration)
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
	m.layout = components.ComputeLayout(m.width, m.height, m.showSidebar)

	// Update component widths
	m.headerComp.SetWidth(m.layout.ContentW)
	m.statusBarComp.SetWidth(m.layout.ContentW)
	m.msgRenderer.SetWidth(m.layout.ContentW)

	if m.showSidebar {
		m.sidebarComp.SetSize(m.layout.SidebarW, m.layout.Height-m.layout.HeaderH-m.layout.StatusH)
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

func (m *AppModel) addMessage(role, content string) {
	m.addMessageFull(role, content, "", nil, nil, 0, 0, 0)
}

func (m *AppModel) addMessageWithThinking(role, content, thinking string) {
	m.addMessageFull(role, content, thinking, nil, nil, 0, 0, 0)
}

func (m *AppModel) addMessageFull(role, content, thinking string, thoughts, tools []string, inTokens, outTokens int, duration time.Duration) {
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
	})
	m.renderMessages()
}

func (m *AppModel) renderMessages() {
	var sb strings.Builder
	sb.WriteString(bannerContent())

	mode := "ask"
	if m.supervisor != nil {
		mode = string(m.supervisor.GetMode())
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
			mode,
		)
		sb.WriteString(rendered)
		sb.WriteString("\n")
	}

	// Render current stream
	if m.currentStream != "" || m.currentThinking != "" || m.currentExplore != "" {
		thinking := m.currentThinking
		if !m.showThinking {
			thinking = ""
		}
		rendered := m.msgRenderer.RenderStream(m.currentStream, thinking, mode)
		sb.WriteString(rendered)
	}

	m.viewport.SetContent(sb.String())
	m.viewport.GotoBottom()
}

func (m *AppModel) handleCommand(cmd string, args []string) {
	switch cmd {
	case "help":
		m.addMessage("System", `Perintah tersedia:
  [Tab]              — Ganti mode agen (cycle: ask → rush → plan)
  /mode [ask|rush|plan] — Ganti mode agen
  /model [provider] [model] — Ganti LLM provider/model
  /help              — Tampilkan bantuan ini
  /memory            — Lihat memori tersimpan
  /mcp               — Lihat MCP servers dan tools
  /session [list|new|info|switch|end] — Kelola sessions
  /clear             — Bersihkan layar
  exit               — Keluar dari Smara`)
	case "clear":
		m.messages = []ChatMessage{}
		m.renderMessages()
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
			yaStyle = yaStyle.Background(lipgloss.Color("#04B575")).Foreground(lipgloss.Color("#FAFAFA")).Bold(true)
			tidakStyle = tidakStyle.Foreground(lipgloss.Color("#767676"))
		} else {
			yaStyle = yaStyle.Foreground(lipgloss.Color("#767676"))
			tidakStyle = tidakStyle.Background(lipgloss.Color("#FF3366")).Foreground(lipgloss.Color("#FAFAFA")).Bold(true)
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
	for _, msg := range m.messages {
		totalTokens += msg.InputTokens + msg.OutputTokens
	}
	statusBar := m.statusBarComp.Render(components.StatusContext{
		Mode:       mode,
		Provider:   provider,
		Model:      modelName,
		TokenCount: totalTokens,
		Processing: m.processing,
	})

	// ─── Sidebar ───────────────────────────────────────────────
	var sidebar string
	if m.showSidebar && m.layout.SidebarW > 0 {
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

	// ─── Final Layout ──────────────────────────────────────────
	if m.showSidebar && m.layout.SidebarW > 0 {
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
