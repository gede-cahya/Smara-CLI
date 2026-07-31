# Release Notes — Smara CLI v1.20.71

Update Smara CLI v1.20.71 menghadirkan optimasi performa besar-besaran untuk *streaming response*, penanganan prompt panjang, pengolahan gambar instan, serta eliminasi penundaan pre-processing di Omniroute/9Router.

---

## 🚀 Perubahan Utama & Peningkatan Performa

### 1. **Optimasi Fast-Path Skema Tools (108 KB → ~2 KB Payload)**
- Prompt penjelasan kode mentah, analisis gambar, serta perintah lanjutan (`lanjutkan`) kini secara otomatis dikirim dengan **0 tools** (payload pangkas dari 108 KB menjadi ~2 KB).
- Menghilangkan *overhead* 80+ tools MCP yang sebelumnya menyebabkan penundaan 503 / connection test di Omniroute.

### 2. **Pengolahan Gambar Lokal & OCR Cepat**
- Limit waktu Tesseract OCR lokal dipercepat dari 15s/20s menjadi **maksimal 6 detik**.
- Ditambahkan indikator status transparan di UI: `Analyzing Image: Mengekstrak teks OCR & metadata gambar...`.

### 3. **Perpanjangan Turn Timeout & Smart Context Capping**
- Post-tool LLM Timeout ditingkatkan dari 45s menjadi 180s (3 menit), dan Turn Timeout ditingkatkan dari 60s menjadi 300s (5 menit) untuk mendukung pengeluaran script 400+ baris tanpa terpotong.
- Penambahan pemotongan riwayat pesan lampau (*smart history capping*) di atas 12,000 karakter untuk menjaga stabilitas *context window*.

### 4. **Pembersihan & Eliminasi Context7 Auto-Injector**
- Penghapusan total blok pre-processing `Context7Injector` dari pipeline supervisor untuk menghilangkan penundaan 15–20 detik pada sesi baru dengan prompt/kode panjang.

### 5. **Penganganan Embedding Fast-Path & Context Timeout**
- Menambahkan fast-path skip 0ms untuk model embedding Omniroute/antigravity yang tidak mendukung `/v1/embeddings`.
- Menambahkan *context timeout* 2 detik ketat pada panggilan embedding custom provider.

---

## 📦 Asset Rilis

| Platform | Nama File Asset |
| :--- | :--- |
| Linux AMD64 | `smara-v1.20.71-linux-amd64.tar.gz` |
| Linux ARM64 | `smara-v1.20.71-linux-arm64.tar.gz` |
| macOS AMD64 | `smara-v1.20.71-darwin-amd64.tar.gz` |
| macOS ARM64 | `smara-v1.20.71-darwin-arm64.tar.gz` |
| Windows AMD64 | `smara-v1.20.71-windows-amd64.zip` |

---

## 🔧 Cara Memperbarui

Download asset sesuai platform dari halaman GitHub Release `v1.20.71`, atau jalankan:

```bash
sudo smara update 1.20.71
```
