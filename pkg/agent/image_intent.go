package agent

import "strings"

func isDirectImageGenerationRequest(prompt string) bool {
	p := strings.ToLower(strings.TrimSpace(prompt))
	if p == "" {
		return false
	}

	verbs := []string{"buat", "buatkan", "bikin", "generate", "hasilkan", "create", "make", "draw", "desain", "design"}
	objects := []string{"logo", "gambar", "image", "poster", "ilustrasi", "illustration", "icon", "ikon", "banner", "sticker", "maskot", "mascot"}

	hasVerb := false
	for _, v := range verbs {
		if strings.Contains(p, v) {
			hasVerb = true
			break
		}
	}
	if !hasVerb {
		return false
	}
	for _, o := range objects {
		if strings.Contains(p, o) {
			return true
		}
	}
	return false
}

func enhanceImagePrompt(prompt string) string {
	p := strings.TrimSpace(prompt)
	if p == "" {
		return p
	}
	lower := strings.ToLower(p)
	if strings.Contains(lower, "logo") && len([]rune(p)) < 120 {
		return p + ". Buat sebagai logo brand profesional: modern, elegan, minimalis, rapi, mudah dikenali, komposisi seimbang, vektor-style, high quality, clean typography jika ada teks, warna harmonis, latar belakang sederhana. Hindari watermark, mockup, foto realistis, dan elemen berantakan."
	}
	return p
}
