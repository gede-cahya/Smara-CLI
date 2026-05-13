# PRD v1.17.0 — Build Testing Plan

Dokumen test plan manual untuk rilis Smara CLI v1.17.0. Mencakup checklist end-to-end per fitur dan acceptance criteria.

---

## Environment Setup

### Prerequisites
- Go 1.23+
- Node.js 18+ (untuk Web UI)
- SQLite 3
- Ollama (opsional, untuk local LLM) atau API key (Anthropic/OpenAI/OpenRouter)
- VPS/SSH dummy (opsional, untuk test SSH)
- Telegram Bot Token (opsional, untuk test platform)

### Build
```bash
make build        # Native binary
make test         # Unit tests
cd web && npm i && npm run build  # Frontend
```

---

## Fitur: Skill Tree & Analytics

### Acceptance Criteria
- Skill dapat memiliki `parent_id`, `category_path`, dan `dependencies`.
- API `/api/skills/tree` mengembalikan hierarki lengkap.
- API `/api/skills/tree?format=graph` mengembalikan nodes + edges untuk React Flow.
- API `/api/skills/stats?name=X` mengembalikan `total_runs`, `success_rate`, `avg_duration_ms`, `last_run`.
- API `/api/skills/timeline?name=X` mengembalikan daftar eksekusi terakhir.
- API `/api/skills/analytics` mengembalikan `top_skills` dan `struggling`.
- CLI `smara skill tree` menampilkan hierarki di terminal.
- CLI `smara skill stats X` dan `smara skill analytics` tersedia.

### Skenario Positif
1. Buat skill dengan `category_path: ["deploy", "backend"]`, jalankan via web → muncul di tree.
2. Jalankan skill 3x → stats menunjukkan `total_runs: 3`.
3. Buka tab "Skill Tree" di web UI → tree terekspansi.
4. Buka tab "Analytics" → card total runs dan success rate terisi.

### Skenario Negatif
1. Request `/api/skills/stats` tanpa `name` → 400 Bad Request.
2. Request `/api/skills/tree/NONEXISTENT` → 404 Not Found.
3. Tracker tidak tersedia (DB belum diinisialisasi) → 503 Service Unavailable.

---

## Fitur: Auto-Refinement Engine

### Acceptance Criteria
- `ShouldRefine` mengembalikan `true` jika success rate < threshold (default 70%) dan total runs >= 3.
- `BuildRefinementPromptFull` menyusun prompt yang mengandung skill JSON + execution history + feedback.
- CLI `smara skill refine X` menghasilkan proposed JSON refinement (butuh LLM provider).

### Skenario Positif
1. Jalankan skill 3x dengan 2 kegagalan → `ShouldRefine` = true.
2. Jalankan `smara skill refine X` dengan provider aktif → muncul proposed JSON.

### Skenario Negatif
1. Jalankan `smara skill refine X` tanpa provider → error "LLM provider tidak tersedia".

---

## Fitur: Execution Tracker

### Acceptance Criteria
- Tabel `skill_executions` dan `skill_improvements` dibuat otomatis saat DB init.
- Setiap `handleSkillRun` web API memicu `LogRun` ke tracker.
- `GlobalAnalytics` menghitung top skills dan struggling skills secara akurat.

### Skenario Positif
1. Jalankan skill via web UI → cek SQLite `skill_executions` bertambah 1 row.
2. Cek `/api/skills/analytics` → total_runs meningkat.

---

## Fitur: Web UI — Skill Graph

### Acceptance Criteria
- Tab "Skill Tree" menampilkan hierarki kategorisasi (collapsible).
- Tab "Analytics" menampilkan summary cards, top skills, struggling skills, dan recent activity.
- React Flow graph menampilkan nodes dan edges parent/dependency.

### Skenario Positif
1. Buka web UI → klik tab "Skill Tree" → tree muncul.
2. Klik tab "Analytics" → cards dan timeline muncul.

---

## Fitur: MCP Auto-Discovery

### Acceptance Criteria
- Saat startup, Smara membaca `mcp_config.json` (OpenCode, Windsurf, Smara-native).
- MCP server remote tersambung via `mcp.NewRemoteClient`.
- Daftar tool dari setiap MCP muncul di system prompt agen.

### Skenario Positif
1. Konfigurasi MCP valid → semua tool tersedia di `/api/mcp`.
2. `smara start` → system prompt mencantumkan nama tool dari MCP.

---

## Fitur: SSH Remote Control

### Acceptance Criteria
- `smara ssh add-host` menyimpan konfigurasi host ke SQLite.
- `smara ssh exec` mengeksekusi perintah di remote dan menampilkan output.
- Agen dapat menggunakan tool `ssh_exec` saat mode rush/plan.

### Skenario Positif
1. `smara ssh add-host test --host 127.0.0.1 --user test` → host tersimpan.
2. Agen: "run docker ps on test" → eksekusi `ssh_exec` berhasil.

---

## Fitur: Dashboard Monitoring

### Acceptance Criteria
- `smara dashboard` menampilkan TUI real-time dengan metrik platform, LLM, MCP.
- Flag `--once` menghasilkan snapshot tanpa loop.
- Flag `--refresh 5s` mengatur interval refresh.

---

## Fitur: Safety & Audit

### Acceptance Criteria
- Plan Mode (read-only) tidak mengizinkan tool write tanpa persetujuan.
- Build Mode (read-write) mencatat tindakan ke audit log.
- Audit log tersimpan sebagai JSON Lines.

---

## Regression Testing
- `go test ./...` lulus tanpa error.
- `make build` berhasil di linux/amd64.
- `cd web && npx tsc --noEmit` lulus tanpa error TypeScript.
- CLI commands yang sudah ada (`start`, `serve`, `memory`, `skill`, `ssh`, `dashboard`) tetap berfungsi.

---

## Definisi Pass/Fail
- **Pass**: Semua skenario positif berhasil dieksekusi sesuai acceptance criteria. Regression test lulus.
- **Fail**: Terdapat skenario positif yang gagal, error runtime, atau regression test gagal.
