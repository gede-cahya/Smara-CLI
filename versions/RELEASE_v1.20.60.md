# Release Notes — Smara CLI v1.20.60

Update Smara CLI v1.20.60 membawa peningkatan fitur, stabilitas, dokumentasi, workflow automation, dan packaging release agar instalasi maupun \ lebih konsisten lintas platform.

## Highlights

### 🚀 Release Automation & Distribution

Release ini menyiapkan workflow rilis yang lebih aman dan kompatibel:

1. **Auto-version release** — workflow dapat menentukan versi otomatis dari \/tag terakhir bila parameter versi tidak diberikan.
2. **GitHub Release assets lengkap** — release menyertakan raw binary dan archive kompatibel updater lama/new updater.
3. **Updater-compatible packaging** — asset \ untuk Linux/macOS dan \ untuk Windows divalidasi sebelum release dianggap selesai.

### 🧩 Smara CLI Improvements

Perubahan utama sejak release sebelumnya:

- chore: add smara-backup-* to gitignore
- chore: remove old backup file
- feat: add 20+ core builtin tools and fix platform session handling
- docs: update docs site v1.20.34

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

- `.gitignore`
- `docs-site/package.json`
- `docs-site/public/version.json`
- `internal/agent/builtin_tools.go`
- `internal/agent/builtin_tools_eleven_test.go`
- `internal/agent/builtin_tools_real_integration_test.go`
- `internal/platform/gateway.go`
- `internal/platform/gateway_session.go`
- `internal/platform/telegram/telegram.go`
- `internal/platform/types.go`
- `pkg/agent/builtin_tools.go`
- `roadmap/README.md`
- `roadmap/archive/image-flow-roadmap.md`
- `roadmap/archive/parallel-task-orchestration.md`
- `roadmap/archive/parallel-tasks-ui-auto-orchestration.md`
- `roadmap/archive/skill-system-upgrade-roadmap.md`
- `skills/claude-obsidian-autoresearch.json`
- `skills/claude-obsidian-canvas.json`
- `skills/claude-obsidian-defuddle.json`
- `skills/claude-obsidian-obsidian-bases.json`
- `skills/claude-obsidian-obsidian-markdown.json`
- `skills/claude-obsidian-save.json`
- `skills/claude-obsidian-think.json`
- `skills/claude-obsidian-wiki-cli.json`
- `skills/claude-obsidian-wiki-fold.json`
- `skills/claude-obsidian-wiki-ingest.json`
- `skills/claude-obsidian-wiki-lint.json`
- `skills/claude-obsidian-wiki-mode.json`
- `skills/claude-obsidian-wiki-query.json`
- `skills/claude-obsidian-wiki-retrieve.json`
- `skills/claude-obsidian-wiki.json`
- `smara-backup-pre-update`

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
