# Release Notes — Smara CLI v1.20.70

Update Smara CLI v1.20.70 menambahkan dukungan domain Finance & Statistical Analytics pada PRD generator, serta pembersihan otomatis frasa perintah dari narasi dokumen PRD.

## Highlights

### 🚀 Finance & Statistical Analytics Domain Support

Release ini menambahkan sintesis domain khusus untuk aplikasi Keuangan dan Analisis Statistik:
1. **Financial & Statistical Context**: Narasi *Overview*, *Problem Statement*, *Goals*, dan *Requirements* yang disesuaikan secara khusus untuk pengolahan transaksi keuangan, rasio finansial, dan pemodelan statistik.
2. **Clean Request Prefix Stripping**: Otomatis mengeliminasi frasa perintah awal seperti `buatkan website `, `bikin app `, `tolong buatkan ` dari bagian narasi PRD fallback agar kalimat dokumen 100% natural.
3. **Dedicated Mermaid Flowchart & Diagrams**: Visualisasi *User Flowchart*, *Sequence Diagram*, *State Machine*, dan *Roadmap (Gantt Chart)* yang dirancang khusus untuk alur kerja analitik keuangan.

## Tested

- Unit test Discord PRD: `go test -v ./internal/platform/discord/...` ✓
- Unit test Agent Core: `go test -v ./internal/agent/...` ✓
- Cross-compile: Linux AMD64/ARM64, macOS AMD64/ARM64, Windows AMD64 ✓
- Checksums: `SHA256SUMS` ✓

## Platform Artifacts

| Platform | Archive |
|----------|---------|
| Linux AMD64 | `smara-v1.20.70-linux-amd64.tar.gz` |
| Linux ARM64 | `smara-v1.20.70-linux-arm64.tar.gz` |
| macOS AMD64 | `smara-v1.20.70-darwin-amd64.tar.gz` |
| macOS ARM64 | `smara-v1.20.70-darwin-arm64.tar.gz` |
| Windows AMD64 | `smara-v1.20.70-windows-amd64.zip` |

Raw archives and `SHA256SUMS` are attached for direct/manual downloads.

## Upgrade

Download asset sesuai platform dari halaman GitHub Release `v1.20.70`, atau jalankan:

```bash
sudo smara update 1.20.70
```
