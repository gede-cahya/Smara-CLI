package components

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
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
	inTokens, outTokens int, duration time.Duration, mode string, modelName string, expandedCode bool) string {

	var sb strings.Builder
	contentWidth := r.width - 8 // padding + border margins
	if contentWidth < 20 {
		contentWidth = 20
	}

	// Time stamp — soft muted with leading dot for visual rhythm
	timeStr := r.theme.MessageTime.Render("◦ " + time.Now().Format("15:04"))

	// Role prefix — text only, no background to avoid the trailing
	// "ghost block" artifact next to the timestamp.
	var prefix string
	var bubbleStyle lipgloss.Style
	var icon string

	switch role {
	case "User":
		icon = "👤"
		prefix = lipgloss.NewStyle().Foreground(r.theme.AccentBlue).Bold(true).Render(fmt.Sprintf("%s  You", icon))
		bubbleStyle = r.theme.MessageUser.Width(contentWidth)
	case "Agent":
		modeLabel := strings.ToUpper(mode)
		icon = r.modeIcon(mode)
		prefixColor := r.theme.ModeColor(mode)
		modelTag := ""
		if modelName != "" {
			modelTag = "  " + lipgloss.NewStyle().Foreground(ClrMuted).Faint(true).Render("· "+modelName)
		}
		prefix = lipgloss.NewStyle().Foreground(prefixColor).Bold(true).Render(fmt.Sprintf("%s  Smara", icon)) +
			"  " + lipgloss.NewStyle().Foreground(prefixColor).Background(ClrSurface).Padding(0, 1).Render(modeLabel) +
			modelTag
		bubbleStyle = r.theme.MessageAgent.Width(contentWidth).BorderForeground(prefixColor)
	case "System":
		icon = "🔔"
		var col = r.theme.AccentYellow
		if strings.HasPrefix(content, "Error") {
			icon = "❌"
			col = r.theme.AccentRed
		}
		prefix = lipgloss.NewStyle().Foreground(col).Bold(true).Render(fmt.Sprintf("%s  System", icon))
		bubbleStyle = r.theme.MessageSystem.Width(contentWidth).BorderForeground(col)
	case "Terminal":
		icon = "▸"
		prefix = lipgloss.NewStyle().Foreground(r.theme.AccentGreen).Bold(true).Render(fmt.Sprintf("%s  Terminal", icon))
		bubbleStyle = r.theme.MessageTerminal.Width(contentWidth)
	default:
		icon = "🌀"
		prefix = lipgloss.NewStyle().Foreground(r.theme.AccentGreen).Bold(true).Render(fmt.Sprintf("%s  Smara", icon))
		bubbleStyle = r.theme.MessageAgent.Width(contentWidth)
	}

	// Build content with processing
	renderedContent := r.processContent(content, contentWidth, expandedCode)

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

	// Header line: time · prefix (no overlapping background)
	headerLine := timeStr + "  " + prefix
	sb.WriteString(headerLine + "\n")
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

	// Completion footer for Agent messages with stats
	if role == "Agent" && (inTokens > 0 || duration > 0) {
		footer := r.RenderCompletionFooter(modelName, duration, inTokens, outTokens, contentWidth)
		sb.WriteString(footer)
		sb.WriteString("\n")
	}

	return sb.String()
}

