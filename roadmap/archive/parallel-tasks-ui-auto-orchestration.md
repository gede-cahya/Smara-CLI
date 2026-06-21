# Parallel Tasks UI & Auto Orchestration Roadmap

## Status

**Implemented / Verified**

Roadmap ini melanjutkan implementasi `parallel-task-orchestration.md` dengan fokus pada:

1. UI Web untuk memantau parallel task orchestration.
2. Integrasi otomatis agar Smara memakai parallel orchestration saat task cocok.
3. Config user-facing untuk enable/disable, concurrency, dry-run, dan fallback serial.

---

## Goals

- Smara otomatis mendeteksi task kompleks yang aman untuk dipecah menjadi beberapa subtask paralel.
- User dapat melihat progress parallel orchestration dari UI Web.
- UI menampilkan batch, subtask, status, risk level, durasi, error, dan final report.
- Task mutating/destructive tetap melewati safety guardrail dan approval.
- Ada konfigurasi yang mudah diubah tanpa perlu mengedit kode.

---

## Non-Goals

- Membuat multi-agent distributed execution lintas mesin.
- Menghilangkan approval untuk task berisiko.
- Menjadikan semua task selalu paralel.
- Mengganti workflow engine yang sudah ada.

---

## Proposed UX

Tambahkan tab sidebar baru:

```text
Parallel Tasks
```

Halaman ini berisi:

1. **Status Card**
   - Auto orchestration enabled/disabled.
   - Current run status: idle, planning, running, waiting approval, completed, failed.
   - Active run ID.
   - Total subtask.
   - Running/success/failed/skipped counts.

2. **Config Panel**
   - Toggle enable/disable auto orchestration.
   - Max concurrency input.
   - Dry-run toggle.
   - Serial fallback toggle.
   - Auto threshold selector.

3. **Execution Timeline**
   - Event list:
     - orchestration started
     - plan generated
     - batch started
     - subtask started
     - subtask completed
     - approval required
     - orchestration completed

4. **Batch Lanes**
   - Batch/wave cards.
   - Parallel, serial, gated labels.
   - Batch duration.
   - Batch status.

5. **Subtask Cards**
   - Title/description.
   - Status.
   - Risk level.
   - Dependencies.
   - Tool calls used.
   - Duration.
   - Error output if failed.

6. **Final Report Panel**
   - Aggregated markdown summary.
   - Failed/skipped task notes.
   - Suggested next action.

---

## Backend Plan

### 1. Config

Add user-facing config:

```yaml
parallel_orchestration:
  enabled: true
  max_concurrency: 4
  dry_run: false
  serial_fallback: true
  auto_threshold: complex
```

Expected behavior:

- `enabled: false` disables auto orchestration.
- `max_concurrency` controls executor parallelism.
- `dry_run: true` plans/schedules but does not execute actual task handlers.
- `serial_fallback: true` falls back to normal agent loop if orchestration fails.
- `auto_threshold` determines when orchestration is triggered.

Suggested threshold values:

```text
conservative
balanced
complex
aggressive
```

Default recommendation:

```yaml
auto_threshold: complex
```

---

### 2. Auto Decision

Create a lightweight decision layer before normal agent execution.

Pseudo logic:

```go
if !config.ParallelOrchestration.Enabled {
    return runNormalAgent(task)
}

if !ShouldAutoOrchestrate(task, mode, context) {
    return runNormalAgent(task)
}

result, err := runParallelOrchestration(task)
if err != nil && config.ParallelOrchestration.SerialFallback {
    return runNormalAgent(task)
}

return result
```

Initial heuristic:

- Enable orchestration when task includes multiple independent read-only actions:
  - cek
  - analisis
  - cari
  - baca
  - audit
  - rangkum
  - inspect
  - list
  - grep
  - test after analysis
- Avoid orchestration for:
  - very short/simple prompt
  - direct shell command only
  - explicit sequential instruction
  - destructive task without planning/approval
  - task that edits the same file repeatedly

---

### 3. Status Snapshot Store

Add in-memory orchestration status store:

```go
type OrchestrationStatus struct {
    RunID       string
    Status      string
    StartedAt   time.Time
    UpdatedAt   time.Time
    CompletedAt *time.Time
    Config      ParallelOrchestrationConfig
    Plan        *workflow.Plan
    Batches     []BatchStatus
    Subtasks    []SubtaskStatus
    Events      []OrchestrationEvent
    Report      string
    Error       string
}
```

