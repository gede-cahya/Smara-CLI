# Release Notes — Smara CLI v1.13.0

**Release Date**: 2026-07-10  
**Codename**: *Comprehensive*  
**Full Changelog**: `v1.12.0...v1.13.0`

---

## 🎯 Executive Summary

Rilis v1.13.0 menyempurnakan fondasi Skill Ecosystem, menambahkan kemampuan SSH file transfer, meningkatkan integrasi MCP (termasuk OpenCode & remote), dan melengkapi dokumentasi serta unit test untuk menjamin stabilitas rilis.

---

## ✨ New Features

### 🧩 Skill Ecosystem v2
- **Install from URL**: `smara skill install <url>` — download dan validasi skill dari remote URL atau GitHub raw.
- **Markdown Skill Support**: Buat skill dari format Markdown dengan syntax alami.
- **Skill Search**: Cari skill di semua registry yang terdaftar dengan `smara skill search [query]`.
- **Skill Publish**: Publikasikan skill ke marketplace/registry dengan `smara skill publish <nama>`.
- **Registry Sync**: Sinkronisasi cache lokal untuk semua registry via `smara skill registry sync`.
- **Skill Info**: Tampilkan detail parameter, steps, dan metadata skill.
- **Skill Update**: Update skill yang sudah di-install dari source URL asli.
- **Skill Executor Integration**: Workflow agents dapat menjalankan skill tersimpan melalui prefix `skill:`.

### 📁 SSH File Transfer
- **Upload via SFTP/SCP**: `smara ssh upload <host> <local> <remote>` dengan support direktori rekursif (SFTP).
- **Download via SFTP/SCP**: `smara ssh download <host> <remote> <local>` dengan preserve permission.
- **Transfer Logs**: Riwayat upload/download tersimpan di database dengan `smara ssh transfer-logs`.
- **Method Override**: Flag `--method sftp|scp` untuk memilih protokol transfer.

### 🌐 MCP Improvements
- **OpenCode Auto-Discovery**: Auto-load MCP servers dari konfigurasi `~/.config/opencode/mcp.json` secara paralel.
- **Remote MCP Support**: Koneksi ke MCP server remote via SSE/WebSocket dengan `mcp.NewRemoteClient`.
- **Parallel Connection**: Semua MCP server (Windsurf, OpenCode, native) terhubung secara paralel untuk startup lebih cepat.

### 🎨 TUI Enhancements
- **Hyperlink Component**: Support deteksi dan render hyperlink interaktif di pesan agen.
- **Message Selection**: `Ctrl+S` untuk seleksi pesan historis, `Enter/C` untuk copy ke clipboard.
- **Clipboard Paste**: `Ctrl+V` untuk paste dari clipboard ke input prompt.

### 🖥️ Desktop (Wails)
- Struktur `smara-desktop/` diperbarui dengan build targets untuk Darwin, Windows, dan Linux.
- Frontend build system menggunakan Wails v2 dengan Go backend.

---

## 🔧 Improvements

- **SSH Store**: Database SSH sekarang menyimpan log eksekusi dan transfer terpisah dengan schema tabel baru.
- **Config Parsing**: Support konfigurasi `skill_registries` di `config.yaml`.
- **Agent Supervisor**: Enhanced session management dengan `SessionRegistry` dan `SessionManager` di `pkg/agent`.
- **Safety & Audit**: Audit log JSON Lines tetap aktif untuk semua tindakan agen.
- **Build System**: Makefile tetap support cross-platform release untuk linux/amd64, darwin/amd64, darwin/arm64, windows/amd64.

---

## � Bug Fixes

### Session Persistence on Exit
- **ESC/Ctrl+Q/Ctrl+D/exit**: Sesi sekarang selalu tersimpan ke database sebelum TUI keluar (via `defer SaveSession()` di `start.go`).
- **Ctrl+C (SIGINT)**: Signal handler sekarang memanggil `supervisor.SaveSession()` sebelum `os.Exit(0)`.
- **Terminal killed/closed (SIGHUP)**: Menambah handler `SIGHUP` dan `SIGQUIT` agar session tetap tersimpan saat terminal window ditutup atau proses di-kill.

### Version Display
- Banner TUI menampilkan versi dinamis dari `version.go` (`1.13.0`) daripada hardcoded `v1.8.0` lama. `AppVersion` di `internal/ui/app.go` sekarang di-set oleh `cmd/smara/start.go`.

## �🛠️ Breaking Changes

**None** — v1.13.0 adalah rilis minor yang fully backward-compatible dengan v1.12.0.

---

## 📋 Upgrade Guide

### From v1.12.0
```bash
# Update via CLI
smara update

# Atau build dari source
git pull origin main
make build
make install
```

### New Config Options (Optional)
Tambahkan ke `config.yaml` untuk mengaktifkan skill marketplace:
```yaml
skill_registries:
  - name: default
    url: https://raw.githubusercontent.com/gede-cahya/Smara-Skills/main/registry.json
```

---

## 🧪 Testing

- Semua package core (`pkg/agent`, `internal/agent`, `internal/memory`, `internal/audit`) memiliki unit test dengan testify.
- Dokumen test plan manual tersedia di `PRD_v1.13.0_Testing.md`.
- Jalankan: `go test ./...`

---

## 📦 Assets

| Platform | Asset |
|----------|-------|
| Linux AMD64 | `smara-v1.13.0-linux-amd64.tar.gz` |
| macOS AMD64 | `smara-v1.13.0-darwin-amd64.tar.gz` |
| macOS ARM64 | `smara-v1.13.0-darwin-arm64.tar.gz` |
| Windows AMD64 | `smara-v1.13.0-windows-amd64.zip` |

---

## 🙏 Contributors

Lead Dev: Gede Cahya

---

*Terima kasih telah menggunakan Smara CLI. Semoga bermanfaat!* 🌀
