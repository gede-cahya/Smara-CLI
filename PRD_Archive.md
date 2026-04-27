# PRD: Fitur Archive – Smara Desktop v2.0

## 1. Overview

Fitur Archive memungkinkan pengguna untuk **mengarsipkan (soft-archive)** dan **mengelola** sesi (session), memori (memory), dan workspace tanpa menghapusnya secara permanen dari database. Arsip tetap tersimpan di SQLite, tersembunyi dari tampilan default, dan dapat dipulihkan atau dihapus permanen kapan saja.

## 2. Goals

- Membersihkan daftar sesi/memori aktif tanpa kehilangan data historis.
- Mengelola workspace yang tidak aktif sementara.
- Menyediakan UI yang jelas untuk berpindah antara mode "Aktif" dan "Diarsipkan".
- Mencegah kecelakaan penghapusan dengan konfirmasi sebelum delete permanen.

## 3. Scope

| Entitas | Archive | Unarchive | Delete Permanen | UI |
|---|---|---|---|---|
| **Session** | ✅ | ✅ | ✅ | Tab di sidebar |
| **Memory** | ✅ | ✅ | ✅ | API tersedia (belum ada UI khusus) |
| **Workspace** | ✅ | ✅ | ❌ | API tersedia (belum ada UI khusus) |

**Catatan:** Untuk rilis MVP (v2.0), UI Archive hanya tersedia untuk **Session** di sidebar utama. Archive Memory dan Workspace tersedia via API Go dan bisa diakses via CLI. UI untuk Memory/Workspace Archive dapat ditambahkan di rilis berikutnya.

## 4. Technical Design

### 4.1 Database Schema

Tiga tabel utama ditambahkan kolom soft-archive:

| Kolom | Tipe | Default | Deskripsi |
|---|---|---|---|
| `is_archived` | INTEGER / BOOLEAN | 0 / false | Flag soft-archive |
| `archived_at` | DATETIME / TEXT | NULL | Timestamp saat diarsipkan |

**Tabel yang terpengaruh:**
- `sessions`
- `memories`
- `workspaces`

**Migrasi:** Saat init, sistem secara otomatis menambahkan kolom `is_archived` dan `archived_at` jika belum ada (backward-compatible).

**Index tambahan:**
```sql
CREATE INDEX idx_sessions_archived ON sessions(is_archived, workspace_id);
```

### 4.2 API Go (Backend)

#### Session Store (`pkg/session/store.go`)
```go
ArchiveSession(id string) error
UnarchiveSession(id string) error
ListArchivedSessions(workspaceID int64) ([]Session, error)
DeleteArchivedSession(id string) error
```

#### Memory Store (`pkg/memory/store.go`)
```go
ArchiveMemory(id int64) error
UnarchiveMemory(id int64) error
ListArchivedMemories(workspaceID int64, limit int) ([]Memory, error)
DeleteArchivedMemory(id int64) error

ArchiveWorkspace(id int64) error
UnarchiveWorkspace(id int64) error
ListArchivedWorkspaces() ([]Workspace, error)
```

#### App Bindings (`smara-desktop/app.go`)
Semua method di atas diekspos sebagai public method pada struct `App` sehingga tersedia untuk frontend Wails.

### 4.3 Frontend (React + TypeScript)

#### State
```ts
const [archiveTab, setArchiveTab] = useState<'active' | 'archived'>('active')
const [archivedSessions, setArchivedSessions] = useState<Session[]>([])
```

#### Tab Switcher
- Dua tombol: **Active** | **Archived**
- Berada di atas daftar sesi di sidebar
- Style: toggle pill buttons dengan state aktif berwarna primary

#### Active Sessions List
- Hover pada item sesi menampilkan tombol **Archive** (ikon `Archive`)
- Klik Archive → memanggil `ArchiveSession(id)` → refresh list

#### Archived Sessions List
- Setiap item ditampilkan dengan style berbeda (bg-muted, ikon arsip)
- Hover menampilkan:
  - **Restore** (ikon `RotateCcw`) → `UnarchiveSession(id)`
  - **Delete** (ikon `Trash2`) → `DeleteArchivedSession(id)` + konfirmasi

#### Wails Bridge
File `wailsjs/go/main/App.d.ts` dan `App.js` diperbarui secara manual untuk menyertakan semua method Archive agar TypeScript compiler tidak error. File `wailsjs/go/models.ts` juga diperbarui dengan field `is_archived` dan `archived_at`.

## 5. User Flow

```
[User] melihat daftar sesi aktif di sidebar
   |
   | hover pada sesi → muncul tombol Archive
   v
[User] klik Archive
   |
   v
[Sesi] di-flag is_archived=1, archived_at=now
   |
   v
[UI] sesi hilang dari tab "Active", muncul di tab "Archived"
   |
   v
[User] beralih ke tab "Archived"
   |
   v
[User] bisa: [Restore] → kembali ke Active
         atau: [Delete Permanen] → konfirmasi → hapus dari DB
```

## 6. Edge Cases & Safety

| Kasus | Penanganan |
|---|---|
| Arsipkan sesi yang sedang aktif | Langsung archive, session ID aktif di-set ke `null` |
| Restore session | Otomatis muncul kembali di tab Active |
| Delete permanen | Konfirmasi dialog browser `confirm()` |
| Database lama tanpa kolom archive | Auto-migrasi saat startup |
| Wails bridge belum tersedia (dev mode browser) | Semua call dibungkus `typeof fn !== 'function'` guard |

## 7. Future Enhancements (v2.x)

- **Memory Archive UI:** Tab atau halaman terpisah untuk melihat dan mengelola archived memories.
- **Workspace Archive UI:** Dropdown/workspace switcher dengan section "Archived Workspaces".
- **Batch Operations:** Select multiple items lalu archive/unarchive/delete sekaligus.
- **Auto-Archive:** Pengaturan TTL untuk otomatis mengarsipkan sesi yang tidak aktif selama X hari.
- **Search Archive:** Full-text search di dalam tab Archived.

## 8. Acceptance Criteria

- [x] Pengguna dapat mengarsipkan sesi dari sidebar.
- [x] Pengguna dapat melihat daftar sesi yang diarsipkan di tab "Archived".
- [x] Pengguna dapat memulihkan sesi yang diarsipkan.
- [x] Pengguna dapat menghapus permanen sesi yang diarsipkan dengan konfirmasi.
- [x] Backend Go mengkompilasi tanpa error.
- [x] Frontend TypeScript mengkompilasi tanpa error.
- [x] Database schema kompatibel backward (auto-migrasi).

---

*Dibuat: 2026-04-26*
*Versi: 1.0*
*Status: Implemented & Ready*
