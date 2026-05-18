package platform

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const maxAutoVisualAttachments = 4

var fencedBlockRe = regexp.MustCompile("(?s)```([A-Za-z0-9_-]*)\\s*\\n(.*?)\\n```")

// autoVisualAttachmentsFromResponse detects visual-friendly content in a text
// response and produces local attachment files for chat platforms that cannot
// render interactive UI (Telegram, Discord, WhatsApp, etc.).
func autoVisualAttachmentsFromResponse(content string) []Attachment {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}

	var attachments []Attachment
	seen := map[string]bool{}
	add := func(att Attachment) {
		if att.FilePath == "" || seen[att.FilePath] || len(attachments) >= maxAutoVisualAttachments {
			return
		}
		seen[att.FilePath] = true
		attachments = append(attachments, att)
	}

	for _, block := range extractFencedBlocks(content) {
		lang := strings.ToLower(strings.TrimSpace(block.lang))
		body := strings.TrimSpace(block.body)
		if body == "" {
			continue
		}
		switch lang {
		case "mermaid", "mmd":
			if att, err := renderMermaidAttachment(body); err == nil {
				add(att)
			}
		case "svg", "xml":
			if looksLikeSVG(body) {
				if atts, err := renderSVGPreviewAttachments(body, "svg-preview"); err == nil {
					for _, att := range atts {
						add(att)
					}
				}
			}
		case "json":
			if att, err := renderJSONVisualAttachment(body); err == nil {
				add(att)
			}
		case "csv":
			if att, err := renderCSVTableAttachment(body); err == nil {
				add(att)
			}
		case "markdown", "md":
			if att, err := renderMarkdownDownloadAttachment(body, "contoh"); err == nil {
				add(att)
			}
		}
	}

	if len(attachments) < maxAutoVisualAttachments {
		if table := firstMarkdownTable(content); table != "" {
			if att, err := renderMarkdownTableAttachment(table); err == nil {
				add(att)
			}
		}
	}

	return attachments
}

type fencedBlock struct{ lang, body string }

func extractFencedBlocks(content string) []fencedBlock {
	matches := fencedBlockRe.FindAllStringSubmatch(content, -1)
	blocks := make([]fencedBlock, 0, len(matches))
	for _, m := range matches {
		if len(m) >= 3 {
			blocks = append(blocks, fencedBlock{lang: m[1], body: m[2]})
		}
	}
	return blocks
}

func renderMermaidAttachment(source string) (Attachment, error) {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(source))
	url := "https://mermaid.ink/img/" + encoded + "?type=png&bgColor=white"
	path, err := downloadAutoVisual(url, "mermaid", ".png")
	if err != nil {
		return writeAutoVisualFile("mermaid", ".mmd", []byte(source), "file", "text/plain")
	}
	return fileAttachment(path, "image", "image/png")
}

func renderJSONVisualAttachment(source string) (Attachment, error) {
	var v any
	if err := json.Unmarshal([]byte(source), &v); err != nil {
		return Attachment{}, err
	}
	rows, ok := jsonRows(v)
	if !ok || len(rows) == 0 {
		return Attachment{}, fmt.Errorf("json tidak cocok divisualkan")
	}
	return renderRowsAttachment(rows, "json-data")
}

func renderCSVTableAttachment(source string) (Attachment, error) {
	r := csv.NewReader(strings.NewReader(source))
	records, err := r.ReadAll()
	if err != nil || len(records) == 0 {
		return Attachment{}, fmt.Errorf("csv tidak valid")
	}
	return renderMatrixAttachment(records, "csv-table")
}

func renderMarkdownTableAttachment(table string) (Attachment, error) {
	lines := strings.Split(strings.TrimSpace(table), "\n")
	var rows [][]string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "|") || isMarkdownSeparatorRow(line) {
			continue
		}
		line = strings.Trim(line, "|")
		parts := strings.Split(line, "|")
		row := make([]string, 0, len(parts))
		for _, p := range parts {
			row = append(row, strings.TrimSpace(p))
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return Attachment{}, fmt.Errorf("table kosong")
	}
	return renderMatrixAttachment(rows, "markdown-table")
}

func jsonRows(v any) ([]map[string]string, bool) {
	arr, ok := v.([]any)
	if !ok {
		return nil, false
	}
	rows := make([]map[string]string, 0, len(arr))
	for _, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		row := map[string]string{}
		for k, val := range obj {
			row[k] = fmt.Sprint(val)
		}
		rows = append(rows, row)
	}
	return rows, true
}

