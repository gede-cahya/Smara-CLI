# Smara CLI 🌀
**Autonomous Multi-Agent Terminal v1.6.1**

[![Go Version](https://img.shields.io/github/go-mod/go-version/gede-cahya/Smara-CLI)](https://golang.org)
[![License](https://img.shields.io/github/license/gede-cahya/Smara-CLI)](LICENSE)
[![Latest Release](https://img.shields.io/github/v/release/gede-cahya/Smara-CLI)](https://github.com/gede-cahya/Smara-CLI/releases/latest)

Smara (Sanskerta: स्मृति — *Ingatan*) adalah terminal pintar berbasis Go yang mengorkestrasi agen AI otonom dengan memori tim yang tersinkronisasi dan integrasi MCP (Model Context Protocol).

---

## ✨ Fitur Utama
- **Multi-Agent System**: Arsitektur Supervisor-Worker untuk pendelegasian tugas yang kompleks.
- **3 Mode Agen**:
  - `ask` (💬): Tanya-jawab cepat tanpa tools.
  - `rush` (⚡): Eksekusi cepat, langsung bertindak menggunakan tools.
  - `plan` (📋): Membuat rencana dan meminta persetujuan sebelum eksekusi.
- **Platform Integration**: Jalankan Smara sebagai bot di **Telegram** dan **Discord**.
- **Multi-Provider LLM**: Mendukung **Ollama (local)**, **Anthropic**, **OpenAI**, dan **OpenRouter**.
- **Persistent Sessions**: Riwayat percakapan dan status agen kini tersimpan secara otomatis di SQLite, memungkinkan Anda melanjutkan sesi sebelumnya.
- **Smart Memory**: Menggunakan SQLite & Vector Search untuk menyimpan konteks percakapan lintas platform.
- **MCP Integration**: Secara otomatis mendeteksi dan menghubungkan ke server MCP dari OpenCode.
- **Auto-Update**: Sistem pembaruan otomatis bawaan menggunakan perintah `smara update`.

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

---

## 🔑 Setup & Konfigurasi

### 1. Login ke Provider
Gunakan perintah `login` untuk menyimpan API key secara aman:
```bash
smara login
```

### 2. Pilih Model
```bash
smara provider select
```

### 3. Konfigurasi Bot (Optional)
Tambahkan token di `~/.smara/config.yaml` atau via environment variables:
```bash
export SMARA_TELEGRAM_TOKEN="your_bot_token"
export SMARA_DISCORD_TOKEN="your_bot_token"
```

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

## 📝 Changelog v1.6.1
- **Fix Terminal Output Capture**: Perbaikan mekanisme pembacaan pipe stdout/stderr secara konkuren untuk memastikan LLM mendapatkan seluruh output terminal.
- **Output Safety**: Pembatasan panjang output terminal untuk efisiensi konteks LLM.

---

## 📝 Changelog v1.6.0
- **Persistent Sessions**: Implementasi penyimpanan sesi otomatis ke database SQLite.
- **Auto-Resume**: Kemampuan untuk memuat dan melanjutkan sesi aktif terakhir saat aplikasi dijalankan.
- **Improved History Management**: Sinkronisasi riwayat percakapan yang lebih baik antara memori lokal dan database.

---

## 📝 Changelog v1.5.0
- **Full Streaming Support**: Implementasi real-time streaming untuk OpenAI, OpenRouter, Anthropic, dan Custom Provider.
- **Thinking Blocks**: Dukungan visual untuk fase penalaran (thinking) pada model yang mendukung.
- **Optimized Performance**: Refactor logika streaming untuk efisiensi memory dan latensi yang lebih rendah.

---

## 📝 Changelog v1.4.0
- **Platform Integration**: Dukungan penuh untuk bot **Telegram** dan **Discord**.
- **Server Mode**: Perintah `smara serve` untuk menjalankan bot sebagai layanan.
- **Gateway System**: Routing pesan yang efisien dengan autentikasi dan rate-limiting.
- **Shared Memory**: Sinkronisasi memori antara terminal CLI dan bot messaging.
- **Refactoring**: Peningkatan stabilitas supervisor agent dan penanganan sesi.

---

## 🛠️ Perintah CLI Utama
- `smara start`: Mulai sesi interaktif TUI.
- `smara provider list`: Lihat provider yang tersedia.
- `smara config list`: Cek konfigurasi saat ini.
- `smara update`: Perbarui ke versi terbaru.
- `smara version`: Tampilkan informasi versi.

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
    rate_limit: 20
  discord:
    enabled: true
    token: "YOUR_TOKEN"
    allowed_roles: ["smara-user"]
```

---

## 📄 Lisensi
MIT License - © 2026 Gede Cahya.
