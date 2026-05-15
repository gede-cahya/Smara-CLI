package components

import (
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
)

// ═══════════════════════════════════════════════════════════════
// FadeWave — Time-based fade-in text animation
// ═══════════════════════════════════════════════════════════════

// wordStamp tracks when each word chunk was added for gradient timing.
type wordStamp struct {
	word      string
	timestamp time.Time
}

// FadeWave renders streaming text with a "wave" fade-in effect.
// Recently added words are brighter / more saturated, while older
// words settle into the normal text color. This creates a smooth
// visual pulse as new content arrives.
type FadeWave struct {
	width  int
	words  []wordStamp
	text   string
	lastAt time.Time
}

// NewFadeWave creates a new fade-wave renderer.
func NewFadeWave(width int) *FadeWave {
	return &FadeWave{width: width}
}

// SetWidth updates the render width.
func (fw *FadeWave) SetWidth(width int) {
	fw.width = width
}

// Append adds new text and records timestamps for each word.
func (fw *FadeWave) Append(chunk string) {
	now := time.Now()
	// Split chunk into words, preserving spaces as separate stamps
	parts := strings.SplitAfter(chunk, " ")
	for _, p := range parts {
		if p == "" {
			continue
		}
		fw.words = append(fw.words, wordStamp{word: p, timestamp: now})
	}
	fw.text += chunk
	fw.lastAt = now
}

// Reset clears all tracked state.
func (fw *FadeWave) Reset() {
	fw.words = nil
	fw.text = ""
}

// Render builds the animated string. Words younger than 150ms are
// accented; 150–400ms are bright text; 400–800ms are subtext;
// older than 800ms are muted (dim). This creates a rolling gradient.
func (fw *FadeWave) Render() string {
	if len(fw.words) == 0 {
		return ""
	}
	now := time.Now()

	// Age thresholds for the gradient
	const (
		accentMs  = 80
		brightMs  = 250
		subtextMs = 600
		dimMs     = 1000
	)

	var sb strings.Builder
	for _, w := range fw.words {
		age := now.Sub(w.timestamp).Milliseconds()
		var style lipgloss.Style
		switch {
		case age < accentMs:
			style = lipgloss.NewStyle().Foreground(ClrAccent).Bold(true)
		case age < brightMs:
			style = lipgloss.NewStyle().Foreground(ClrText)
		case age < subtextMs:
			style = lipgloss.NewStyle().Foreground(ClrSubtext)
		case age < dimMs:
			style = lipgloss.NewStyle().Foreground(ClrMuted)
		default:
			style = lipgloss.NewStyle().Foreground(ClrMuted).Faint(true)
		}
		sb.WriteString(style.Render(w.word))
	}

	return sb.String()
}

// IsStale returns true if all words are older than the given duration.
func (fw *FadeWave) IsStale(d time.Duration) bool {
	if len(fw.words) == 0 {
		return true
	}
	return time.Since(fw.lastAt) > d
}

// Text returns the plain accumulated text without styling.
func (fw *FadeWave) Text() string {
	return fw.text
}
