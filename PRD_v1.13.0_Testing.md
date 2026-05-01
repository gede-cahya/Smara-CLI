# 🌀 Smara CLI — PRD: v1.13.0 Build Testing

> **Smara** (Sanskerta: स्मृति) — "Ingatan" | Autonomous Multi-Agent Terminal
> **Versi PRD**: 1.0.0 | **Tanggal**: 2026-07-10 | **Status**: Final
> **Target Release**: v1.13.0

---

## I. Executive Summary

PRD ini mendefinisikan test plan manual dan acceptance criteria lengkap untuk rilis Smara CLI v1.13.0. Dokumen ini mencakup end-to-end checklist per fitur, skenario positif & negatif, environment setup, test data, dan definisi pass/fail untuk setiap skenario.

### Visi
> _"Setiap fitur v1.13.0 terverifikasi melalui skenario real-world sebelum tag rilis dibuat."_

---

## II. Environment Setup

### Prerequisite
| Komponen | Versi Min | Catatan |
|----------|-----------|---------|
| Go | 1.23.0 | `go version` |
| Node.js | 18.x | Untuk build Wails Desktop |
| Wails CLI | 2.x | `wails version` |
| Ollama | 0.3.x | Untuk test provider lokal |
| SQLite3 | 3.40+ | Runtime dependency |
| OpenSSH Client | 8.0+ | Untuk test SSH |
| make | any | Build automation |

### API Key (Opsional tapi direkomendasikan)
- Anthropic API Key (Claude 3.5 Sonnet)
- OpenAI API Key
- OpenRouter API Key

### VPS SSH Dummy / Test Target
- IP/Hostname test server dengan SSH key-based auth
- User: `testuser`, Key: `~/.ssh/id_rsa_test`
- Direktori test: `/home/testuser/smara-test/`

### Test Workspace
```bash
mkdir -p ~/.smara-test/
export SMARA_CONFIG_DIR=~/.smara-test/
```

---

## III. Fitur & Acceptance Criteria

### F1 — Version Bump & Build
**Acceptance Criteria:**
- `smara version` menampilkan `v1.13.0`
- `make build` berhasil tanpa error
- `make test` PASS 100%
- `make release` menghasilkan binary untuk linux/amd64, darwin/amd64, darwin/arm64, windows/amd64

| Skenario | Langkah | Ekspektasi | Status |
|----------|---------|------------|--------|
| F1-S1 | `go run ./cmd/smara version` | Output `🌀 Smara v1.13.0` | ☐ |
| F1-S2 | `make build` | Binary `smara` tercreate | ☐ |
| F1-S3 | `make test` | Semua package PASS | ☐ |
| F1-S4 | `make release` | Archive di `dist/` untuk 4 platform | ☐ |
| F1-N1 | Build dengan Go 1.22 | Diharapkan compile error atau warning | ☐ |

**Pass:** F1-S1 ~ F1-S4 semua ☐ → ☑  
**Fail:** Salah satu skenario gagal → jangan tag rilis

---

### F2 — TUI / `smara start`
**Acceptance Criteria:**
- TUI boot dalam < 2 detik
- Mode `ask`, `rush`, `plan`, `test`, `workflow` dapat di-switch
- Phase pipeline muncul saat generate
- Message selection (Ctrl+S) dan clipboard (Enter/C) berfungsi
- Ctrl+V paste dari clipboard berfungsi
- Tool call visualization muncul saat tool dieksekusi

| Skenario | Langkah | Ekspektasi | Status |
|----------|---------|------------|--------|
| F2-S1 | `smara start --mode ask`, tanya "hello" | Tampil respons dalam < 5 detik | ☐ |
| F2-S2 | Switch ke `rush`, minta "buat folder test123" | Folder test123 tercreate | ☐ |
| F2-S3 | Switch ke `plan`, minta "buat file a.txt" | Muncul rencana, tunggu approve | ☐ |
| F2-S4 | Ctrl+S pada pesan historis, Enter | Pesan tercopy ke clipboard | ☐ |
| F2-S5 | Ctrl+V di input prompt | Konten clipboard tertulis | ☐ |
| F2-S6 | Minta "analisis file go.mod" | Phase pipeline muncul (Thinking→Analyzing→Generating) | ☐ |
| F2-N1 | `smara start --mode invalid` | Error: mode tidak valid | ☐ |
| F2-N2 | TUI tanpa provider config | Warning muncul, fallback ke TUI offline | ☐ |

---

### F3 — MCP Auto-Discovery & Remote
**Acceptance Criteria:**
- Windsurf MCP config terdeteksi otomatis
- OpenCode MCP config terdeteksi otomatis
- MCP remote via SSE/WebSocket terhubung
- Tools dari semua MCP tersedia di system prompt

