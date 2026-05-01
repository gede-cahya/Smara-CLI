package components

import (
	"strings"
	"testing"
)

func TestHyperlinkURLs_noURL(t *testing.T) {
	input := "Hello, world!"
	got := HyperlinkURLs(input)
	if got != input {
		t.Errorf("Expected unchanged text, got %q", got)
	}
}

func TestHyperlinkURLs_singleURL(t *testing.T) {
	input := "Check out https://example.com for more info."
	got := HyperlinkURLs(input)
	if !strings.Contains(got, "\x1b]8;;https://example.com\x1b\\") {
		t.Errorf("Expected OSC 8 hyperlink for https://example.com, got %q", got)
	}
}

func TestHyperlinkURLs_multipleURLs(t *testing.T) {
	input := "Visit https://foo.com and http://bar.org."
	got := HyperlinkURLs(input)
	if !strings.Contains(got, "\x1b]8;;https://foo.com\x1b\\") {
		t.Errorf("Expected OSC 8 hyperlink for https://foo.com, got %q", got)
	}
	if !strings.Contains(got, "\x1b]8;;http://bar.org\x1b\\") {
		t.Errorf("Expected OSC 8 hyperlink for http://bar.org, got %q", got)
	}
}

func TestHyperlinkURLs_trailingPunctuation(t *testing.T) {
	input := "See https://example.com."
	got := HyperlinkURLs(input)
	// URL itself should be hyperlink, period should remain after terminator
	if strings.Contains(got, "\x1b]8;;https://example.com.\x1b\\") {
		t.Errorf("Trailing period should be outside OSC 8, got %q", got)
	}
	if !strings.Contains(got, "https://example.com\x1b]8;;\x1b\\.") {
		t.Errorf("Expected period after OSC 8 terminator, got %q", got)
	}
}
