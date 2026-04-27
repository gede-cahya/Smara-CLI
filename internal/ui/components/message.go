package components

import (
	"fmt"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
)

// ═══════════════════════════════════════════════════════════════
// Message Bubble Component — Styled Chat Messages
// ═══════════════════════════════════════════════════════════════

// MessageRenderer handles rendering of chat messages.
type MessageRenderer struct {
	theme *Theme
	width int
}

// NewMessageRenderer creates a new message renderer.
func NewMessageRenderer(width int) *MessageRenderer {
	return &MessageRenderer{
		theme: GetTheme(),
		width: width,
	}
}

// SetWidth updates the render width.
func (r *MessageRenderer) SetWidth(width int) {
	r.width = width
}

// RenderMessage renders a single chat message as a styled bubble.
func (r *MessageRenderer) RenderMessage(role, content, thinking string, thoughts, tools []string,
	inTokens, outTokens int, duration time.Duration, mode string) string {

	var sb strings.Builder
	contentWidth := r.width - 8 // padding + border margins
	if contentWidth < 20 {
		contentWidth = 20
	}

	// Time stamp
	timeStr := r.theme.MessageTime.Render(time.Now().Format("15:04"))

	// Role prefix with icon
	var prefix string
	var bubbleStyle lipgloss.Style
	var icon string

	switch role {
	case "User":
		icon = "💬"
		prefix = r.theme.MessageUser.Foreground(r.theme.AccentBlue).Bold(true).Render(fmt.Sprintf("%s You", icon))
		bubbleStyle = r.theme.MessageUser.Width(contentWidth)
	case "Agent":
		modeLabel := strings.ToUpper(mode)
		icon = r.modeIcon(mode)
		prefixColor := r.theme.ModeColor(mode)
		prefix = lipgloss.NewStyle().Foreground(prefixColor).Bold(true).Render(fmt.Sprintf("%s Smara [%s]", icon, modeLabel))
		bubbleStyle = r.theme.MessageAgent.Width(contentWidth)
	case "System":
		icon = "🔔"
		if strings.HasPrefix(content, "Error") {
			icon = "❌"
			prefix = r.theme.MessageSystem.Foreground(r.theme.AccentRed).Bold(true).Render(fmt.Sprintf("%s System", icon))
		} else {
			prefix = r.theme.MessageSystem.Foreground(r.theme.AccentYellow).Bold(true).Render(fmt.Sprintf("%s System", icon))
		}
		bubbleStyle = r.theme.MessageSystem.Width(contentWidth)
	case "Terminal":
		icon = "$"
		prefix = r.theme.MessageTerminal.Foreground(r.theme.AccentGreen).Bold(true).Render(fmt.Sprintf("%s Terminal", icon))
		bubbleStyle = r.theme.MessageTerminal.Width(contentWidth)
	default:
		icon = "🌀"
		prefix = r.theme.MessageAgent.Foreground(r.theme.AccentGreen).Bold(true).Render(fmt.Sprintf("%s Smara", icon))
		bubbleStyle = r.theme.MessageAgent.Width(contentWidth)
	}

	// Build content with processing
	renderedContent := r.processContent(content, contentWidth)

	// Thinking block (if expanded)
	var thinkingBlock string
	if thinking != "" {
		thinkingBlock = r.renderThinking(thinking, contentWidth)
	}

	// Thoughts
	var thoughtsBlock string
	if len(thoughts) > 0 {
		thoughtsBlock = r.renderThoughts(thoughts, contentWidth)
	}

	// Tools executed
	var toolsBlock string
	if len(tools) > 0 {
		toolsBlock = r.renderTools(tools)
	}

	// Stats
	var statsBlock string
	if role == "Agent" && inTokens > 0 {
		statsBlock = r.renderStats(inTokens, outTokens, duration)
	}

	// Assemble the bubble
	sb.WriteString(fmt.Sprintf("%s %s\n", timeStr, prefix))
	if thinkingBlock != "" {
		sb.WriteString(thinkingBlock)
		sb.WriteString("\n")
	}
	if thoughtsBlock != "" {
		sb.WriteString(thoughtsBlock)
		sb.WriteString("\n")
	}
	if toolsBlock != "" {
		sb.WriteString(toolsBlock)
		sb.WriteString("\n")
	}

	// Content bubble
	if renderedContent != "" {
		sb.WriteString(bubbleStyle.Render(renderedContent))
		sb.WriteString("\n")
	}

	if statsBlock != "" {
		sb.WriteString(statsBlock)
		sb.WriteString("\n")
	}

	return sb.String()
}

// RenderStream renders a streaming message.
func (r *MessageRenderer) RenderStream(content, thinking string, mode string) string {
	contentWidth := r.width - 8
	if contentWidth < 20 {
		contentWidth = 20
	}

	var sb strings.Builder
	live := r.theme.LiveIndicator.Render("● LIVE")
	icon := r.modeIcon(mode)
	modeLabel := strings.ToUpper(mode)
	prefixColor := r.theme.ModeColor(mode)
	prefix := lipgloss.NewStyle().Foreground(prefixColor).Bold(true).Render(fmt.Sprintf("%s Smara [%s]", icon, modeLabel))

	sb.WriteString(fmt.Sprintf("%s %s\n", live, prefix))

	if thinking != "" {
		thinkingBlock := r.renderThinking(thinking, contentWidth)
		sb.WriteString(thinkingBlock)
		sb.WriteString("\n")
	}

	if content != "" {
		bubbleStyle := r.theme.MessageAgent.Width(contentWidth)
		sb.WriteString(bubbleStyle.Render(r.processContent(content, contentWidth)))
		sb.WriteString("\n")
	}

	return sb.String()
}

