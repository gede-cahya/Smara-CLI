# PRD: Terminal Stability & Deep Workspace Discovery

## 1. Pendahuluan
Dokumen ini merinci solusi untuk masalah TUI freeze saat operasi paste dan keterbatasan agen dalam memahami konteks file/folder di luar direktori kerja saat ini.

## 2. Masalah Utama
1.  **TUI Freeze**: Saat pengguna melakukan paste teks panjang (seperti path file yang dalam atau blok kode), TUI Bubbletea mengalami lag/freeze karena memproses setiap karakter sebagai event `KeyMsg` terpisah.
2.  **Konteks Workspace Dangkal**: Agen hanya memetakan file level-1 saat startup, sehingga sering "kehilangan jejak" folder atau file di direktori yang lebih dalam.
3.  **Keterbatasan Navigasi**: Agen kesulitan mencari file di luar CWD (Current Working Directory) atau memahami posisi absolutnya di sistem.

## 3. Solusi Teknis

### A. Terminal Stability (Paste Optimization)
*   **Bracketed Paste Mode**: Mengaktifkan mode ini di Bubbletea agar terminal mengirimkan sinyal awal dan akhir paste.
*   **Explicit Paste Handling**: Menangani `tea.PasteMsg` secara langsung untuk menyisipkan seluruh string ke `textarea` sekaligus, menghindari overhead pemrosesan per karakter.

### B. Deep Workspace Discovery
*   **Recursive Startup Scan**: Meningkatkan fungsi `discoverProjectContext` untuk memindai direktori hingga kedalaman 2 level (configurable) dan mendeteksi file konfigurasi penting (Makefile, Docker, package.json, dll) secara otomatis.
*   **Absolute Path Injection**: Menyertakan path absolut CWD dalam sistem prompt agar agen memiliki orientasi spasial yang jelas.

### C. Enhanced Filesystem Tools
*   **New Tool: `get_cwd`**: Memberikan informasi path absolut saat ini kepada agen secara instan.
*   **Improved Tool: `search_path`**: Memungkinkan pencarian rekursif dengan parameter `root` yang fleksibel (termasuk `..` atau `/`) untuk menemukan file di luar workspace saat ini.

## 4. Rencana Implementasi (Selesai di v1.8.0)
1.  Update `internal/ui/app.go` untuk dukungan paste.
2.  Update `internal/agent/supervisor.go` untuk pemindaian workspace lebih mendalam.
3.  Update `internal/agent/builtin_tools.go` dengan tool baru dan perbaikan tool lama.
4.  Update metadata versi dan README untuk transparansi fitur.

## 5. Verifikasi
*   Uji coba paste teks > 1000 karakter ke TUI.
*   Uji coba agen menemukan file di folder `internal/agent/` tepat setelah startup tanpa bantuan user.
*   Uji coba perintah `search_path` ke folder parent (`..`).
