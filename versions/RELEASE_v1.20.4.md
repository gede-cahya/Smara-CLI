# Release Notes — Smara CLI v1.20.4

Update Smara CLI v1.20.4 memperkuat workflow automation, custom workflow UI/API, planning templates, serta guard keamanan untuk data sensitif.

## Highlights

### 🧩 Custom Workflow & Skill Automation — Expanded

Release ini menambahkan dan merapikan kemampuan workflow kustom agar lebih siap dipakai dari CLI maupun web UI:

1. **Custom workflow runtime** — dukungan penyimpanan/memori workflow dan eksekusi workflow kustom ditingkatkan.
2. **Web API dan UI workflow** — halaman Custom Workflow, Skills, dan Chat mendapat update agar pengalaman menjalankan otomasi lebih mulus.
3. **Bundled skills** — skill bawaan dapat dimuat/diuji lebih konsisten, termasuk skill planning template.

### 📝 Planning Templates

- Menambahkan template planning: agile-minsky, clarify-requirements, implementation-plan, risk-review, dan test-plan.
- Menambahkan unit test untuk memastikan template planning dan bundled skills tetap valid.

### 🔐 Sensitive Guard & Platform Reliability

- Menambahkan sensitive guard di layer platform untuk membantu mencegah ekspos data sensitif.
- Sinkronisasi update di package `internal/platform` dan `pkg/platform`.
- Perbaikan alur agent mode/supervisor serta handler web terkait workflow.

## Tested

- Tests: `go test ./...` ✓
- Web build + embedded dist: `make sync-dist` ✓
- Cross-compile: Linux AMD64, macOS AMD64/ARM64, Windows AMD64 ✓

## Files Changed

- `Makefile`
- `VERSION`
- `cmd/smara/serve.go`
- `cmd/smara/skill.go`
- `cmd/smara/update.go`
- `cmd/smara/version.go`
- `internal/agent/builtin_tools.go`
- `internal/agent/mode.go`
- `internal/agent/mode_test.go`
- `internal/agent/supervisor.go`
- `internal/agent/workflow/custom.go`
- `internal/agent/workflow/custom_runner.go`
- `internal/agent/workflow/workflow_test.go`
- `internal/config/config.go`
- `internal/platform/gateway.go`
- `internal/web/handlers.go`
- `pkg/agent/mode.go`
- `pkg/agent/mode_test.go`
- `pkg/platform/gateway.go`
- `web/src/api.ts`
- `web/src/pages/Chat.tsx`
- `web/src/pages/CustomWorkflow.tsx`
- `web/src/pages/Skills.tsx`
- `internal/agent/planning_template.go`
- `internal/agent/planning_template_test.go`
- `internal/agent/workflow/custom_memory.go`
- `internal/platform/sensitive_guard.go`
- `internal/platform/sensitive_guard_test.go`
- `internal/skill/bundled.go`
- `internal/skill/bundled_test.go`
- `internal/web/custom_workflow_prompt_test.go`
- `pkg/platform/sensitive_guard.go`
- `skills/planning-agile-minsky.json`
- `skills/planning-clarify-requirements.json`
- `skills/planning-implementation-plan.json`
- `skills/planning-risk-review.json`
- `skills/planning-test-plan.json`


## Platform Artifacts

| Platform       | Archive                                    |
| -------------- | ------------------------------------------ |
| Linux AMD64    | `smara-v1.20.4-linux-amd64.tar.gz`          |
| macOS AMD64    | `smara-v1.20.4-darwin-amd64.tar.gz`         |
| macOS ARM64    | `smara-v1.20.4-darwin-arm64.tar.gz`         |
| Windows AMD64  | `smara-v1.20.4-windows-amd64.zip`           |

## Upgrade

```bash
smara update 1.20.4
sudo systemctl restart smara   # kalau jalan sebagai service di VPS
```
