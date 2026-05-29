package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/mcp"
)

var claudeSensitiveInlinePattern = regexp.MustCompile(`(?i)(password|token|secret|api[_-]?key|authorization|private[_-]?key)\s*[:=]\s*[^\s,;]+`)

type claudeHookEvent struct {
	SessionID      string                 `json:"session_id"`
	TranscriptPath string                 `json:"transcript_path"`
	CWD            string                 `json:"cwd"`
	HookEventName  string                 `json:"hook_event_name"`
	ToolName       string                 `json:"tool_name"`
	ToolInput      map[string]interface{} `json:"tool_input"`
	ToolResponse   interface{}            `json:"tool_response"`
}

var claudeHookCmd = &cobra.Command{
	Use:   "claude-hook",
	Short: "Claude Code hook untuk change journal Smara",
	Long:  "Membaca payload hook Claude Code dari stdin, lalu mencatat perubahan bermakna ke Smara memory dan Obsidian.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runClaudeHook(cmd.Context(), os.Stdin)
	},
}

var claudeHookInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Pasang hook Claude Code untuk project ini",
	RunE: func(cmd *cobra.Command, args []string) error {
		settingsPath, _ := cmd.Flags().GetString("settings")
		command, _ := cmd.Flags().GetString("command")
		if strings.TrimSpace(command) == "" {
			command = "smara claude-hook"
		}
		if strings.TrimSpace(settingsPath) == "" {
			settingsPath = filepath.Join(".claude", "settings.local.json")
		}
		if err := installClaudeHook(settingsPath, command); err != nil {
			return err
		}
		fmt.Printf("Claude Code hook terpasang di %s\n", settingsPath)
		return nil
	},
}

func init() {
	claudeHookInstallCmd.Flags().String("settings", "", "path settings Claude Code")
	claudeHookInstallCmd.Flags().String("command", "smara claude-hook", "command hook yang dipanggil Claude Code")
	claudeHookCmd.AddCommand(claudeHookInstallCmd)
}

func runClaudeHook(ctx context.Context, r io.Reader) error {
	cfg := config.Get()
	if !cfg.ChangeJournal.Enabled {
		return nil
	}

	payload, err := io.ReadAll(r)
	if err != nil {
		return nil
	}
	payload = []byte(strings.TrimSpace(string(payload)))
	if len(payload) == 0 {
		return nil
	}

	var event claudeHookEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil
	}
	if !isMeaningfulClaudeTool(event.ToolName, event.ToolInput) {
		return nil
	}

	entry := buildClaudeCodeChangeJournalEntry(event)
	if cfg.ChangeJournal.MemoryEnabled {
		store, closeStore, err := openMemoryStore(ctx, cfg)
		if err == nil {
			_, _ = store.Save(entry, "smara-change,auto-journal,claude-code", "claude_code", cfg.ActiveWorkspaceID, nil)
			_ = closeStore()
		}
	}
	if cfg.ChangeJournal.ObsidianEnabled {
		appendClaudeCodeEntryToObsidian(cfg, entry)
	}
	return nil
}

func isMeaningfulClaudeTool(tool string, input map[string]interface{}) bool {
	name := strings.ToLower(strings.TrimSpace(tool))
	if name == "" {
		return false
	}
	readPrefixes := []string{"read", "grep", "glob", "ls", "webfetch", "websearch", "task", "todowrite", "notebookread"}
	for _, prefix := range readPrefixes {
		if name == prefix || strings.HasPrefix(name, prefix+"_") || strings.HasPrefix(name, prefix+"-") {
			return false
		}
	}
	writeTools := []string{"write", "edit", "multiedit", "notebookedit"}
	for _, writeTool := range writeTools {
		if name == writeTool || strings.Contains(name, writeTool) {
			return true
		}
	}
	mutationParts := []string{"create", "update", "set", "manage", "post", "upload", "apply", "generate", "import", "download", "connect", "disconnect", "schedule", "deploy", "write", "edit", "patch", "delete", "remove", "remember"}
	for _, part := range mutationParts {
		if name == part || strings.Contains(name, part+"_") || strings.Contains(name, "_"+part) || strings.Contains(name, part+"-") || strings.Contains(name, "-"+part) {
			return true
		}
	}
	if name == "bash" || strings.Contains(name, "bash") {
		command, _ := input["command"].(string)
		return isMeaningfulClaudeCommand(command)
	}
	return false
}

