# SMARA.md — Agentic Coding Rules

File ini adalah aturan utama yang **wajib dibaca agent pertama kali** sebelum melakukan perubahan kode di project Smara.

## 1. Prinsip Utama

- Pahami konteks project sebelum mengedit file.
- Jangan langsung refactor besar tanpa alasan jelas.
- Prioritaskan perubahan kecil, aman, dan mudah di-review.
- Jangan menghapus fitur, file, atau konfigurasi tanpa instruksi eksplisit.
- Jika ragu, baca kode terkait lebih dulu, lalu jelaskan asumsi sebelum bertindak.
- Jawab dan komunikasikan progres dalam Bahasa Indonesia kecuali user meminta bahasa lain.

## 2. Alur Wajib Sebelum Coding

Sebelum membuat perubahan, agent harus:

1. Baca `SMARA.md` ini.
2. Pahami struktur project minimal dari folder utama:
   - `cmd/`
   - `internal/`
   - `pkg/`
   - `web/`
   - `docs/`
   - `skills/`
3. Cari file yang relevan menggunakan search/grep, bukan menebak lokasi.
4. Baca file target sebelum mengedit.
5. Identifikasi dampak perubahan ke backend, CLI, web UI, memory, skill, atau config.
6. Buat rencana singkat jika task menyentuh lebih dari satu file.

## 3. Aturan Editing Kode

- Gunakan patch/edit terarah, jangan rewrite file besar kalau tidak perlu.
- Pertahankan style kode yang sudah ada.
- Jangan mengganti nama public API, command, config key, atau struktur data persisted tanpa migrasi/kompatibilitas.
- Jangan menambahkan dependency baru kecuali benar-benar perlu.
- Jangan menyimpan secret, token, private key, password, atau credential ke repository.
- Jangan mengubah file binary/build output kecuali memang diminta.
- Jangan menjalankan command destruktif seperti `rm -rf`, reset git, drop database, atau overwrite config tanpa konfirmasi.

## 4. Project Smara: Area Penting

### Backend / CLI Go

- Kode Go umumnya berada di `cmd/`, `internal/`, dan `pkg/`.
- Setelah mengubah Go, jalankan minimal:

```bash
go test ./...
```

Jika terlalu lama atau gagal karena environment, laporkan alasannya dan jalankan test yang lebih spesifik.

### Web UI

- Kode web berada di `web/`.
- Setelah mengubah web, jalankan:

```bash
cd web && npm run build
```

Jika ada lint/test script yang relevan, jalankan juga.

### Memory System

Untuk perubahan terkait memory:

- Pastikan tidak merusak format data memory lama.
- Jaga kompatibilitas dengan memory existing.
- Jangan menghapus memory user tanpa instruksi eksplisit.
- Jika menambah fitur graph/memory visualization, pastikan node, edge, selection, dan state tetap stabil.

### Skills System

Untuk perubahan terkait skills:

- Jangan menghapus skill user.
- Pastikan format skill tetap backward-compatible.
- Jika menambah skill baru, gunakan nama kebab-case.

### SSH / Remote / VPS

- Jangan deploy atau restart service tanpa instruksi user.
- Sebelum menjalankan command remote, jelaskan target host jika berisiko.
- Jangan menampilkan credential di output.

## 5. Aturan Graph Memory Interaktif

Jika task menyentuh graph memory di Smara Web:

- Node harus bisa tetap terbaca dan tidak saling menumpuk secara ekstrem.
- Interaksi drag harus tidak merusak posisi/physics graph.
- Klik node harus memiliki state visual yang jelas.
- Neighbor/connected node harus bisa dibedakan dari node lain.
- Edge terkait node terpilih harus ikut diberi highlight.
- Klik background atau tombol close harus bisa clear selection.
- Hindari perubahan yang membuat graph berat untuk memory berjumlah besar.

## 6. Validasi Wajib Setelah Perubahan

Setelah coding, agent harus menjalankan validasi sesuai area:

- Go backend/CLI:

```bash
go test ./...
```

- Web:

```bash
cd web && npm run build
```

- Dokumentasi saja:
  - Pastikan markdown rapi dan link/path masuk akal.

Jika validasi gagal:

1. Baca error.
2. Perbaiki jika masih dalam scope.
3. Jalankan ulang validasi.
4. Jika tidak bisa diperbaiki karena dependency/environment, jelaskan secara spesifik.

## 7. Format Laporan ke User

Setelah selesai, jawab ringkas dengan:

- File yang diubah.
- Ringkasan perubahan.
- Command validasi yang dijalankan.
- Status hasil validasi.
- Catatan risiko jika ada.

Contoh:

```markdown
Selesai.

File diubah:
- `web/src/...`

Perubahan:
- Menambahkan drag node pada memory graph.
- Menambahkan highlight node terpilih dan neighbor.

Validasi:
- `cd web && npm run build` ✅
```

## 8. Hal yang Harus Dihindari

- Jangan bilang selesai kalau belum menjalankan validasi yang relevan.
- Jangan membuat perubahan di luar permintaan user.
- Jangan menyembunyikan error build/test.
- Jangan menebak arsitektur tanpa membaca file.
- Jangan overwrite pekerjaan user yang belum di-commit.
- Jangan menghapus data memory, skill, config, atau session tanpa izin.

## 9. Jika Task Besar

Jika task besar atau multi-phase:

1. Pecah menjadi tahap kecil.
2. Kerjakan tahap paling aman dulu.
3. Validasi tiap tahap.
4. Laporkan progres.
5. Jangan lanjut ke perubahan destruktif tanpa konfirmasi.

---

**Instruksi singkat untuk agent:** baca file ini dulu, pahami scope, edit seperlunya, validasi, lalu laporkan hasil dengan jujur.
