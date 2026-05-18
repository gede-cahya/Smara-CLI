package browser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func WriteReport(res Result) error {
	var b strings.Builder
	b.WriteString("# Browser Subagent Report\n\n")
	b.WriteString("## Prompt\n\n")
	b.WriteString(maskSecrets(res.Prompt) + "\n\n")
	b.WriteString("## Environment\n\n")
	b.WriteString(fmt.Sprintf("- URL: %s\n", res.URL))
	b.WriteString("- Browser: Chromium via go-rod\n")
	b.WriteString(fmt.Sprintf("- Started: %s\n", res.StartedAt.Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("- Finished: %s\n\n", res.FinishedAt.Format("2006-01-02 15:04:05")))
	b.WriteString("## Steps\n\n")
	b.WriteString("| Step | Action | Target | Status |\n|---|---|---|---|\n")
	for i, sr := range res.Steps {
		target := sr.Step.Target
		if strings.EqualFold(sr.Step.Target, "password") || strings.Contains(strings.ToLower(sr.Step.Target), "password") {
			target = "password"
		}
		status := sr.Status
		if sr.Error != "" {
			status += ": " + sr.Error
		}
		b.WriteString(fmt.Sprintf("| %d | %s | %s | %s |\n", i+1, sr.Step.Action, target, status))
	}
	b.WriteString("\n## Screenshots\n\n")
	if res.ScreenshotPath != "" {
		name := filepath.Base(res.ScreenshotPath)
		b.WriteString(fmt.Sprintf("![Screenshot](./%s)\n\n", name))
		b.WriteString(fmt.Sprintf("File: `%s`\n\n", res.ScreenshotPath))
	} else {
		b.WriteString("Tidak ada screenshot.\n\n")
	}
	if len(res.ConsoleErrors) > 0 {
		b.WriteString("## Console Errors\n\n")
		for _, e := range res.ConsoleErrors {
			b.WriteString("- " + e + "\n")
		}
		b.WriteString("\n")
	}
	if len(res.NetworkErrors) > 0 {
		b.WriteString("## Network Errors\n\n")
		for _, e := range res.NetworkErrors {
			b.WriteString("- " + e + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## Result\n\n")
	b.WriteString("Status: `" + res.Status + "`\n")
	return os.WriteFile(res.ReportPath, []byte(b.String()), 0644)
}

func maskSecrets(s string) string {
	lower := strings.ToLower(s)
	idx := strings.Index(lower, "password")
	if idx < 0 {
		return s
	}
	prefix := s[:idx]
	rest := s[idx:]
	for _, q := range []string{"'", "\""} {
		start := strings.Index(rest, q)
		if start >= 0 {
			end := strings.Index(rest[start+1:], q)
			if end >= 0 {
				return prefix + rest[:start+1] + "***" + rest[start+1+end:]
			}
		}
	}
	return s
}