func isMeaningfulClaudeCommand(command string) bool {
	cmd := strings.ToLower(strings.TrimSpace(command))
	if cmd == "" {
		return false
	}
	mutatingSignals := []string{
		"gofmt -w", "go build", "npm run build", "pnpm build", "yarn build", "bun run build",
		"mkdir", "touch", "rm ", "rm -", "mv ", "cp ", "chmod", "chown", "tee ", ">", ">>",
		"git commit", "git add", "git rm", "git mv", "apply_patch", "patch ", "python ", "node ",
	}
	for _, signal := range mutatingSignals {
		if strings.Contains(cmd, signal) {
			return true
		}
	}
	return false
}

func buildClaudeCodeChangeJournalEntry(event claudeHookEvent) string {
	targets := extractClaudeHookTargets(event)
	if len(targets) == 0 {
		targets = []string{"(not detected)"}
	}
	return fmt.Sprintf("## %s — Claude Code Change\n\n- Source: `claude-code`\n- Event: `%s`\n- Tool: `%s`\n- Workspace: %s\n- Session: %s\n- Targets: %s\n- Outcome: %s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		redactClaudeText(truncateClaude(event.HookEventName, 120)),
		redactClaudeText(truncateClaude(event.ToolName, 120)),
		redactClaudePath(event.CWD),
		redactClaudeText(truncateClaude(event.SessionID, 120)),
		strings.Join(targets, "; "),
		redactClaudeText(truncateClaude(formatClaudeToolOutcome(event.ToolResponse), 240)),
	)
}

func extractClaudeHookTargets(event claudeHookEvent) []string {
	keys := []string{"file_path", "path", "notebook_path", "command", "url", "name"}
	targets := make([]string, 0, len(keys)+1)
	for _, key := range keys {
		value, ok := event.ToolInput[key]
		if !ok {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			continue
		}
		if isSensitiveClaudeKey(key) {
			targets = append(targets, key+"=[REDACTED]")
			continue
		}
		if strings.Contains(strings.ToLower(key), "path") || strings.Contains(strings.ToLower(key), "file") {
			text = redactClaudePath(text)
		} else {
			text = redactClaudeText(truncateClaude(text, 180))
		}
		targets = append(targets, key+"="+text)
	}
	if event.TranscriptPath != "" {
		targets = append(targets, "transcript="+redactClaudePath(event.TranscriptPath))
	}
	return uniqueClaudeStrings(targets)
}