// processContent handles code blocks, inline code, and basic formatting.
func (r *MessageRenderer) processContent(content string, width int) string {
	lines := strings.Split(content, "\n")
	var result []string
	inCodeBlock := false
	var codeLang string
	var codeLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Code block start/end
		if strings.HasPrefix(trimmed, "```") {
			if !inCodeBlock {
				inCodeBlock = true
				codeLang = strings.TrimPrefix(trimmed, "```")
				codeLang = strings.TrimSpace(codeLang)
				continue
			} else {
				inCodeBlock = false
				// Render code block
				codeContent := strings.Join(codeLines, "\n")
				if codeLang != "" {
					langLabel := r.theme.CodeLang.Render(codeLang)
					result = append(result, r.theme.CodeBlock.Width(width-4).Render(langLabel+"\n"+codeContent))
				} else {
					result = append(result, r.theme.CodeBlock.Width(width-4).Render(codeContent))
				}
				codeLines = nil
				codeLang = ""
				continue
			}
		}

		if inCodeBlock {
			codeLines = append(codeLines, line)
			continue
		}

		// Inline code
		line = r.processInlineCode(line)
		// Bold
		line = r.processBold(line)
		// Italic
		line = r.processItalic(line)

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

func (r *MessageRenderer) processInlineCode(line string) string {
	// Simple inline code: `code`
	for strings.Contains(line, "`") {
		start := strings.Index(line, "`")
		if start == -1 {
			break
		}
		end := strings.Index(line[start+1:], "`")
		if end == -1 {
			break
		}
		end += start + 1
		code := line[start+1 : end]
		styled := r.theme.CodeInline.Render(code)
		line = line[:start] + styled + line[end+1:]
	}
	return line
}

func (r *MessageRenderer) processBold(line string) string {
	for strings.Contains(line, "**") {
		start := strings.Index(line, "**")
		if start == -1 {
			break
		}
		end := strings.Index(line[start+2:], "**")
		if end == -1 {
			break
		}
		end += start + 2
		text := line[start+2 : end]
		styled := lipgloss.NewStyle().Bold(true).Render(text)
		line = line[:start] + styled + line[end+2:]
	}
	return line
}

func (r *MessageRenderer) processItalic(line string) string {
	for strings.Contains(line, "*") {
		start := strings.Index(line, "*")
		if start == -1 || (start+1 < len(line) && line[start+1] == '*') {
			break
		}
		end := strings.Index(line[start+1:], "*")
		if end == -1 {
			break
		}
		end += start + 1
		text := line[start+1 : end]
		styled := lipgloss.NewStyle().Italic(true).Render(text)
		line = line[:start] + styled + line[end+1:]
	}
	return line
}

func (r *MessageRenderer) renderThinking(thinking string, width int) string {
	header := r.theme.ThinkingHeader.Render("💭 Thinking...")
	content := r.theme.ThinkingContent.Width(width - 6).Render(thinking)
	return r.theme.ThinkingBlock.Width(width - 4).Render(header + "\n" + content)
}

func (r *MessageRenderer) renderThoughts(thoughts []string, width int) string {
	var sb strings.Builder
	sb.WriteString(r.theme.ThinkingHeader.Render("💡 Thought Process:\n"))
	for _, t := range thoughts {
		sb.WriteString("  ")
		sb.WriteString(r.theme.ThinkingContent.Render("• " + t))
		sb.WriteString("\n")
	}
	return r.theme.ThinkingBlock.Width(width - 4).Render(sb.String())
}

func (r *MessageRenderer) renderTools(tools []string) string {
	var sb strings.Builder
	sb.WriteString(r.theme.StatsLabel.Render("🛠️ Tools: "))
	for i, t := range tools {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(r.theme.StatsValue.Render(t))
	}
	return sb.String()
}

func (r *MessageRenderer) renderStats(inTokens, outTokens int, duration time.Duration) string {
	var sb strings.Builder
	sb.WriteString("  ")
	sb.WriteString(r.theme.StatsLabel.Render("In: "))
	sb.WriteString(r.theme.StatsValue.Render(fmt.Sprintf("%d", inTokens)))
	sb.WriteString(r.theme.StatsLabel.Render(" | Out: "))
	sb.WriteString(r.theme.StatsValue.Render(fmt.Sprintf("%d", outTokens)))
	sb.WriteString(r.theme.StatsLabel.Render(" | Total: "))
	sb.WriteString(r.theme.StatsValue.Render(fmt.Sprintf("%d", inTokens+outTokens)))
	sb.WriteString(r.theme.StatsLabel.Render(" | "))
	sb.WriteString(r.theme.StatsValue.Render(duration.Round(time.Millisecond).String()))
	return sb.String()
}

func (r *MessageRenderer) modeIcon(mode string) string {
	switch mode {
	case "ask":
		return "💬"
	case "rush":
		return "⚡"
	case "plan":
		return "📋"
	case "test":
		return "🧪"
	default:
		return "🌀"
	}
}
