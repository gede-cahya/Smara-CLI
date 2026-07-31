# Release Notes — Smara CLI v1.21.0

Update Smara CLI v1.21.0 merealisasikan spesifikasi PRD **Smara Advanced Scheduler & Resilience System**, menghadirkan peningkatan besar pada resiliensi otomatisasi, penjadwalan cron standar, auto-retry exponential backoff, ketergantungan tugas (DAG chaining), serta manajemen background service OS.

---

## 🚀 Perubahan Utama & Fitur Baru

### 1. **Dukungan Full Standard Cron Engine (`robfig/cron/v3`)**
- Mendukung format Cron standar 5-field dan 6-field (misal `*/5 9-17 * * 1-5`, `0 0 * * *`) serta *descriptors* (`@hourly`, `@daily`, `@weekly`, `@monthly`).
- Tetap kompatibel penuh dengan format intuitif bawaan (`every 15m`, `daily 09:00`).

### 2. **Auto-Retry & Exponential Backoff pada Kegagalan Job**
- Penambahan flag `--retries <n>` dan `--retry-interval <sec>` pada perintah `smara schedule add`.
- Saat eksekusi job mengalami kegagalan, sistem secara otomatis melakukan percobaan ulang (*retrying*) dengan jeda bertahap eksponensial (`10s ➔ 20s ➔ 40s`) sebelum mencatat status `failed`.

### 3. **DAG Job Chaining & Dependency (`--after <job_id>`)**
- Penambahan flag `--after <job_id>` untuk mengatur alur eksekusi berantai.
- Job turunan hanya akan dieksekusi jika job induk sebelumnya berstatus `success`.

### 4. **Manajemen Systemd Service Manager (`smara schedule service`)**
- `smara schedule service install` ➔ Otomatis membuat dan mendaftarkan service `smara-scheduler.service` di Systemd user unit agar otomatis aktif saat server/OS di-reboot.
- `smara schedule service status` ➔ Memeriksa keberadaan file dan status service.
- `smara schedule service uninstall` ➔ Menghapus service unit.

---

## 📦 Asset Rilis

| Platform | Nama File Asset |
| :--- | :--- |
| Linux AMD64 | `smara-v1.21.0-linux-amd64.tar.gz` |
| Linux ARM64 | `smara-v1.21.0-linux-arm64.tar.gz` |
| macOS AMD64 | `smara-v1.21.0-darwin-amd64.tar.gz` |
| macOS ARM64 | `smara-v1.21.0-darwin-arm64.tar.gz` |
| Windows AMD64 | `smara-v1.21.0-windows-amd64.zip` |

---

## 🔧 Cara Memperbarui

Download asset sesuai platform dari halaman GitHub Release `v1.21.0`, atau jalankan:

```bash
sudo smara update 1.21.0
```
