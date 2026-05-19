# Release v1.20.9

Smara CLI v1.20.9 improves Smara Web observability and tool/assistant UX for the web workflow.

## Highlights

- Added per-response Smara Web metadata for input tokens, output tokens, total tokens, duration, model/provider, request prompt, and estimated cost.
- Preserved response stats across session refreshes so token/cost metadata stays visible below assistant messages.
- Refined chat bubble footer layout with stats on the left and message time on the right.
- Expanded model pricing estimation for newer OpenAI GPT-5 family models plus OpenRouter/custom fallbacks.
- Improved WebSocket response payloads for both regular chat and web session chat flows.
- Added safer tool result previews and additional UI/agent mode support tests.

## Validation

- `npm --prefix web run build`
- `go test ./internal/metrics ./internal/web ./internal/ui ./internal/agent`

## Assets

Cross-platform binary archives are attached to the GitHub Release:

- Linux amd64
- Linux arm64
- Windows amd64
- macOS amd64
- macOS arm64
- SHA256 checksums
