# Release Notes — Smara CLI v1.18.0

## Highlights

### 🕸️ Graphify — Knowledge Graph
- **Go Codebase Parsing**: Walk directory and parse all Go files using `go/ast` to extract functions, methods, structs, interfaces, types, variables, imports, and rationale comments (`// WHY:`, `// NOTE:`, `// HACK:`, `// IMPORTANT:`).
- **Call Graph Extraction**: Build directed edges between functions/methods based on call-site analysis.
- **SQLite Persistence**: Store graph nodes, edges, and metadata in dedicated SQLite tables (`graph_nodes`, `graph_edges`, `graph_metadata`).
- **Natural Language Query**: Search graph nodes by keyword, extract subgraphs with configurable BFS depth, and find shortest paths between any two nodes.
- **Token Reduction**: Serialize subgraphs into compact text for LLM system-prompt injection (budget-limited).
- **Community Detection**: Connected-components clustering assigned to nodes.
- **Export Formats**: JSON (full graph), SVG (placeholder), GraphML (XML), Neo4j Cypher (`CREATE` statements).
- **Agent Integration**: Auto-detects codebase questions and injects relevant subgraph context into system prompts before LLM generation.

### New CLI Commands
- `smara graphify init [path]` — Parse Go codebase and persist graph.
- `smara graphify query <text>` — Keyword search with `--depth` and `--budget`.
- `smara graphify path <from> <to>` — Shortest path between two nodes.
- `smara graphify explain <node>` — Neighborhood explanation with `--depth`.
- `smara graphify export` — Export to JSON, SVG, GraphML, or Neo4j Cypher.
- `smara graphify list` / `smara graphify delete <name>` — Graph lifecycle.

### New Web API Endpoints
- `POST /api/graph/init` — Initialize graph from path.
- `GET /api/graph/list` — List stored graphs.
- `GET /api/graph/get?id=` — Graph metadata.
- `GET /api/graph/query?id=&q=&depth=` — Subgraph search.
- `GET /api/graph/nodes?id=&type=&language=&limit=` — Paginated node list.
- `GET /api/graph/neighbors?id=&node_id=&depth=` — BFS neighborhood.
- `GET /api/graph/path?id=&from=&to=` — Shortest path.
- `POST /api/graph/export?id=&format=` — Export download.

### Built-in Agent Tools
- `graphify_init` — Parse and store a graph from a directory.
- `graphify_query` — Search a stored graph by keywords.

### New Skills
- `skills/graphify-init.json` — Graph initialization via skill workflow.
- `skills/graphify-query.json` — Graph querying via skill workflow.

---

## Changes Since v1.17.0

### Added
- `internal/graphify/` package (6 files):
  - `graph.go` — Core `Graph` struct, BFS shortest path, subgraph extraction, god score.
  - `ast.go` — Go AST parser for packages, functions, methods, structs, interfaces, imports, call graph.
  - `query.go` — Keyword search, path finding, node explanation.
  - `cluster.go` — Connected-components community detection.
  - `token_reduction.go` — Compact graph serialization for LLM context.
  - `store.go` — SQLite persistence layer with schema, save, load, delete.
- `cmd/smara/graphify.go` — CLI commands (init, query, path, explain, export, list, delete).
- `internal/web/graph_handlers.go` — 9 HTTP API handlers for graph CRUD and query.
- `internal/agent/graph_context.go` — Auto-detect codebase queries and inject graph context.
- `internal/agent/builtin_tools.go` — `graphify_init` and `graphify_query` built-in tools.
- `internal/memory/store.go` — Graph schema migration (`graph_nodes`, `graph_edges`, `graph_metadata` tables + indexes).
- `skills/graphify-init.json` / `skills/graphify-query.json` — Skill definitions.

### Modified
- `cmd/smara/version.go` — bumped to 1.18.0.
- `Makefile` — bumped to 1.18.0.
- `README.md` — synced Graphify features, CLI commands, and testing section.
- `internal/agent/supervisor.go` — auto-injects graph context on codebase questions.
- `internal/web/server.go` — added graph API route registration.

---

## Upgrade Guide

1. Pull latest code: `git pull origin main`
2. Rebuild: `make build`
3. Restart `smara serve` (web UI will auto-update on next build).
4. Initialize a graph: `smara graphify init ./cmd --name my-project`

---

## Known Limitations
- Graphify currently only parses **Go** codebases. TypeScript, Python, and JavaScript AST support are planned.
- SVG export produces a placeholder (`<svg>` with graph name label); full D3/vis.js layout is planned for Web UI integration.
- `--update` incremental re-parse uses SHA256 corpus hash field but does not yet skip unchanged files automatically.
