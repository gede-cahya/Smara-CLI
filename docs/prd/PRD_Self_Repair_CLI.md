# PRD: Smara Self-Repair / Self-Healing CLI

**Status**: Draft  
**Target Version**: v1.16.0+  
**Author**: Cascade  
**Date**: 2026-05-02

---

## 1. Overview

Smara CLI saat ini memiliki auto-restart via `systemd` pada server deployment, tetapi tidak memiliki mekanisme self-repair saat berjalan secara lokal (desktop/terminal). PRD ini mendefinisikan subsistem **Self-Repair / Self-Healing** yang mendeteksi dan memperbaiki kegagalan umum pada startup maupun runtime, tanpa mengharuskan user untuk mengedit file manual atau menghapus data.

---

## 2. Goals

- **Zero-config recovery**: Saat startup gagal karena DB corrupt atau config invalid, Smara mencoba memperbaiki secara otomatis (dengan backup) sebelum menyerah.
- **Explicit repair tools**: User dapat menjalankan `smara doctor` untuk melihat kondisi sistem dan `smara repair` untuk memperbaiki masalah yang terdeteksi.
- **Non-destructive**: Semua operasi repair harus membackup data lama sebelum mutasi.
- **Transparency**: Laporkan setiap tindakan repair ke user melalui terminal (log berwarna).

---

## 3. Non-Goals

- Menggantikan `Restart=always` pada systemd service deployment.
- Diagnosis jaringan/infrastruktur di luar konektivitas MCP endpoint.
- Perbaikan bug kode Smara itu sendiri (bukan data/runtime).

---

## 4. CLI Surface

### 4.1 `smara doctor`

Menjalankan pemeriksaan menyeluruh terhadap komponen Smara dan mencetak laporan kesehatan.

```bash
smara doctor                    # full diagnostic
smara doctor --json             # output machine-readable JSON
smara doctor --watch            # loop check setiap 30 detik (v2)
```

**Output example (terminal)**:

```
[OK]   Config file      ~/.smara/config.yaml  (valid YAML, 12 keys)
[WARN] SQLite DB        ~/.smara/smara.db     (size 0 bytes — possible corruption)
[OK]   Session store    3 sessions, 1 active
[OK]   MCP: figma       connected (12 tools)
[FAIL] MCP: blender     connection refused (PID 12345 died)
[OK]   Disk space       42 GB free
```

### 4.2 `smara repair`

Memperbaiki masalah yang terdeteksi secara otomatis.

```bash
smara repair                    # auto-fix semua yang bisa diperbaiki
smara repair --dry-run          # preview tanpa mutasi
smara repair --module=db        # hanya repair modul DB
smara repair --module=mcp       # hanya repair modul MCP
smara repair --module=config    # hanya repair modul config
```

---

## 5. Diagnostic & Repair Modules

### 5.1 Module: SQLite Store Health (`db`)

**Checks**:
- Apakah file DB bisa dibuka (`sql.Open`)?
- Apakah `PRAGMA integrity_check` OK?
- Apakah tabel wajib (`memories`, `workspaces`, `sessions`) ada?
- Apakah ukuran file DB = 0 byte (indikasi corrupt)?

**Auto-repair (startup)**:
- Jika `integrity_check` gagal atau size = 0:
  1. Rename file ke `<dbpath>.corrupt.<timestamp>`.
  2. Buat DB baru dengan schema lengkap.
  3. Cetak warning ke UI: `DB corrupt dibackup ke ... dan direcreate.`

**Manual repair (`smara repair --module=db`)**:
- Jalankan `VACUUM` jika DB besar / fragmented.
- Hapus session `ended` yang lebih tua dari retention (opsional flag `--purge-old-sessions`).

### 5.2 Module: Config Validation (`config`)

**Checks**:
- File config bisa dibaca dan merupakan valid YAML.
- Field wajib ada dan valid: `provider`, `model` (bukan kosong jika provider memerlukan model).
- API key tidak boleh placeholder / string kosong untuk provider yang membutuhkannya.
- Permission file tidak terlalu terbuka (world-readable).

**Auto-repair (startup)**:
- Jika config invalid / tidak bisa di-parse:
  1. Backup ke `<config>.bak.<timestamp>`.
  2. Tulis minimal default config (provider = `ollama`, model = `llama3.2`, db path default).
  3. Cetak instruksi: `Config invalid dibackup. Jalankan 'smara login' untuk mengatur provider/API key.`

**Manual repair**:
- `smara repair --module=config` hanya membackup dan menulis default jika invalid.

### 5.3 Module: MCP Connection Health (`mcp`)

