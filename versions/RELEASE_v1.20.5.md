# Release v1.20.5

## Highlights

- Added Smara Discord auto-visual attachments for AI-generated SVG and Markdown outputs.
- Added SVG handling that saves generated SVG blocks and attempts PNG conversion for Discord-friendly preview.
- Added Markdown export attachment support so generated `.md` content can be downloaded directly.
- Added/updated Smara Discord PRD wizard and generator components/tests.
- Improved web chat rendering support with richer Markdown/table/diagram dependencies.

## Quality Gates

- `go test ./internal/platform/discord ./internal/platform`
- `npm --prefix web run build`
- `npm --prefix smara-desktop/frontend run build`

## Assets

Release artifacts include Linux, Windows, and macOS builds plus SHA256 checksums.
