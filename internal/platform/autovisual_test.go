package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenderSVGPreviewAttachmentsCreatesPNG(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="120" height="60"><rect width="120" height="60" fill="#7c3aed"/><text x="10" y="35" fill="white">Smara</text></svg>`
	atts, err := renderSVGPreviewAttachments(svg, "test-svg")
	if err != nil {
		t.Fatalf("renderSVGPreviewAttachments error: %v", err)
	}
	if len(atts) == 0 {
		t.Fatal("expected at least one attachment")
	}
	if _, err := os.Stat(atts[0].FilePath); err != nil {
		t.Fatalf("first attachment missing: %v", err)
	}
	if atts[0].MimeType == "image/png" && filepath.Ext(atts[0].FilePath) != ".png" {
		t.Fatalf("png attachment should use .png extension: %s", atts[0].FilePath)
	}
}

func TestRenderMarkdownDownloadAttachment(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	att, err := renderMarkdownDownloadAttachment("# Contoh\n\nIsi markdown", "contoh")
	if err != nil {
		t.Fatalf("renderMarkdownDownloadAttachment error: %v", err)
	}
	if filepath.Ext(att.FilePath) != ".md" {
		t.Fatalf("expected .md file, got %s", att.FilePath)
	}
	if att.MimeType != "text/markdown; charset=utf-8" {
		t.Fatalf("unexpected mime type: %s", att.MimeType)
	}
}
