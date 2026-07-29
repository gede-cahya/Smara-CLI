package platform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
)

// RenderStatusMessage returns a polished, compact progress card for chat platforms.
func RenderStatusMessage(platform, phase, detail string, elapsed time.Duration) string {
	icon := phaseEmoji(phase)
	detail = strings.TrimSpace(detail)
	if detail == "" {
		detail = "Smara sedang memproses permintaan Anda"
	}
	// Defense: status detail might contain DSML from thought summaries
	detail = llm.SanitizeForUser(detail)

	switch strings.ToLower(platform) {
	case "discord":
		return fmt.Sprintf("%s **%s**\n> %s%s", icon, titleCase(phase), detail, elapsedSuffix(elapsed))
	default:
		return fmt.Sprintf("%s *%s*\n%s%s", icon, titleCase(phase), detail, elapsedSuffix(elapsed))
	}
}

// RenderPlatformResponse wraps the final assistant response with a tasteful
// platform-aware header/footer. It keeps the original content readable while
// making Telegram, WhatsApp, Discord, and other chat surfaces feel less flat.
// Also guarantees zero DSML leakage by sanitizing input first.
func RenderPlatformResponse(platform, content, modelName string, duration time.Duration, tools, inputTokens, outputTokens int) string {
	// Hardening: sanitize DSML at render boundary so even if upstream missed it, user never sees raw tags
	content = llm.SanitizeForUser(content)
	content = strings.TrimSpace(content)
	if content == "" {
		content = "Selesai."
	}

	totalTokens := inputTokens + outputTokens
	var meta []string
	if duration > 0 {
		meta = append(meta, fmt.Sprintf("⏱ %.1fs", duration.Seconds()))
	}
	if tools > 0 {
		meta = append(meta, fmt.Sprintf("🔧 %d tools", tools))
	}
	if totalTokens > 0 {
		meta = append(meta, fmt.Sprintf("∑ %d tk", totalTokens))
	}
	if inputTokens > 0 || outputTokens > 0 {
		meta = append(meta, fmt.Sprintf("⇣ %d ⇡ %d", inputTokens, outputTokens))
	}
	if modelName != "" {
		meta = append(meta, "🏷 "+modelName)
	}

	platform = strings.ToLower(platform)
	switch platform {
	case "discord":
		return renderDiscordResponse(content, meta)
	case "whatsapp", "wa":
		return renderWhatsAppResponse(content, meta)
	case "telegram", "telegra":
		return renderTelegramResponse(content, meta)
	default:
		return renderGenericResponse(content, meta)
	}
}

func renderTelegramResponse(content string, meta []string) string {
	var sb strings.Builder
	sb.WriteString("🌀 *Smara Response*\n\n")
	sb.WriteString(polishMarkdown(content))
	if len(meta) > 0 {
		sb.WriteString("\n\n")
		sb.WriteString("_" + strings.Join(meta, " • ") + "_")
	}
	return sb.String()
}

func renderWhatsAppResponse(content string, meta []string) string {
	var sb strings.Builder
	sb.WriteString("🌀 *Smara Response*\n\n")
	sb.WriteString(polishMarkdown(content))
	if len(meta) > 0 {
		sb.WriteString("\n\n")
		sb.WriteString("_" + strings.Join(meta, " • ") + "_")
	}
	return sb.String()
}

func renderDiscordResponse(content string, meta []string) string {
	var sb strings.Builder
	sb.WriteString("### 🌀 Smara Response\n")
	sb.WriteString(polishMarkdown(content))
	if len(meta) > 0 {
		sb.WriteString("\n\n")
		sb.WriteString("-# " + strings.Join(meta, " • "))
	}
	return sb.String()
}

func renderGenericResponse(content string, meta []string) string {
	if len(meta) == 0 {
		return content
	}
	return content + "\n\n" + strings.Join(meta, " • ")
}

func polishMarkdown(content string) string {
	content = strings.TrimSpace(content)
	content = normalizeJSONCodeFences(content)
	content = compactBlankLinesOutsideCode(content)
	bulletRe := regexp.MustCompile(`(?m)^\s*-\s+`)
	content = bulletRe.ReplaceAllString(content, "• ")
	return content
}

func normalizeJSONCodeFences(content string) string {
	re := regexp.MustCompile("(?s)```([A-Za-z0-9_-]*)\\n(.*?)```")
	return re.ReplaceAllStringFunc(content, func(match string) string {
		groups := re.FindStringSubmatch(match)
		if len(groups) < 3 || !strings.EqualFold(groups[1], "json") {
			return match
		}
		var out bytes.Buffer
		if err := json.Indent(&out, []byte(strings.TrimSpace(groups[2])), "", "  "); err != nil {
			return match
		}
		return "```" + groups[1] + "\n" + out.String() + "\n```"
	})
}

func compactBlankLinesOutsideCode(content string) string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	inCode := false
	blank := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCode = !inCode
			blank = false
			out = append(out, line)
			continue
		}
		if !inCode && strings.TrimSpace(line) == "" {
			if blank {
				continue
			}
			blank = true
			out = append(out, "")
			continue
		}
		blank = false
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func elapsedSuffix(elapsed time.Duration) string {
	if elapsed <= 0 {
		return ""
	}
	return fmt.Sprintf("\n_%.0fs elapsed_", elapsed.Seconds())
}

func titleCase(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "_", " "))
	if s == "" {
		return "Processing"
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func modeTitle(mode agent.Mode) string {
	info := agent.GetModeInfo(mode)
	return fmt.Sprintf("%s %s", info.Emoji, info.Label)
}
