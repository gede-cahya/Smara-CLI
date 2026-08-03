#!/usr/bin/env bash
# ==============================================================================
# Installer Script for MCP Shared Memory Hooks & Rules Integration
# Configures: Claude Code, Antigravity, and Smara CLI
# ==============================================================================

set -e

COLOR_GREEN="\033[0;32m"
COLOR_CYAN="\033[0;36m"
COLOR_RESET="\033[0m"

echo -e "${COLOR_CYAN}Installing Auto Session Handover & Knowledge Storage rules...${COLOR_RESET}"

# 1. Build and install Go binary if needed
echo "Building mcp-shared-memory binary..."
go build -o bin/mcp-shared-memory ./cmd/mcp-shared-memory

# Copy binary to user PATH (~/.local/bin)
mkdir -p ~/.local/bin
cp bin/mcp-shared-memory ~/.local/bin/
export PATH="$HOME/.local/bin:$PATH"

# 2. Rule definition content
RULE_CONTENT='
## Auto Session Handover & Knowledge Storage Protocol (mcp-shared-memory)

Always enforce the following MCP tool workflows:

1. **Session Start (Automatic Resume)**:
   - Call `get_session_handover(project_name)` upon starting a session or resuming work to load previous progress, pending tasks, and recent architectural changes.

2. **Knowledge Base Storage**:
   - Save critical architecture decisions, configurations, rules, and APIs using `store_knowledge(category, title, content, tags)`.
   - Categories: `architecture`, `config`, `convention`, `decisions`.
   - Search knowledge when context is missing via `query_knowledge(query, category)`.

3. **Session End / Handover**:
   - Before finishing a session or transferring to another AI client (Claude Code / Antigravity / Smara), call `save_session_handover(project_name, summary, remaining_tasks, key_decisions, code_context)`.
'

# 3. Configure Antigravity & Smara CLI (~/.smara/AGENTS.md)
mkdir -p ~/.smara
if ! grep -q "Auto Session Handover & Knowledge Storage Protocol" ~/.smara/AGENTS.md 2>/dev/null; then
    echo "$RULE_CONTENT" >> ~/.smara/AGENTS.md
    echo -e "${COLOR_GREEN}✓ Appended rules to ~/.smara/AGENTS.md${COLOR_RESET}"
else
    echo "✓ Rules already exist in ~/.smara/AGENTS.md"
fi

# 4. Configure Claude Code (~/.claude/CLAUDE.md)
mkdir -p ~/.claude
if ! grep -q "Auto Session Handover & Knowledge Storage Protocol" ~/.claude/CLAUDE.md 2>/dev/null; then
    echo "$RULE_CONTENT" >> ~/.claude/CLAUDE.md
    echo -e "${COLOR_GREEN}✓ Appended rules to ~/.claude/CLAUDE.md${COLOR_RESET}"
else
    echo "✓ Rules already exist in ~/.claude/CLAUDE.md"
fi

# 5. Register MCP tool in Claude Code config (~/.claude.json)
CLAUDE_CONFIG="$HOME/.claude.json"
BIN_PATH="$HOME/.local/bin/mcp-shared-memory"

if [ -f "$CLAUDE_CONFIG" ]; then
    if ! grep -q "mcp-shared-memory" "$CLAUDE_CONFIG"; then
        python3 -c "
import json, sys
path = '$CLAUDE_CONFIG'
with open(path, 'r') as f:
    data = json.load(f)
if 'mcpServers' not in data:
    data['mcpServers'] = {}
data['mcpServers']['mcp-shared-memory'] = {
    'command': '$BIN_PATH',
    'args': []
}
with open(path, 'w') as f:
    json.dump(data, f, indent=2)
"
        echo -e "${COLOR_GREEN}✓ Configured mcp-shared-memory in ~/.claude.json${COLOR_RESET}"
    fi
fi

# 6. Register MCP tool in Smara / Antigravity config (~/.smara/mcp.json)
SMARA_MCP_CONFIG="$HOME/.smara/mcp.json"
if [ ! -f "$SMARA_MCP_CONFIG" ]; then
    echo '{"mcpServers":{}}' > "$SMARA_MCP_CONFIG"
fi
python3 -c "
import json
path = '$SMARA_MCP_CONFIG'
with open(path, 'r') as f:
    data = json.load(f)
if 'mcpServers' not in data:
    data['mcpServers'] = {}
data['mcpServers']['mcp-shared-memory'] = {
    'command': '$BIN_PATH',
    'args': []
}
with open(path, 'w') as f:
    json.dump(data, f, indent=2)
"
echo -e "${COLOR_GREEN}✓ Configured mcp-shared-memory in ~/.smara/mcp.json${COLOR_RESET}"

echo -e "\n${COLOR_GREEN}Installation Complete! All agents now automatically bridge memory.${COLOR_RESET}"
