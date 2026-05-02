# Release Notes — Smara CLI v1.16.0

**Release Date**: 2026-05-02
**Codename**: *Medic*
**Full Changelog**: `v1.15.0...v1.16.0`

---

## 🎯 Executive Summary

Rilis v1.16.0 menghadirkan fitur **Self-Repair / Self-Healing CLI** — sistem diagnosis dan perbaikan otomatis untuk Smara CLI. Dengan command `smara doctor` dan `smara repair`, user dapat memeriksa kesehatan DB, config, MCP, session, dan disk. Smara juga akan menjalankan auto-repair saat startup jika menemukan config invalid atau DB corrupt.

---

## ✨ New Features

### 🩺 Self-Repair CLI (`doctor` & `repair` Commands)
- **`smara doctor`**: Jalankan diagnostic penuh untuk semua komponen:
  - **DB Health**: Cek file existence, size, SQLite integrity (`PRAGMA integrity_check`), dan required tables.
  - **Config Health**: Validasi YAML, cek required keys (`provider`, `model`), cek permission file (harus 600).
  - **MCP Health**: Test connectivity ke semua MCP server yang dikonfigurasi (local & remote).
  - **Session Health**: Deteksi session aktif yang terlalu lama (>24h) dan stale lock files.
  - **Disk Health**: Cek directory writable dan available disk space.
- **`smara repair`**: Perbaiki masalah yang terdeteksi secara otomatis:
  - **DB Repair**: Backup corrupt DB & recreate dengan full schema.
  - **Config Repair**: Backup invalid config & tulis ulang default config.
  - **Session Repair**: Mark orphaned sessions sebagai `ended` & hapus stale lock files.
  - **MCP Repair**: Reconnect semua MCP server yang disconnect.
- **Dry-Run Mode**: `smara repair --dry-run` untuk preview aksi tanpa benar-benar dieksekusi.
- **Module Filtering**: `--module=db|config|mcp|session|disk` untuk fokus diagnostic/repair satu modul.
- **JSON Output**: `smara doctor --json` untuk output parsable oleh script/monitoring.

### 🔧 Auto-Repair at Startup
- Saat `smara start` dijalankan, Smara otomatis mengecek kondisi kritis (config & DB).
- Jika config invalid → backup & reset ke default config.
- Jika DB corrupt → backup & recreate schema.
- Jika auto-repair gagal (misal disk read-only) → exit dengan pesan error yang jelas.

---

## 📁 Improvements

### Directory Structure
- Semua PRD file dipindahkan ke folder terorganisir `docs/prd/` untuk kebersihan repository.
- Package `internal/repair/` berisi modular diagnostic & repair logic yang bisa di-extend di masa depan.

### Backup System
- Setiap repair yang menghapus/recreate file akan otomatis membuat **timestamped backup** (e.g. `config.yaml.bak.20260502-120000`).
- Retention: maksimal **5 backup** per file, backup lama otomatis dihapus.

---

## 🐛 Bug Fixes

- **Import Cycle**: Refactored `internal/repair/modules/` ke flat package `internal/repair/` untuk menghindari import cycle dengan `internal/config`.

---

## 📋 Upgrade Guide

### From v1.15.0
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
- Test doctor: `smara doctor`
- Test repair dry-run: `smara repair --dry-run`
- Test auto-repair: rename/hapus `~/.smara/config.yaml` lalu jalankan `smara start`

---

## 📦 Assets

| Platform | Asset |
|----------|-------|
| Linux AMD64 | `smara-v1.16.0-linux-amd64.tar.gz` |
| macOS AMD64 | `smara-v1.16.0-darwin-amd64.tar.gz` |
| macOS ARM64 | `smara-v1.16.0-darwin-arm64.tar.gz` |
| Windows AMD64 | `smara-v1.16.0-windows-amd64.zip` |

---

## 🙏 Contributors

Lead Dev: Gede Cahya

---

*Terima kasih telah menggunakan Smara CLI. Semoga bermanfaat!* 🌀
