# Release Notes — Smara CLI v1.15.0

**Release Date**: 2026-05-01
**Codename**: *AutoForge*
**Full Changelog**: `v1.14.0...v1.15.0`

---

## 🎯 Executive Summary

Rilis v1.15.0 menghadirkan fitur **Auto-Serve** untuk project web yang di-generate oleh workflow agent, dukungan multi-runtime server (Node.js, Bun, Go, PHP, Python static), perbaikan bug `run_command` hang, dan normalisasi DSML Unicode pipe. Bot Telegram dan Discord kini otomatis serve project dan memberikan URL publik ke user tanpa intervensi manual.

---

## ✨ New Features

### 🌐 Auto-Serve Project (`serve_project` Built-in Tool)
- **Auto-detect Runtime**: Tool otomatis mendeteksi jenis project berdasarkan file yang ada:
  - `server.js` / `app.js` / `index.js` → `node server.js`
  - `server.ts` / `index.ts` → `npx tsx server.ts` / `npx ts-node`
  - `bun.lockb` + `.ts` → `bun run index.ts`
  - `go.mod` / `main.go` → `go run main.go`
  - `index.php` → `php -S 0.0.0.0:PORT`
  - Cuma `index.html` (static) → `python3 -m http.server`
- **Auto-Assign Port**: Scan port kosong di range **8000–8999** otomatis.
- **PORT Env Injection**: Set environment variables `PORT`, `SERVER_PORT`, `HTTP_PORT`, `APP_PORT` agar app backend bind ke port yang benar.
- **IP Publik Auto-Detect**: Gunakan `ifconfig.me` untuk mendeteksi IP publik VPS dan generate URL lengkap (`http://IP:PORT/`).
- **Background Process**: Server berjalan di background dengan **process group** (bisa di-kill bersama).

### 🤖 Workflow Mode Auto-Serve
- **System Prompt Update**: Mode workflow sekarang diinstruksikan untuk **selalu memanggil `serve_project`** setelah membuat project web atau backend (Node.js, Go, PHP, Bun).
- **User Experience**: User cukup kirim `buatkan website todo list` dan bot otomatis generate file + serve + kirim URL publik ke chat.

---

## 🔧 Improvements

### Terminal Stability
- **`run_command` Timeout & Process Group**:
  - Setiap shell command kini punya **timeout 30 detik**.
  - Menggunakan `Setpgid: true` untuk membuat **process group baru**.
  - Jika timeout, **SIGKILL dikirim ke seluruh process group** (termasuk background child processes), mencegah hang forever.
  - Pipe stdout/stderr di-close secara eksplisit setelah `cmd.Wait()` selesai atau timeout.

### DSML Parsing Robustness
- **Fullwidth Unicode Pipe Normalization**: Karakter Unicode fullwidth (`｜｜DSML｜｜`, `U+FF5C`) kini otomatis di-normalisasi ke ASCII pipe (`||`) sebelum regex DSML parsing.
- **Always Extract**: `ExtractToolCallsFromContent` selalu dipanggil dan hasilnya di-merge dengan native tool calls, memastikan konten bersih meskipun model output format DSML.

---

## 🐛 Bug Fixes

- **Bot Hang / No Response**: `run_command` yang men-spawn background process (misal `node server.js &`) menyebabkan pipe stdout/stderr terbuka forever → bot hang. **Fixed** dengan timeout + process group kill.
- **Raw DSML Tags in Bot Response**: Model kadang mengeluarkan tag `｜｜DSML｜｜` (fullwidth) yang tidak ter-parse. **Fixed** dengan `normalizePipes()` dan guaranteed extraction.

---

## 📋 Upgrade Guide

### From v1.14.0
```bash
# Update via CLI
smara update

# Atau build dari source
git pull origin main
make build
make install
```

---

## 🧪 Testing

- Jalankan: `go test ./...`
- Test bot: kirim `buatkan website todo list` ke Telegram/Discord → harusnya dapat URL publik.

---

## 📦 Assets

| Platform | Asset |
|----------|-------|
| Linux AMD64 | `smara-v1.15.0-linux-amd64.tar.gz` |
| Linux ARM64 | `smara-v1.15.0-linux-arm64.tar.gz` |
| macOS AMD64 | `smara-v1.15.0-darwin-amd64.tar.gz` |
| macOS ARM64 | `smara-v1.15.0-darwin-arm64.tar.gz` |
| Windows AMD64 | `smara-v1.15.0-windows-amd64.zip` |

---

## 🙏 Contributors

Lead Dev: Gede Cahya

---

*Terima kasih telah menggunakan Smara CLI. Semoga bermanfaat!* 🌀
