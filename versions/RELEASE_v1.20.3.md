# Release Notes — Smara CLI v1.20.3

Update Smara CLI v1.20.3 untuk meningkatkan dukungan analisis gambar dari Discord dan menjaga proses OCR lebih stabil.

## Highlights

### 🖼️ Discord Image Attachment & Reply Support — Improved End-to-End

Release ini memperkuat alur bot Discord saat user mengirim atau me-reply gambar:

1. **Attachment dan embed image Discord** — adapter sekarang mengambil gambar dari attachment, embed image, dan thumbnail.

2. **Referenced message support** — saat user reply pesan Discord yang berisi gambar, bot mengambil referenced message lalu meneruskan gambar tersebut ke gateway.

3. **Direct image analysis path** — jika prompt user memang meminta analisis gambar, gateway langsung menjalankan tool analisis gambar pada file yang sudah di-download, sehingga jawaban lebih cepat dan tidak bergantung pada reasoning umum.

4. **OCR timeout guard** — proses `tesseract` dibatasi timeout 20 detik agar platform bot tidak menggantung terlalu lama pada gambar berat atau OCR bermasalah.

### 🧰 Automation & Reliability Tooling

- Workflow release tetap memakai format referensi `RELEASE_v1.19.2.md`.
- Skill release diperbarui agar default repo path langsung ke `/home/cahya/2026/Smara CLI`.
- Version bump source disinkronkan ke `VERSION` dan `cmd/smara/version.go`.

## Tested

- Build: `go build ./...` ✓
- Tests: `go test ./...` ✓
- Cross-compile: Linux AMD64, macOS AMD64/ARM64, Windows AMD64 ✓

## Files Changed

- `VERSION`
- `cmd/smara/version.go`
- `internal/agent/analyze_image.go`
- `internal/platform/discord/discord.go`
- `internal/platform/gateway.go`
- `pkg/platform/discord/discord.go`

## Platform Artifacts

| Platform       | Archive                                       |
| -------------- | --------------------------------------------- |
| Linux AMD64    | `smara-v1.20.3-linux-amd64.tar.gz`         |
| macOS AMD64    | `smara-v1.20.3-darwin-amd64.tar.gz`        |
| macOS ARM64    | `smara-v1.20.3-darwin-arm64.tar.gz`        |
| Windows AMD64  | `smara-v1.20.3-windows-amd64.zip`          |

## Upgrade

```bash
smara update 1.20.3
sudo systemctl restart smara   # kalau jalan sebagai service di VPS
```
