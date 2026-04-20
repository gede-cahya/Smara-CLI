// Package ui provides the interactive terminal REPL for Smara.
package ui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// ErrInterrupted is returned when the user interrupts input with Ctrl+C
var ErrInterrupted = fmt.Errorf("interrupted")

// ANSI color codes
const (
	Reset  = "\033[0m"
	Bold   = "\033[1m"
	Dim    = "\033[2m"
	Italic = "\033[3m"

	Cyan    = "\033[36m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Red     = "\033[31m"
	Magenta = "\033[35m"
	Blue    = "\033[34m"
	White   = "\033[37m"

	BgCyan    = "\033[46m"
	BgBlue    = "\033[44m"
	BgYellow  = "\033[43m"
	BgGreen   = "\033[42m"
	BgMagenta = "\033[45m"
)

// Mode colors mapped by mode name
var ModeColors = map[string]string{
	"ask":  Cyan,
	"rush": Yellow,
	"plan": Magenta,
}

// Mode emojis
var ModeEmojis = map[string]string{
	"ask":  "💬",
	"rush": "⚡",
	"plan": "📋",
}

// ModeOrder defines the cycle order for Tab key
var ModeOrder = []string{"ask", "rush", "plan"}

// Prompt manages the interactive REPL loop with raw terminal input.
type Prompt struct {
	history      []string
	currentMode  string
	onModeChange func(newMode string) // callback when mode changes via Tab
}

// NewPrompt creates a new interactive prompt.
func NewPrompt() *Prompt {
	return &Prompt{
		history:     make([]string, 0),
		currentMode: "ask",
	}
}

// SetMode updates the prompt's displayed mode.
func (p *Prompt) SetMode(mode string) {
	p.currentMode = mode
}

// OnModeChange sets a callback that fires when Tab cycles the mode.
func (p *Prompt) OnModeChange(fn func(newMode string)) {
	p.onModeChange = fn
}

// PrintBanner displays the Smara startup banner.
func PrintBanner() {
	banner := `
` + Cyan + Bold + `  ███████╗███╗   ███╗ █████╗ ██████╗  █████╗ 
  ██╔════╝████╗ ████║██╔══██╗██╔══██╗██╔══██╗
  ███████╗██╔████╔██║███████║██████╔╝███████║
  ╚════██║██║╚██╔╝██║██╔══██║██╔══██╗██╔══██║
  ███████║██║ ╚═╝ ██║██║  ██║██║  ██║██║  ██║
  ╚══════╝╚═╝     ╚═╝╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═╝` + Reset + `
` + Dim + `  स्मृति — Autonomous Multi-Agent Terminal v1.2.0` + Reset + `
`
	fmt.Println(banner)
}

// PrintInfo displays an info message.
func PrintInfo(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if globalProgram != nil {
		InjectLog("System", msg)
	} else {
		fmt.Printf("  %s▸%s %s\n", Cyan, Reset, msg)
	}
}

// PrintSuccess displays a success message.
func PrintSuccess(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if globalProgram != nil {
		InjectLog("System", "✓ "+msg)
	} else {
		fmt.Printf("  %s✓%s %s\n", Green, Reset, msg)
	}
}

// PrintWarning displays a warning message.
func PrintWarning(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if globalProgram != nil {
		InjectLog("System", "⚠ "+msg)
	} else {
		fmt.Printf("  %s⚠%s %s\n", Yellow, Reset, msg)
	}
}

// PrintError displays an error message.
func PrintError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if globalProgram != nil {
		InjectLog("System", "Error: "+msg)
	} else {
		fmt.Printf("  %s✗%s %s\n", Red, Reset, msg)
	}
}

// PrintAgent displays agent output with mode indicator.
func PrintAgent(content string, mode string) {
	if globalProgram != nil {
		InjectLog("Agent", content)
		return
	}
	emoji := ModeEmojis[mode]
	if emoji == "" {
		emoji = "🌀"
	}
	color := ModeColors[mode]
	if color == "" {
		color = Magenta
	}

	label := strings.ToUpper(mode)
	fmt.Printf("\n  %s%s%s Smara [%s]:%s\n", Bold, color, emoji, label, Reset)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		fmt.Printf("  %s\n", line)
	}
	fmt.Println()
}

// PrintModeChange displays a mode change notification.
func PrintModeChange(mode, emoji, description string) {
	if globalProgram != nil {
		InjectLog("System", fmt.Sprintf("Mode changed to %s: %s", strings.ToUpper(mode), description))
		return
	}
	color := ModeColors[mode]
	if color == "" {
		color = Cyan
	}
	fmt.Printf("\n  %s%s━━━ Mode: %s %s ━━━%s\n", Bold, color, emoji, strings.ToUpper(mode), Reset)
	fmt.Printf("  %s%s%s\n\n", Dim, description, Reset)
}

