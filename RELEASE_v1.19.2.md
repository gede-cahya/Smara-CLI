# Release Notes — Smara CLI v1.19.2

Hotfix untuk Telegram bot supaya benar-benar bisa **lihat gambar** yang dikirim user, bukan cuma menjawab dari knowledge umum.

## Highlights

### 🖼️ Telegram Photo Attachment Support — Fixed End-to-End

Sebelum patch ini, Telegram bot menerima foto user tapi **tidak download file-nya**. Caption pun salah ambil (`tgMsg.Text` selalu kosong untuk pesan foto). Hasilnya: bot jawab pertanyaan tentang "gambar ini" pakai general knowledge.

Yang diperbaiki:

1. **Caption ekstraksi** — `convertMessage` sekarang fallback ke `tgMsg.Caption` saat `tgMsg.Text` kosong. Ini standard di Telegram untuk pesan foto/video.

2. **Attachment download** — Telegram adapter expose `DownloadAttachment(ctx, fileID) (path, error)` via interface baru `platform.AttachmentDownloader`. File disimpan ke `~/.smara/clip-images/tg-<fileID>.jpg`.

3. **Prompt injection di gateway** — sebelum supervisor dipanggil, gateway iterasi `msg.Attachments`, download semua tipe `image`, lalu prefix prompt dengan `[image:/path/to/file.jpg]` token. Plus steering message:
   > [Sistem: pesan ini menyertakan gambar. Pakai tool analyze_image dengan path tersebut...]
   
   Ini bikin agent otomatis panggil `analyze_image` daripada menebak.

### Cara Pakai

Cukup kirim foto ke bot Telegram dengan caption yang berisi pertanyaan:

```
[foto]
caption: "ini error apa?"
```

Bot akan:
1. Download foto ke local file
2. Panggil tool `analyze_image` → dapat metadata (size/dimensi/format) + OCR text via tesseract
3. Jawab berdasarkan content gambar

Untuk OCR, install tesseract di server bot:
```bash
# Ubuntu/Debian
sudo apt install tesseract-ocr tesseract-ocr-eng tesseract-ocr-ind
# Arch
sudo pacman -S tesseract tesseract-data-eng tesseract-data-ind
```

## Tested

- Build: `go build ./...` ✓
- Manual test di Telegram VPS: photo + caption "ini gambar apa" → bot jawab dengan metadata JPEG 267×108 + OCR result ✓
- Backward compat: pesan teks tanpa attachment tetap jalan seperti biasa ✓

## Files Changed

- `internal/platform/telegram/telegram.go` — `DownloadAttachment`, caption fallback
- `internal/platform/adapter.go` — `AttachmentDownloader` interface
- `internal/platform/gateway.go` — download + inject `[image:/path]` token

## Platform Artifacts

| Platform       | Archive                                       |
| -------------- | --------------------------------------------- |
| Linux AMD64    | `smara-v1.19.2-linux-amd64.tar.gz`            |
| macOS AMD64    | `smara-v1.19.2-darwin-amd64.tar.gz`           |
| macOS ARM64    | `smara-v1.19.2-darwin-arm64.tar.gz`           |
| Windows AMD64  | `smara-v1.19.2-windows-amd64.zip`             |

## Upgrade

```bash
smara update 1.19.2
sudo systemctl restart smara   # kalau jalan sebagai service di VPS
```
