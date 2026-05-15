# Smara CLI 🌀
**Autonomous Multi-Agent Terminal v1.19.2**

[![Go Version](https://img.shields.io/github/go-mod/go-version/gede-cahya/Smara-CLI)](https://golang.org)
[![License](https://img.shields.io/github/license/gede-cahya/Smara-CLI)](LICENSE)
[![Latest Release](https://img.shields.io/github/v/release/gede-cahya/Smara-CLI)](https://github.com/gede-cahya/Smara-CLI/releases/latest)
[![Documentation](https://img.shields.io/badge/docs-smara--docs.vercel.app-bef264?logo=vercel)](https://smara-docs.vercel.app)

Smara (Sanskerta: स्मृति — *Ingatan*) adalah terminal pintar berbasis Go yang mengorkestrasi agen AI otonom dengan memori tim yang tersinkronisasi dan integrasi MCP (Model Context Protocol).

---

## ✨ Fitur Utama
- **Multi-Agent System**: Arsitektur Supervisor-Worker untuk pendelegasian tugas yang kompleks.
- **3 Mode Agen**:
  - `ask` (💬): Tanya-jawab cepat tanpa tools.
  - `rush` (⚡): Eksekusi cepat, langsung bertindak menggunakan tools.
  - `plan` (📋): Membuat rencana dan meminta persetujuan sebelum eksekusi.
- **🎨 Crush TUI Design**: Antarmuka terminal ultra-minimalis dengan palet warna **Pastel Green** yang premium.
- **🎬 Live Generate Animation**:
  - **Phase Tracking Pipeline**: Visual pipeline fase otomatis (`Thinking` → `Analyzing` → `Exploring` → `Generating`) dengan deskripsi real-time.
  - **Animated Thinking**: Spinner dots interaktif dengan rotasi fase.
  - **Real-time Stats**: Timer elapsed dan statistik penggunaan token (In/Out) yang persisten.
  - **Streaming Cursor**: Feedback visual real-time saat teks sedang di-generate.
  - **Tool Call Visualization**: Tampilan langsung saat agen mengeksekusi tool (▸) dan menerima hasil (◂).
- **🔀 Message Selection & Clipboard**: `Ctrl+S` untuk menyeleksi pesan historis, `Enter/C` untuk copy ke clipboard. `Ctrl+V` untuk paste dari clipboard.
- **🌐 MCP Auto-Discovery**: Auto-load MCP servers dari konfigurasi **Windsurf IDE**, **OpenCode**, dan file konfigurasi Smara-native secara paralel.
- **🛜 Remote MCP Support**: Koneksi ke MCP server remote via SSE/WebSocket (`mcp.NewRemoteClient`).
- **🧩 Skill Ecosystem v3 — Skill Tree & Analytics**:
  - Hierarchical skill tree dengan parent-child dan dependency edges.
  - Execution tracker (SQLite) untuk logging run, stats, timeline, dan success rate.
  - Skill Dashboard: visualisasi top skills, struggling skills, dan recent activity.
  - Interactive dependency graph (React Flow) untuk visualisasi hubungan antar skill.
  - Auto-Refinement Engine: usulkan perbaikan skill berdasarkan eksekusi history & feedback.
- **📁 SSH File Transfer**: Upload/download file dan direktori via SFTP/SCP langsung dari CLI.
- **Platform Integration**: Jalankan Smara sebagai bot di **Telegram**, **Discord**, dan **WhatsApp**.
- **Multi-Provider LLM**: Mendukung **Ollama (local)**, **Anthropic**, **OpenAI**, dan **OpenRouter**.
- **📦 Workspace Management**: Isolasi proyek, memori, dan sesi antar ruang kerja yang berbeda.
- **🧠 Smart Memory v2**: Hybrid Search, Versioning, dan Categorization untuk basis pengetahuan agen.
- **📊 Dashboard Monitoring**: TUI real-time untuk memantau metrik platform, LLM, dan MCP.
- **🖥️ SSH Remote Control**: Kelola VPS/Server langsung dari agen — `ssh_exec`, `ssh_view_file`, `ssh_list_dir` sebagai built-in agent tools.
- **👤 User Profile**: Profil adaptif pengguna (verbosity, risk tolerance, primary domains) yang diinject ke system prompt.
- **⏰ Nudge System**: Scheduled prompts/reminders dengan ekspresi cron untuk tugas rutin.
- **🛡️ Two-Step Safety**: Plan Mode (read-only) vs Build Mode (read-write) dengan Auto-Revert jika eksekusi gagal.
- **🔍 LSP Integration**: Language Server Protocol untuk Go, TypeScript, Python, dan Rust.
- **📜 Dedicated Audit Log**: Logging terstruktur JSON Lines untuk semua tindakan agen.
- **🕸️ Graphify — Knowledge Graph**: Parse codebase Go menjadi knowledge graph, simpan di SQLite, query dengan natural language, dan inject konteks graph ke system prompt agen secara otomatis. Export ke JSON/SVG/GraphML/Neo4j.
- **🧬 Memory Graph — Memori Saling Terhubung**: Setiap memori bisa di-link satu sama lain dengan relasi (`refines`, `supports`, `follows`, dst.) dan bobot. **Auto-link Smart Engine** otomatis pilih mode: 🧠 semantic (cosine similarity embedding) atau 📝 lexical fallback (Jaccard token overlap dengan stopword EN+ID) — jalan untuk semua provider. Visualisasi interaktif via tab Graph di Smara Web (React Flow) atau standalone `smara memory graph` (vis-network).
- **🖼️ Clipboard Image & Vision Tools**: Ctrl+V di TUI ambil gambar dari clipboard sistem (X11/Wayland/macOS/Windows), simpan ke `~/.smara/clip-images/`, dan inject `[image:/path]` ke prompt. Built-in tool `analyze_image` ekstrak metadata + OCR (tesseract); `clip_paste_image` & `clip_copy_image` untuk agent.
- **🧠 Adaptive Iteration Budget**: Batas eksekusi tool dinamis per mode (ASK 5, RUSH 15, WORKFLOW 30, dst.) dengan auto-extension saat model produktif dan stuck-loop detector kalau tool yang sama dipanggil berulang. Configurable via `agent_max_iterations`.
- **Auto-Update**: Sistem pembaruan otomatis bawaan menggunakan perintah `smara update`.
- **🌥️ Cloud Memory**: Sinkronisasi memori local-first antar device/workspace via Turso/libSQL embedded replica, dengan conflict policy, audit log redacted, dan mode headless CI.
- **🧾 Run History & Replay**: Semua workflow dapat dicatat sebagai run terstruktur, ditampilkan via `smara run history`, dan diulang dengan `smara run replay`.
- **🧪 Eval Suite**: `smara eval run` untuk smoke/evaluasi provider aktif dengan output human-readable atau JSON.
- **🛡️ Policy CLI**: `smara policy` untuk allow/ask/deny tool automation berdasarkan risk, action, dan target.
- **⏱️ Scheduler & Sharing**: `smara schedule` untuk job lokal terjadwal dan `smara share` untuk export/import artifact berbagi.
- **💾 Memory Save Compatibility**: `smara memory save <content>` kembali tersedia untuk backward compatibility local/cloud memory.

---

## 🌥️ Cloud Memory

Cloud Memory membuat database memori Smara tetap **local-first** (baca/tulis tetap cepat secara lokal) sambil tersinkronisasi ke Turso/libSQL untuk multi-device.

Quick start:
```bash
smara memory cloud login
smara memory cloud enable
smara memory cloud status
```

Multi-device: jalankan `smara memory cloud login` lalu `smara memory cloud enable` di device kedua; Smara akan memakai database cloud workspace yang sama bila sudah ada.

Workspace isolation: setiap workspace dipetakan ke database cloud terpisah. Contoh:
```bash
smara workspace create client-x
smara memory cloud workspaces
```
Gunakan `smara workspace create client-x --local-only` untuk skip provisioning cloud saat offline/kuota penuh.

Headless/CI mode:
```bash
export SMARA_CLOUD_TOKEN=...
export SMARA_CLOUD_ORG=...
export SMARA_CLOUD_REGION=sin
smara memory cloud login --headless
smara memory cloud enable
```
Contoh Docker minimal: set env vars di container, mount `~/.smara` sebagai volume, lalu jalankan command di atas saat bootstrap.

Conflict policy tersedia di config `cloud_memory.conflict_policy`:
- `lww` — default, last-write-wins deterministik.
- `manual` — konflik ditahan untuk review via `smara memory cloud conflicts`.
- `archive-loser` — pemenang dipakai, versi kalah tetap diarsipkan.
- `merge-content` — gabungkan konten catatan bebas.

Privacy & security: token tidak disimpan di YAML config; kredensial memakai keyring, env, atau fallback file `0600`. Audit log otomatis redact Bearer/JWT/token. Konten memori dan embedding tetap di database Turso milik user; embedding default tetap lewat Ollama lokal.

Troubleshooting cepat:
- Keyring tidak tersedia → Smara fallback ke `~/.smara/cloud-creds.json` mode `0600`.
- Replica bermasalah → nonaktifkan cloud, backup `~/.smara/memory.db`, lalu `enable` ulang untuk bootstrap.
- Kuota penuh → `status` tampilkan quota; kurangi data/cleanup atau naikkan kuota provider.

---

## 🎨 TUI Experience
Smara CLI menggunakan sistem desain **Crush** yang mengutamakan estetika dan kejelasan informasi:
- **Pastel Palette**: Dominasi warna hijau muda (`#bef264`) pada background gelap untuk kontras tinggi dan kelelahan mata rendah.
- **Harmonica Physics**: Sidebar dan transisi UI menggunakan animasi spring physics yang organik.
- **Completion Footer**: Detail model dan performa (durasi, token) disematkan rapi di bawah setiap respons agen.
- **Phase Pipeline**: Progress bar visual yang menampilkan fase kognitif agen secara real-time.
- **Fade Wave Animation**: Efek gelombang transisi antar fase untuk pengalaman visual yang halus.


---

## 🚀 Instalasi Cepat

### Linux / macOS
```bash
curl -fsSL https://raw.githubusercontent.com/gede-cahya/Smara-CLI/main/install.sh | sh
```

### Windows (PowerShell)
```powershell
irm https://raw.githubusercontent.com/gede-cahya/Smara-CLI/main/install.ps1 | iex
```

### Desktop (Wails)
Smara juga tersedia dalam bentuk aplikasi desktop modern:
```bash
cd smara-desktop
wails build
# atau untuk development
wails dev
```
*Memerlukan Go 1.23+, Node.js, dan Wails CLI terinstall.*

---

## 📖 Panduan Penggunaan
Untuk melihat panduan interaktif langsung di terminal, jalankan:
```bash
smara guide
```

### 1. Login ke Provider
Gunakan perintah `login` untuk menyimpan API key secara aman:
```bash
smara login
```

### 2. Pilih Model
```bash
smara provider select
```

---

## 📦 Workspace & Proyek
Gunakan workspace untuk memisahkan konteks antar proyek:
```bash
smara workspace create "Project X"
smara workspace use "Project X"
smara workspace list
```

---

## �️ Graphify — Knowledge Graph

Generate and query knowledge graphs from Go codebases for deep structural understanding.

```bash
# Parse codebase and store graph
smara graphify init ./cmd --name smara-cmd

# Query graph with natural language
smara graphify query "auth flow" --name smara-cmd --depth 2

# Find shortest path between nodes
smara graphify path "A" "B" --name smara-cmd

# Explain a node and its neighborhood
smara graphify explain "NodeID" --name smara-cmd --depth 1

# Export graph
smara graphify export --name smara-cmd --format json     # JSON
smara graphify export --name smara-cmd --format svg       # SVG
smara graphify export --name smara-cmd --format graphml   # GraphML
smara graphify export --name smara-cmd --format neo4j     # Neo4j Cypher

# List & manage stored graphs
smara graphify list
smara graphify delete smara-cmd
```

When the agent detects a codebase question, it **auto-injects** the relevant subgraph into the system prompt for context-aware answers.

---

## �️ SSH Remote Control (Agent Tools)
Agen dapat mengeksekusi perintah di VPS/Server langsung dari percakapan:
```bash
# Tambah host SSH
smara ssh add-host prod --host 192.168.1.1 --user ubuntu --key ~/.ssh/id_rsa

# Eksekusi perintah remote
smara ssh exec prod "docker ps -a"

# Sesi SSH interaktif
smara ssh connect prod

# Upload file ke remote
smara ssh upload prod ./local-file.txt /home/ubuntu/

# Download file dari remote
smara ssh download prod /var/log/app.log ./logs/

# Generate key pair
smara ssh keygen --name deploy-key --type ed25519

# Riwayat eksekusi & transfer
smara ssh logs --limit 20
smara ssh transfer-logs --limit 20
```

Saat `smara start`, agen otomatis mendapatkan konteks semua host SSH yang tersimpan di system prompt.

---

## 📊 Monitoring Dashboard
Pantau aktivitas bot dan performa agen secara real-time:
```bash
smara dashboard
```
*Gunakan flag `--once` untuk snapshot cepat atau `--refresh 5s` untuk mengatur interval.*

---

## � Skill Ecosystem

Kelola reusable automation skills dan marketplace:

```bash
# Jalankan skill tersimpan
smara skill run deploy-backend

# Install skill dari URL
smara skill install https://example.com/skills/deploy.json

# Buat skill baru dari stdin (JSON atau Markdown)
smara skill create deploy-backend --format json

# Cari skill di registry
smara skill search "deploy"

# Info detail skill
smara skill info deploy-backend

# Publikasikan ke registry
smara skill publish deploy-backend

# Sync cache registry
smara skill registry sync

# Skill Tree & Analytics
smara skill tree                    # Tampilkan hierarki skill tree
smara skill stats deploy-backend    # Statistik eksekusi skill
smara skill refine deploy-backend   # Trigger manual refinement
smara skill analytics               # Global skill analytics
```

---

## �� Manajemen Memori
Kelola basis pengetahuan agen Anda:
```bash
# Pencarian Hybrid (Semantic + Keyword)
smara memory search "cara deploy nextjs" --hybrid

# Kelola Kategori
smara category create "Coding" --description "Snippet dan tutorial"
smara category list

# Versi & Rollback
smara memory history [ID]
smara memory rollback [ID] [VersionID]

# Backup & Restore
smara memory export backup.zip --format zip
smara memory import backup.zip
```

---

## 🧬 Memory Graph — Memori Saling Terhubung

Setiap memori dapat dihubungkan satu sama lain. Auto-link engine otomatis pilih mode terbaik berdasarkan ketersediaan embedding di provider Anda.

```bash
# Manual link
smara memory link 12 34 --relation refines --weight 0.8 --note "iterasi v2"
smara memory unlink 7
smara memory links 12

# Auto-link otomatis (semantic kalau provider punya embeddings, lexical kalau tidak)
smara memory autolink --threshold 0.78 --top-k 5

# Visualisasi interaktif
smara memory graph              # buka di browser default
smara memory graph --port 7878  # custom port
smara memory graph --export graph.json    # export untuk dipakai tool lain
```

Visualisasi juga tersedia di Smara Web — pindah ke tab **Memory → Graph** untuk explore graph dengan node hover, search filter, dan klik untuk drill-down ke detail memori beserta neighbor-nya. Tema mengikuti **Pastel Green Crush** design system.

---

## 🌐 Smara Serve (Platform Bot)
Smara dapat dijalankan sebagai layanan bot yang aktif terus-menerus.

```bash
# Jalankan semua platform yang dikonfigurasi
smara serve

# Jalankan platform spesifik dengan mode default 'plan'
smara serve --platform telegram --mode plan
```

### 🤖 Perintah Bot Messaging:
- `/ask <prompt>` — Kirim pertanyaan cepat.
- `/mode <ask|rush|plan>` — Ganti mode agen.
- `/mcp` — Lihat daftar tool yang tersedia.
- `/clear` — Reset sesi percakapan.

---

## 🛠️ Perintah CLI Lainnya
- `smara start`: Mulai sesi interaktif TUI.
- `smara provider list`: Lihat provider yang tersedia.
- `smara config list`: Cek konfigurasi saat ini.
- `smara update`: Perbarui ke versi terbaru.
- `smara version`: Tampilkan informasi versi.
- `smara ssh`: Manajemen VPS via SSH (add-host, exec, connect, upload, download, keygen, logs).
- `smara skill`: Kelola reusable automation skill (create, run, install, search, publish, info, delete).
- `smara explore`: Eksplorasi struktur proyek secara visual.
- `smara guide`: Panduan interaktif langsung di terminal.
- `smara dashboard`: Monitoring real-time TUI.
- `smara serve`: Jalankan platform bot (Telegram, Discord, WhatsApp).
- `smara graphify`: Generate knowledge graph dari codebase Go (init, query, path, explain, export, list, delete).
- `smara memory graph`: Visualisasi memory graph interaktif (vis-network di browser).
- `smara memory link/unlink/links`: Kelola koneksi antar memori secara manual.
- `smara memory autolink`: Bangun koneksi otomatis (semantic atau lexical fallback).

---

## ⚙️ Konfigurasi Detail (`config.yaml`)
```yaml
provider: anthropic
model: claude-3-5-sonnet-latest
ollama_host: http://localhost:11434
platforms:
  telegram:
    enabled: true
    token: "YOUR_TOKEN"
    allowed_users: ["12345678"]
  discord:
    enabled: true
    token: "YOUR_TOKEN"
    allowed_roles: ["smara-user"]
  whatsapp:
    enabled: true

skill_registries:
  - name: default
    url: https://raw.githubusercontent.com/gede-cahya/Smara-Skills/main/registry.json
```

---

## 🧪 Testing

Jalankan suite unit test lengkap:
```bash
go test ./...
```

Test spesifik package:
```bash
go test ./pkg/agent/... ./internal/agent/... ./internal/memory/... ./internal/audit/...
```

Build & release cross-platform:
```bash
make test
make release
```

---

## 📄 Lisensi
MIT License - © 2026 Gede Cahya.