// printPromptPrefix renders the mode-aware prompt prefix.
func (p *Prompt) printPromptPrefix() {
	color := ModeColors[p.currentMode]
	if color == "" {
		color = Cyan
	}
	emoji := ModeEmojis[p.currentMode]
	if emoji == "" {
		emoji = ">"
	}
	fmt.Printf("  %s %s%ssmara>%s ", emoji, Bold, color, Reset)
}

// clearLine clears the current terminal line.
func clearLine() {
	fmt.Printf("\r\033[K")
}

// nextMode returns the next mode in cycle order.
func (p *Prompt) nextMode() string {
	for i, m := range ModeOrder {
		if m == p.currentMode {
			return ModeOrder[(i+1)%len(ModeOrder)]
		}
	}
	return ModeOrder[0]
}

// ReadLine reads a line of input with raw terminal mode for Tab key support.
func (p *Prompt) ReadLine() (string, error) {
	// Get the file descriptor for stdin
	fd := int(os.Stdin.Fd())

	// Switch to raw mode to capture individual key presses
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		// Fallback to simple readline if raw mode fails
		return p.readLineFallback()
	}
	defer term.Restore(fd, oldState)

	var input []byte
	cursorPos := 0

	p.printPromptPrefix()

	buf := make([]byte, 32)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			term.Restore(fd, oldState)
			return "", err
		}

		for i := 0; i < n; i++ {
			b := buf[i]

			switch {
			case b == 0x09: // Tab — cycle mode
				newMode := p.nextMode()
				p.currentMode = newMode
				if p.onModeChange != nil {
					p.onModeChange(newMode)
				}
				// Redraw prompt with new mode, keeping current input
				clearLine()
				p.printPromptPrefix()
				fmt.Printf("%s", string(input))
				// Position cursor correctly
				if cursorPos < len(input) {
					fmt.Printf("\033[%dD", len(input)-cursorPos)
				}

			case b == 0x0D || b == 0x0A: // Enter
				fmt.Printf("\r\n")
				line := strings.TrimSpace(string(input))
				if line != "" {
					p.history = append(p.history, line)
				}
				term.Restore(fd, oldState)
				return line, nil

			case b == 0x03: // Ctrl+C
				fmt.Printf("\r\n")
				term.Restore(fd, oldState)
				return "", fmt.Errorf("interrupt")

			case b == 0x04: // Ctrl+D (EOF)
				fmt.Printf("\r\n")
				term.Restore(fd, oldState)
				return "", fmt.Errorf("EOF")

			case b == 0x7F || b == 0x08: // Backspace
				if cursorPos > 0 {
					// Remove character before cursor
					input = append(input[:cursorPos-1], input[cursorPos:]...)
					cursorPos--
					// Redraw
					clearLine()
					p.printPromptPrefix()
					fmt.Printf("%s", string(input))
					if cursorPos < len(input) {
						fmt.Printf("\033[%dD", len(input)-cursorPos)
					}
				}

			case b == 0x15: // Ctrl+U — clear line
				input = input[:0]
				cursorPos = 0
				clearLine()
				p.printPromptPrefix()

			case b == 0x17: // Ctrl+W — delete last word
				if cursorPos > 0 {
					// Find previous word boundary
					j := cursorPos - 1
					for j > 0 && input[j-1] == ' ' {
						j--
					}
					for j > 0 && input[j-1] != ' ' {
						j--
					}
					input = append(input[:j], input[cursorPos:]...)
					cursorPos = j
					clearLine()
					p.printPromptPrefix()
					fmt.Printf("%s", string(input))
					if cursorPos < len(input) {
						fmt.Printf("\033[%dD", len(input)-cursorPos)
					}
				}

			case b == 0x1B: // Escape sequence (arrow keys, etc.)
				if i+2 < n && buf[i+1] == '[' {
					switch buf[i+2] {
					case 'C': // Right arrow
						if cursorPos < len(input) {
							cursorPos++
							fmt.Printf("\033[C")
						}
					case 'D': // Left arrow
						if cursorPos > 0 {
							cursorPos--
							fmt.Printf("\033[D")
						}
					case 'A': // Up arrow — previous history
						// Simple history up
						if len(p.history) > 0 {
							input = []byte(p.history[len(p.history)-1])
							cursorPos = len(input)
							clearLine()
							p.printPromptPrefix()
							fmt.Printf("%s", string(input))
						}
					case 'B': // Down arrow — clear
						input = input[:0]
						cursorPos = 0
						clearLine()
						p.printPromptPrefix()
					}
					i += 2 // Skip the escape sequence bytes
				}

				// Handle Ctrl+Shift+C (copy) and Ctrl+Shift+V (paste)
				// These are escape sequences in raw terminal mode
				if i+1 < n {
					// Ctrl+Shift+C = \e + C (0x1B 0x43)
					// Ctrl+Shift+V = \e + V (0x1B 0x56)
					if buf[i+1] == 'C' || buf[i+1] == 'V' {
						// Notify user about copy/paste in raw mode
						if buf[i+1] == 'C' {
							fmt.Printf("\r\n  %s💡%s Copy — select text in terminal%s\r\n", Yellow, Reset, Dim)
						} else {
							fmt.Printf("\r\n  %s💡%s Paste — press %sCtrl+Shift+V%s again after restoring terminal%s\r\n",
								Yellow, Reset, Bold, Reset, Dim)
						}
						// Redraw prompt
						clearLine()
						p.printPromptPrefix()
						fmt.Printf("%s", string(input))
						if cursorPos < len(input) {
							fmt.Printf("\033[%dD", len(input)-cursorPos)
						}
						i++ // Skip the extra byte
						continue
					}
				}

			case b >= 0x20 && b < 0x7F: // Printable ASCII
				// Check for '?' to show keyboard shortcuts
				if b == '?' && len(input) == 0 {
					PrintKeyboardShortcuts()
					clearLine()
					p.printPromptPrefix()
					continue
				}
				// Insert character at cursor position
				if cursorPos == len(input) {
					input = append(input, b)
				} else {
					input = append(input[:cursorPos+1], input[cursorPos:]...)
					input[cursorPos] = b
				}
				cursorPos++
				// Redraw from cursor
				clearLine()
				p.printPromptPrefix()
				fmt.Printf("%s", string(input))
				if cursorPos < len(input) {
					fmt.Printf("\033[%dD", len(input)-cursorPos)
				}

			default:
				// Handle multi-byte UTF-8 characters
				if b >= 0x80 {
					// Collect remaining bytes of UTF-8 sequence
					remaining := 0
					if b&0xE0 == 0xC0 {
						remaining = 1
					} else if b&0xF0 == 0xE0 {
						remaining = 2
					} else if b&0xF8 == 0xF0 {
						remaining = 3
					}
					utfBytes := []byte{b}
					for r := 0; r < remaining && i+1 < n; r++ {
						i++
						utfBytes = append(utfBytes, buf[i])
					}
					input = append(input[:cursorPos], append(utfBytes, input[cursorPos:]...)...)
					cursorPos += len(utfBytes)
					clearLine()
					p.printPromptPrefix()
					fmt.Printf("%s", string(input))
					if cursorPos < len(input) {
						// Calculate display width difference
						fmt.Printf("\033[%dD", len(input)-cursorPos)
					}
				}
			}
		}
	}
}

