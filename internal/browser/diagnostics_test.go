package browser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScreenshotForTargetFallbackReportAttachments(t *testing.T) {
	res := Result{
		ID:             "test",
		Prompt:         "Buka browser http://localhost:3000 dan ambil screenshot",
		URL:            "http://localhost:3000",
		ArtifactDir:    t.TempDir(),
		ScreenshotPath: filepath.Join(t.TempDir(), "navbar-mobile.png"),
		Status:         "pass",
		StartedAt:      time.Now(),
		FinishedAt:     time.Now(),
		ConsoleErrors:  []string{"error: boom"},
		NetworkErrors:  []string{"req: failed"},
	}
	res.ReportPath = filepath.Join(res.ArtifactDir, "report.md")
	if err := os.WriteFile(res.ScreenshotPath, []byte("png"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := WriteReport(res); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(res.ReportPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{"Console Errors", "Network Errors", "navbar-mobile.png", "Status: `pass`"} {
		if !strings.Contains(text, want) {
			t.Fatalf("report tidak mengandung %q:\n%s", want, text)
		}
	}
}
