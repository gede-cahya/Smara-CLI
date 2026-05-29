package platform

import "testing"

func TestIsImageGenerationPromptSkipsSoftwareFeatureRequests(t *testing.T) {
	cases := []string{
		"buatkan fitur image to image nya",
		"implement image to image",
		"tambah fitur upload gambar",
		"buatkan component image editor",
	}
	for _, tc := range cases {
		if isImageGenerationPrompt(tc) {
			t.Fatalf("expected %q not to route to image generation", tc)
		}
	}
}

func TestIsImageGenerationPromptAllowsVisualGeneration(t *testing.T) {
	cases := []string{
		"buatkan logo smara",
		"buatkan gambar kucing lucu",
		"generate poster event",
	}
	for _, tc := range cases {
		if !isImageGenerationPrompt(tc) {
			t.Fatalf("expected %q to route to image generation", tc)
		}
	}
}
