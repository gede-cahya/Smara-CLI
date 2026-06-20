#!/bin/sh
# Post-commit hook: auto-refresh graphify graph after each commit
# Install: cp scripts/git-hook-post-commit.sh .git/hooks/post-commit && chmod +x .git/hooks/post-commit

# Get the graph name from the repo root directory
GRAPH_NAME=$(basename "$(git rev-parse --show-toplevel)")

smara graphify update . --name "$GRAPH_NAME" >/dev/null 2>&1
