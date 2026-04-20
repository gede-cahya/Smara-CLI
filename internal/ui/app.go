package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gede-cahya/Smara-CLI/internal/agent"
)

// Style definitions
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
)

// Global reference for programmatic messaging
var globalProgram *tea.Program

// ChatMessage represents a single message in the UI
type ChatMessage struct {
	Role     string // "System", "User", "Agent"
	Content  string
	Thinking string
	Time     time.Time
}

// Supervisor interface to avoid circular dependency
type AppSupervisor interface {
	ProcessPrompt(ctx context.Context, prompt string) (string, string, error)
	GetMode() agent.Mode
	SetMode(mode agent.Mode)
}

// AppModel is the Bubbletea model for our TUI
type AppModel struct {
	viewport    viewport.Model
	textarea    textarea.Model
	messages    []ChatMessage
	err         error
	width       int
	height      int
	supervisor  AppSupervisor
	ctx         context.Context
	cancel      context.CancelFunc
	processing  bool
	
	// History management
	cmdHistory  []string
	historyIdx  int

	// Command handler callback
	onCommand   func(cmd string, args []string)
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

	ctx, cancel := context.WithCancel(context.Background())

	return AppModel{
		textarea:   ta,
		viewport:   vp,
		messages:   []ChatMessage{},
		supervisor: sup,
		ctx:        ctx,
		cancel:     cancel,
		cmdHistory: []string{},
		historyIdx: -1,
		onCommand:  onCmd,
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
		"\n" + dimStyle.Render("  स्मृति — Autonomous Multi-Agent Terminal v1.3.0\n  Ketik /help untuk daftar perintah.\n")
}

// Init initializes the app
func (m AppModel) Init() tea.Cmd {
	return textarea.Blink
}

// ProcessMsg is sent when the supervisor finishes processing
type ProcessMsg struct {
	Response string
	Thinking string
	Err      error
}

// LogMsg allows external systems to inject messages into the UI
type LogMsg struct {
	Message ChatMessage
}

// Update handles messages and state changes
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
		cmds  []tea.Cmd
	)

	m.textarea, tiCmd = m.textarea.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, tiCmd, vpCmd)

	switch msg := msg.(type) {
	case tea.KeyMsg:
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

		case tea.KeyTab:
			// Cycle modes
			if m.supervisor != nil {
				currentMode := m.supervisor.GetMode()
				var nextMode agent.Mode
				switch currentMode {
				case "ask": nextMode = "rush"
				case "rush": nextMode = "plan"
				case "plan": nextMode = "ask"
				default: nextMode = "ask"
				}
				m.supervisor.SetMode(nextMode)
				m.addMessage("System", fmt.Sprintf("Mode diubah menjadi: %s", nextMode))
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
				// Send to supervisor
				m.processing = true
				sup := m.supervisor
				ctx := m.ctx
				
				cmds = append(cmds, func() tea.Msg {
					resp, thinking, err := sup.ProcessPrompt(ctx, v)
					return ProcessMsg{Response: resp, Thinking: thinking, Err: err}
				})
			}
		}

	case ProcessMsg:
		m.processing = false
		if msg.Err != nil {
			if msg.Err.Error() == "context canceled" {
				// Already handled in KeyCtrlC
			} else {
				m.addMessage("System", fmt.Sprintf("Error: %v", msg.Err))
			}
		} else {
			m.addMessageWithThinking("Agent", msg.Response, msg.Thinking)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		m.textarea.SetWidth(msg.Width - 4)
		
		vpHeight := msg.Height - m.textarea.Height() - 5 // Header + borders + input
		if vpHeight < 5 {
			vpHeight = 5
		}
		m.viewport.Width = msg.Width - 4
		m.viewport.Height = vpHeight
		m.renderMessages()
		
	case LogMsg:
		m.messages = append(m.messages, msg.Message)
		m.renderMessages()
	}

	return m, tea.Batch(cmds...)
}

func (m *AppModel) addMessage(role, content string) {
	m.addMessageWithThinking(role, content, "")
}

func (m *AppModel) addMessageWithThinking(role, content, thinking string) {
	m.messages = append(m.messages, ChatMessage{
		Role:     role,
		Content:  content,
		Thinking: thinking,
		Time:     time.Now(),
	})
	m.renderMessages()
}

func (m *AppModel) renderMessages() {
	var sb strings.Builder
	sb.WriteString(bannerContent())
	
	for _, msg := range m.messages {
		timeStr := dimStyle.Render(msg.Time.Format("15:04"))
		
		var prefix string
		var renderedContent string
		
		switch msg.Role {
		case "User":
			prefix = userStyle.Render("User:")
			renderedContent = messageStyle.Render(msg.Content)
		case "Agent":
			mode := "Agent"
			if m.supervisor != nil {
				mode = strings.ToUpper(string(m.supervisor.GetMode()))
			}
			prefix = agentStyle.Render(fmt.Sprintf("Smara [%s]:", mode))
			renderedContent = messageStyle.Render(msg.Content)
		case "System":
			if strings.HasPrefix(msg.Content, "Error") {
				prefix = errStyle.Render("System:")
				renderedContent = errStyle.Render(msg.Content)
			} else {
				prefix = infoStyle.Render("System:")
				renderedContent = dimStyle.Render(msg.Content)
			}
		}

		var thinkingContent string
		if msg.Thinking != "" {
			thinkingContent = thinkingStyle.Render(msg.Thinking) + "\n"
		}

		sb.WriteString(fmt.Sprintf("%s %s\n%s%s\n\n", timeStr, prefix, thinkingContent, renderedContent))
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

// View renders the UI
func (m AppModel) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	mode := "ASK"
	if m.supervisor != nil {
		mode = strings.ToUpper(string(m.supervisor.GetMode()))
	}
	
	header := titleStyle.Render(fmt.Sprintf(" Smara CLI - Mode: %s ", mode))
	if m.processing {
		header += " " + warnStyle.Render("Sedang memproses...")
	}
	
	// Create main layout
	return fmt.Sprintf(
		"%s\n%s\n%s",
		header,
		borderStyle.Render(m.viewport.View()),
		m.textarea.View(),
	)
}

// Programmatic message injection
func InjectLog(role, content string) {
	if globalProgram != nil {
		globalProgram.Send(LogMsg{
			Message: ChatMessage{
				Role:    role,
				Content: content,
				Time:    time.Now(),
			},
		})
	} else {
		// Fallback to normal print if TUI isn't running
		fmt.Printf("[%s] %s\n", role, content)
	}
}

// TUI-compatible Print overrides

// PrintInfo replaces the standard PrintInfo when using TUI
func TUIPrintInfo(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	InjectLog("System", msg)
}

// PrintSuccess replaces the standard PrintSuccess when using TUI
func TUIPrintSuccess(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	InjectLog("System", "✓ "+msg)
}

// PrintWarning replaces the standard PrintWarning when using TUI
func TUIPrintWarning(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	InjectLog("System", "⚠ "+msg)
}

// PrintError replaces the standard PrintError when using TUI
func TUIPrintError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	InjectLog("System", "Error: "+msg)
}

// SetGlobalProgram sets the global program for log injection
func SetGlobalProgram(p *tea.Program) {
	globalProgram = p
}

// NewProgram creates a new bubbletea program
func NewProgram(m AppModel) *tea.Program {
	// Use AltScreen so it feels like a full app
	return tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
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
			Time:    time.Now(), // we might not have the actual time, but that's fine
		})
	}
	m.renderMessages()
}
