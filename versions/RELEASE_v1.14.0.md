# Release Notes — Smara CLI v1.14.0

**Release Date**: 2026-05-01
**Codename**: *SessionForge*
**Full Changelog**: `v1.13.0...v1.14.0`

---

## 🎯 Executive Summary

Rilis v1.14.0 menghadirkan sistem manajemen session yang lebih powerful dengan AI-powered search, session picker interaktif keyboard-driven, session deletion, dan session-to-session context bridge. Fitur ini dirancang untuk produktivitas tinggi bagi pengguna yang mengelola banyak conversation session dalam satu workspace.

---

## ✨ New Features

### 🗑️ Session Deletion
- **CLI Command**: `/session delete <id>` — Hapus session dari registry in-memory dan SQLite store secara permanen.
- **Safety Guard**: Tidak dapat menghapus session yang sedang aktif (current). Gunakan `/session end` terlebih dahulu.
- **TUI Feedback**: Konfirmasi sukses/error muncul langsung di chat area.

### 🎯 Session Picker Overlay
- **Keyboard Shortcut**: `F3` — Toggle overlay session picker di tengah layar TUI.
- **Interaktif**:
  - `↑ / ↓` — Navigasi daftar session.
  - `Enter` — Switch ke session yang dipilih (history di-load ulang ke chat area).
  - `d` — Hapus session dengan konfirmasi Ya/Tidak.
  - `Esc` — Tutup overlay.
  - **Filter as-you-type**: Ketik nama atau ID untuk menyaring daftar session secara real-time.
- **Display**: Nama session, ID prefix (8 chars), jumlah pesan, dan timestamp terakhir update.

### 🔍 AI-Powered Session Search
- **CLI Command**: `/session search <query>` — Cari session secara semantik menggunakan embedding LLM.
- **Mekanisme**:
  - Generate embedding dari query pengguna.
  - Concatenate `Name + Context + History.Content` per session menjadi searchable text.
  - Generate embedding per session, hitung cosine similarity.
  - Rank hasil berdasarkan relevance score.
- **Output**: Top 5 hasil dengan relevansi score dan snippet preview.

### 🌉 Session-to-Session Context Bridge
- **CLI Command**: `/session new <nama> --carry-over=3` — Buat session baru sambil membawa N turn terakhir dari session aktif.
- **Mekanisme**: Saat create session baru, history terakhir (user + assistant pairs) di-copy dari current session ke session baru.
- **Default**: `--carry-over` tanpa angka = 3 turns (6 messages).

---

## 🔧 Improvements

- **Help Overlay**: Diperbarui dengan dokumentasi `F3` (Session Picker), `Ctrl+P` (Command Palette), dan command baru `/session delete` serta `/session search`.
- **Session Registry**: Method `Delete(id)` ditambahkan ke `SessionRegistry` untuk menghapus dari in-memory map.
- **Supervisor**: Method `GetSessionHistory(id)` untuk mengambil history message dari session tertentu.
- **Cosine Similarity**: Utility `cosineSimilarity(a, b []float32)` untuk perhitungan embedding relevance.

---

## 🐛 Bug Fixes

**None** — v1.14.0 adalah rilis minor yang fully backward-compatible dengan v1.13.0.

---

## 📋 Upgrade Guide

### From v1.13.0
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
- Semua package core lolos unit test.

---

## 📦 Assets

| Platform | Asset |
|----------|-------|
| Linux AMD64 | `smara-v1.14.0-linux-amd64.tar.gz` |
| macOS AMD64 | `smara-v1.14.0-darwin-amd64.tar.gz` |
| macOS ARM64 | `smara-v1.14.0-darwin-arm64.tar.gz` |
| Windows AMD64 | `smara-v1.14.0-windows-amd64.zip` |

---

## 🙏 Contributors

Lead Dev: Gede Cahya

---

*Terima kasih telah menggunakan Smara CLI. Semoga bermanfaat!* 🌀
