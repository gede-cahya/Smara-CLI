package agent

import (
	"fmt"
	"regexp"
	"strings"
)

func decodeEntities(s string) string {
	replacements := map[string]string{
		"&amp;":    "&",
		"&lt;":     "<",
		"&gt;":     ">",
		"&quot;":   "\"",
		"&apos;":   "'",
		"&#39;":    "'",
		"&nbsp;":   " ",
		"&hellip;": "…",
		"&mdash;":  "—",
		"&ndash;":  "–",
		"&rsquo;":  "'",
		"&lsquo;":  "'",
		"&rdquo;":  "\"",
		"&ldquo;":  "\"",
	}
	for old, newValue := range replacements {
		s = strings.ReplaceAll(s, old, newValue)
	}
	// Generic numeric entity decode (limited).
	s = regexp.MustCompile(`&#\d+;`).ReplaceAllStringFunc(s, func(m string) string {
		var n int
		fmt.Sscanf(m, "&#%d;", &n)
		if n > 0 && n < 0x10FFFF {
			return string(rune(n))
		}
		return m
	})
	return s
}
