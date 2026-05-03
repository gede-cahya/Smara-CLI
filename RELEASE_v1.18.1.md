# Release Notes — Smara CLI v1.18.1

## Highlights

### 🔧 Built-in Skill Marketplace Fallback
- **Problem**: Skill registry dari `gede-cahya/smara-skills` (GitHub) sering mengembalikan 404 karena repo belum publik.
- **Solution**: Smara CLI sekarang punya **built-in marketplace registry** yang di-embed langsung ke binary via `//go:embed`.
- `smara skill search` — tetap return hasil meskipun semua external registries gagal (offline mode).
- `smara skill registry sync` — built-in manifest selalu di-sync ke cache lokal, tidak tergantung internet.
- `smara skill install <nama>` — Context7 skills masih bisa di-install tanpa marketplace external.

### 📦 Files Changed
- `internal/skill/builtin.go` — embed `builtin_marketplace.json` ke binary.
- `internal/skill/builtin_marketplace.json` — manifest built-in (15 skills: nextjs, react, go, docker, kubernetes, python, fastapi, prisma, postgres, mongodb, git, vercel, laravel, tailwindcss, typescript).
- `internal/skill/registry.go` — `Search()` sekarang fallback ke built-in ketika external gagal.
- `internal/skill/registry_cache.go` — `SyncRegistries()` selalu sync built-in ke cache.
- `internal/agent/context7_registry.go` — Context7 registry juga mencoba embed pertama.
- `internal/agent/context7_registry_embed.go` — helper embed untuk Context7 registry.
- `internal/agent/context7_registry.json` — embedded copy dari `skills/registry/index.json`.

## Upgrade Guide

1. Update via binary:
   ```bash
   smara update 1.18.1
   ```
2. Atau download manual dari GitHub Releases.

## Platform Artifacts

| Platform | Archive |
|----------|---------|
| Linux AMD64 | `smara-v1.18.1-linux-amd64.tar.gz` |
| macOS AMD64 | `smara-v1.18.1-darwin-amd64.tar.gz` |
| macOS ARM64 | `smara-v1.18.1-darwin-arm64.tar.gz` |
| Windows AMD64 | `smara-v1.18.1-windows-amd64.zip` |