Status values:

```text
idle
planning
scheduled
running
waiting_approval
completed
failed
cancelled
```

Subtask status values:

```text
pending
running
success
failed
skipped
blocked
waiting_approval
```

---

### 4. Events

Emit events from orchestration lifecycle:

```text
orchestration_started
orchestration_planned
orchestration_scheduled
orchestration_batch_started
orchestration_subtask_started
orchestration_subtask_completed
orchestration_waiting_approval
orchestration_completed
orchestration_failed
```

Each event should include:

- `run_id`
- `type`
- `message`
- `timestamp`
- optional `batch_id`
- optional `subtask_id`
- optional `status`
- optional `risk_level`

---

### 5. Web API

Add endpoints:

```http
GET /api/orchestration/status
GET /api/orchestration/config
POST /api/orchestration/config
```

Optional later:

```http
POST /api/orchestration/cancel
POST /api/orchestration/rerun
```

Response example:

```json
{
  "run_id": "orch_20260101_120000",
  "status": "running",
  "summary": {
    "total": 8,
    "running": 2,
    "success": 4,
    "failed": 0,
    "skipped": 0,
    "pending": 2
  },
  "batches": [],
  "subtasks": [],
  "events": [],
  "report": ""
}
```

---

## Frontend Plan

### 1. API Client

Add functions in `web/src/api.ts`:

```ts
export async function getParallelTasksStatus(): Promise<OrchestrationStatus>
export async function getParallelTasksConfig(): Promise<ParallelOrchestrationConfig>
export async function updateParallelTasksConfig(config: ParallelOrchestrationConfig): Promise<ParallelOrchestrationConfig>
```

---

### 2. Types

Create or extend frontend types:

```ts
export type OrchestrationRunStatus =
  | 'idle'
  | 'planning'
  | 'scheduled'
  | 'running'
  | 'waiting_approval'
  | 'completed'
  | 'failed'
  | 'cancelled'

export interface ParallelOrchestrationConfig {
  enabled: boolean
  maxConcurrency: number
  dryRun: boolean
  serialFallback: boolean
  autoThreshold: 'conservative' | 'balanced' | 'complex' | 'aggressive'
}

export interface OrchestrationStatus {
  runId: string
  status: OrchestrationRunStatus
  summary: {
    total: number
    pending: number
    running: number
    success: number
    failed: number
    skipped: number
    blocked: number
  }
  batches: OrchestrationBatch[]
  subtasks: OrchestrationSubtask[]
  events: OrchestrationEvent[]
  report: string
  error?: string
}
```

---

### 3. Page Component

Create:

```text
web/src/pages/ParallelTasks.tsx
```

Main sections:

- Header and status summary.
- Config controls.
- Current run summary.
- Batch lanes.
- Subtask grid.
- Event timeline.
- Final report markdown panel.

Polling strategy for initial version:

```text
Refresh every 2s while status is planning/running/waiting_approval.
Refresh every 10s while idle/completed/failed.
```

Later can be upgraded to websocket events.

---

### 4. Sidebar Navigation

Update `web/src/App.tsx` or route registry:

```text
Parallel Tasks
```

Suggested icon:

- `GitBranch`
- `Network`
- `Workflow`
- `Split`

---

### 5. Chat Integration

When a message triggers orchestration, Chat should show a compact card:

```text
Parallel orchestration aktif
8 subtasks · 3 batches · 2 running
[Open Parallel Tasks]
```

This can be added after backend event/status is available.

---

## Implementation Phases

### Phase 1 — Backend Config & Auto Decision

- [x] Add `ParallelOrchestrationConfig`.
- [x] Load/save config from existing config system.
- [x] Implement `ShouldAutoOrchestrate`.
- [x] Add tests for decision heuristic.

**Exit criteria:** config exists and auto decision can be tested independently.

---

### Phase 2 — Supervisor Integration

- [x] Integrate orchestration before/inside agent supervisor flow.
- [x] Map config to `workflow.ExecutorConfig`.
- [x] Add serial fallback.
- [x] Ensure mutating/destructive subtasks still need safety approval.

**Exit criteria:** complex task can trigger orchestration automatically; simple task uses normal flow.

---

