package web

import (
	"strings"
	"testing"
)

func TestInjectAttachmentSteerRequiresAutomaticImageAnalysis(t *testing.T) {
	prompt := injectAttachmentSteer("ini kenapa?\n\n[image:/tmp/screenshot.png]")

	if !strings.Contains(prompt, "Backend otomatis menjalankan analyze_image") {
		t.Fatalf("expected automatic analyze_image guidance, got %q", prompt)
	}
	if strings.Contains(prompt, "hanya jika user eksplisit") {
		t.Fatalf("image analysis must not require a second explicit instruction: %q", prompt)
	}
}

func TestInjectAttachmentSteerKeepsImageEditOnEditTool(t *testing.T) {
	prompt := injectAttachmentSteer("ubah gambar ini jadi kartun\n\n[image:/tmp/photo.png]")

	if !strings.Contains(prompt, "Gunakan tool edit_image langsung") {
		t.Fatalf("expected edit_image guidance, got %q", prompt)
	}
	if strings.Contains(prompt, "Backend otomatis menjalankan analyze_image") {
		t.Fatalf("image edit must not be routed through analyze_image: %q", prompt)
	}
}