// readLineFallback is used when raw terminal mode is not available.
func (p *Prompt) readLineFallback() (string, error) {
	p.printPromptPrefix()
	buf := make([]byte, 4096)
	n, err := os.Stdin.Read(buf)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(buf[:n]))
	if line != "" {
		p.history = append(p.history, line)
	}
	return line, nil
}

// ReadLineWithCancel reads a line of input with context cancellation support.
// When context is cancelled, returns ErrInterrupted.
func (p *Prompt) ReadLineWithCancel(ctx context.Context) (string, error) {
	// Create a channel to receive result from ReadLine
	resultCh := make(chan struct {
		line string
		err  error
	}, 1)

	// Run ReadLine in a goroutine
	go func() {
		line, err := p.ReadLine()
		resultCh <- struct {
			line string
			err  error
		}{line, err}
	}()

	// Wait for either completion or cancellation
	select {
	case <-ctx.Done():
		return "", ErrInterrupted
	case result := <-resultCh:
		// Convert "interrupt" error from ReadLine to our ErrInterrupted
		if result.err != nil && result.err.Error() == "interrupt" {
			return "", ErrInterrupted
		}
		return result.line, result.err
	}
}

// IsExitCommand checks if the input is an exit command.
func IsExitCommand(input string) bool {
	lower := strings.ToLower(input)
	return lower == "exit" || lower == "quit" || lower == ":q" || lower == "keluar"
}

// IsCommand checks if the input is a special command (starts with /).
func IsCommand(input string) bool {
	return strings.HasPrefix(input, "/")
}

// ParseCommand extracts command and arguments from a /command input.
func ParseCommand(input string) (string, []string) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return "", nil
	}
	cmd := strings.TrimPrefix(parts[0], "/")
	return cmd, parts[1:]
}

