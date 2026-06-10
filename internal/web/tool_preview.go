package web

import (
	"bufio"
	"regexp"
	"strings"
)

const maxToolPreviewLen = 700
const maxSourcePreviewLen = 20000

var numberedSourceLineRE = regexp.MustCompile(`^\d+$`)

// formatToolResultPreview keeps web chat tool results readable by replacing
// noisy terminal-style dumps with compact structured summaries.
func formatToolResultPreview(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return "(kosong)"
	}
	if isNumberedSourcePreview(trimmed) {
		return truncateSourcePreview(trimmed, maxSourcePreviewLen)
	}
	trimmed = strings.ReplaceAll(trimmed, "▶", "")
	if summary, ok := compactSystemdStatusPreview(trimmed); ok {
		return summary
	}
	if summary, ok := compactHTTPResponsePreview(trimmed); ok {
		return summary
	}
	if summary, ok := compactBuildLogPreview(trimmed); ok {
		return summary
	}
	if summary, ok := compactScraperPreview(trimmed); ok {
		return summary
	}
	return truncatePreview(singleLine(trimmed), maxToolPreviewLen)
}

func isNumberedSourcePreview(output string) bool {
	lines := strings.Split(output, "\n")
	if len(lines) < 2 {
		return false
	}

	numbered := 0
	for _, line := range lines {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 2)
		if len(parts) != 2 {
			continue
		}
		if numberedSourceLineRE.MatchString(strings.TrimSpace(parts[0])) {
			numbered++
		}
	}
	return numbered >= 2 && numbered*2 >= len(lines)
}

func truncateSourcePreview(output string, maxLen int) string {
	if len(output) <= maxLen {
		return output
	}
	return strings.TrimRight(output[:maxLen], "\r\n") + "\n[... source preview truncated ...]"
}

func compactSystemdStatusPreview(output string) (string, bool) {
	lines := normalizedNonEmptyLines(output)
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "● ") || !strings.Contains(lines[0], ".service") {
		return "", false
	}

	serviceLine := strings.TrimPrefix(lines[0], "● ")
	service := serviceLine
	desc := ""
	if parts := strings.SplitN(serviceLine, " - ", 2); len(parts) == 2 {
		service = strings.TrimSpace(parts[0])
		desc = strings.TrimSpace(parts[1])
	}

	active := ""
	loaded := ""
	pid := ""
	memory := ""
	cpu := ""
	url := ""
	importantLogs := make([]string, 0, 4)
	seenLogs := map[string]bool{}

	for _, line := range lines[1:] {
		clean := stripJournalPrefix(strings.TrimSpace(line))
		lower := strings.ToLower(clean)
		switch {
		case strings.HasPrefix(clean, "Loaded:"):
			loaded = strings.TrimSpace(strings.TrimPrefix(clean, "Loaded:"))
		case strings.HasPrefix(clean, "Active:"):
			active = strings.TrimSpace(strings.TrimPrefix(clean, "Active:"))
		case strings.HasPrefix(clean, "Main PID:"):
			pid = strings.TrimSpace(strings.TrimPrefix(clean, "Main PID:"))
		case strings.HasPrefix(clean, "Memory:"):
			memory = strings.TrimSpace(strings.TrimPrefix(clean, "Memory:"))
		case strings.HasPrefix(clean, "CPU:"):
			cpu = strings.TrimSpace(strings.TrimPrefix(clean, "CPU:"))
		case strings.Contains(lower, "server is running at") || strings.Contains(lower, "started development server"):
			if u := extractFirstURLLike(clean); u != "" {
				url = u
			} else {
				importantLogs = appendUniqueLog(importantLogs, seenLogs, clean)
			}
		case strings.Contains(lower, "database initialized"), strings.Contains(lower, "migrated:"), strings.Contains(lower, "blockchain monitoring started"):
			importantLogs = appendUniqueLog(importantLogs, seenLogs, clean)
		case strings.Contains(lower, "failed"), strings.Contains(lower, "error"), strings.Contains(lower, "warning"):
			importantLogs = appendUniqueLog(importantLogs, seenLogs, clean)
		}
	}

	stateIcon := "⚪"
	if strings.Contains(strings.ToLower(active), "active (running)") {
		stateIcon = "🟢"
	} else if strings.Contains(strings.ToLower(active), "failed") {
		stateIcon = "🔴"
	} else if active != "" {
		stateIcon = "🟡"
	}

	parts := []string{"🧩 Service", service}
	if desc != "" {
		parts = append(parts, desc)
	}
	if active != "" {
		parts = append(parts, stateIcon+" "+compactSince(active))
	}
	if loaded != "" {
		parts = append(parts, "loaded "+firstFieldBeforeSemicolon(loaded))
	}
	if pid != "" {
		parts = append(parts, "pid "+pid)
	}
	if memory != "" {
		parts = append(parts, "mem "+memory)
	}
	if cpu != "" {
		parts = append(parts, "cpu "+cpu)
	}
	if url != "" {
		parts = append(parts, "url "+url)
	}
	if len(importantLogs) > 0 {
		parts = append(parts, "logs "+truncatePreview(strings.Join(importantLogs, " | "), 220))
	}
	return strings.Join(parts, " · "), true
}

