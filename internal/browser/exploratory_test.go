package browser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlanCheckoutUsesErrorCheck(t *testing.T) {
	task, err := Plan("Tolong navigasikan browser ke halaman checkout di http://localhost:8000/checkout. Cobalah klik tombol 'Bayar' tanpa mengisi form data diri. Periksa apakah peringatan error merah muncul lalu ambil screenshot.")
	if err != nil {
		t.Fatal(err)
	}
	var hasClick, hasErrorCheck bool
	for _, step := range task.Steps {
		if step.Action == "click" && step.Target == "Bayar" {
			hasClick = true
		}
		if step.Action == "error-check" && step.Target == "checkout-error" {
			hasErrorCheck = true
		}
	}
	if !hasClick || !hasErrorCheck {
		t.Fatalf("steps=%+v", task.Steps)
	}
}

func TestWriteReportIncludesExploratoryErrorCheckRecommendation(t *testing.T) {
	dir := t.TempDir()
	res := Result{
		Prompt:      "checkout bayar kosong",
		URL:         "http://localhost:8000/checkout",
		ArtifactDir: dir,
		ReportPath:  filepath.Join(dir, "report.md"),
		Status:      "fail",
		StartedAt:   time.Now(),
		FinishedAt:  time.Now(),
		Steps:       []StepResult{{Step: Step{Action: "error-check", Target: "checkout-error"}, Status: "fail"}},
		ErrorChecks: []ErrorCheck{{Target: "checkout-error", FoundError: false, HasRedStyle: false, Status: "fail", Error: "pesan error tidak ditemukan"}},
	}
	if err := WriteReport(res); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(res.ReportPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{"Exploratory Error Checks", "checkout-error", "Recommendations", "validasi form"} {
		if !strings.Contains(text, want) {
			t.Fatalf("report missing %q:\n%s", want, text)
		}
	}
}
