package agent

import (
	"context"
	"fmt"
	"image"
	_ "image/gif" // register decoders
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// analyzeImageFile inspects an image file. It always returns metadata; if
// tesseract is available, OCR text is included. The result is plain text the
// agent can reason over.
func analyzeImageFile(path, ocrLang string, includeMeta bool) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("path tidak valid: %w", err)
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("file tidak ditemukan: %w", err)
	}
	if st.IsDir() {
		return "", fmt.Errorf("path adalah direktori, bukan file gambar")
	}

	ext := strings.ToLower(filepath.Ext(abs))
	supported := map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
		".bmp": true, ".webp": true, ".tiff": true,
	}
	if !supported[ext] {
		return "", fmt.Errorf("format %s tidak didukung (gunakan png/jpg/jpeg/gif/bmp/webp/tiff)", ext)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("📸 Image Analysis — %s\n\n", filepath.Base(abs)))

	if includeMeta {
		writeImageMetadata(&b, abs, st)
	}

	// OCR via tesseract (best-effort, optional).
	if ocrText, err := runTesseract(abs, ocrLang); err == nil && strings.TrimSpace(ocrText) != "" {
		b.WriteString("\n── OCR Text (tesseract) ──\n")
		b.WriteString(strings.TrimSpace(ocrText))
		b.WriteString("\n")
	} else if err != nil {
		b.WriteString("\n── OCR ──\n")
		b.WriteString(fmt.Sprintf("OCR tidak tersedia: %v\n", err))
		b.WriteString("Untuk extract text dari gambar, install tesseract:\n")
		b.WriteString("  Linux  : sudo pacman -S tesseract tesseract-data-eng tesseract-data-ind\n")
		b.WriteString("           # atau: sudo apt install tesseract-ocr tesseract-ocr-eng tesseract-ocr-ind\n")
		b.WriteString("  macOS  : brew install tesseract tesseract-lang\n")
		b.WriteString("  Windows: pakai installer dari https://github.com/UB-Mannheim/tesseract/wiki\n")
	}

	return b.String(), nil
}

// writeImageMetadata appends a metadata block for the file.
func writeImageMetadata(b *strings.Builder, path string, st os.FileInfo) {
	b.WriteString("── Metadata ──\n")
	b.WriteString(fmt.Sprintf("Path        : %s\n", path))
	b.WriteString(fmt.Sprintf("Size        : %d bytes (%.1f KB)\n", st.Size(), float64(st.Size())/1024))
	b.WriteString(fmt.Sprintf("Modified    : %s\n", st.ModTime().Format(time.RFC3339)))

	// Try to decode dimensions/format using stdlib image package (PNG, JPEG, GIF only).
	if f, err := os.Open(path); err == nil {
		defer f.Close()
		if cfg, format, err := image.DecodeConfig(f); err == nil {
			b.WriteString(fmt.Sprintf("Dimensions  : %dx%d\n", cfg.Width, cfg.Height))
			b.WriteString(fmt.Sprintf("Format      : %s\n", format))
		} else {
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".webp" || ext == ".bmp" || ext == ".tiff" {
				b.WriteString(fmt.Sprintf("Format      : %s (dimensi tidak ter-decode oleh stdlib)\n",
					strings.TrimPrefix(ext, ".")))
			}
		}
	}
}

// runTesseract runs tesseract OCR on the file with the given language.
// Returns extracted text, or an error if tesseract is missing/failed.
func runTesseract(imgPath, lang string) (string, error) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		return "", fmt.Errorf("tesseract tidak terinstall di sistem")
	}
	if lang == "" {
		lang = "eng"
	}
	// `tesseract input stdout -l <lang>` writes text to stdout.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tesseract", imgPath, "stdout", "-l", lang)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("tesseract timeout setelah 20 detik")
	}
	if err != nil {
		// Tesseract may complain about missing language packs; surface that.
		s := string(out)
		// Strip the "Estimating resolution..." noise.
		s = stripTesseractNoise(s)
		if strings.Contains(s, "Failed loading language") || strings.Contains(s, "Tessdata") {
			return "", fmt.Errorf("language pack '%s' tidak tersedia. Coba 'eng' saja, atau install language pack: %s", lang, strings.TrimSpace(s))
		}
		return "", fmt.Errorf("tesseract gagal: %v — %s", err, strings.TrimSpace(s))
	}
	return stripTesseractNoise(string(out)), nil
}

func stripTesseractNoise(s string) string {
	out := s
	for _, prefix := range []string{
		"Estimating resolution as",
		"Tesseract Open Source OCR Engine",
	} {
		for {
			idx := strings.Index(out, prefix)
			if idx < 0 {
				break
			}
			end := strings.IndexByte(out[idx:], '\n')
			if end < 0 {
				out = out[:idx]
				break
			}
			out = out[:idx] + out[idx+end+1:]
		}
	}
	return out
}
