# Smara CLI v1.20.6

## Highlights

- Added Browser Subagent MVP for CLI browser automation.
- Added `smara browser run` command for natural-language browser tasks.
- Added URL/server availability checks before browser execution.
- Added Chromium automation with screenshot artifacts and Markdown reports.
- Integrated Browser Subagent with Smara platform/Discord gateway fast-path intent detection.
- Discord/browser runs can return PNG screenshots and `report.md` artifacts.
- Added localhost warning for Discord/browser contexts.
- Added component-focused screenshots for navbar and error/validation areas.
- Added browser diagnostics capture for console errors and network failures.

## Validation

- `go test ./internal/browser ./internal/platform ./cmd/smara`
- `npm --prefix web run build`
- `npm --prefix smara-desktop/frontend run build`
