# Release v1.20.8

Smara CLI v1.20.8 ships the completed Embodied Smara Assistant MVP and the Browser Subagent improvements.

## Highlights

- Completed Browser Subagent Phase 5–6: richer Markdown report export, CLI browser run/e2e commands, and Discord-ready attachments.
- Completed Embodied Smara Assistant Phase 3–6 MVP:
  - Magic Pointer Autopilot observe-plan-execute-recover loop.
  - Voice Assistant CLI/Web API/push-to-talk MVP.
  - Anime 3D Character Assistant placeholder with avatar states and lip-sync MVP.
  - Remote Desktop Assistant Mode with desktop-agent pairing, screenshot observation, remote task proxy, emergency stop, and resume.
- Updated Smara Web with Magic Pointer, Voice, Avatar, and Remote Desktop tabs.
- Updated roadmap documentation to mark Phase 1–6 MVP complete.

## Validation

```bash
go test ./internal/browser ./internal/desktopbridge ./internal/magicpointer ./internal/voice ./internal/avatar ./internal/web ./internal/platform ./cmd/smara
npm --prefix web run build
npm --prefix docs-site run build
go build -o /tmp/smara-release-smoke ./cmd/smara
```

All validation checks passed before release packaging.
