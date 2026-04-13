# Smara CLI 🌀

**Autonomous Multi-Agent Terminal v1.0.0**

Smara adalah terminal pintar berbasis Go yang mengorkestrasi agen AI otonom dengan memori tim yang tersinkronisasi dan integrasi MCP (Model Context Protocol).

## ✨ Fitur Utama
- **Multi-Agent System**: Arsitektur Supervisor-Worker untuk pendelegasian tugas.
- **3 Mode Agen**:
  - `ask` (💬): Tanya-jawab cepat tanpa tools.
  - `rush` (⚡): Eksekusi cepat, langsung bertindak menggunakan tools.
  - `plan` (📋): Membuat rencana dan meminta persetujuan sebelum eksekusi.
- **Tab-to-Cycle Mode**: Ganti mode agen secara instan dengan menekan tombol **Tab** di terminal.
- **Smart Memory**: Menggunakan SQLite & Vector Search untuk menyimpan konteks percakapan.
- **MCP Integration**: Secara otomatis mendeteksi dan menghubungkan ke server MCP dari OpenCode.
- **Cross-Platform**: Berjalan di Linux, macOS, dan Windows.

## 🚀 Instalasi

### Linux / macOS (via curl)
```bash
curl -fsSL https://raw.githubusercontent.com/cahya/smara/main/install.sh | sh
```

### Windows (PowerShell)
```powershell
irm https://raw.githubusercontent.com/cahya/smara/main/install.ps1 | iex
```

### Arch Linux (AUR/Pacman)
```bash
git clone https://github.com/cahya/smara.git
cd smara
makepkg -si
```

### Build dari Source
```bash
# Clone repository
git clone https://github.com/cahya/smara.git
cd smara

# Build menggunakan Makefile
make
sudo make install
```

## 🛠️ Cara Penggunaan

Cukup jalankan perintah berikut untuk memulai sesi interaktif:
```bash
smara start
```

### Perintah di dalam REPL:
- **Tab**: Ganti mode (ask → rush → plan)
- **/mode [name]**: Pindah ke mode spesifik
- **/mcp**: Lihat daftar MCP server yang terhubung
- **/memory**: Lihat riwayat memori agen
- **/clear**: Bersihkan layar terminal
- **exit**: Keluar

## ⚙️ Konfigurasi
Konfigurasi disimpan di `~/.smara/config.yaml`. Smara secara otomatis mengimpor MCP server yang terdaftar di OpenCode (`~/.config/opencode/opencode.json`).

## 📄 Lisensi
MIT License - © 2026 Cahya.
