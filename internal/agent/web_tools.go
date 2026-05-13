package agent

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ---- Web fetch ------------------------------------------------------------

// fetchWebPage downloads a single URL and returns readable plain text after
// stripping scripts, styles, and HTML tags. Size-capped to prevent huge
// pages from blowing up the LLM context.
//
// Flow: GET URL → strip <script>/<style>/<nav>/<header>/<footer> blocks →
// collapse whitespace → convert tags to newlines → trim → cap at maxChars.
func fetchWebPage(rawURL string, maxChars int) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", fmt.Errorf("url wajib diisi")
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("url tidak valid (butuh http/https): %q", rawURL)
	}
	if maxChars <= 0 || maxChars > 200000 {
		maxChars = 20000
	}

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "id-ID,id;q=0.9,en;q=0.8")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gagal fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d dari %s", resp.StatusCode, rawURL)
	}

	// Cap to 200 KB raw body to protect memory.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 200*1024))
	if err != nil {
		return "", fmt.Errorf("gagal baca body: %w", err)
	}

	ct := resp.Header.Get("Content-Type")
	text := string(body)
	if strings.Contains(strings.ToLower(ct), "json") {
		// JSON is already readable — return as-is (capped).
		return capText(text, maxChars, rawURL, ct), nil
	}

	text = htmlToText(text)
	return capText(text, maxChars, rawURL, ct), nil
}

