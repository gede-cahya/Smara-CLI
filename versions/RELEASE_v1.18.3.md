# Release Notes — Smara CLI v1.18.3

## Highlights

### 🎨 Web Dashboard Enhancements
- **Custom Workflow**: New tab in web dashboard for creating and running custom agent workflows with visual node editor.
- **Graphify Integration**: Direct knowledge graph visualization and querying from the web UI.
- **Skill Constellation**: Interactive dependency graph view for skill relationships using React Flow.

### 🧠 Agent Improvements
- **Skill Parameter Substitution**: `__PARAM__name` placeholders now supported in skill step arguments — enables dynamic skill composition.
- **Context7 Fallback**: Supervisor automatically routes `resolve` and `get-library-documentation` calls to connected Context7 MCP servers even when tool discovery initially fails.
- **`skill_run` Tool**: Agents can now execute skills directly via the `skill_run` built-in tool.

### 📁 Workspace CLI
- `smara workspace create <nama> --path <dir>` — specify custom workspace path instead of defaulting to current directory.

### 📡 Platform Integrations
- Enhanced gateway routing for multi-platform bot responses.
- Improved Discord, Telegram, and WhatsApp platform adapters with better message handling.

### 🛠️ Fixes
- Non-constant format string fix in `start.go` PrintInfo call (v1.18.2).

## Upgrade Guide

1. Update via binary:
   ```bash
   smara update 1.18.3
   ```
2. Atau download manual dari GitHub Releases.

## Platform Artifacts

| Platform | Archive |
|----------|---------|
| Linux AMD64 | `smara-v1.18.3-linux-amd64.tar.gz` |
| macOS AMD64 | `smara-v1.18.3-darwin-amd64.tar.gz` |
| macOS ARM64 | `smara-v1.18.3-darwin-arm64.tar.gz` |
| Windows AMD64 | `smara-v1.18.3-windows-amd64.zip` |
