# Release Notes — Smara CLI v1.20.65

Update Smara CLI v1.20.65 membawa peningkatan fitur, stabilitas, dokumentasi, workflow automation, dan packaging release agar instalasi maupun `smara update` lebih konsisten lintas platform.

## Highlights

### 🚀 Release Automation & Distribution

1. **Auto-version release** — workflow dapat menentukan versi otomatis dari `VERSION`/tag terakhir bila parameter versi tidak diberikan.
2. **GitHub Release assets lengkap** — release menyertakan raw binary dan archive kompatibel updater lama/new updater.
3. **Updater-compatible packaging** — asset `.tar.gz` untuk Linux/macOS dan `.zip` untuk Windows divalidasi sebelum release dianggap selesai.

### 🧩 Smara CLI Improvements

- chore: add smara-backup-* to gitignore
- chore: remove old backup file
- feat: add 20+ core builtin tools and fix platform session handling
- docs: update docs site v1.20.34

### 📝 Documentation & Quality Gates

- Menjalankan audit dokumentasi CLI agar coverage command tetap lengkap.
- Menjalankan build web/docs sebelum commit/tag release.
- Menambahkan validasi checksum untuk semua asset release.

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
| Linux AMD64    | `smara-v1.20.65-linux-amd64.tar.gz`        |
| Linux ARM64    | `smara-v1.20.65-linux-arm64.tar.gz`        |
| macOS AMD64    | `smara-v1.20.65-darwin-amd64.tar.gz`       |
| macOS ARM64    | `smara-v1.20.65-darwin-arm64.tar.gz`       |
| Windows AMD64  | `smara-v1.20.65-windows-amd64.zip`         |

## Upgrade

```bash
smara update 1.20.65
sudo systemctl restart smara
```