| Skenario | Langkah | Ekspektasi | Status |
|----------|---------|------------|--------|
| F3-S1 | Buat `~/.codeium/windsurf/mcp_config.json` dengan 1 server | `smara start` log: "MCP 'x' terhubung (n tools)" | ☐ |
| F3-S2 | Buat `~/.config/opencode/mcp.json` dengan 1 server | `smara start` log: "MCP 'y' terhubung" | ☐ |
| F3-S3 | Konfigurasi remote MCP di `config.yaml` | Koneksi berhasil, tools tersedia | ☐ |
| F3-N1 | MCP config JSON invalid | Warning, skip server, TUI tetap jalan | ☐ |
| F3-N2 | Remote MCP unreachable | Warning timeout, TUI tetap jalan | ☐ |

---

### F4 — SSH Remote Control
**Acceptance Criteria:**
- Add-host berhasil menyimpan ke DB
- Exec menjalankan perintah remote dan log ke DB
- Connect membuka sesi interaktif
- Upload/download file via SFTP/SCP berhasil
- Keygen menghasilkan key pair valid
- Riwayat exec dan transfer dapat dilihat via `logs`

| Skenario | Langkah | Ekspektasi | Status |
|----------|---------|------------|--------|
| F4-S1 | `smara ssh add-host dummy --host <ip> --user testuser --key ~/.ssh/id_rsa_test` | Host tersimpan | ☐ |
| F4-S2 | `smara ssh exec dummy "uname -a"` | Output Linux, log tersimpan | ☐ |
| F4-S3 | `smara ssh connect dummy`, ketik `ls`, `exit` | Sesi interaktif berfungsi | ☐ |
| F4-S4 | `smara ssh upload dummy ./local.txt /tmp/` | File muncul di remote, log tersimpan | ☐ |
| F4-S5 | `smara ssh download dummy /tmp/remote.txt ./` | File muncul di lokal | ☐ |
| F4-S6 | `smara ssh keygen --name test-key --type ed25519` | File `test-key` dan `test-key.pub` tercreate | ☐ |
| F4-S7 | `smara ssh logs --limit 5` | Tabel log muncul | ☐ |
| F4-S8 | `smara ssh transfer-logs --limit 5` | Tabel transfer log muncul | ☐ |
| F4-N1 | Exec ke host tanpa key/password | Error: koneksi gagal | ☐ |
| F4-N2 | Upload ke path yang tidak ada permission | Error: upload gagal | ☐ |

---

### F5 — Skill Ecosystem v2
**Acceptance Criteria:**
- Skill dapat dibuat dari JSON dan Markdown stdin
- Skill dapat dijalankan via `smara skill run`
- Skill dapat di-install dari URL
- Skill dapat dicari, di-info, dan dihapus
- Publish ke registry berhasil (jika registry tersedia)
- Registry sync berhasil

| Skenario | Langkah | Ekspektasi | Status |
|----------|---------|------------|--------|
| F5-S1 | `echo '{"name":"test","description":"d","steps":[]}' | smara skill create test --format json` | Skill tersimpan | ☐ |
| F5-S2 | `smara skill run test` | Eksekusi skill (kosong = segera selesai) | ☐ |
| F5-S3 | `smara skill install <valid-url>` | Skill terdownload dan tersimpan | ☐ |
| F5-S4 | `smara skill list` | Daftar skill termasuk "test" | ☐ |
| F5-S5 | `smara skill info test` | Detail skill ditampilkan | ☐ |
| F5-S6 | `smara skill delete test` | Skill terhapus | ☐ |
| F5-S7 | `smara skill registry sync` | Cache registry tersinkronisasi | ☐ |
| F5-N1 | Install dari URL invalid | Error: gagal install skill | ☐ |
| F5-N2 | Run skill yang tidak ada | Error: skill tidak ditemukan | ☐ |

---

### F6 — Memory & Workspace
**Acceptance Criteria:**
- Hybrid search mengembalikan hasil relevan
- Category CRUD berfungsi
- Memory versioning dan rollback berfungsi
- Export/import berfungsi

| Skenario | Langkah | Ekspektasi | Status |
|----------|---------|------------|--------|
| F6-S1 | `smara memory search "deploy" --hybrid` | Hasil pencarian muncul | ☐ |
| F6-S2 | `smara category create "TestCat"` | Kategori tersimpan | ☐ |
| F6-S3 | `smara memory export backup.zip` | File ZIP tercreate | ☐ |
| F6-N1 | Search dengan query kosong | Tidak crash, hasil kosong | ☐ |

---

### F7 — Dashboard
**Acceptance Criteria:**
- `smara dashboard` menampilkan TUI dengan metrik
- `--once` menghasilkan snapshot tanpa TUI interaktif
- `--refresh` mengatur interval update

| Skenario | Langkah | Ekspektasi | Status |
|----------|---------|------------|--------|
| F7-S1 | `smara dashboard` | TUI dashboard muncul, bisa quit dengan `q` | ☐ |
| F7-S2 | `smara dashboard --once` | Output text snapshot, exit otomatis | ☐ |
| F7-N1 | Dashboard tanpa data | TUI muncul dengan state kosong, tidak crash | ☐ |