// capText truncates and adds a short header so the LLM knows origin & type.
func capText(body string, maxChars int, origin, ct string) string {
	body = strings.TrimSpace(body)
	truncated := false
	if len(body) > maxChars {
		body = body[:maxChars]
		truncated = true
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Fetched: %s\nContent-Type: %s\n", origin, ct)
	if truncated {
		fmt.Fprintf(&sb, "(dipotong ke %d karakter dari total halaman)\n", maxChars)
	}
	sb.WriteString("----\n")
	sb.WriteString(body)
	return sb.String()
}

// htmlToText strips scripts, styles, and tags; collapses whitespace.
func htmlToText(html string) string {
	// Remove script / style / noscript / svg / iframe / nav / header / footer
	// blocks including their content.
	blocks := []string{"script", "style", "noscript", "svg", "iframe", "nav", "header", "footer", "form"}
	for _, tag := range blocks {
		re := regexp.MustCompile(`(?is)<` + tag + `\b[^>]*>.*?</` + tag + `>`)
		html = re.ReplaceAllString(html, " ")
	}
	// Convert <br> and block-level closes to newlines for readability.
	breakRe := regexp.MustCompile(`(?i)<br\s*/?>|</(p|div|h[1-6]|li|tr|section|article)>`)
	html = breakRe.ReplaceAllString(html, "\n")

	// Strip all remaining tags.
	tagRe := regexp.MustCompile(`<[^>]+>`)
	html = tagRe.ReplaceAllString(html, "")

	// Decode a handful of common HTML entities.
	html = decodeEntities(html)

	// Collapse excessive whitespace.
	html = regexp.MustCompile(`[ \t]+`).ReplaceAllString(html, " ")
	html = regexp.MustCompile(`\n\s*\n\s*\n+`).ReplaceAllString(html, "\n\n")
	return strings.TrimSpace(html)
}

func decodeEntities(s string) string {
	replacements := map[string]string{
		"&amp;":  "&",
		"&lt;":   "<",
		"&gt;":   ">",
		"&quot;": "\"",
		"&apos;": "'",
		"&#39;":  "'",
		"&nbsp;": " ",
		"&hellip;": "…",
		"&mdash;": "—",
		"&ndash;": "–",
		"&rsquo;": "'",
		"&lsquo;": "'",
		"&rdquo;": "\"",
		"&ldquo;": "\"",
	}
	for old, new := range replacements {
		s = strings.ReplaceAll(s, old, new)
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

// ---- Export data ----------------------------------------------------------

// exportData writes a structured dataset (array of objects) into a file in
// CSV, JSON, Markdown table, or PDF format. Returns a human-readable
// status string with the final path so the LLM can tell the user where
// the file lives.
func exportData(args map[string]interface{}) (string, error) {
	format := strings.ToLower(strings.TrimSpace(getStr(args, "format")))
	if format == "markdown" {
		format = "md"
	}
	switch format {
	case "csv", "json", "md", "pdf":
	default:
		return "", fmt.Errorf("format harus salah satu dari: csv, json, md, pdf (got %q)", format)
	}

	rawData, ok := args["data"].([]interface{})
	if !ok {
		return "", fmt.Errorf("argumen 'data' wajib berupa array of objects")
	}
	if len(rawData) == 0 {
		return "", fmt.Errorf("'data' tidak boleh kosong")
	}

	// Normalize into []map[string]interface{}
	rows := make([]map[string]interface{}, 0, len(rawData))
	for i, r := range rawData {
		m, ok := r.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("data[%d] bukan object", i)
		}
		rows = append(rows, m)
	}

	// Determine column order.
	var columns []string
	if raw, ok := args["columns"].([]interface{}); ok && len(raw) > 0 {
		for _, c := range raw {
			if s, ok := c.(string); ok && s != "" {
				columns = append(columns, s)
			}
		}
	}
	if len(columns) == 0 {
		// Infer from union of keys across rows (stable ordering: first-seen wins).
		seen := map[string]bool{}
		for _, r := range rows {
			keys := make([]string, 0, len(r))
			for k := range r {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if !seen[k] {
					seen[k] = true
					columns = append(columns, k)
				}
			}
		}
	}

	title := strings.TrimSpace(getStr(args, "title"))
	path := strings.TrimSpace(getStr(args, "path"))
	if path == "" {
		ts := time.Now().Format("20060102-150405")
		ext := format
		if format == "md" {
			ext = "md"
		}
		path = filepath.Join(os.TempDir(), fmt.Sprintf("export-%s.%s", ts, ext))
	}

	// Ensure parent dir exists.
	if dir := filepath.Dir(path); dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}

	switch format {
	case "csv":
		if err := writeCSV(path, columns, rows); err != nil {
			return "", err
		}
	case "json":
		payload := rows
		if err := writeJSON(path, payload); err != nil {
			return "", err
		}
	case "md":
		if err := writeMarkdown(path, title, columns, rows); err != nil {
			return "", err
		}
	case "pdf":
		result, err := writePDF(path, title, columns, rows)
		if err != nil {
			return "", err
		}
		return result, nil
	}

	info, _ := os.Stat(path)
	size := int64(0)
	if info != nil {
		size = info.Size()
	}
	return fmt.Sprintf("✓ Export %s: %d baris, %d kolom → %s (%d bytes)", format, len(rows), len(columns), path, size), nil
}

func writeCSV(path string, columns []string, rows []map[string]interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("gagal buat file: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write(columns); err != nil {
		return err
	}
	for _, r := range rows {
		rec := make([]string, len(columns))
		for i, c := range columns {
			rec[i] = valueToString(r[c])
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	return w.Error()
}

func writeJSON(path string, payload interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("gagal buat file: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func writeMarkdown(path, title string, columns []string, rows []map[string]interface{}) error {
	var sb strings.Builder
	if title != "" {
		sb.WriteString("# " + title + "\n\n")
	}
	sb.WriteString("| " + strings.Join(columns, " | ") + " |\n")
	sb.WriteString("| " + strings.Repeat("--- | ", len(columns)) + "\n")
	for _, r := range rows {
		cells := make([]string, len(columns))
		for i, c := range columns {
			cells[i] = strings.ReplaceAll(valueToString(r[c]), "|", "\\|")
			cells[i] = strings.ReplaceAll(cells[i], "\n", " ")
		}
		sb.WriteString("| " + strings.Join(cells, " | ") + " |\n")
	}
	sb.WriteString(fmt.Sprintf("\n_Generated by Smara at %s (%d rows)_\n", time.Now().Format(time.RFC3339), len(rows)))
	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// writePDF tries pandoc first, falls back to wkhtmltopdf. If neither tool
// is installed, writes the Markdown version and returns a status that
// tells the user how to install a PDF converter. That way the data is
// never lost.
func writePDF(pdfPath, title string, columns []string, rows []map[string]interface{}) (string, error) {
	// Write a temporary Markdown first, then let external tool convert.
	mdPath := strings.TrimSuffix(pdfPath, ".pdf") + ".md"
	if err := writeMarkdown(mdPath, title, columns, rows); err != nil {
		return "", err
	}

	if _, err := exec.LookPath("pandoc"); err == nil {
		// pandoc with xelatex for Unicode support; fall back to plain output
		// if xelatex isn't installed.
		cmd := exec.Command("pandoc", mdPath, "-o", pdfPath, "--pdf-engine=xelatex")
		out, err := cmd.CombinedOutput()
		if err != nil {
			// Retry without PDF engine (wkhtmltopdf path inside pandoc).
			cmd = exec.Command("pandoc", mdPath, "-o", pdfPath)
			out, err = cmd.CombinedOutput()
			if err != nil {
				return fmt.Sprintf("⚠ pandoc gagal: %s\n\nData sudah disimpan sebagai Markdown di %s. Install xelatex atau wkhtmltopdf untuk PDF.", strings.TrimSpace(string(out)), mdPath), nil
			}
		}
		info, _ := os.Stat(pdfPath)
		size := int64(0)
		if info != nil {
			size = info.Size()
		}
		return fmt.Sprintf("✓ Export PDF via pandoc: %d baris → %s (%d bytes). Markdown sumber: %s", len(rows), pdfPath, size, mdPath), nil
	}

	if _, err := exec.LookPath("wkhtmltopdf"); err == nil {
		// Convert MD → HTML with a simple wrapper first.
		htmlPath := strings.TrimSuffix(pdfPath, ".pdf") + ".html"
		if err := writeHTMLFromRows(htmlPath, title, columns, rows); err == nil {
			cmd := exec.Command("wkhtmltopdf", htmlPath, pdfPath)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Sprintf("⚠ wkhtmltopdf gagal: %s\n\nHTML sumber: %s, Markdown sumber: %s", strings.TrimSpace(string(out)), htmlPath, mdPath), nil
			}
			info, _ := os.Stat(pdfPath)
			size := int64(0)
			if info != nil {
				size = info.Size()
			}
			return fmt.Sprintf("✓ Export PDF via wkhtmltopdf: %d baris → %s (%d bytes)", len(rows), pdfPath, size), nil
		}
	}

	return fmt.Sprintf("⚠ Tidak ada pandoc / wkhtmltopdf di sistem — PDF tidak bisa dibuat. Data sudah disimpan sebagai Markdown di %s. Install: `sudo apt install pandoc texlive-xetex` atau `sudo apt install wkhtmltopdf`.", mdPath), nil
}

func writeHTMLFromRows(path, title string, columns []string, rows []map[string]interface{}) error {
	var sb strings.Builder
	sb.WriteString("<!doctype html><html><head><meta charset='utf-8'>")
	sb.WriteString("<style>body{font-family:sans-serif;padding:24px;}table{border-collapse:collapse;width:100%;}th,td{border:1px solid #444;padding:6px 10px;text-align:left;}th{background:#eee;}</style>")
	if title != "" {
		sb.WriteString("<title>" + escapeHTML(title) + "</title>")
	}
	sb.WriteString("</head><body>")
	if title != "" {
		sb.WriteString("<h1>" + escapeHTML(title) + "</h1>")
	}
	sb.WriteString("<table><thead><tr>")
	for _, c := range columns {
		sb.WriteString("<th>" + escapeHTML(c) + "</th>")
	}
	sb.WriteString("</tr></thead><tbody>")
	for _, r := range rows {
		sb.WriteString("<tr>")
		for _, c := range columns {
			sb.WriteString("<td>" + escapeHTML(valueToString(r[c])) + "</td>")
		}
		sb.WriteString("</tr>")
	}
	sb.WriteString("</tbody></table>")
	sb.WriteString(fmt.Sprintf("<p><small>Generated by Smara at %s (%d rows)</small></p>", time.Now().Format(time.RFC3339), len(rows)))
	sb.WriteString("</body></html>")
	return os.WriteFile(path, []byte(sb.String()), 0644)
}

func escapeHTML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	)
	return r.Replace(s)
}

// valueToString converts any JSON-decoded value into a display string.
func valueToString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		// Drop trailing .0 for integers so CSV cells look clean.
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	case []interface{}:
		parts := make([]string, len(t))
		for i, x := range t {
			parts[i] = valueToString(x)
		}
		return strings.Join(parts, ", ")
	case map[string]interface{}:
		b, _ := json.Marshal(t)
		return string(b)
	default:
		return fmt.Sprintf("%v", v)
	}
}


// isBlockingError returns true if the HTTP error looks like a case where
// switching to a headless browser might succeed: 403, 503, 429, or any
// mention of Cloudflare in the error text. Timeouts and DNS failures are
// NOT blocking errors — we don't retry those with a browser.
func isBlockingError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	// HTTP status codes that often accompany anti-bot pages.
	statusMarkers := []string{"403", "429", "503", "451", "520", "521", "522", "526"}
	for _, m := range statusMarkers {
		if strings.Contains(msg, "http "+m) || strings.Contains(msg, " "+m+" ") {
			return true
		}
	}
	antiBotMarkers := []string{"cloudflare", "captcha", "forbidden", "blocked"}
	for _, m := range antiBotMarkers {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}
