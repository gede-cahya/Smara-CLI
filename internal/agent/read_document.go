package agent

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// stripAttachmentWrapper removes the [image:/path] / [file:/path] wrappers
// the front-end and adapters use to surface attachments. Tools accept the
// wrapped form for convenience so the agent can pass the raw token without
// post-processing.
func stripAttachmentWrapper(path string) string {
	path = strings.TrimSpace(path)
	for _, prefix := range []string{"[image:", "[file:"} {
		if strings.HasPrefix(path, prefix) && strings.HasSuffix(path, "]") {
			return strings.TrimSuffix(strings.TrimPrefix(path, prefix), "]")
		}
	}
	return path
}

// detectBinaryKind classifies a file based on its magic bytes / extension.
// Returns ("image", true) for images, ("pdf", true) for PDFs, etc.
// For text files returns ("", false). The byte scan is cheap and avoids
// spawning any external process.
func detectBinaryKind(path string, content []byte) (kind string, isBinary bool) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".pdf":
		return "pdf", true
	case ".docx", ".doc", ".odt", ".rtf":
		return "document", true
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp", ".svg", ".tiff", ".ico":
		return "image", true
	case ".zip", ".tar", ".gz", ".bz2", ".xz", ".7z", ".rar":
		return "archive", true
	case ".mp3", ".wav", ".flac", ".ogg", ".m4a":
		return "audio", true
	case ".mp4", ".mkv", ".webm", ".mov", ".avi":
		return "video", true
	case ".so", ".dll", ".dylib", ".exe", ".o", ".a":
		return "binary", true
	}

	// Magic byte sniff (top 4-8 bytes) — covers files without extension.
	if len(content) >= 4 {
		head := content[:min(8, len(content))]
		switch {
		case bytes.HasPrefix(head, []byte("%PDF-")):
			return "pdf", true
		case bytes.HasPrefix(head, []byte{0x89, 'P', 'N', 'G'}):
			return "image", true
		case bytes.HasPrefix(head, []byte{0xFF, 0xD8, 0xFF}):
			return "image", true
		case bytes.HasPrefix(head, []byte{'G', 'I', 'F', '8'}):
			return "image", true
		case bytes.HasPrefix(head, []byte{0x50, 0x4B, 0x03, 0x04}):
			// ZIP container — could be docx/xlsx/jar/zip.
			if ext == "" {
				return "archive", true
			}
		}
	}

	// Heuristic: a NUL byte in the first 8 KiB strongly implies binary.
	scan := content
	if len(scan) > 8192 {
		scan = scan[:8192]
	}
	if bytes.IndexByte(scan, 0) >= 0 {
		return "binary", true
	}
	return "", false
}

// extractDocumentText pulls plain text out of a document file. The choice of
// extractor depends on the file kind: pdftotext for PDF, pandoc for DOCX
// and similar word-processor formats, and a direct read for plain text.
//
// Returns the extracted text and a short label of which backend produced it.
func extractDocumentText(path string) (text string, source string, err error) {
	if _, err := os.Stat(path); err != nil {
		return "", "", fmt.Errorf("file tidak ditemukan: %w", err)
	}
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".pdf":
		return extractPDF(path)
	case ".docx", ".odt", ".rtf", ".doc", ".epub", ".html", ".htm":
	return extractWithPandoc(path)
	case ".txt", ".md", ".markdown", ".log", ".csv", ".tsv", ".json", ".yaml", ".yml",
		".toml", ".ini", ".cfg", ".conf", ".rst", ".tex", ".sh", ".bash", ".zsh",
		".py", ".go", ".js", ".ts", ".tsx", ".jsx", ".rs", ".java", ".c", ".cpp",
		".h", ".hpp", ".rb", ".php", ".sql", ".xml", ".env":
		b, err := os.ReadFile(path)
		if err != nil {
			return "", "", err
		}
		return string(b), "plain-text", nil
	}

	// Unknown extension — try plain read with a binary safety check.
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	if kind, isBin := detectBinaryKind(path, b); isBin {
		return "", "", fmt.Errorf("file biner (%s) dengan ekstensi %q tidak didukung — extractor yang tersedia: pdftotext, pandoc", kind, ext)
	}
	return string(b), "plain-text", nil
}

// extractPDF runs `pdftotext -layout -nopgbrk -enc UTF-8 <path> -` and
// returns stdout. Falls back to a clear error message instructing the user
// to install poppler if the binary is missing.
func extractPDF(path string) (string, string, error) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		return "", "", fmt.Errorf(
			"pdftotext tidak terinstall — install dulu: " +
				"Linux: `sudo pacman -S poppler` atau `apt install poppler-utils`; " +
				"macOS: `brew install poppler`; " +
				"Windows: `choco install xpdf-utils` atau download poppler-windows",
		)
	}
	cmd := exec.Command("pdftotext", "-layout", "-nopgbrk", "-enc", "UTF-8", path, "-")
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("pdftotext gagal: %w (stderr: %s)", err, strings.TrimSpace(errBuf.String()))
	}
	return out.String(), "pdftotext", nil
}

// extractWithPandoc converts DOCX/ODT/RTF/EPUB/HTML to plain text via pandoc.
func extractWithPandoc(path string) (string, string, error) {
	if _, err := exec.LookPath("pandoc"); err != nil {
		return "", "", fmt.Errorf(
			"pandoc tidak terinstall — install dulu: " +
				"Linux: `sudo pacman -S pandoc` atau `apt install pandoc`; " +
				"macOS: `brew install pandoc`",
		)
	}
	cmd := exec.Command("pandoc", "-t", "plain", "--wrap=none", path)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("pandoc gagal: %w (stderr: %s)", err, strings.TrimSpace(errBuf.String()))
	}
	return out.String(), "pandoc", nil
}