func compactHTTPResponsePreview(output string) (string, bool) {
	firstLine := firstNonEmptyLine(output)
	if !strings.HasPrefix(firstLine, "HTTP/") {
		return "", false
	}
	contentType, server, cache, matchedPath, body := "", "", "", "", ""
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "content-type:"):
			contentType = strings.TrimSpace(line[len("content-type:"):])
		case strings.HasPrefix(lower, "server:"):
			server = strings.TrimSpace(line[len("server:"):])
		case strings.HasPrefix(lower, "x-vercel-cache:"):
			cache = strings.TrimSpace(line[len("x-vercel-cache:"):])
		case strings.HasPrefix(lower, "cf-cache-status:") && cache == "":
			cache = strings.TrimSpace(line[len("cf-cache-status:"):])
		case strings.HasPrefix(lower, "x-matched-path:"):
			matchedPath = strings.TrimSpace(line[len("x-matched-path:"):])
		case strings.HasPrefix(line, "{") || strings.HasPrefix(line, "["):
			body = line
		}
	}
	parts := []string{"🌐 HTTP", "status " + firstLine}
	if body != "" {
		parts = append(parts, "body "+truncatePreview(body, 120))
	}
	if contentType != "" {
		parts = append(parts, "type "+contentType)
	}
	if matchedPath != "" {
		parts = append(parts, "path "+matchedPath)
	}
	if cache != "" {
		parts = append(parts, "cache "+cache)
	}
	if server != "" {
		parts = append(parts, "server "+server)
	}
	return strings.Join(parts, " · "), true
}

func compactBuildLogPreview(output string) (string, bool) {
	lines := normalizedNonEmptyLines(output)
	buildLines := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(line, "Building:") {
			buildLines = append(buildLines, strings.TrimSpace(strings.TrimPrefix(line, "Building:")))
		}
	}
	if len(buildLines) == 0 {
		return "", false
	}
	status, runtime, framework, command, warning, current := "", "", "", "", "", ""
	for _, line := range buildLines {
		lower := strings.ToLower(line)
		switch {
		case strings.Contains(lower, "installing dependencies"):
			status = "installing dependencies"
		case strings.Contains(lower, "checked ") && strings.Contains(lower, "packages"):
			status = line
		case strings.Contains(lower, "detected next.js version"):
			framework = strings.TrimSpace(strings.TrimPrefix(line, "Detected "))
		case strings.HasPrefix(lower, "bun install"):
			runtime = line
		case strings.Contains(lower, "running "):
			command = strings.Trim(line[strings.Index(lower, "running ")+len("running "):], " \\\" ")
		case strings.HasPrefix(line, "$"):
			command = strings.TrimSpace(strings.TrimPrefix(line, "$"))
		case strings.HasPrefix(line, "⚠") || strings.Contains(lower, "warning") || strings.Contains(lower, "deprecated"):
			warning = strings.TrimSpace(line)
		case strings.Contains(lower, "creating an optimized production build"):
			current = "creating optimized production build"
		case strings.Contains(lower, "compiled successfully") || strings.Contains(lower, "build completed") || strings.Contains(lower, "success"):
			current = "build completed"
		}
	}
	if current == "" {
		current = status
	}
	if current == "" {
		current = buildLines[len(buildLines)-1]
	}
	parts := []string{"🏗️ Build", "status " + current}
	if framework != "" {
		parts = append(parts, framework)
	}
	if runtime != "" {
		parts = append(parts, runtime)
	}
	if command != "" {
		parts = append(parts, "cmd "+command)
	}
	if warning != "" {
		parts = append(parts, "warning "+truncatePreview(warning, 100))
	}
	return strings.Join(parts, " · "), true
}

