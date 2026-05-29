package agent

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/pkg/config"
	"github.com/gede-cahya/Smara-CLI/pkg/mcp"
	"github.com/gede-cahya/Smara-CLI/pkg/safety"
)

var sensitiveInlinePattern = regexp.MustCompile(`(?i)(password|token|secret|api[_-]?key|authorization|private[_-]?key)\s*[:=]\s*[^\s,;]+`)

type changeTraceStep struct {
	Tool string
	Args map[string]interface{}
}

func (s *Supervisor) captureChangeJournalAsync(userPrompt string, result *PromptResult) {
	if result == nil {
		return
	}
	trace := cloneTraceSteps(s.lastToolTrace)
	resultCopy := *result
	resultCopy.ToolsExecuted = append([]string(nil), result.ToolsExecuted...)
	go s.captureChangeJournal(userPrompt, &resultCopy, trace)
}

func (s *Supervisor) captureChangeJournal(userPrompt string, result *PromptResult, trace []changeTraceStep) {
	cfg := config.Get().ChangeJournal
	if !cfg.Enabled || result == nil || !hasMeaningfulChange(result.ToolsExecuted, trace) {
		return
	}

	entry := buildChangeJournalEntry(userPrompt, result, trace, s.mode)
	if cfg.MemoryEnabled && s.memStore != nil {
		var emb []float32
		if s.provider != nil {
			emb, _ = s.provider.GenerateEmbedding(entry)
		}
		if _, err := s.memStore.Save(entry, fmt.Sprintf("smara-change,auto-journal,mode:%s", s.mode), "change_journal", s.workspaceID, emb); err != nil {
			log.Printf("[change-journal] memory save failed: %v", err)
		}
	}

	if cfg.ObsidianEnabled {
		s.appendChangeJournalToObsidian(cfg, entry)
	}
}

func hasMeaningfulChange(tools []string, trace []changeTraceStep) bool {
	for _, tool := range tools {
		if isMeaningfulChangeTool(tool) {
			return true
		}
	}
	for _, step := range trace {
		if isMeaningfulChangeTool(step.Tool) {
			return true
		}
	}
	return false
}

func isMeaningfulChangeTool(tool string) bool {
	name := strings.ToLower(strings.TrimSpace(tool))
	if name == "" {
		return false
	}
	readPrefixes := []string{"read", "view", "list", "search", "analyze", "analyse", "get", "resolve", "query", "fetch"}
	readTools := map[string]struct{}{
		"web_search": {}, "search_memories": {}, "analyze_workspace": {}, "grep_search": {}, "search_path": {}, "get_cwd": {},
	}
	if _, ok := readTools[name]; ok {
		return false
	}
	for _, prefix := range readPrefixes {
		if name == prefix || strings.HasPrefix(name, prefix+"_") || strings.HasPrefix(name, prefix+"-") {
			return false
		}
	}
	if safety.IsWriteTool(name) || safety.IsExecuteTool(name) {
		return true
	}
	mutationParts := []string{
		"create", "update", "set", "manage", "post", "upload", "apply", "generate", "import", "download",
		"connect", "disconnect", "schedule", "deploy", "write", "edit", "patch", "delete", "remove", "remember",
	}
	for _, part := range mutationParts {
		if name == part || strings.Contains(name, part+"_") || strings.Contains(name, "_"+part) || strings.Contains(name, part+"-") || strings.Contains(name, "-"+part) {
			return true
		}
	}
	return false
}

func buildChangeJournalEntry(userPrompt string, result *PromptResult, trace []changeTraceStep, mode Mode) string {
	tools := append([]string(nil), result.ToolsExecuted...)
	if len(tools) == 0 {
		for _, step := range trace {
			tools = append(tools, step.Tool)
		}
	}
	if len(tools) == 0 {
		tools = []string{"(none)"}
	}

	targets := extractChangeTargets(trace)
	if len(targets) == 0 {
		targets = []string{"(not detected)"}
	}

	return fmt.Sprintf("## %s — Smara Change\n\n- Mode: `%s`\n- Request: %s\n- Outcome: %s\n- Tools: `%s`\n- Targets: %s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		mode,
		redactSensitiveText(truncate(userPrompt, 500)),
		redactSensitiveText(truncate(result.Response, 500)),
		strings.Join(uniqueStrings(tools), "`, `"),
		strings.Join(targets, "; "),
	)
}

