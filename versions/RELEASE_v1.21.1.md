# Release Notes — Smara CLI v1.21.1

Release **v1.21.1** menambahkan **Web UI Scheduler Dashboard** bawaan dan REST API `/api/scheduler/jobs`.

---

## 🚀 Perubahan Utama

1. **Web UI Scheduler Dashboard**:
   - Menu baru **"Scheduler"** pada Sidebar Web UI di grup *Build*.
   - Tabel visual interaktif untuk memantau semua jadwal cronjob, status eksekusi terakhir (*Success*, *Retrying*, *Failed*), *next run*, max retries, dan *DAG dependencies*.
   - Modal Form interaktif untuk membuat jadwal cronjob baru langsung dari Web UI.
   - Tombol *Run Now* dan *Hapus*.

2. **REST API `/api/scheduler/jobs`**:
   - `GET /api/scheduler/jobs` ➔ Mengambil daftar jadwal cronjob.
   - `POST /api/scheduler/jobs` ➔ Membuat/memicu eksekusi jadwal.
   - `DELETE /api/scheduler/jobs?id=...` ➔ Menghapus jadwal.
