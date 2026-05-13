# Release Notes — Smara CLI v1.19.0

## Highlights

### 🕸️ Memory Graph — Memori yang Saling Terhubung
Fitur baru utama. Setiap memori sekarang bisa dihubungkan satu sama lain dan divisualisasikan sebagai jaringan interaktif.

- **Schema `memory_links`**: tabel relasi (source, target, relation, weight, auto_linked, note) dengan FK cascade ke `memories`.
- **Manual link**: hubungkan dua memori dengan relasi kustom (`refines`, `follows`, `contradicts`, `supports`, dll).
- **Auto-link Smart Engine**:
  - Mode **🧠 semantic** (cosine similarity dari embedding) saat ≥ 30% memori punya vektor.
  - Mode **📝 lexical fallback** (Jaccard token overlap, stopword EN+ID) saat provider tidak menyediakan endpoint `/embeddings`.
  - Pemilihan otomatis — jalan untuk **semua provider** (OpenAI, Anthropic, Ollama, OpenRouter, custom).
- **Visualisasi interaktif**:
  - Tab baru **Graph** di halaman Memory (Smara Web) menggunakan `@xyflow/react`.
  - Standalone server `smara memory graph` dengan vis-network (offline-friendly via CDN).
  - Color-scale by node degree, edge solid untuk manual link, dashed untuk auto-link.
  - Side panel detail dengan klik-untuk-navigasi ke neighbor.
  - Live filter, threshold/top-k tunable di UI.

### 🧠 CLI Commands Baru
```bash
smara memory link 12 34 --relation refines --weight 0.8 --note "v2 dari ide ini"
smara memory unlink 7
smara memory links 12
smara memory autolink --threshold 0.78 --top-k 5
smara memory graph              # buka visualisasi di browser
smara memory graph --port 7878 --no-open
smara memory graph --export graph.json
```

### 🌐 Web API Baru
- `GET /api/memories/graph` — nodes + edges siap pakai untuk visualisasi.
- `GET /api/memories/links?memory_id=N` — list link untuk satu memori.
- `POST /api/memories/links` — buat link manual.
- `DELETE /api/memories/links?id=N` — hapus link.
- `POST /api/memories/autolink` — jalankan auto-link engine, returns mode & stats.

### 🎨 Web UI Improvements (Carry-over)
- **Custom Workflow** tab: visual node editor untuk multi-agent workflow.
- **Graphify Integration**: query knowledge graph dari web.
- **Skill Constellation**: enhanced React Flow visualization untuk skill dependency.
- **Skill Tree**: redesigned hierarchy view dengan parameter substitution.

### 🔧 Agent & Platform
- `__PARAM__name` placeholder untuk skill step arguments — composable skills.
- Context7 fallback routing di supervisor.
- `skill_run` tool untuk eksekusi skill langsung dari agent.
- Gateway routing improvements untuk Telegram/Discord/WhatsApp.

### 🛠️ Fixes & Internals
- Memory store schema migration auto-create `memory_links` saat `Init`.
- LLM custom provider, openai client refinements.
- MCP client robustness untuk discovery & remote connections.
- Workflow blueprint, orchestrator, runner improvements.

## Upgrade Guide

```bash
smara update 1.19.0
# atau download manual dari Releases
```

Setelah upgrade, jalankan sekali:
```bash
smara memory autolink   # bangun koneksi otomatis dari memori existing
smara memory graph      # buka visualisasi
```

## Platform Artifacts

| Platform       | Archive                                       |
| -------------- | --------------------------------------------- |
| Linux AMD64    | `smara-v1.19.0-linux-amd64.tar.gz`            |
| macOS AMD64    | `smara-v1.19.0-darwin-amd64.tar.gz`           |
| macOS ARM64    | `smara-v1.19.0-darwin-arm64.tar.gz`           |
| Windows AMD64  | `smara-v1.19.0-windows-amd64.zip`             |

## Tested

- `go build ./...` ✓
- `go test ./internal/memory/...` ✓ (3 new tests for lexical fallback)
- `npm run build` (web frontend) ✓
- Cross-platform release build via `make release` ✓
