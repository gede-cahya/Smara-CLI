# Release Notes — Smara CLI v1.20.2

Update Smara CLI v1.20.2 dengan perbaikan dan peningkatan dari commit terbaru.

## Highlights

### 🚀 Release Update

Ringkasan perubahan utama pada release ini:

- `4bb4790` chore(release): v1.20.2

### 🧰 Automation & Reliability Tooling

- Workflow commit, tag, build, dan upload release dibuat konsisten.
- Format body GitHub Release mengikuti referensi `RELEASE_v1.19.2.md`.
- Artifact platform tetap memakai pola nama `smara-v1.20.2-<platform>.<ext>`.

## Tested

- Build: `go build ./...` ✓
- Tests: `go test ./...` ✓
- Cross-compile: Linux AMD64, macOS AMD64/ARM64, Windows AMD64 ✓

## Files Changed

- `VERSION`
- `internal/platform/discord/discord.go`
- `internal/platform/gateway.go`
- `internal/platform/telegram/telegram.go`
- `pkg/platform/adapter.go`
- `pkg/platform/discord/discord.go`
- `pkg/platform/gateway.go`
- `pkg/platform/telegram/telegram.go`

## Platform Artifacts

| Platform       | Archive                                       |
| -------------- | --------------------------------------------- |
| Linux AMD64    | `smara-v1.20.2-linux-amd64.tar.gz`         |
| macOS AMD64    | `smara-v1.20.2-darwin-amd64.tar.gz`        |
| macOS ARM64    | `smara-v1.20.2-darwin-arm64.tar.gz`        |
| Windows AMD64  | `smara-v1.20.2-windows-amd64.zip`          |

## Upgrade

```bash
smara update 1.20.2
sudo systemctl restart smara   # kalau jalan sebagai service di VPS
```
