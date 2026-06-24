# Smara CLI v1.20.66 Release Notes

**Release Date:** 2026-06-25

## 🚀 Highlights

- **20+ Core Builtin Tools** — Major expansion of built-in toolset for enhanced agent capabilities
- **Platform Session Handling** — Fixed session management for multi-platform deployment
- **Backup System** — Added automated backup scripts for 9Drive cloud storage integration
- **Documentation Updates** — Synced docs site with latest release

## 📦 Changes

- `feat`: Add 20+ core builtin tools and fix platform session handling
- `chore`: Add smara-backup-* to gitignore
- `chore`: Remove old backup file
- `docs`: Update docs site v1.20.34
- `release`: Bump version to 1.20.66

## 📦 Downloads

| Platform | File | Size |
|----------|------|------|
| Linux (amd64) | `smara-v1.20.66-linux-amd64.tar.gz` | ~19 MB |
| macOS (Intel) | `smara-v1.20.66-darwin-amd64.tar.gz` | ~19 MB |
| macOS (Apple Silicon) | `smara-v1.20.66-darwin-arm64.tar.gz` | ~18 MB |
| Windows (amd64) | `smara-v1.20.66-windows-amd64.zip` | ~18 MB |

## 🔧 Install

```bash
# Linux / macOS
tar -xzf smara-v1.20.66-linux-amd64.tar.gz
sudo install -Dm755 smara-v1.20.66-linux-amd64 /usr/local/bin/smara

# Windows
# Extract zip and add smara.exe to PATH
```

## Checksums

Built with Go 1.24.4 | CGO_ENABLED=1 | LDFLAGS: -s -w