### Phase 3 — Orchestration Status Store

- [x] Add in-memory status snapshot.
- [x] Store current run state.
- [x] Track batch and subtask lifecycle.
- [x] Track final report and errors.

**Exit criteria:** backend can expose current orchestration state without frontend.

---

### Phase 4 — Web API

- [x] Add `GET /api/orchestration/status`.
- [x] Add `GET /api/orchestration/config`.
- [x] Add `POST /api/orchestration/config`.
- [x] Add handler tests if web handlers are tested in project.

**Exit criteria:** API can be called from browser/devtools/curl.

---

### Phase 5 — Parallel Tasks UI

- [x] Create `web/src/pages/ParallelTasks.tsx`.
- [x] Add config controls.
- [x] Add status cards.
- [x] Add batch lanes.
- [x] Add subtask cards.
- [x] Add event timeline.
- [x] Add report panel.
- [x] Add sidebar navigation.

**Exit criteria:** UI builds and displays real backend status.

---

### Phase 6 — Chat Indicator

- [x] Display compact orchestration indicator in Chat.
- [x] Link to Parallel Tasks page.
- [x] Show running/success/failed summary if available.

**Exit criteria:** user can see from Chat when orchestration is active.

---

### Phase 7 — Verification & Hardening

- [x] `go test ./internal/agent/... ./internal/web/... ./internal/config/...`
- [x] `cd web && npm run build`
- [x] Manual test read-only complex task.
- [x] Manual test mutating task with approval.
- [x] Manual test config disabled.
- [x] Manual test dry-run.
- [x] Manual test fallback serial.

**Exit criteria:** feature is safe, visible, configurable, and stable.

---

## Verification Commands

Backend targeted tests:

```bash
go test ./internal/agent/...
go test ./internal/web/...
go test ./internal/config/...
```

Frontend build:

```bash
cd web
npm run build
```

Full regression if time allows:

```bash
go test ./...
```

---

## Manual QA Scenarios

### Scenario 1 — Complex Read-only Task

Prompt:

```text
Analisis project ini: cek struktur folder, cari TODO, baca config utama, cek dependency, lalu rangkum masalah utama.
```

Expected:

- Auto orchestration triggered.
- Several read-only subtasks run in parallel.
- UI shows running subtasks.
- Final report appears.

---

### Scenario 2 — Simple Task

Prompt:

```text
Apa isi README?
```

Expected:

- Normal agent flow.
- Orchestration not triggered.

---

### Scenario 3 — Mutating Task

Prompt:

```text
Cari error build, perbaiki file yang perlu, lalu jalankan test.
```

Expected:

- Analysis can run in orchestration.
- File edits require approval/safety gate.
- Tests run after edits.
- UI shows gated/waiting approval status where applicable.

---

### Scenario 4 — Config Disabled

Config:

```yaml
parallel_orchestration:
  enabled: false
```

Expected:

- All tasks use normal agent flow.
- UI shows disabled state.

---

### Scenario 5 — Dry Run

Config:

```yaml
parallel_orchestration:
  dry_run: true
```

Expected:

- Plan/schedule generated.
- No actual tool execution.
- UI shows skipped/dry-run result.

---

## Risks

### Risk: Auto orchestration too aggressive

Mitigation:

- Default threshold `complex`.
- Fallback serial enabled.
- User can disable.

### Risk: Unsafe task runs in parallel

Mitigation:

- Use safety classifier.
- Mutating/destructive tasks gated.
- Read-only-only parallelism by default.

### Risk: Frontend status stale

Mitigation:

- Initial polling.
- Later upgrade to websocket events.

### Risk: Race condition on file edits

Mitigation:

- Resource conflict detection.
- Same-file mutating subtasks should not run concurrently.

---

## Rollback Plan

If the feature causes regression:

1. Set config:

```yaml
parallel_orchestration:
  enabled: false
```

2. Hide UI nav item if needed.
3. Keep orchestration engine code but disable auto trigger.
4. Revert supervisor integration only if necessary.

---

## Future Enhancements

- Websocket live progress instead of polling.
- Visual DAG graph with dependencies.
- Pause/resume/cancel orchestration.
- Rerun failed subtask.
- Persist orchestration history.
- Export report as markdown.
- Per-project orchestration profiles.
- Adaptive concurrency based on CPU/memory.