// PrintHelp displays available REPL commands.
func PrintHelp() {
	fmt.Println()
	fmt.Print("  " + Bold + White + "Perintah tersedia:" + Reset + "\n")
	fmt.Print("  " + Yellow + "[Tab]" + Reset + "              — Ganti mode agen (cycle: ask → rush → plan)\n")
	fmt.Print("  " + Yellow + "/mode [ask|rush|plan]" + Reset + " — Ganti mode agen\n")
	fmt.Print("  " + Yellow + "/model [provider] [model]" + Reset + " — Ganti LLM provider/model\n")
	fmt.Print("  " + Yellow + "/help" + Reset + "              — Tampilkan bantuan ini\n")
	fmt.Print("  " + Yellow + "/memory" + Reset + "            — Lihat memori tersimpan\n")
	fmt.Print("  " + Yellow + "/mcp" + Reset + "               — Lihat MCP servers dan tools\n")
	fmt.Print("  " + Yellow + "/session [list|new|info|switch|end]" + Reset + " — Kelola sessions\n")
	fmt.Print("  " + Yellow + "/clear" + Reset + "             — Bersihkan layar\n")
	fmt.Print("  " + Yellow + "exit" + Reset + "               — Keluar dari Smara\n")
	fmt.Println()
	fmt.Print("  " + Bold + "Mode agen:" + Reset + "\n")
	fmt.Print("  " + Cyan + "💬 ask" + Reset + "   — Tanya-jawab langsung\n")
	fmt.Print("  " + Yellow + "⚡ rush" + Reset + "  — Eksekusi cepat, langsung bertindak\n")
	fmt.Print("  " + Magenta + "📋 plan" + Reset + "  — Buat rencana dulu, lalu eksekusi\n")
	fmt.Println()
}

// PrintUsageStats displays usage statistics after each prompt.
// Accepts agent.Stats but displays it in a user-friendly format.
func PrintUsageStats(promptCount int, totalTokens int, avgTokens int, totalCost float64, totalDuration string) {
	if promptCount == 0 {
		return
	}
	fmt.Printf("  %s📊 Stats:%s prompts=%d tokens=%d avg=%d cost=$%.4f duration=%s\n",
		Dim, Reset,
		promptCount,
		totalTokens,
		avgTokens,
		totalCost,
		totalDuration,
	)
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
var spinIdx = 0

// SpinnerStart begins a spinner animation (call in goroutine).
func SpinnerStart(msg string) {
	fmt.Printf("\r  %s%s%s", Cyan, spinnerFrames[0], Reset)
	fmt.Printf(" %s...", msg)
}

// SpinnerTick updates the spinner animation.
func SpinnerTick() {
	spinIdx = (spinIdx + 1) % len(spinnerFrames)
	fmt.Printf("\r  %s%s%s", Cyan, spinnerFrames[spinIdx], Reset)
}

// SpinnerStop ends the spinner animation.
func SpinnerStop(success bool) {
	clearLine()
	if success {
		fmt.Printf("\r  %s✓%s %s\n", Green, Reset, "Done")
	} else {
		fmt.Printf("\r  %s✗%s %s\n", Red, Reset, "Failed")
	}
}

// PrintStatusBar displays a status bar at the bottom of the terminal.
func PrintStatusBar(mode string, promptCount int, totalTokens int) {
	emoji := ModeEmojis[mode]
	if emoji == "" {
		emoji = "🌀"
	}
	color := ModeColors[mode]
	if color == "" {
		color = Cyan
	}
	fmt.Printf("\r  %s%s[%s]%s prompts=%d tokens=%d", Bold, color, emoji, Reset, promptCount, totalTokens)
}

// PrintKeyboardShortcuts displays keyboard shortcuts overlay.
func PrintKeyboardShortcuts() {
	fmt.Println()
	fmt.Print("  " + Bold + Yellow + "⌨️ Keyboard Shortcuts:" + Reset + "\n")
	fmt.Print("  " + Yellow + "[Tab]" + Reset + "        — Cycle mode (ask → rush → plan)\n")
	fmt.Print("  " + Yellow + "[↑/↓]" + Reset + "       — Command history\n")
	fmt.Print("  " + Yellow + "[Ctrl+U]" + Reset + "     — Clear current line\n")
	fmt.Print("  " + Yellow + "[Ctrl+W]" + Reset + "     — Delete last word\n")
	fmt.Print("  " + Yellow + "[Ctrl+Shift+C]" + Reset + " — Copy selection\n")
	fmt.Print("  " + Yellow + "[Ctrl+Shift+V]" + Reset + " — Paste from clipboard\n")
	fmt.Print("  " + Yellow + "[Ctrl+C]" + Reset + "     — Interrupt/Cancel\n")
	fmt.Print("  " + Yellow + "[Ctrl+D]" + Reset + "     — Exit\n")
	fmt.Print("  " + Yellow + "[?]" + Reset + "          — Show this help\n")
	fmt.Println()
}