// RenderStream renders a streaming message with live animation.
func (r *MessageRenderer) RenderStream(content, thinking string, mode string, elapsed time.Duration, dotFrame int, cursorVisible bool, modelName string, phases []PhaseInfo, fadeText string) string {
	contentWidth := r.width - 8
	if contentWidth < 20 {
		contentWidth = 20
	}

	var sb strings.Builder
	live := r.theme.LiveIndicator.Render("● LIVE")
	icon := r.modeIcon(mode)
	modeLabel := strings.ToUpper(mode)
	prefixColor := r.theme.ModeColor(mode)
	
	modelLabel := ""
	if modelName != "" {
		modelLabel = fmt.Sprintf(" (%s)", modelName)
	}
	prefix := lipgloss.NewStyle().Foreground(prefixColor).Bold(true).Render(fmt.Sprintf("%s Smara [%s]%s", icon, modeLabel, modelLabel))

	sb.WriteString(fmt.Sprintf("%s %s\n", live, prefix))

	// Phase stepper — real pipeline phases from backend
	if len(phases) > 0 {
		stepper := NewPhaseStepper(contentWidth)
		stepper.SetWidth(contentWidth)
		for i := range phases {
			if phases[i].Active {
				stepper.SetActive(phases[i].Name)
				break
			}
		}
		stepperStr := stepper.Render(phases)
		if stepperStr != "" {
			sb.WriteString(stepperStr)
			sb.WriteString("\n")
		}
	} else if content == "" {
		// Fallback: still thinking with no phase data yet
		dotStr := r.theme.ThinkingDots.Render(dotsSpinner(dotFrame))
		elapsedStr := r.theme.ThinkingElapsed.Render(fmt.Sprintf("%.1fs", elapsed.Seconds()))
		phase := thinkingPhase(elapsed)
		header := r.theme.ThinkingHeader.Render(fmt.Sprintf("💭 %s %s  %s", phase, dotStr, elapsedStr))

		var thinkContent string
		if thinking != "" {
			thinkContent = r.theme.ThinkingContent.Width(contentWidth - 6).Render(thinking)
		}

		block := header
		if thinkContent != "" {
			block = header + "\n" + thinkContent
		}
		sb.WriteString(r.theme.ThinkingBlock.Width(contentWidth - 2).Render(block))
		sb.WriteString("\n")
	}

	// Determine active phase for conditional content rendering
	activePhase := ""
	for _, p := range phases {
		if p.Active {
			activePhase = p.Name
			break
		}
	}

	// Phase-specific live content area
	switch activePhase {
	case "Thinking":
		if thinking != "" {
			thinkingBlock := r.renderThinking(thinking, contentWidth)
			sb.WriteString(thinkingBlock)
			sb.WriteString("\n")
		}
	case "Analyzing":
		for _, p := range phases {
			if p.Active && p.Content != "" {
				sb.WriteString(r.renderAnalysisPreview(p.Content, contentWidth))
				sb.WriteString("\n")
				break
			}
		}
	case "Exploring":
		for _, p := range phases {
			if p.Active && p.Content != "" {
				sb.WriteString(r.renderExplorePreview(p.Content, contentWidth))
				sb.WriteString("\n")
				break
			}
		}
	case "Generating":
		// Fade-wave animated text
		if fadeText != "" {
			bubbleStyle := r.theme.MessageAgent.Width(contentWidth)
			sb.WriteString(HyperlinkURLs(bubbleStyle.Render(fadeText)))
			sb.WriteString("\n")
		} else if content != "" {
			bubbleStyle := r.theme.MessageAgent.Width(contentWidth)
			streamContent := r.processContent(content, contentWidth, true)
			if cursorVisible {
				streamContent += r.theme.StreamCursor.Render("▌")
			}
			sb.WriteString(HyperlinkURLs(bubbleStyle.Render(streamContent)))
			sb.WriteString("\n")
		}
	default:
		// No recognized phase — show standard stream
		if thinking != "" {
			thinkingBlock := r.renderThinking(thinking, contentWidth)
			sb.WriteString(thinkingBlock)
			sb.WriteString("\n")
		}
		if content != "" {
			bubbleStyle := r.theme.MessageAgent.Width(contentWidth)
			streamContent := r.processContent(content, contentWidth, true)
			if cursorVisible {
				streamContent += r.theme.StreamCursor.Render("▌")
			}
			sb.WriteString(HyperlinkURLs(bubbleStyle.Render(streamContent)))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// RenderCompletionFooter renders the model + stats footer below a completed Agent message.
func (r *MessageRenderer) RenderCompletionFooter(modelName string, duration time.Duration, inTokens, outTokens int, width int) string {
	var parts []string

	if modelName != "" {
		parts = append(parts, r.theme.CompletionModel.Render(fmt.Sprintf("🏷️ %s", modelName)))
	}
	if duration > 0 {
		parts = append(parts, r.theme.CompletionStats.Render(fmt.Sprintf("⏱ %.1fs", duration.Seconds())))
	}
	if inTokens > 0 {
		parts = append(parts, r.theme.CompletionStats.Render(fmt.Sprintf("⇣ %d tk", inTokens)))
	}
	if outTokens > 0 {
		parts = append(parts, r.theme.CompletionStats.Render(fmt.Sprintf("⇡ %d tk", outTokens)))
	}

	if len(parts) == 0 {
		return ""
	}

	footerContent := strings.Join(parts, "  ")
	return r.theme.CompletionFooter.Width(width).Render(footerContent)
}

// dotsSpinner returns the current dots spinner frame.
func dotsSpinner(frame int) string {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	return frames[frame%len(frames)]
}

// thinkingPhase returns the current phase text based on elapsed time.
// Rotates every 2 seconds: Thinking → Analyzing → Planning response → Generating
func thinkingPhase(elapsed time.Duration) string {
	phases := []string{"Thinking...", "Analyzing...", "Planning response...", "Generating..."}
	idx := int(elapsed.Seconds()/2) % len(phases)
	return phases[idx]
}

// processContent handles markdown rendering via glamour, with optional code block collapsing.
func (r *MessageRenderer) processContent(content string, width int, expandedCode bool) string {
	if !expandedCode {
		content = collapseCodeBlocks(content)
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		// Fallback to plain text on error
		return content
	}

	out, err := renderer.Render(content)
	if err != nil {
		return content
	}

	return HyperlinkURLs(strings.TrimRight(out, "\n"))
}

// collapseCodeBlocks replaces fenced code blocks with a compact indicator line.
func collapseCodeBlocks(content string) string {
	re := regexp.MustCompile("(?s)```(\\w*)\\n(.*?)```")
	return re.ReplaceAllStringFunc(content, func(match string) string {
		groups := re.FindStringSubmatch(match)
		if len(groups) < 3 {
			return match
		}
		lang := groups[1]
		code := groups[2]
		lines := strings.Count(code, "\n")
		if !strings.HasSuffix(code, "\n") && code != "" {
			lines++
		}
		if lang == "" {
			lang = "text"
		}
		return fmt.Sprintf("▶ *Code: `%s` (%d lines) — press `/expand` to view*", lang, lines)
	})
}

func (r *MessageRenderer) renderThinking(thinking string, width int) string {
	toggle := NewThinkingToggle(width)
	toggle.SetContent(thinking)
	return toggle.Render()
}

func (r *MessageRenderer) renderAnalysisPreview(analysis string, width int) string {
	header := r.theme.ThinkingHeader.Render("🔍 Analyzing...")
	var body strings.Builder
	lines := strings.Split(analysis, "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		body.WriteString("  • ")
		body.WriteString(r.theme.ThinkingContent.Render(line))
		body.WriteString("\n")
	}
	return r.theme.ThinkingBlock.Width(width - 2).Render(header + "\n" + body.String())
}

func (r *MessageRenderer) renderExplorePreview(explore string, width int) string {
	header := r.theme.ThinkingHeader.Render("🛠 Exploring...")
	var body strings.Builder
	lines := strings.Split(explore, "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		body.WriteString("  ▸ ")
		body.WriteString(r.theme.ThinkingContent.Render(line))
		body.WriteString("\n")
	}
	return r.theme.ThinkingBlock.Width(width - 2).Render(header + "\n" + body.String())
}

func (r *MessageRenderer) renderThoughts(thoughts []string, width int) string {
	var sb strings.Builder
	sb.WriteString(r.theme.ThinkingHeader.Render("💡 Thought Process:\n"))
	for _, t := range thoughts {
		sb.WriteString("  ")
		sb.WriteString(r.theme.ThinkingContent.Render("• " + t))
		sb.WriteString("\n")
	}
	return r.theme.ThinkingBlock.Width(width - 2).Render(sb.String())
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
