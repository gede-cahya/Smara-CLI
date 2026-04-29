PRD: Antigravity SSH-Remote Control Engine

Versi: 1.0

Status: Draft

Pemilik Produk: Antigravity Engineering Team
1. Pendahuluan & Objektif

Memberikan kemampuan kepada pengguna untuk mengelola, mengonfigurasi, dan menjalankan perintah pada VPS (Virtual Private Server) secara langsung melalui dashboard Antigravity menggunakan protokol SSH tanpa perlu menginstal agent tambahan di sisi server (agentless).

Tujuan Utama:

    Menyederhanakan manajemen multi-server.

    Meningkatkan keamanan melalui manajemen SSH Key terpusat.

    Mengotomatisasi tugas rutin (update, install, monitoring).

2. Target Pengguna

    DevOps Engineer: Yang perlu melakukan deployment cepat ke banyak server.

    System Administrator: Yang memantau kesehatan server dan melakukan troubleshooting.

    Developer: Yang ingin melakukan konfigurasi environment tanpa meninggalkan dashboard utama.

3. Fitur Utama (Functional Requirements)
3.1. SSH Key Management Center

    Generate Key Pair: Sistem dapat membuat pasangan kunci RSA/Ed25519.

    Vault Integration: Menyimpan private key secara aman dalam encrypted vault.

    Key Injection: Otomatis menyuntikkan public key ke VPS baru saat proses provisioning.

3.2. Remote Command Execution (RCE)

    One-Click Scripts: Library berisi skrip siap pakai (misal: Install Docker, Nginx, Patching).

    Custom Command: Kolom input untuk mengetikkan perintah Bash secara langsung.

    Bulk Execution: Kemampuan menjalankan satu perintah ke grup server (misal: tag production).

3.3. Web-Based SSH Terminal

    Emulated Terminal: Integrasi Xterm.js untuk memberikan pengalaman terminal langsung di browser.

    Session Persistence: Sesi tetap berjalan meskipun browser ditutup (opsional dengan tmux/screen integration).

3.4. Execution Logs & Audit Trail

    Output Capture: Mencatat log stdout dan stderr dari setiap perintah yang dijalankan.

    Audit Log: Mencatat siapa yang menjalankan perintah apa, di server mana, dan kapan.

4. Spesifikasi Teknis
Komponen	Teknologi / Protokol
Protokol	SSHv2
Otentikasi	Key-based Authentication (Preferred), Password (Fallback)
Concurrency	Go Channels atau Python Celery untuk eksekusi paralel
Security	AES-256 encryption untuk data at rest (Keys)
Transport	Paramiko (Python) atau Crypto/SSH (Go)
5. Alur Kerja User (User Flow)

    Koneksi: User memilih instance VPS di dashboard.

    Otentikasi: Antigravity mengambil Private Key dari Vault.

    Handshake: Sistem melakukan SSH Handshake ke port 22 target.

    Eksekusi: User mengirim perintah -> Sistem mengeksekusi -> Output dikirim kembali ke UI.

    Logging: Hasil eksekusi disimpan di database untuk riwayat.

6. Aturan Keamanan (Non-Functional Requirements)

    Port Scanning: Sistem harus memvalidasi port 22 terbuka sebelum mencoba koneksi.

    Timeout: Maksimal waktu tunggu eksekusi adalah 300 detik untuk mencegah proses gantung.

    Rate Limiting: Membatasi jumlah perintah per detik untuk menghindari deteksi Brute Force oleh sistem keamanan internal VPS (seperti Fail2Ban).

    No Data Leak: Private key tidak boleh pernah ditampilkan secara plain-text di UI setelah dibuat.

7. Timeline & MVP (Minimum Viable Product)

    Fase 1 (MVP): Manajemen SSH Key sederhana dan terminal berbasis web (1:1).

    Fase 2: Kemampuan Bulk Execution (1:Many) dan Library Script.

    Fase 3: Integrasi penjadwalan (Cron job via Antigravity).

8. Kriteria Penerimaan (Acceptance Criteria)

    User dapat terhubung ke VPS dalam waktu < 3 detik.

    Perintah sudo dapat dijalankan dengan penanganan prompt password yang aman.

    Log eksekusi muncul secara real-time tanpa delay yang signifikan.