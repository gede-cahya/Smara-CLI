# Release Notes — Smara CLI v1.19.1

Patch release with major operational improvements: adaptive iteration limits, image clipboard, and configurable platform timeouts. All carried over from real-world usage feedback on Telegram/Discord bot deployments.

## Highlights

### 🧠 Adaptive Iteration Budget — Tidak lagi “batas eksekusi tool tercapai”
Iteration cap di agentic loop sekarang **dinamis**, bukan hard-coded 10. Empat mekanisme:

1. **Per-mode base budget** — tergantung mode aktif:
   - `ASK`: base 5, hard cap 15
   - `RUSH`: base 15, hard cap 40
   - `PLAN`: base 12, hard cap 30
   - `TEST`: base 10, hard cap 25
   - `WORKFLOW`: base 30, hard cap 80
2. **User override** via `agent_max_iterations` di config (mengalahkan default mode, tetap dapat extension dalam ceiling-nya).
3. **Progress extension** — kalau di akhir budget model masih panggil tool yang **diverse** (≥50% unique di window 8 terakhir), budget extend +25% per cycle sampai hard cap.
4. **Stuck-loop detector** — auto break + steering message kalau:
   - Tool yang sama dipanggil 3x berturut dengan args sama
   - Atau muncul ≥4x di window 8 terakhir (oscillating A/B/A/B)

Konfigurasi:
```yaml
agent_max_iterations: 50      # hard cap eksekusi tool per prompt (override mode default)
```

### ⏱️ Configurable Platform Prompt Timeout
Telegram/Discord/WhatsApp bot timeout naik dari hardcoded 5 menit → **default 10 menit**, fully configurable. Pesan timeout sekarang kasih instruksi cara naikkan limit.

```yaml
platform_prompt_timeout: 1200  # 20 menit, untuk task multi-SSH yang panjang
```

### 🖼️ Image Clipboard & Vision Tools

**Ctrl+V di TUI** sekarang juga ambil **gambar** dari clipboard, bukan cuma text:
- Linux X11: `xclip` (auto-detected)
- Linux Wayland: `wl-paste` (auto-detected)
- macOS: built-in via `osascript`
- Windows: built-in via PowerShell

Image disimpan ke `~/.smara/clip-images/clip-<timestamp>.png` (auto-prune ≥1 jam atau >50 file). Smara inject `[image:/path]` token ke textarea + toast notification.

Tiga **built-in tools** baru untuk agent:

1. **`analyze_image`** — analisa file gambar
   - Metadata otomatis (size, dimensions, format) via stdlib image package
   - OCR via tesseract (default `eng+ind`); kalau ga terinstall, kasih instruksi install
   - Strip wrapper `[image:/path]` otomatis kalau token raw di-pass
2. **`clip_paste_image`** — agent ambil image dari clipboard sistem dan dapat path
3. **`clip_copy_image`** — agent push image ke clipboard sistem (untuk hasil generate diagram, dst.)

Setup OCR (opsional):
```bash
# Arch / CachyOS
sudo pacman -S tesseract tesseract-data-eng tesseract-data-ind
# Ubuntu / Debian
sudo apt install tesseract-ocr tesseract-ocr-eng tesseract-ocr-ind
# macOS
brew install tesseract tesseract-lang
```

### 🎨 TUI Chat Bubble Redesign — Crush Ribbon Style
- Vertical accent ribbon kiri tiap bubble (warna sesuai role)
- User: biru, Agent: pastel green (mode-colored: cyan ASK / pink RUSH / violet PLAN), System: amber, Terminal: green
- Mode badge inline di header (`💬 Smara [ASK]` dengan bg surface)
- Model name muted di-faint (`· gpt-5.5`)
- Hilangkan "ghost label" di kanan timestamp — penyebab adalah background style yang meluber, sekarang prefix render plain (tanpa bg)
- Time stamp dengan leading dot `◦ 22:20` untuk visual rhythm
- Terminal icon `▸` (clean arrow) menggantikan `$`

## Tested

- `go build ./...` ✓
- `go test ./internal/memory/... ./internal/agent/... ./internal/ui/...` ✓
- New tests:
  - `TestIterationBudget_*` (7 tests covering mode defaults, user override, stuck-loop detection, oscillating pattern, progress extension)
  - `TestAnalyzeImageFile_*` (3 tests: PNG real decode, error paths, wrapper strip)
  - `TestStripTesseractNoise`
  - `TestSmaraTempImagePath`, `TestPruneOldClipImages`, `TestPruneClipsKeeps50`
- Cross-platform release: linux/amd64, darwin/amd64, darwin/arm64, windows/amd64

## Upgrade

```bash
smara update 1.19.1
```

Atau download manual dari Releases dan replace binary. Setelah upgrade, optional:

```yaml
# ~/.smara/config.yaml
agent_max_iterations: 50
platform_prompt_timeout: 1200
```

Lalu restart bot kalau jalan sebagai service:
```bash
sudo systemctl restart smara
```

## Platform Artifacts

| Platform       | Archive                                       |
| -------------- | --------------------------------------------- |
| Linux AMD64    | `smara-v1.19.1-linux-amd64.tar.gz`            |
| macOS AMD64    | `smara-v1.19.1-darwin-amd64.tar.gz`           |
| macOS ARM64    | `smara-v1.19.1-darwin-arm64.tar.gz`           |
| Windows AMD64  | `smara-v1.19.1-windows-amd64.zip`             |
