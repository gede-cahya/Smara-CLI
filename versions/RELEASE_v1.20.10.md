# Release v1.20.10

Smara CLI v1.20.10 focuses on scalable memory graph rendering, safer web session handling, and docs-site automation checks.

## Highlights

- Added server-side memory graph slicing metadata and neighborhood/search modes for large graph usage.
- Added a lightweight canvas renderer for large Memory Graph views while keeping React Flow for smaller graphs.
- Made Web Sessions safer and lighter with compact DTO responses, history limits, safe session snapshots, and cancel/delete cleanup.
- Improved Chat session polling so preview refreshes do not overwrite the active conversation history.
- Added robust JSON handling for empty API responses in the web client.
- Added docs audit automation and CI workflow for CLI documentation coverage.

## Validation

- `go test ./internal/web ./cmd/smara`
- `npm --prefix web run build`
- `npm --prefix docs-site run docs:check`

## Notes

- Web build still emits the existing CSS warning for `file: /path`; build succeeds.
- Docs audit reports 108/108 CLI commands covered.
