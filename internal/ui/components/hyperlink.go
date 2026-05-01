package components

import (
	"regexp"
	"strings"
)

// urlRegex matches URLs (http, https, ftp).
var urlRegex = regexp.MustCompile(`https?://[^\s<>"{}|\\^\[\]]+`)

// HyperlinkURLs wraps all URLs in text with OSC 8 terminal hyperlinks.
// Format: \e]8;;URL\e\\TEXT\e]8;;\e\\
// This makes URLs clickable in terminals that support OSC 8 (iTerm2, GNOME Terminal,
// Windows Terminal, Kitty, etc.).
func HyperlinkURLs(text string) string {
	// Find all URLs with their positions
	matches := urlRegex.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}

	var sb strings.Builder
	last := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		// Write text before this URL
		sb.WriteString(text[last:start])

		url := text[start:end]
		// Strip trailing punctuation that likely isn't part of the URL
		cleanURL := strings.TrimRight(url, ".,:;!?)'\"")
		suffix := url[len(cleanURL):]

		// Write OSC 8 hyperlink
		sb.WriteString("\x1b]8;;")
		sb.WriteString(cleanURL)
		sb.WriteString("\x1b\\")
		sb.WriteString(cleanURL)
		sb.WriteString("\x1b]8;;\x1b\\")
		sb.WriteString(suffix)

		last = end
	}
	// Write remaining text
	sb.WriteString(text[last:])
	return sb.String()
}
