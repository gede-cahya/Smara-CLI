# Release Notes — Smara CLI v1.17.0

## Highlights

### 🧩 Skill Tree & Analytics
- **Hierarchical Skill Tree**: Skills now support `parent_id`, `category_path`, and `dependencies` for building structured automation ecosystems.
- **Execution Tracker**: New SQLite-backed `ExecutionTracker` logs every skill run with duration, success/failure, workspace, and triggered-by metadata.
- **Skill Dashboard (Web UI)**: New "Skill Tree" and "Analytics" tabs in the web interface.
  - Hierarchical tree view with collapsible categories.
  - Interactive dependency graph powered by React Flow.
  - Analytics cards: total runs, success rate, top skills, struggling skills.
  - Recent activity timeline.
- **Auto-Refinement Engine**: LLM-driven skill improvement suggestions based on execution history and user feedback.
- **New API Endpoints**:
  - `GET /api/skills/tree` — full skill hierarchy.
  - `GET /api/skills/tree?format=graph` — graph nodes + edges for visualization.
  - `GET /api/skills/stats?name=X` — per-skill execution statistics.
  - `GET /api/skills/timeline?name=X` — execution history timeline.
  - `GET /api/skills/analytics` — global analytics (top skills, struggling skills).
  - `POST /api/skills/refine` — trigger LLM refinement prompt.
  - `POST /api/skills/dependencies` — update skill dependencies.
- **New CLI Commands**:
  - `smara skill tree` — display skill hierarchy.
  - `smara skill stats <name>` — execution statistics.
  - `smara skill refine <name>` — manual refinement trigger.
  - `smara skill analytics` — global analytics summary.

### 🎨 Web UI Enhancements
- Added `@xyflow/react` for interactive skill dependency graph visualization.
- Lazy-loaded tab components for better bundle performance.
- Sub-tab persistence in localStorage.

### 🔧 Backend Improvements
- `Skill` struct extended with tree/dependency fields (`ParentID`, `CategoryPath`, `Dependencies`, `Children`).
- `NewServer` auto-initializes `ExecutionTracker` from the SQLite memory store.
- Skill run API endpoint now auto-logs executions when `SkillTracker` is available.
- Public package (`pkg/skill`) re-exports new tracker and tree types.

---

## Changes Since v1.16.0

### Added
- `internal/skill/tracker.go` — ExecutionTracker with SQLite schema and query methods.
- `internal/skill/tree.go` — TreeManager for building, validating, and querying skill dependency trees.
- `internal/skill/auto_refiner.go` — Auto-refinement engine with LLM integration.
- `web/src/pages/SkillTree.tsx` — Hierarchical skill tree UI.
- `web/src/pages/SkillGraph.tsx` — React Flow dependency graph.
- `web/src/pages/SkillDashboard.tsx` — Analytics dashboard.
- `PRD_v1.17.0_Testing.md` — Comprehensive build testing plan.

### Modified
- `cmd/smara/version.go` — bumped to 1.17.0.
- `Makefile` — bumped to 1.17.0.
- `README.md` — synced Skill Tree features, CLI commands, and testing section.
- `internal/skill/types.go` — extended Skill struct with tree fields.
- `internal/web/server.go` — added SkillTracker field and initialization.
- `internal/web/handlers.go` — added skill tree, stats, timeline, analytics, refine, and dependency handlers.
- `web/src/App.tsx` — added "Skill Tree" and "Analytics" navigation tabs.
- `web/src/api.ts` — extended SkillItem interface with new fields.
- `pkg/skill/skill.go` — re-exported ExecutionTracker, TreeManager, and related types.
- `cmd/smara/skill.go` — added `skill tree`, `skill stats`, `skill refine`, `skill analytics` commands.

### Fixed
- `internal/skill/tree.go` — removed unused variable causing Go compiler error.

---

## Upgrade Guide

1. Pull latest code: `git pull origin main`
2. Rebuild: `make build`
3. Restart `smara serve` (web UI will auto-update on next build).
4. Existing skills remain compatible — new tree fields are optional.

---

## Known Limitations
- `smara skill stats` and `smara skill analytics` CLI commands currently print placeholder text when no DB tracker is available.
- Auto-refinement requires an active LLM provider configuration.
