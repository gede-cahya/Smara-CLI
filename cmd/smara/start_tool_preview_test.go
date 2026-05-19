package main

import "testing"

func TestFormatToolResultPreviewHTTP(t *testing.T) {
	input := `HTTP/2 400
▶date: Tue, 19 May 2026 12:24:50 GMT
▶content-type: application/json
▶server: cloudflare
▶x-matched-path: /api/image/proxy
▶x-vercel-cache: MISS
▶cf-cache-status: DYNAMIC
▶
▶{"error":"Missing url parameter"}`

	got := formatToolResultPreview(input)
	want := `🌐 HTTP · status HTTP/2 400 · body {"error":"Missing url parameter"} · type application/json · path /api/image/proxy · cache MISS · server cloudflare`
	if got != want {
		t.Fatalf("unexpected preview\nwant: %s\n got: %s", want, got)
	}
}

func TestFormatToolResultPreviewBuildLog(t *testing.T) {
	input := `▶Building: Installing dependencies...
▶Building: bun install v1.3.12 (700fc117)
▶Building: Checked 899 installs across 920 packages (no changes) [228.00ms]
▶Building: Detected Next.js version: 16.1.6
▶Building: Running "bun run build"
▶Building: $ bun next build
▶Building: ▲ Next.js 16.1.6 (Turbopack)
▶Building: ⚠ The "middleware" file convention is deprecated. Please use "proxy" instead. Learn more: https://nextjs.org/docs/messages/middleware-to-proxy
▶Building: Creating an optimized production build ...`

	got := formatToolResultPreview(input)
	want := `🏗️ Build · status creating optimized production build · Next.js version: 16.1.6 · bun install v1.3.12 (700fc117) · cmd bun next build · warning ⚠ The "middleware" file convention is deprecated. Please use "proxy" instead. Learn more: https://...`
	if got != want {
		t.Fatalf("unexpected preview\nwant: %s\n got: %s", want, got)
	}
}

func TestFormatToolResultPreviewSingleLine(t *testing.T) {
	got := formatToolResultPreview("hello\nworld")
	if got != "hello world" {
		t.Fatalf("unexpected preview: %q", got)
	}
}
