# Release Notes — Smara CLI v1.21.2

Release **v1.21.2** menambahkan dukungan **Prefixed MCP Tool Routing** (`server:tool`, `server/tool`, `server__tool`) untuk Smara Web & CLI.

---

## 🚀 Perubahan Utama

1. **Prefixed MCP Tool Routing**:
   - Menyelesaikan isu error `"tool 'server:tool_name' tidak ditemukan di route map"` saat model LLM memanggil MCP tool dengan menyertakan nama server sebagai prefix (misal `codebase-memory-mcp:index_repository`).
   - Pendaftaran otomatis alias prefix (`server:tool`, `server/tool`, `server__tool`) pada `rebuildToolRoute` dan *fallback resolution* di `executeToolCall`.

2. **Pengujian End-to-End dengan Playwright**:
   - Penambahan pengujian otomatis Playwright (`mcp-tool-call.spec.ts`) untuk memverifikasi pemanggilan MCP tool ber-prefix di Smara Web.
