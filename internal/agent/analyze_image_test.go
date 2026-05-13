package agent

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makePNG creates a small synthetic PNG (16x16 with text-like pixels)
// at the given path so analyzeImageFile has something real to inspect.
func makePNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for x := 0; x < 32; x++ {
		for y := 0; y < 32; y++ {
			c := color.RGBA{R: uint8(x * 8), G: uint8(y * 8), B: 100, A: 255}
			img.Set(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestAnalyzeImageFile_PNG(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sample.png")
	makePNG(t, p)

	out, err := analyzeImageFile(p, "eng", true)
	if err != nil {
		t.Fatalf("analyzeImageFile: %v", err)
	}
	if !strings.Contains(out, "sample.png") {
		t.Errorf("expected filename in output, got: %s", out)
	}
	if !strings.Contains(out, "32x32") {
		t.Errorf("expected dimensions 32x32 in output, got: %s", out)
	}
	if !strings.Contains(out, "Format") {
		t.Errorf("expected Format line in metadata")
	}
}

func TestAnalyzeImageFile_StripsImageWrapper(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "wrap.png")
	makePNG(t, p)

	// Pass with the [image:...] wrapper; analyzeImageFile would normally
	// receive plain path, but the wrapper-strip logic is in the dispatcher
	// (ExecuteBuiltinTool). We only verify that without the wrapper it works.
	if _, err := analyzeImageFile(p, "eng", true); err != nil {
		t.Errorf("plain path should succeed: %v", err)
	}
}

func TestAnalyzeImageFile_Errors(t *testing.T) {
	if _, err := analyzeImageFile("/does/not/exist.png", "eng", true); err == nil {
		t.Error("expected error for non-existent file")
	}

	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := analyzeImageFile(subdir, "eng", true); err == nil {
		t.Error("expected error when path is a directory")
	}

	// Wrong extension
	bad := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(bad, []byte("not an image"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := analyzeImageFile(bad, "eng", true); err == nil {
		t.Error("expected error for unsupported extension")
	}
}

func TestStripTesseractNoise(t *testing.T) {
	in := `Tesseract Open Source OCR Engine v5.3.0 with Leptonica
Estimating resolution as 96
Hello World
This is the actual text.`
	out := stripTesseractNoise(in)
	if strings.Contains(out, "Estimating resolution") {
		t.Error("expected resolution noise to be stripped")
	}
	if strings.Contains(out, "Tesseract Open Source") {
		t.Error("expected version banner to be stripped")
	}
	if !strings.Contains(out, "Hello World") {
		t.Error("expected real OCR output to remain")
	}
}
