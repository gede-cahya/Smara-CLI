# Release Notes — Smara CLI v1.20.24

Update Smara CLI v1.20.24 membawa peningkatan fitur, stabilitas, dokumentasi, workflow automation, dan packaging release agar instalasi maupun \ lebih konsisten lintas platform.

## Highlights

### 🚀 Release Automation & Distribution

Release ini menyiapkan workflow rilis yang lebih aman dan kompatibel:

1. **Auto-version release** — workflow dapat menentukan versi otomatis dari \/tag terakhir bila parameter versi tidak diberikan.
2. **GitHub Release assets lengkap** — release menyertakan raw binary dan archive kompatibel updater lama/new updater.
3. **Updater-compatible packaging** — asset \ untuk Linux/macOS dan \ untuk Windows divalidasi sebelum release dianggap selesai.

### 🧩 Smara CLI Improvements

Perubahan utama sejak release sebelumnya:

- release: v1.20.20
- docs: improve release notes v1.20.31

### 📝 Documentation & Quality Gates

- Menjalankan audit dokumentasi CLI agar coverage command tetap lengkap.
- Menjalankan build web/docs sebelum commit/tag release.
- Menambahkan validasi checksum untuk semua asset release.

## Tested

- Tests: \ok  	github.com/gede-cahya/Smara-CLI/internal/web	(cached)
ok  	github.com/gede-cahya/Smara-CLI/cmd/smara	(cached) ✓
- Web build: \ ✓
- Docs check: \ ✓
- Cross-compile: Linux AMD64/ARM64, macOS AMD64/ARM64, Windows AMD64 ✓
- Checksums: \ ✓

## Files Changed

- `VERSION`
- `cmd/smara/version.go`
- `versions/RELEASE_v1.20.20.md`
- `versions/RELEASE_v1.20.31.md`

## Platform Artifacts

| Platform       | Archive                                    |
| -------------- | ------------------------------------------ |
| Linux AMD64    | \        |
| Linux ARM64    | \        |
| macOS AMD64    | \       |
| macOS ARM64    | \       |
| Windows AMD64  | \         |

Raw binary assets are also attached for direct/manual downloads.

## Upgrade

🌀 Memeriksa pembaruan Smara...
❌ Gagal mendapatkan informasi rilis: GitHub API mengembalikan status: 404 Not Found
