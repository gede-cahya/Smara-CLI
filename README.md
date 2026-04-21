# Smara CLI 🌀

**Autonomous Multi-Agent Terminal v1.4.0**

Smara adalah terminal pintar berbasis Go yang mengorkestrasi agen AI otonom dengan memori tim yang tersinkronisasi dan integrasi MCP (Model Context Protocol).

## ✨ Fitur Utama
- **Multi-Agent System**: Arsitektur Supervisor-Worker untuk pendelegasian tugas.
- **3 Mode Agen**:
  - `ask` (💬): Tanya-jawab cepat tanpa tools.
  - `rush` (⚡): Eksekusi cepat, langsung bertindak menggunakan tools.
  - `plan` (📋): Membuat rencana dan meminta persetujuan sebelum eksekusi.
- **Multi-Provider LLM**: Mendukung **Ollama (local)**, **Anthropic**, **OpenAI**, dan **OpenRouter**.
- **Platform Integration**: Jalankan Smara sebagai bot di **Telegram** dan **Discord**.
- **Tab-to-Cycle Mode**: Ganti mode agen secara instan dengan menekan tombol **Tab** di terminal.
- **Session Management**: Simpan dan kelola riwayat percakapan dalam sesi yang terpisah.
- **Smart Memory**: Menggunakan SQLite & Vector Search untuk menyimpan konteks percakapan.
- **Thinking Support**: Dukungan visual untuk blok pemikiran (*chain-of-thought*) dari model AI di antarmuka TUI.
- **MCP Integration**: Secara otomatis mendeteksi dan menghubungkan ke server MCP dari OpenCode.
- **Interactive TUI**: Antarmuka terminal modern yang interaktif berbasis Bubble Tea.
- **Auto-Update**: Sistem pembaruan otomatis bawaan menggunakan perintah `smara update`.
- **Cross-Platform**: Berjalan di Linux, macOS, dan Windows.

## 🚀 Instalasi

### Linux / macOS (via curl)
```bash
curl -fsSL https://raw.githubusercontent.com/gede-cahya/Smara-CLI/main/install.sh | sh
```

### Windows (PowerShell)
```powershell
irm https://raw.githubusercontent.com/gede-cahya/Smara-CLI/main/install.ps1 | iex
```

### Arch Linux (AUR/Pacman)
```bash
git clone https://github.com/gede-cahya/Smara-CLI.git
cd Smara-CLI
makepkg -si
```

### Build dari Source
```bash
# Clone repository
git clone https://github.com/gede-cahya/Smara-CLI.git
cd Smara-CLI

# Build menggunakan Makefile
make
sudo make install
```

## 🔑 Setup & Login

Smara membutuhkan akses ke LLM (Large Language Model) untuk berfungsi. Secara default, Smara akan mencari **Ollama** di `localhost:11434`. Jika Anda ingin menggunakan provider cloud (OpenAI, Anthropic, OpenRouter), ikuti langkah berikut:

### 1. Simpan API Key
Jalankan perintah login untuk menyimpan API key secara aman di konfigurasi lokal:
```bash
smara login
```
Anda akan dipandu melalui wizard interaktif untuk memilih provider dan memasukkan API key. Atau gunakan flag untuk provider spesifik:
```bash
smara login --provider openai --key "sk-..."
```

### 2. Pilih Provider Aktif
Setelah login, Anda bisa memilih provider dan model mana yang ingin digunakan:
```bash
# Secara interaktif (direkomendasikan)
smara provider select

# Atau secara manual
smara provider set anthropic
smara provider set-model claude-3-5-sonnet-latest
```

### 3. Cek Koneksi
Pastikan Smara bisa terhubung ke LLM yang dipilih:
```bash
smara provider test
```

## 🛠️ Cara Penggunaan

### Memulai Sesi
Cukup jalankan perintah berikut untuk memulai sesi interaktif:
```bash
smara start
```
Gunakan flag `--mode` untuk memulai dengan mode tertentu (misal: `smara start --mode plan`).

### Perintah CLI Utama:
- `smara provider list`: Lihat provider dan model yang tersedia.
- `smara provider select`: Pilih provider/model secara interaktif (TUI).
- `smara config list`: Lihat semua konfigurasi saat ini.
- `smara version`: Cek versi yang terinstall.
- `smara update [versi]`: Perbarui Smara CLI ke versi terbaru atau versi spesifik.

## 🌐 Smara Serve (Platform Bot)
Smara dapat dijalankan sebagai bot di platform messaging. Mode ini memungkinkan Anda atau tim Anda berinteraksi dengan Smara melalui Telegram atau Discord.

### Jalankan Server:
```bash
# Jalankan semua platform yang dikonfigurasi
smara serve

# Jalankan platform spesifik
smara serve --platform telegram
smara serve --platform telegram,discord
```

### Konfigurasi Bot:
Tambahkan token bot di `~/.smara/config.yaml` atau melalui environment variables:
```bash
export SMARA_TELEGRAM_TOKEN="your_bot_token"
export SMARA_DISCORD_TOKEN="your_bot_token"
```

### Fitur Platform:
- **Telegram**: Dukungan penuh untuk mode `ask`, `rush`, dan `plan`.
- **Discord**: Integrasi role-based access control.
- **Shared Memory**: Memori yang dipelajari di terminal (CLI) juga tersedia bagi bot platform, dan sebaliknya.

### Perintah di dalam REPL (Interactive Mode):
- **Tab**: Ganti mode agen (cycle: ask → rush → plan).
- **?**: Tampilkan bantuan keyboard shortcut.
- **/help**: Tampilkan daftar perintah REPL.
- **/mode [ask|rush|plan]**: Pindah ke mode agen spesifik.
- **/model [provider] [model]**: Ganti LLM provider/model saat runtime.
- **/session [list|new|switch|end]**: Kelola sesi percakapan.
- **/mcp**: Lihat daftar MCP server dan tools yang terhubung.
- **/memory**: Lihat riwayat memori agen.
- **/clear**: Bersihkan layar terminal.
- **exit / keluar**: Keluar dari aplikasi.

## ⚙️ Konfigurasi
Konfigurasi disimpan di `~/.smara/config.yaml`. Smara secara otomatis mengimpor MCP server yang terdaftar di OpenCode (`~/.config/opencode/opencode.json`).

## 📄 Lisensi
MIT License - © 2026 Gede Cahya.