func formatClaudeToolOutcome(response interface{}) string {
	if response == nil {
		return "Tool completed"
	}
	data, err := json.Marshal(response)
	if err != nil {
		return fmt.Sprint(response)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err == nil {
		for _, key := range []string{"error", "status", "success"} {
			if value, ok := m[key]; ok {
				return fmt.Sprintf("%s=%v", key, value)
			}
		}
	}
	return "Tool completed"
}

func appendClaudeCodeEntryToObsidian(cfg *config.SmaraConfig, entry string) {
	server, ok := findClaudeHookObsidianServer(cfg)
	if !ok {
		return
	}
	clientCfg := mcp.MCPServerConfig{
		Name:    server.Name,
		Type:    server.Type,
		Command: server.Command,
		Args:    server.Args,
		URL:     server.URL,
		Headers: server.Headers,
		Env:     server.Env,
		Enabled: server.Enabled,
	}
	var client *mcp.Client
	var err error
	if strings.EqualFold(clientCfg.Type, "remote") {
		client, err = mcp.NewRemoteClient(clientCfg)
	} else {
		client, err = mcp.NewClient(clientCfg)
	}
	if err != nil || client == nil {
		return
	}
	defer client.Close()
	tools, err := client.ListTools()
	if err != nil {
		return
	}
	tool := obsidianHookUpdateToolName(tools)
	if tool == "" {
		return
	}
	note := strings.TrimSpace(cfg.ChangeJournal.ObsidianNote)
	if note == "" {
		note = "Second Brain/Smara/Change Log.md"
	}
	_, _ = client.CallTool(tool, map[string]interface{}{
		"targetType":       "filePath",
		"targetIdentifier": note,
		"modificationType": "wholeFile",
		"wholeFileMode":    "append",
		"createIfNeeded":   true,
		"content":          "\n\n---\n" + entry,
	})
}

func findClaudeHookObsidianServer(cfg *config.SmaraConfig) (config.MCPServer, bool) {
	preferred := strings.ToLower(strings.TrimSpace(cfg.ChangeJournal.ObsidianServer))
	var fallback []config.MCPServer
	for _, server := range cfg.MCPServers {
		if !server.Enabled {
			continue
		}
		name := strings.ToLower(server.Name)
		if preferred != "" && name == preferred {
			return server, true
		}
		if strings.Contains(name, "obsidian") {
			fallback = append(fallback, server)
		}
	}
	if len(fallback) == 0 {
		return config.MCPServer{}, false
	}
	sort.Slice(fallback, func(i, j int) bool { return fallback[i].Name < fallback[j].Name })
	return fallback[0], true
}

func obsidianHookUpdateToolName(tools []mcp.Tool) string {
	for _, want := range []string{"obsidian_update_note", "update_note"} {
		for _, tool := range tools {
			if tool.Name == want {
				return tool.Name
			}
		}
	}
	return ""
}

func installClaudeHook(settingsPath, command string) error {
	settings := map[string]interface{}{}
	if data, err := os.ReadFile(settingsPath); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("gagal parse settings Claude Code: %w", err)
		}
	}
	settings["hooks"] = mergeClaudeHookSettings(settings["hooks"], command)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath, append(data, '\n'), 0o600)
}

func mergeClaudeHookSettings(existing interface{}, command string) map[string]interface{} {
	hooks, ok := existing.(map[string]interface{})
	if !ok {
		hooks = map[string]interface{}{}
	}
	postToolUse, _ := hooks["PostToolUse"].([]interface{})
	entry := map[string]interface{}{
		"matcher": "Write|Edit|MultiEdit|NotebookEdit|Bash|mcp__.*__(create|update|set|manage|post|upload|apply|generate|import|download|connect|disconnect|schedule|deploy|write|edit|patch|delete|remove|remember).*",
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": command,
			},
		},
	}
	postToolUse = append(postToolUse, entry)
	hooks["PostToolUse"] = postToolUse
	return hooks
}

func redactClaudeText(text string) string {
	return claudeSensitiveInlinePattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := regexp.MustCompile(`[:=]`).Split(match, 2)
		if len(parts) == 0 {
			return "[REDACTED]"
		}
		return strings.TrimSpace(parts[0]) + "=[REDACTED]"
	})
}

func redactClaudePath(path string) string {
	text := strings.TrimSpace(path)
	if text == "" {
		return "(empty)"
	}
	lower := strings.ToLower(text)
	for _, part := range []string{".env", "id_rsa", "id_ed25519", "credentials", "secret", "token", "private"} {
		if strings.Contains(lower, part) {
			return "[REDACTED_PATH]"
		}
	}
	return redactClaudeText(truncateClaude(text, 180))
}

func isSensitiveClaudeKey(key string) bool {
	k := strings.ToLower(key)
	for _, part := range []string{"password", "token", "secret", "api_key", "apikey", "authorization", "private_key", "privatekey"} {
		if strings.Contains(k, part) {
			return true
		}
	}
	return false
}

func truncateClaude(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func uniqueClaudeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