**Checks**:
- Untuk MCP local: apakah process masih running (PID dari PID file)?
- Untuk MCP remote: apakah endpoint merespond HTTP health-check (jika ada) atau connection test?
- Apakah ada zombie client di internal registry?

**Auto-repair (runtime / startup)**:
- Saat `start` atau `serve`, jika koneksi MCP gagal:
  1. Coba reconnect 1x dengan backoff 2 detik.
  2. Jika masih gagal, tandai sebagai `degraded` (bukan fatal) dan lanjutkan.
- `smara repair --module=mcp` akan force-close semua client lalu reconnect paralel.

### 5.4 Module: Session Recovery (`session`)

**Checks**:
- Session dengan state `active` tapi tidak ada di process list / lock file stale.
- Lock file (`*.lock`) yang tersisa dari crash sebelumnya.

**Auto-repair**:
- Saat startup, scan semua session `active` yang lebih tua dari 24 jam → ubah ke `ended`.
- Hapus stale lock file (tidak ada PID yang merujuk).

### 5.5 Module: Disk & Permission (`disk`)

**Checks**:
- `$HOME/.smara` bisa ditulis?
- Disk space > 100 MB?
- File permission config tidak `644` atau lebih terbuka?

**Auto-repair**:
- Tidak ada auto-repair otomatis untuk permission (security risk).
- Cetak warning dengan perintah fix manual (`chmod 600 ...`).

---

## 6. Startup Integration

Di `cmd/smara/start.go`, alur startup dimodifikasi:

```
1. Config Load
   └── If invalid → trigger ConfigRepair (backup + default) → continue
2. DB Init
   └── If open/integrity fails → trigger DBRepair (backup + recreate) → continue
3. MCP Connect
   └── If any fails → retry 1x → mark degraded → continue (never fatal)
4. Session Load
   └── Orphaned active sessions → auto-mark ended
5. TUI Start
```

Jika repair pada step 1 atau 2 gagal total (misal disk read-only), exit dengan error code dan pesan user-friendly.

---

## 7. Architecture

### 7.1 Package Structure

```
internal/
  repair/
    doctor.go         # Diagnostic runner, collects check results
    repair.go         # Repair executor, applies fixes
    modules/
      db.go           # SQLite health checks & repair
      config.go       # YAML validation & default rewrite
      mcp.go          # MCP health & reconnect
      session.go      # Orphaned session cleanup
      disk.go         # Permission & space checks
    backup.go         # Helper: timestamped backup of any file
```

### 7.2 Types

```go
type CheckResult struct {
    Module   string       // "db", "config", "mcp", "session", "disk"
    Status   HealthStatus // OK, Warn, Fail
    Message  string
    Fixable  bool         // can repair module auto-fix this?
    Suggestion string     // user-friendly action
}

type RepairAction struct {
    Module      string
    Description string
    DryRun      bool
    Run         func() error
}
```

### 7.3 Backup Helper

```go
func BackupFile(src string) (backupPath string, err error)
```
- Pola: `<src>.backup.<RFC3339>`.
- Retention: hanya simpan 5 backup terakhir per file (hapus yang lebih tua secara otomatis).

---

## 8. UI / Output

- Gunakan existing `internal/ui` color helpers (`PrintSuccess`, `PrintWarning`, `PrintError`) agar konsisten dengan TUI Smara.
- `smara doctor --json` mengeluarkan array `[]CheckResult` untuk integrasi script/CI.

---

## 9. Security Considerations

- Backup file config mungkin mengandung API key. Backup harus diberi permission `600` (owner read-only).
- Repair tidak boleh mengubah permission file menjadi lebih longgar.
- Never auto-elevate privilege (no `sudo` calls).

---

## 10. Acceptance Criteria

- [ ] `smara doctor` berjalan tanpa error dan menampilkan status semua modul.
- [ ] `smara repair --dry-run` menampilkan daftar aksi tanpa memutasi file.
- [ ] Jika DB corrupt, startup `smara start` otomatis backup & recreate DB, lalu lanjut.
- [ ] Jika config invalid, startup otomatis backup & tulis default, lalu lanjut (dengan warning).
- [ ] MCP yang gagal connect di startup di-mark `degraded`, tidak mematikan seluruh aplikasi.
- [ ] Stale session lock dibersihkan saat startup.
- [ ] Backup config/DB lebih dari 5 instance otomatis dihapus.

---

## 11. Future Enhancements (v2)

- `smara doctor --watch`: background health monitor dengan interval konfigurabel.
- Web dashboard health widget (integrasi dengan `PRD_Dashboard.md`).
- Self-repair notification ke platform messaging (Telegram/Discord) saat deployed sebagai service.
