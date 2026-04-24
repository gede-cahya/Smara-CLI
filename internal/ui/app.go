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
		showSidebar:     false,
		sidebarWidth:    30,
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
				// m.addMessage("System", fmt.Sprintf("Mode diubah menjadi: %s", nextMode)) // Removed to prevent viewport clutter
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

			// Detect if content is primarily code or tool output
			if strings.Contains(msg.Content, "```") || strings.Contains(msg.Content, "package ") || strings.Contains(msg.Content, "import ") {
				renderedContent = codingStyle.Render(msg.Content)
			} else {
				renderedContent = messageStyle.Render(msg.Content)
			}
		case "System":
			if strings.HasPrefix(msg.Content, "Error") {
				prefix = errStyle.Render("System:")
				renderedContent = errStyle.Render(msg.Content)
			} else {
				prefix = infoStyle.Render("System:")
				renderedContent = dimStyle.Render(msg.Content)
			}
		case "Terminal":
			prefix = terminalStyle.Render("$")
			// Terminal output is dimmed and bracketed like the reference image
			lines := strings.Split(msg.Content, "\n")
			var terminalRows []string
			for _, line := range lines {
				if line != "" {
					terminalRows = append(terminalRows, dimStyle.Render(line))
				}
			}
			renderedContent = strings.Join(terminalRows, "\n")
		}

		var thinkingContent string
		if msg.Thinking != "" {
			thinkingContent = thinkingStyle.Render("Thinking: "+msg.Thinking) + "\n"
		}

		var thoughtsContent string
		if len(msg.Thoughts) > 0 {
			thoughtsContent = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700")).Render("Thought: ") +
				strings.Join(msg.Thoughts, "\n          ") + "\n"
		}

		var workedContent string
		if len(msg.ToolsExecuted) > 0 {
			workedContent = dimStyle.Render("Worked: ") + infoStyle.Render(strings.Join(msg.ToolsExecuted, ", ")) + "\n"
		}

		stats := ""
		if msg.Role == "Agent" && msg.InputTokens > 0 {
			stats = dimStyle.Render(fmt.Sprintf("\n(In: %d | Out: %d | Total: %d | %s)",
				msg.InputTokens, msg.OutputTokens, msg.InputTokens+msg.OutputTokens,
				msg.Duration.Round(time.Millisecond)))
		}

		// Distinct separation: prefix on top or beside depending on role
		if msg.Role == "Terminal" {
			sb.WriteString(fmt.Sprintf("%s %s %s\n\n", timeStr, prefix, renderedContent))
		} else {
			sb.WriteString(fmt.Sprintf("%s %s\n%s%s%s%s%s\n\n", timeStr, prefix, thinkingContent, thoughtsContent, workedContent, renderedContent, stats))
		}
	}

	// Append current stream if any
	if m.currentStream != "" || m.currentThinking != "" || m.currentExplore != "" {
		mode := "Agent"
		if m.supervisor != nil {
			mode = strings.ToUpper(string(m.supervisor.GetMode()))
		}
		prefix := agentStyle.Render(fmt.Sprintf("Smara [%s]:", mode))

		var thinkingContent string
		if m.currentThinking != "" {
			thinkingContent = thinkingStyle.Render("Thinking: "+m.currentThinking) + "\n"
		}

		var renderedContent string
		if strings.Contains(m.currentStream, "```") || strings.Contains(m.currentStream, "package ") {
			renderedContent = codingStyle.Render(m.currentStream)
		} else {
			renderedContent = messageStyle.Render(m.currentStream)
		}

		if m.currentExplore != "" {
			exploreLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true).Render("Explore:")
			exploreContent := lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("#7D56F4")).
				PaddingLeft(1).
				MarginLeft(1).
				Render(m.currentExplore)
			sb.WriteString(fmt.Sprintf("%s %s\n%s\n%s %s\n\n", dimStyle.Render("LIVE"), prefix, thinkingContent, exploreLabel, exploreContent))
			if m.currentStream != "" {
				sb.WriteString(fmt.Sprintf("%s\n", renderedContent))
			}
		} else {
			sb.WriteString(fmt.Sprintf("%s %s\n%s%s\n\n", dimStyle.Render("LIVE"), prefix, thinkingContent, renderedContent))
		}
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
	if m.supervisor != nil {
		provider, modelName := m.supervisor.GetModelInfo()
		header += " " + dimStyle.Render(fmt.Sprintf("[%s / %s]", provider, modelName))
	}
	if m.processing {
		status := m.statusText
		if status == "" {
			status = "Sedang memproses..."
		}
		header += " " + warnStyle.Render(fmt.Sprintf("%s %s", m.spinner.View(), status))
	}

	// Confirmation UI
	inputArea := ""
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

	// Create main layout
	return fmt.Sprintf(
		"%s\n%s\n%s",
		header,
		borderStyle.Render(m.viewport.View()),
		inputArea,
	)
}

// Programmatic message injection
func InjectLog(role, content string) {
	if globalProgram != nil {
		// Send asynchronously to avoid deadlock when called from within Update()
		go globalProgram.Send(LogMsg{
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

// processFileMentions searches for @filename in the prompt and injects file content
func (m *AppModel) processFileMentions(prompt string) string {
	// Simple regex for @path/to/file.ext
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
	// Use AltScreen so it feels like a full app
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
			Time:    time.Now(), // we might not have the actual time, but that's fine
		})
	}
	m.renderMessages()
}
