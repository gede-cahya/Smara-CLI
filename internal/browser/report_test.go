package browser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteReportMasksPasswordTarget(t *testing.T) {
	dir := t.TempDir()
	res := Result{
		Prompt:         "login password 'secret'",
		URL:            "http://localhost:3000",
		ArtifactDir:    dir,
		ReportPath:     filepath.Join(dir, "report.md"),
		ScreenshotPath: filepath.Join(dir, "dashboard.png"),
		Status:         "pass",
		StartedAt:      time.Now(),
		FinishedAt:     time.Now(),
		Steps:          []StepResult{{Step: Step{Action: "fill", Target: "password", Value: "secret"}, Status: "pass"}},
	}
	if err := WriteReport(res); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(res.ReportPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "dashboard.png") {
		t.Fatal("missing screenshot")
	}
	if strings.Contains(s, "secret") {
		t.Fatal("secret leaked")
	}
}