func renderRowsAttachment(rows []map[string]string, name string) (Attachment, error) {
	colsMap := map[string]bool{}
	for _, row := range rows {
		for k := range row {
			colsMap[k] = true
		}
	}
	cols := make([]string, 0, len(colsMap))
	for k := range colsMap {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	matrix := [][]string{cols}
	for _, row := range rows {
		line := make([]string, len(cols))
		for i, col := range cols {
			line[i] = row[col]
		}
		matrix = append(matrix, line)
	}
	return renderMatrixAttachment(matrix, name)
}

func renderMatrixAttachment(rows [][]string, name string) (Attachment, error) {
	if len(rows) == 0 {
		return Attachment{}, fmt.Errorf("rows kosong")
	}
	const cellW, cellH = 170, 34
	cols := 0
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	if cols == 0 {
		return Attachment{}, fmt.Errorf("columns kosong")
	}
	if cols > 8 {
		cols = 8
	}
	shownRows := rows
	if len(shownRows) > 20 {
		shownRows = shownRows[:20]
	}
	width := cols*cellW + 32
	height := len(shownRows)*cellH + 72
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height)
	b.WriteString(`<rect width="100%" height="100%" fill="#0f172a"/>`)
	b.WriteString(`<text x="16" y="28" fill="#e2e8f0" font-family="Arial, sans-serif" font-size="16" font-weight="700">Smara Auto Visual</text>`)
	startY := 46
	for r, row := range shownRows {
		for c := 0; c < cols; c++ {
			x := 16 + c*cellW
			y := startY + r*cellH
			fill := "#1e293b"
			textFill := "#e2e8f0"
			weight := "400"
			if r == 0 {
				fill = "#7c3aed"
				textFill = "#ffffff"
				weight = "700"
			} else if r%2 == 0 {
				fill = "#111827"
			}
			fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s" stroke="#334155"/>`, x, y, cellW, cellH, fill)
			val := ""
			if c < len(row) {
				val = truncateVisualText(row[c], 22)
			}
			fmt.Fprintf(&b, `<text x="%d" y="%d" fill="%s" font-family="Arial, sans-serif" font-size="12" font-weight="%s">%s</text>`, x+8, y+22, textFill, weight, html.EscapeString(val))
		}
	}
	if len(rows) > len(shownRows) {
		fmt.Fprintf(&b, `<text x="16" y="%d" fill="#94a3b8" font-family="Arial, sans-serif" font-size="12">+ %d baris lainnya</text>`, height-16, len(rows)-len(shownRows))
	}
	b.WriteString(`</svg>`)
	if atts, err := renderSVGPreviewAttachments(b.String(), name); err == nil && len(atts) > 0 {
		return atts[0], nil
	}
	return writeAutoVisualFile(name, ".svg", []byte(b.String()), "file", "image/svg+xml")
}

func firstMarkdownTable(content string) string {
	lines := strings.Split(content, "\n")
	for i := 0; i < len(lines)-1; i++ {
		if strings.Contains(lines[i], "|") && isMarkdownSeparatorRow(lines[i+1]) {
			j := i
			var out []string
			for ; j < len(lines); j++ {
				line := strings.TrimSpace(lines[j])
				if !strings.Contains(line, "|") || line == "" {
					break
				}
				out = append(out, line)
			}
			return strings.Join(out, "\n")
		}
	}
	return ""
}

func isMarkdownSeparatorRow(line string) bool {
	line = strings.TrimSpace(strings.Trim(line, "|"))
	if line == "" {
		return false
	}
	for _, part := range strings.Split(line, "|") {
		p := strings.TrimSpace(part)
		p = strings.Trim(p, ":")
		if len(p) < 3 || strings.Trim(p, "-") != "" {
			return false
		}
	}
	return true
}

func truncateVisualText(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max-1]) + "…"
}

func downloadAutoVisual(url, prefix, ext string) (string, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	att, err := writeAutoVisualFile(prefix, ext, body, "image", "image/png")
	return att.FilePath, err
}

func writeAutoVisualFile(prefix, ext string, body []byte, typ, mime string) (Attachment, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Attachment{}, err
	}
	dir := filepath.Join(home, ".smara", "auto-visuals")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return Attachment{}, err
	}
	sum := sha1.Sum(body)
	name := fmt.Sprintf("%s-%s%s", sanitizeArtifactName(prefix), hex.EncodeToString(sum[:])[:12], ext)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, body, 0644); err != nil {
		return Attachment{}, err
	}
	return fileAttachment(path, typ, mime)
}

func fileAttachment(path, typ, mime string) (Attachment, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Attachment{}, err
	}
	return Attachment{Type: typ, FilePath: path, FileName: filepath.Base(path), MimeType: mime, Size: info.Size()}, nil
}

func looksLikeSVG(source string) bool {
	s := strings.TrimSpace(source)
	return strings.HasPrefix(strings.ToLower(s), "<svg") || strings.Contains(strings.ToLower(s), "<svg ")
}

func renderSVGPreviewAttachments(source, name string) ([]Attachment, error) {
	svgAtt, err := writeAutoVisualFile(name, ".svg", []byte(source), "file", "image/svg+xml")
	if err != nil {
		return nil, err
	}
	pngPath := strings.TrimSuffix(svgAtt.FilePath, filepath.Ext(svgAtt.FilePath)) + ".png"
	if err := convertSVGToPNG(svgAtt.FilePath, pngPath); err != nil {
		return []Attachment{svgAtt}, nil
	}
	pngAtt, err := fileAttachment(pngPath, "image", "image/png")
	if err != nil {
		return []Attachment{svgAtt}, nil
	}
	return []Attachment{pngAtt, svgAtt}, nil
}

func convertSVGToPNG(svgPath, pngPath string) error {
	converters := [][]string{{"rsvg-convert", "-f", "png", "-o", pngPath, svgPath}, {"magick", svgPath, pngPath}, {"convert", svgPath, pngPath}, {"inkscape", svgPath, "--export-type=png", "--export-filename=" + pngPath}}
	var lastErr error
	for _, args := range converters {
		bin := args[0]
		if _, err := exec.LookPath(bin); err != nil {
			lastErr = err
			continue
		}
		cmd := exec.Command(bin, args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			lastErr = fmt.Errorf("%s: %w: %s", bin, err, strings.TrimSpace(string(out)))
			continue
		}
		if info, err := os.Stat(pngPath); err == nil && info.Size() > 0 {
			return nil
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("converter SVG ke PNG tidak ditemukan")
	}
	return lastErr
}

func renderMarkdownDownloadAttachment(source, name string) (Attachment, error) {
	base := sanitizeArtifactName(name)
	if base == "" {
		base = "contoh"
	}
	return writeAutoVisualFile(base, ".md", []byte(source), "file", "text/markdown; charset=utf-8")
}

func sanitizeArtifactName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