func extractChangeTargets(trace []changeTraceStep) []string {
	keys := []string{"path", "file_path", "targetIdentifier", "target_path", "output_path", "name", "skill_name", "command", "url"}
	var targets []string
	for _, step := range trace {
		for _, key := range keys {
			value, ok := step.Args[key]
			if !ok {
				continue
			}
			targets = append(targets, formatChangeTarget(key, value))
		}
	}
	return uniqueStrings(targets)
}

func formatChangeTarget(key string, value interface{}) string {
	if isSensitiveKey(key) {
		return key + "=[REDACTED]"
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" {
		return key + "=(empty)"
	}
	if isPathKey(key) && isSensitivePath(text) {
		return key + "=[REDACTED_PATH]"
	}
	return key + "=" + redactSensitiveText(truncate(text, 160))
}

func redactSensitiveText(text string) string {
	return sensitiveInlinePattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := regexp.MustCompile(`[:=]`).Split(match, 2)
		if len(parts) == 0 {
			return "[REDACTED]"
		}
		return strings.TrimSpace(parts[0]) + "=[REDACTED]"
	})
}

func isSensitiveKey(key string) bool {
	k := strings.ToLower(key)
	for _, part := range []string{"password", "token", "secret", "api_key", "apikey", "authorization", "private_key", "privatekey"} {
		if strings.Contains(k, part) {
			return true
		}
	}
	return false
}

func isPathKey(key string) bool {
	k := strings.ToLower(key)
	return strings.Contains(k, "path") || strings.Contains(k, "file")
}

func uniqueStrings(values []string) []string {
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

func cloneTraceSteps(trace []changeTraceStep) []changeTraceStep {
	cloned := make([]changeTraceStep, 0, len(trace))
	for _, step := range trace {
		args := make(map[string]interface{}, len(step.Args))
		for k, v := range step.Args {
			args[k] = v
		}
		cloned = append(cloned, changeTraceStep{Tool: step.Tool, Args: args})
	}
	return cloned
}

func (s *Supervisor) appendChangeJournalToObsidian(cfg config.ChangeJournalConfig, entry string) {
	client, tool := s.findObsidianUpdateTool(cfg.ObsidianServer)
	if client == nil || tool == "" {
		return
	}
	note := strings.TrimSpace(cfg.ObsidianNote)
	if note == "" {
		note = "Second Brain/Smara/Change Log.md"
	}
	result, err := client.CallTool(tool, map[string]interface{}{
		"targetType":       "filePath",
		"targetIdentifier": note,
		"modificationType": "wholeFile",
		"wholeFileMode":    "append",
		"createIfNeeded":   true,
		"content":          "\n\n---\n" + entry,
	})
	if err != nil {
		log.Printf("[change-journal] obsidian append failed: %v", err)
		return
	}
	if result != nil && result.IsError {
		log.Printf("[change-journal] obsidian append returned error")
	}
}

func (s *Supervisor) findObsidianUpdateTool(preferredServer string) (*mcp.Client, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	serverNames := make([]string, 0, len(s.mcpInfo))
	preferred := strings.ToLower(strings.TrimSpace(preferredServer))
	if preferred != "" {
		for name := range s.mcpInfo {
			if strings.ToLower(name) == preferred {
				serverNames = append(serverNames, name)
				break
			}
		}
	}
	var fallback []string
	for name := range s.mcpInfo {
		lower := strings.ToLower(name)
		if lower != preferred && strings.Contains(lower, "obsidian") {
			fallback = append(fallback, name)
		}
	}
	sort.Strings(fallback)
	serverNames = append(serverNames, fallback...)

	for _, name := range serverNames {
		info := s.mcpInfo[name]
		if !info.Connected {
			continue
		}
		tool := obsidianUpdateToolName(info.Tools)
		if tool == "" {
			continue
		}
		client := s.mcpClients[name]
		if client != nil {
			return client, tool
		}
	}
	return nil, ""
}

func obsidianUpdateToolName(tools []mcp.Tool) string {
	preferred := []string{"obsidian_update_note", "update_note"}
	for _, want := range preferred {
		for _, tool := range tools {
			if tool.Name == want {
				return tool.Name
			}
		}
	}
	return ""
}
