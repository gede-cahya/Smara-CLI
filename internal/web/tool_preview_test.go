package web

import (
	"strings"
	"testing"
)

func TestFormatToolResultPreviewSystemdStatus(t *testing.T) {
	input := `● komida-backend.service - Komida Backend Server
▶ Loaded: loaded (/home/cahya/.config/systemd/user/komida-backend.service; enabled; preset: enabled)
▶ Active: active (running) since Tue 2026-05-19 20:44:50 WITA; 3s ago
▶ Main PID: 148199 (bun)
▶ Memory: 78.8M (peak: 83.9M)
▶ CPU: 739ms
▶ May 19 20:44:53 cachyos-x8664 bun[148199]: Database initialized
▶ May 19 20:44:53 cachyos-x8664 bun[148199]: Server is running at 0.0.0.0:3481
▶ May 19 20:44:53 cachyos-x8664 bun[148199]: Started development server: http://0.0.0.0:3481`
	got := formatToolResultPreview(input)
	for _, want := range []string{"🧩 Service", "komida-backend.service", "🟢 active (running)", "pid 148199 (bun)", "mem 78.8M", "url http://0.0.0.0:3481"} {
		if !strings.Contains(got, want) {
			t.Fatalf("preview missing %q\n got: %s", want, got)
		}
	}
	if strings.Contains(got, "▶") || strings.Contains(got, "CGroup") {
		t.Fatalf("preview still too raw: %s", got)
	}
}

func TestFormatToolResultPreviewHTTP(t *testing.T) {
	input := `HTTP/2 400
▶content-type: application/json
▶server: cloudflare
▶x-matched-path: /api/image/proxy
▶x-vercel-cache: MISS
▶
▶{"error":"Missing url parameter"}`
	got := formatToolResultPreview(input)
	if !strings.Contains(got, "🌐 HTTP") || !strings.Contains(got, "body {\"error\":\"Missing url parameter\"}") || strings.Contains(got, "▶") {
		t.Fatalf("unexpected HTTP preview: %s", got)
	}
}

func TestFormatToolResultPreviewScraper(t *testing.T) {
	input := `Found 24 manga from Kiryuu
▶popular 24 {
▶ title: "The 100th Regression Of The Max-Level Player",
▶ image: "https://thumbnail.komiku.org/uploads/manga/the-100th-regression-of-the-max-level-player/manga_thumbnail.jpg?w=500",
▶ source: "Kiryuu",
▶ chapter: "Chapter 83",
▶}
▶Scraping detail https://kiryuu.online/api/manga/the-100th-regression-of-the-max-level-player...
▶detail The 100th Regression Of The Max-Level Player 84 https://thumbnail.komiku.org/uploads/manga/the-100th-regression-of-the-max-level-player/manga_thumbnail.jpg?w=500
▶Scraping chapter https://kiryuu.online/read/the-100th-regression-of-the-max-level-player/the-100th-regression-of-the-max-level-player-chapter-83...
▶[Kiryuu] Fetching reader API: https://kiryuu.online/api/read/the-100th-regression-of-the-max-level-player/the-100th-regression-of-the-max-level-player-chapter-83
▶Failed to fetch chapter API: 404
▶chapter undefined undefined`
	got := formatToolResultPreview(input)
	for _, want := range []string{"Scraper", "status error", "source Kiryuu", "found 24 manga", "Chapter 83", "404"} {
		if !strings.Contains(got, want) {
			t.Fatalf("scraper preview missing %q\n got: %s", want, got)
		}
	}
	if strings.Contains(got, "thumbnail.komiku.org") || strings.Contains(got, "▶") || len(got) > maxToolPreviewLen {
		t.Fatalf("scraper preview too noisy: %s", got)
	}
}