---

### F8 — Platform Bot (`smara serve`)
**Acceptance Criteria:**
- Serve dengan `--platform telegram` berjalan tanpa crash
- Bot command `/ask`, `/mode`, `/mcp`, `/clear` berfungsi
- Log platform tercatat

| Skenario | Langkah | Ekspektasi | Status |
|----------|---------|------------|--------|
| F8-S1 | `smara serve --platform telegram --mode ask` (dengan token valid) | Bot online, menerima pesan | ☐ |
| F8-S2 | Kirim `/ask hello` ke bot | Bot merespons | ☐ |
| F8-N1 | Serve tanpa token | Error: token wajib diisi | ☐ |

---

### F9 — Safety & Audit
**Acceptance Criteria:**
- Plan mode tidak mengeksekusi tanpa approval
- Audit log JSON Lines tercatat untuk setiap tindakan
- Auto-revert jika eksekusi gagal (jika diimplementasikan)

| Skenario | Langkah | Ekspektasi | Status |
|----------|---------|------------|--------|
| F9-S1 | Plan mode, minta edit file, jangan approve | File tidak berubah | ☐ |
| F9-S2 | Jalankan tindakan apapun | File audit log bertambah baris | ☐ |

---

### F10 — LSP Integration
**Acceptance Criteria:**
- LSP client dapat diinisialisasi untuk Go/TS/Python/Rust
- Definition hover dan diagnostics tersedia

| Skenario | Langkah | Ekspektasi | Status |
|----------|---------|------------|--------|
| F10-S1 | `smara start`, minta "cek error di file main.go" | Agen menggunakan LSP tool jika tersedia | ☐ |

---

### F11 — Desktop (Wails)
**Acceptance Criteria:**
- `wails build` di `smara-desktop/` menghasilkan executable
- Frontend dapat boot dan menampilkan UI dasar

| Skenario | Langkah | Ekspektasi | Status |
|----------|---------|------------|--------|
| F11-S1 | `cd smara-desktop && wails build` | Binary desktop tercreate | ☐ |
| F11-S2 | Jalankan binary desktop | Window muncul | ☐ |

---

## IV. Regression Checklist (v1.12.x → v1.13.0)

| Fitur v1.12 | Command | Harus Tetap Berfungsi | Status |
|-------------|---------|----------------------|--------|
| Workspace create/use/list | `smara workspace create X` | ✅ | ☐ |
| Provider select/list | `smara provider list` | ✅ | ☐ |
| Config list | `smara config list` | ✅ | ☐ |
| Login | `smara login` | ✅ | ☐ |
| Update | `smara update` | ✅ | ☐ |
| Nudge | `smara nudge list` | ✅ | ☐ |
| Guide | `smara guide` | ✅ | ☐ |
| Explore | `smara explore` | ✅ | ☐ |

---

## V. Test Data & Fixtures

### Sample Skill JSON (`test-skill.json`)
```json
{
  "name": "test-echo",
  "description": "Skill test untuk echo",
  "version": 1,
  "steps": [
    {
      "tool": "run_command",
      "args": {"command": "echo 'skill-test-ok'"}
    }
  ]
}
```

### Sample Markdown Skill (`test-skill.md`)
```markdown
# Skill: test-md

## Description
Skill test dari markdown.

## Steps
1. run_command: `echo markdown-test-ok`
```

### SSH Test Script (`test-remote.sh`)
```bash
#!/bin/bash
echo "hostname=$(hostname)"
echo "date=$(date -Iseconds)"
mkdir -p /tmp/smara-test
echo "ok" > /tmp/smara-test/flag.txt
```

---

## VI. Definisi Pass / Fail

### Pass
- Semua skenario positif (S1, S2, ...) menghasilkan output sesuai ekspektasi
- Tidak ada panic atau crash pada seluruh skenario negatif (N1, N2, ...)
- `make test` PASS 100% tanpa race condition

### Fail
- Salah satu skenario positif tidak sesuai ekspektasi
- Terdapat panic/crash pada skenario apapun
- `make test` terdapat FAIL
- Binary release corrupt atau tidak dapat dijalankan

### Escalation
- Jika skenario F1 (Build) FAIL → **BLOCK RILIS**
- Jika skenario F2 (TUI) FAIL → **BLOCK RILIS**
- Jika > 2 skenario positif lainnya FAIL → **BLOCK RILIS**
- Jika hanya skenario Desktop (F11) FAIL → Rilis CLI tetap jalan, Desktop di-postpone

---

## VII. Sign-off

| Role | Nama | Tanggal | Tanda Tangan (Git) |
|------|------|---------|-------------------|
| Lead Dev | Gede Cahya | | `git tag -s v1.13.0` |
| QA / Tester | | | |
| Release Manager | | | |

**Rilis dapat dilakukan jika dan hanya jika semua skenario kritis (F1, F2, F4, F5) PASS dan sign-off lead dev tercatat.**
