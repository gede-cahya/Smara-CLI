# Release Notes — Smara CLI v1.20.69

Update Smara CLI v1.20.69 membawa peningkatan fitur Discord PRD Wizard (9 Quest Interaktif), generator PRD kontekstual domain (Web/Food, Bot, SaaS), serta isolasi lingkungan eksekusi agen.

## Highlights

### 🚀 Interactive Discord PRD Wizard (9 Quests)

Release ini memperluas alur PRD interaktif pada Discord Bot dari 7 quest menjadi 9 quest komprehensif:
1. **Tech Stack Preference**: Pilihan arsitektur teknis (`Fullstack JS/TS`, `Go REST API`, `Python / AI`, `Mobile Native`, `Auto-Recommend`).
2. **Prioritas Utama MVP**: Pilihan fokus rilis (`Speed-to-Market`, `Security & Data`, `High Scalability`, `Premium UX & Design`).
3. **Pemesanan & Visualisasi Interaktif**: Pengguna dapat menyelesaikan 9 quest melalui tombol Discord hingga generasi file `.md` otomatis.

### 🧩 Contextual Domain PRD Generator & Diagrams

Perubahan utama generator PRD:
- **Judul Produk Bersih**: Mengeliminasi frasa perintah (`Buatkan Website...`) dan mengonversi menjadi judul bersih (contoh: `Website Produk Makanan Instan (Mie)`).
- **Domain Context Awareness**: Mengidentifikasi konteks ide produk secara cerdas untuk menyusun *Overview*, *Problem Statement*, *Goals*, dan *Requirements* yang realistis.
- **Mermaid Diagrams Lengkap**: Generasi otomatis *User Flowchart*, *Sequence Diagram*, *State Machine Diagram*, dan *Implementation Roadmap (Gantt Chart)*.

### 🔧 Agent Safety & Environment Isolation
- Memperbaiki penanganan variabel lingkungan RUSH mode agar terisolasi dengan baik saat terjadi pergantian mode agen.

## Tested

- Unit test Discord PRD: `go test -v ./internal/platform/discord/...` ✓
- Unit test Agent Core: `go test -v ./internal/agent/...` ✓
- Cross-compile: Linux AMD64/ARM64, macOS AMD64/ARM64, Windows AMD64 ✓
- Checksums: `SHA256SUMS` ✓

## Platform Artifacts

| Platform | Archive |
|----------|---------|
| Linux AMD64 | `smara-v1.20.69-linux-amd64.tar.gz` |
| Linux ARM64 | `smara-v1.20.69-linux-arm64.tar.gz` |
| macOS AMD64 | `smara-v1.20.69-darwin-amd64.tar.gz` |
| macOS ARM64 | `smara-v1.20.69-darwin-arm64.tar.gz` |
| Windows AMD64 | `smara-v1.20.69-windows-amd64.zip` |

Raw archives and `SHA256SUMS` are attached for direct/manual downloads.

## Upgrade

Download asset sesuai platform dari halaman GitHub Release `v1.20.69`, lalu ekstrak dan ganti binary `smara` lama dengan versi terbaru.
