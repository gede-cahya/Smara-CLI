# Smara CLI v1.20.7

## Highlights
- Added loop-aware custom workflow agents with validation for count, conditional, retry/backoff, interval, and guarded infinite modes.
- Improved Workflow/Chat UX for custom workflow prompts and browser-subagent orchestration.
- Added browser planner/runner refinements and tests for richer browser task execution.
- Updated embedded web dashboard assets for the new workflow UI.

## Quality
- Added regression coverage for custom workflow prompt handling and browser planning.
- Validated release with `go test ./...` and production web build.