func compactScraperPreview(output string) (string, bool) {
	lines := normalizedNonEmptyLines(output)
	if len(lines) == 0 {
		return "", false
	}

	joinedLower := strings.ToLower(strings.Join(lines, "\n"))
	if !strings.Contains(joinedLower, "scraping") && !strings.Contains(joinedLower, "found ") && !strings.Contains(joinedLower, "fetching reader api") {
		return "", false
	}
	if !strings.Contains(joinedLower, "manga") && !strings.Contains(joinedLower, "chapter") && !strings.Contains(joinedLower, "reader api") {
		return "", false
	}

	source, found, title, chapter, detailChapters := "", "", "", "", ""
	apiURL, failed, lastStep := "", "", ""
	sourceFoundRE := regexp.MustCompile(`(?i)found\s+(\d+)\s+manga\s+from\s+([A-Za-z0-9_-]+)`)
	popularRE := regexp.MustCompile(`(?i)^popular\s+(\d+)`)
	detailRE := regexp.MustCompile(`(?i)^detail\s+(.+?)\s+(\d+)\s+https?://`)
	fetchAPIRE := regexp.MustCompile(`(?i)fetching reader api:\s*(\S+)`)
	failedRE := regexp.MustCompile(`(?i)(failed[^\n]*?:\s*\d+|failed[^\n]*)`)
	chapterRE := regexp.MustCompile(`(?i)^chapter\s+(.+)$`)

	for _, line := range lines {
		clean := strings.TrimSpace(strings.Trim(line, "{}"))
		lower := strings.ToLower(clean)
		if m := sourceFoundRE.FindStringSubmatch(clean); len(m) == 3 {
			found = m[1]
			source = m[2]
		}
		if m := popularRE.FindStringSubmatch(clean); len(m) == 2 && found == "" {
			found = m[1]
		}
		if strings.HasPrefix(lower, "title:") && title == "" {
			title = cleanFieldValue(clean)
		}
		if strings.HasPrefix(lower, "chapter:") && chapter == "" {
			chapter = cleanFieldValue(clean)
		}
		if strings.HasPrefix(lower, "source:") && source == "" {
			source = cleanFieldValue(clean)
		}
		if m := detailRE.FindStringSubmatch(clean); len(m) == 3 {
			title = strings.TrimSpace(m[1])
			detailChapters = m[2]
		}
		if strings.Contains(lower, "scraping detail") {
			lastStep = "detail"
		}
		if strings.Contains(lower, "scraping chapter") {
			lastStep = "chapter"
		}
		if m := fetchAPIRE.FindStringSubmatch(clean); len(m) == 2 {
			apiURL = m[1]
		}
		if m := failedRE.FindStringSubmatch(clean); len(m) >= 2 {
			failed = strings.TrimSpace(m[1])
		}
		if m := chapterRE.FindStringSubmatch(clean); len(m) == 2 && strings.Contains(strings.ToLower(m[1]), "undefined") && failed == "" {
			failed = "chapter undefined"
		}
	}

	if source == "" && strings.Contains(joinedLower, "kiryuu") {
		source = "Kiryuu"
	}
	status, icon := "ok", "🕷️"
	if failed != "" || strings.Contains(joinedLower, "undefined") || strings.Contains(joinedLower, "404") {
		status, icon = "error", "⚠️"
	}

	parts := []string{icon + " Scraper", "status " + status}
	if source != "" {
		parts = append(parts, "source "+source)
	}
	if found != "" {
		parts = append(parts, "found "+found+" manga")
	}
	if title != "" {
		parts = append(parts, "title "+truncatePreview(title, 90))
	}
	if chapter != "" {
		parts = append(parts, "latest "+chapter)
	}
	if detailChapters != "" {
		parts = append(parts, "detail "+detailChapters+" chapters")
	}
	if lastStep != "" {
		parts = append(parts, "step "+lastStep)
	}
	if failed != "" {
		parts = append(parts, "error "+truncatePreview(failed, 90))
	}
	if apiURL != "" {
		parts = append(parts, "api "+truncatePreview(apiURL, 120))
	}
	return strings.Join(parts, " · "), true
}

func cleanFieldValue(line string) string {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return strings.TrimSpace(line)
	}
	v := strings.TrimSpace(line[idx+1:])
	v = strings.TrimSuffix(v, ",")
	v = strings.Trim(v, " \t\r\n\"'")
	return v
}

func normalizedNonEmptyLines(s string) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "▶"))
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func firstNonEmptyLine(s string) string {
	for _, line := range normalizedNonEmptyLines(s) {
		return line
	}
	return ""
}

func singleLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func truncatePreview(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

var journalPrefixRE = regexp.MustCompile(`^[A-Z][a-z]{2}\s+\d+\s+\d{2}:\d{2}:\d{2}\s+\S+\s+\S+\[\d+\]:\s*`)

func stripJournalPrefix(line string) string { return journalPrefixRE.ReplaceAllString(line, "") }

func appendUniqueLog(logs []string, seen map[string]bool, line string) []string {
	line = truncatePreview(singleLine(line), 120)
	if line == "" || seen[line] || len(logs) >= 4 {
		return logs
	}
	seen[line] = true
	return append(logs, line)
}

func extractFirstURLLike(s string) string {
	for _, field := range strings.Fields(s) {
		clean := strings.Trim(field, " .,;()[]{}\"'")
		if strings.HasPrefix(clean, "http://") || strings.HasPrefix(clean, "https://") {
			return clean
		}
		if strings.Count(clean, ":") == 1 && (strings.Contains(clean, "0.0.0.0:") || strings.Contains(clean, "127.0.0.1:") || strings.Contains(clean, "localhost:")) {
			return clean
		}
	}
	return ""
}

func compactSince(active string) string {
	if idx := strings.Index(active, ";"); idx >= 0 {
		return strings.TrimSpace(active[:idx])
	}
	return active
}

func firstFieldBeforeSemicolon(s string) string {
	if idx := strings.Index(s, ";"); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return s
}
