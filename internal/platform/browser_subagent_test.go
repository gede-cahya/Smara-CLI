package platform

import (
	"strings"
	"testing"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/browser"
)

func TestBrowserDiscordSummaryWarnsLocalhostAndListsArtifacts(t *testing.T) {
	res := browser.Result{
		Prompt:         "Buka browser http://localhost:3000 dan ambil screenshot",
		URL:            "http://localhost:3000",
		ArtifactDir:    "/tmp/run",
		ScreenshotPath: "/tmp/run/dashboard.png",
		ReportPath:     "/tmp/run/report.md",
		Status:         "pass",
		StartedAt:      time.Now(),
		FinishedAt:     time.Now(),
	}
	out := browserDiscordSummary(res, nil, res.Prompt)
	for _, want := range []string{"localhost", "dashboard.png", "report.md", "Status: `pass`"} {
		if !strings.Contains(out, want) {
			t.Fatalf("summary tidak mengandung %q:\n%s", want, out)
		}
	}
}

func TestBrowserResultAttachments(t *testing.T) {
	res := browser.Result{ScreenshotPath: "/tmp/a.png", ReportPath: "/tmp/report.md"}
	atts := browserResultAttachments(res)
	if len(atts) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(atts))
	}
	if atts[0].MimeType != "image/png" || atts[1].MimeType != "text/markdown" {
		t.Fatalf("unexpected mime types: %#v", atts)
	}
}
