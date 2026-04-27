Product Requirements Document (PRD): Hybrid Autonomous Super-Agent
1. Ringkasan Eksekutif

Visi Produk: Membangun framework AI Agent otonom yang mampu beroperasi terus-menerus di lingkungan server maupun lokal, dengan tingkat presisi pemanggilan fungsi yang tinggi, serta lapisan keamanan eksekusi (sandbox) untuk mencegah modifikasi kode yang merusak.
Target Pengguna: FullStack Developer, DevOps, dan System Administrator yang membutuhkan asisten agen untuk pemeliharaan sistem, debugging otomatis, dan eksekusi tugas berbasis jadwal.
2. Arsitektur Sistem (The 3-Layer Engine)

Sistem ini menggabungkan tiga pilar arsitektur menjadi satu siklus kognitif (Agentic Loop):
Lapisan (Layer)	Konsep Referensi	Fungsi Utama di Dalam Sistem
Cognitive Layer	Hermes	Otak utama untuk menerjemahkan konteks menjadi parameter JSON yang presisi (Strict Function Calling).
Autonomy Layer	OpenClaw	Mesin detak jantung (heartbeat) untuk analisis hybrid dan eksekusi multi-timeframe di latar belakang.
Execution Layer	OpenCode	Penjaga gerbang keamanan yang memisahkan Plan Mode (Nalar) dan Build Mode (Eksekusi Terminal/File).
3. Kebutuhan Fungsional (Functional Requirements)
3.1. Manajemen Mode Eksekusi (Two-Step Safety)

    Plan Mode (Read-Only): Agen dapat membaca direktori, file, dan log server, lalu mengembalikan output berupa draf tindakan. Pada mode ini, izin tulis (write/execute) diblokir sepenuhnya.

    Build Mode (Read-Write): Agen hanya dapat mengeksekusi perintah terminal atau mengubah file (patching) setelah draf dari Plan Mode lolos validasi (baik validasi aturan internal maupun persetujuan manual jika diperlukan).

    Auto-Revert: Jika perintah eksekusi menghasilkan error code (misal: build failed), sistem otomatis membatalkan perubahan file terakhir.

3.2. Siklus Otonomi Latar Belakang

    Multi-Timeframe Execution: Agen harus dapat diatur untuk menjalankan siklus Observe -> Think dalam interval waktu yang bervariasi (contoh: 1 menit untuk error log, 1 jam untuk pembaruan dependensi).

    Hold State: Agen memiliki kemampuan logis untuk mengembalikan status NO_ACTION jika metrik atau kondisi tidak memenuhi syarat algoritmik, guna menghemat compute resource di VPS.

3.3. Tooling & Integrasi

    LSP (Language Server Protocol) Integration: Agen dapat melakukan kueri referensi kode, definisi fungsi, dan syntax checking sebelum memodifikasi file.

    Terminal Environment: Agen dapat menjalankan perintah secara terisolasi dan menangkap stdout/stderr untuk diumpankan kembali ke siklus pemikirannya.

4. Kebutuhan Non-Fungsional (Non-Functional Requirements)

    Manajemen Memori Konteks (Auto-Compacting): Agen harus memadatkan (compact) riwayat percakapan dan log terminal yang sudah usang agar tidak melampaui batas context window LLM (mencegah amnesia konteks selama loop panjang).

    Kinerja Eksekusi: Runtime harus ringan dan memiliki waktu startup yang cepat agar tidak membebani server saat agen berjalan sebagai daemon di latar belakang.

    Observability: Semua tindakan agen (kapan ia berpikir, fungsi apa yang dipanggil, file apa yang diubah) harus dicatat dalam file log tersendiri untuk tujuan audit.

5. Rekomendasi Tech Stack

    Core Runtime: Bun.js (Sangat direkomendasikan karena kecepatan eksekusinya, dukungan TypeScript native tanpa build step, dan performa I/O yang optimal untuk modifikasi file berulang di VPS/Server).

    LLM Backend:

        Primary/Cognitive: Model lini Hermes (dijalankan via Ollama) untuk local reasoning yang kuat pada function calling.

        Fallback: Model cloud (seperti Claude 3.5 Sonnet) untuk tugas coding kompleks yang gagal diselesaikan di lokal.

    Deployment Environment: Berjalan secara optimal di ekosistem Linux (seperti CachyOS untuk local development atau Ubuntu/Debian untuk deployment VPS).

6. Fase Pengembangan (Milestones)

    Tahap 1: Core Engine & CLI - Membangun runtime dasar di Bun.js yang bisa memparsing Plan Mode dan Build Mode via terminal.

    Tahap 2: Tooling Implementation - Menyambungkan mesin dengan shell execution lokal dan pembacaan file system.

    Tahap 3: Autonomy Loop - Mengimplementasikan scheduler (multi-timeframe) dan auto-compacting memory.

    Tahap 4: Agent Testing - Uji coba mendeploy agen di server untuk memonitor aplikasi web sederhana secara otonom.