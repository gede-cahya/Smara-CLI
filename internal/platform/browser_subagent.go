package platform

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/browser"
)

func (g *Gateway) processBrowserPrompt(ctx context.Context, msg IncomingMessage) error {
	g.mu.RLock()
	adapter, ok := g.adapters[msg.Platform]
	g.mu.RUnlock()
	if !ok {
		return fmt.Errorf("adapter tidak ditemukan: %s", msg.Platform)
	}

	statusMsg := OutgoingMessage{Content: RenderStatusMessage(msg.Platform, "executing", "Menjalankan Browser Subagent...", 0), Format: FormatMarkdown}
	statusID, _ := adapter.SendMessageWithID(ctx, msg.ChannelID, statusMsg)
	_ = adapter.SendTyping(ctx, msg.ChannelID)

	if strings.Contains(strings.ToLower(msg.Content), "localhost") && msg.Platform == "discord" {
		// continue, but make ambiguity explicit in final response
	}

	task, err := browser.Plan(msg.Content)
	if err != nil {
		if statusID != "" {
			_ = adapter.EditMessage(ctx, msg.ChannelID, statusID, OutgoingMessage{Content: "❌ Browser Subagent gagal membuat plan: " + err.Error(), Format: FormatPlain})
			return nil
		}
		return g.sendReply(ctx, msg, "❌ Browser Subagent gagal membuat plan: "+err.Error())
	}

	runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	res, runErr := browser.Run(runCtx, task, browser.Options{Timeout: 45 * time.Second})

	if statusID != "" {
		_ = adapter.EditMessage(ctx, msg.ChannelID, statusID, OutgoingMessage{Content: "✅ Browser Subagent selesai", Format: FormatPlain})
	}

	content := browserDiscordSummary(res, runErr, msg.Content)
	attachments := browserResultAttachments(res)
	return g.sendReplyWithAttachments(ctx, msg, content, attachments)
}

func browserDiscordSummary(res browser.Result, err error, prompt string) string {
	status := res.Status
	if status == "" {
		status = "fail"
	}
	var b strings.Builder
	b.WriteString("🧭 **Browser Subagent Result**\n\n")
	if strings.Contains(strings.ToLower(prompt), "localhost") {
		b.WriteString("⚠️ Catatan: `localhost` mengarah ke mesin tempat bot Smara berjalan, bukan laptop user Discord.\n\n")
	}
	b.WriteString(fmt.Sprintf("- Status: `%s`\n", status))
	if res.URL != "" {
		b.WriteString(fmt.Sprintf("- URL: %s\n", res.URL))
	}
	if res.ArtifactDir != "" {
		b.WriteString(fmt.Sprintf("- Artifacts: `%s`\n", res.ArtifactDir))
	}
	if res.ScreenshotPath != "" {
		b.WriteString(fmt.Sprintf("- Screenshot: `%s`\n", filepath.Base(res.ScreenshotPath)))
	}
	if res.ReportPath != "" {
		b.WriteString(fmt.Sprintf("- Report: `%s`\n", filepath.Base(res.ReportPath)))
	}
	if len(res.ConsoleErrors) > 0 {
		b.WriteString(fmt.Sprintf("- Console errors: `%d`\n", len(res.ConsoleErrors)))
	}
	if len(res.NetworkErrors) > 0 {
		b.WriteString(fmt.Sprintf("- Network errors: `%d`\n", len(res.NetworkErrors)))
	}
	if err != nil {
		b.WriteString("\n❌ Error: " + err.Error() + "\n")
	}
	b.WriteString("\nSaya lampirkan screenshot PNG dan `report.md` jika berhasil dibuat.")
	return b.String()
}

func browserResultAttachments(res browser.Result) []Attachment {
	var atts []Attachment
	seen := map[string]bool{}
	add := func(kind, path, mime string) {
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		atts = append(atts, Attachment{Type: kind, FilePath: path, FileName: filepath.Base(path), MimeType: mime})
	}
	add("image", res.ScreenshotPath, "image/png")
	for _, vc := range res.VisualChecks {
		add("image", vc.ScreenshotPath, "image/png")
	}
	for _, ec := range res.ErrorChecks {
		add("image", ec.ScreenshotPath, "image/png")
	}
	add("file", res.ReportPath, "text/markdown")
	add("file", res.RunJSONPath, "application/json")
	return atts
}
