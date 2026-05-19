package browser

import (
	"encoding/json"
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
	browserName := res.Browser
	if browserName == "" {
		browserName = "Chromium via go-rod"
	}
	mode := res.Mode
	if mode == "" {
		mode = "headless"
	}
	b.WriteString(fmt.Sprintf("- Browser: %s\n", browserName))
	b.WriteString(fmt.Sprintf("- Mode: %s\n", mode))
	if res.Viewport.Width > 0 && res.Viewport.Height > 0 {
		b.WriteString(fmt.Sprintf("- Viewport: %s (%dx%d)\n", res.Viewport.Name, res.Viewport.Width, res.Viewport.Height))
	}
	b.WriteString(fmt.Sprintf("- Started: %s\n", res.StartedAt.Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("- Finished: %s\n", res.FinishedAt.Format("2006-01-02 15:04:05")))
	if res.RunJSONPath != "" {
		b.WriteString(fmt.Sprintf("- Metadata: `%s`\n", res.RunJSONPath))
	}
	b.WriteString("\n")

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
	wroteShot := false
	if res.ScreenshotPath != "" {
		name := filepath.Base(res.ScreenshotPath)
		b.WriteString(fmt.Sprintf("![Screenshot](./%s)\n\n", name))
		b.WriteString(fmt.Sprintf("File: `%s`\n\n", res.ScreenshotPath))
		wroteShot = true
	}
	for _, vc := range res.VisualChecks {
		if vc.ScreenshotPath != "" {
			name := filepath.Base(vc.ScreenshotPath)
			b.WriteString(fmt.Sprintf("![%s %s](./%s)\n\n", vc.Target, vc.Viewport.Name, name))
			wroteShot = true
		}
	}
	for _, ec := range res.ErrorChecks {
		if ec.ScreenshotPath != "" && ec.ScreenshotPath != res.ScreenshotPath {
			name := filepath.Base(ec.ScreenshotPath)
			b.WriteString(fmt.Sprintf("![%s](./%s)\n\n", ec.Target, name))
			wroteShot = true
		}
	}
	if !wroteShot {
		b.WriteString("Tidak ada screenshot.\n\n")
	}
	if len(res.VisualChecks) > 0 {
		b.WriteString("## Visual Checks\n\n")
		b.WriteString("| Target | Viewport | Screenshot | Found Target | Horizontal Overflow | Status |\n|---|---|---|---|---|---|\n")
		for _, vc := range res.VisualChecks {
			shot := "-"
			if vc.ScreenshotPath != "" {
				shot = "./" + filepath.Base(vc.ScreenshotPath)
			}
			b.WriteString(fmt.Sprintf("| %s | %s (%dx%d) | %s | %t | %t | %s |\n", vc.Target, vc.Viewport.Name, vc.Viewport.Width, vc.Viewport.Height, shot, vc.FoundTarget, vc.HorizontalOverflow, vc.Status))
		}
		if res.VisualCheckPath != "" {
			b.WriteString(fmt.Sprintf("\nMetadata: `%s`\n", res.VisualCheckPath))
		}
		b.WriteString("\n")
	}
	if len(res.ErrorChecks) > 0 {
		b.WriteString("## Exploratory Error Checks\n\n")
		b.WriteString("| Target | Found Error | Red Style | Screenshot | Status |\n|---|---|---|---|---|\n")
		for _, ec := range res.ErrorChecks {
			shot := "-"
			if ec.ScreenshotPath != "" {
				shot = "./" + filepath.Base(ec.ScreenshotPath)
			}
			status := ec.Status
			if ec.Error != "" {
				status += ": " + ec.Error
			}
			b.WriteString(fmt.Sprintf("| %s | %t | %t | %s | %s |\n", ec.Target, ec.FoundError, ec.HasRedStyle, shot, status))
		}
		if res.ErrorCheckPath != "" {
			b.WriteString(fmt.Sprintf("\nMetadata: `%s`\n", res.ErrorCheckPath))
		}
		b.WriteString("\n")
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
	if rec := recommendation(res); rec != "" {
		b.WriteString("## Recommendations\n\n")
		b.WriteString(rec + "\n\n")
	}
	b.WriteString("## Discord Ready\n\n")
	b.WriteString("Lampirkan `report.md` dan screenshot `.png` dari folder artifact ini saat mengirim hasil ke Discord.\n\n")
	b.WriteString("## Result\n\n")
	b.WriteString("Status: `" + res.Status + "`\n")
	return os.WriteFile(res.ReportPath, []byte(b.String()), 0644)
}

func WriteRunJSON(res Result) error {
	if res.RunJSONPath == "" {
		return nil
	}
	b, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(res.RunJSONPath, b, 0644)
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

func recommendation(res Result) string {
	var items []string
	for _, vc := range res.VisualChecks {
		if vc.HorizontalOverflow {
			items = append(items, "- Perbaiki horizontal overflow pada viewport "+vc.Viewport.Name+" dengan mengecek width fixed, overflow-x, dan responsive breakpoint.")
		}
		if !vc.FoundTarget {
			items = append(items, "- Target visual `"+vc.Target+"` tidak ditemukan. Tambahkan semantic selector seperti `nav`, `header`, atau `role=navigation` agar komponen mudah divalidasi.")
		}
	}
	for _, ec := range res.ErrorChecks {
		if !ec.FoundError {
			items = append(items, "- Tambahkan validasi form yang menampilkan pesan error saat field wajib kosong.")
		} else if !ec.HasRedStyle {
			items = append(items, "- Pesan error sudah muncul, tetapi styling merah/class error belum terdeteksi. Pastikan warna, role alert, atau class invalid/error konsisten.")
		}
	}
	if len(res.ConsoleErrors) > 0 {
		items = append(items, "- Perbaiki error JavaScript di console sebelum melanjutkan validasi UI.")
	}
	if len(res.NetworkErrors) > 0 {
		items = append(items, "- Periksa failed network request/API agar alur E2E stabil.")
	}
	return strings.Join(items, "\n")
}
