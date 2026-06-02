# Release Notes — Smara CLI v1.20.31

Update Smara CLI v1.20.31 membawa peningkatan stabilitas release automation, sinkronisasi dokumentasi, workflow agent, dan packaging asset agar proses rilis Smara CLI lebih konsisten lintas platform.

## Highlights

### 🚀 Release Automation & Distribution

Release ini memperbaiki alur rilis otomatis agar output GitHub Release lebih rapi dan siap dipakai:

1. **GitHub release agent** — workflow release menjalankan preflight, build asset lintas platform, checksum, commit, tag, push, dan publish release.
2. **Cross-platform assets lengkap** — release menyertakan archive untuk Linux, macOS, dan Windows beserta `SHA256SUMS`.
3. **Release notes konsisten** — format release notes diselaraskan kembali dengan template profesional Smara CLI sebelumnya.

### 🧩 Smara CLI Improvements

Perubahan utama sejak release sebelumnya:

- release: v1.20.31
- Memperbaiki koneksi graph/dependency custom workflow agar edge `depends_on` lebih robust.
- Memperbaiki build web setelah ada duplikasi blok kode di halaman custom workflow.
- Menjalankan ulang build frontend dan binary Smara CLI.
- Menjalankan agent release dan docs-site untuk sinkronisasi release + dokumentasi.

### 📝 Documentation & Quality Gates

- Audit dokumentasi CLI tetap lengkap: 117 command ditemukan dan 117 command tercakup.
- Docs site dicek dan disinkronkan melalui `smara-docs-site-agent`.
- Build frontend dan binary Smara berhasil sebelum release.
- Asset release dibuat untuk target platform utama dan dilengkapi checksum.

## Tested

- Web build: `cd web && npm run build` ✓
- Smara build: `make build` ✓
- Docs audit: `117/117 commands covered` ✓
- Cross-compile: Linux AMD64/ARM64, macOS AMD64/ARM64, Windows AMD64 ✓
- Checksums: `SHA256SUMS` ✓

## Files Changed

- `.smara-release-tag`
- `versions/RELEASE_v1.20.31.md`

## Platform Artifacts

| Platform      | Archive                                  |
| ------------- | ---------------------------------------- |
| Linux AMD64   | `smara-v1.20.31-linux-amd64.tar.gz`      |
| Linux ARM64   | `smara-v1.20.31-linux-arm64.tar.gz`      |
| macOS AMD64   | `smara-v1.20.31-darwin-amd64.tar.gz`     |
| macOS ARM64   | `smara-v1.20.31-darwin-arm64.tar.gz`     |
| Windows AMD64 | `smara-v1.20.31-windows-amd64.zip`       |

Raw binary/archive assets and `SHA256SUMS` are attached for direct/manual downloads.

## Upgrade

Download asset sesuai platform dari halaman GitHub Release v1.20.31, lalu ganti binary `smara` lama dengan versi terbaru.
