package agent

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

// autoDetectAndCapture is called after each completed ProcessPrompt when
// tool calls happened. It records the trace, and if this exact pattern has
// now been seen enough times, automatically creates a skill from it —
// without asking the user.
//
// Runs in a goroutine so the user reply is never blocked.
func (s *Supervisor) autoDetectAndCapture(trace skill.ExecutionTrace) {
	defer func() {
		// Never let auto-skill detection crash the bot.
		if r := recover(); r != nil {
			log.Printf("[auto-skill] panic recovered: %v", r)
		}
	}()

	cfg := config.Get()
	if cfg == nil || !cfg.AutoSkillDetect {
		return
	}
	threshold := cfg.AutoSkillThreshold
	if threshold <= 0 {
		threshold = 3
	}

	if BuiltinDB == nil {
		return
	}

	record, crossed, err := skill.RecordTrace(BuiltinDB, trace, threshold)
	if err != nil {
		log.Printf("[auto-skill] record trace failed: %v", err)
		return
	}
	if record == nil {
		return // trace too short, ignored
	}

	log.Printf("[auto-skill] pattern fp=%s count=%d (threshold=%d, crossed=%v)",
		record.Fingerprint[:12], record.Count, threshold, crossed)

	if !crossed {
		return
	}

	// Pattern crossed the threshold. Generate a skill name & description,
	// then save. Prefer LLM-assisted naming/description if provider is
	// available; fall back to deterministic suggestion.
	name, description := s.generateSkillMeta(trace)
	if name == "" {
		name = skill.SuggestSkillName(trace)
	}
	if description == "" {
		description = "Auto-captured skill dari pola yang terdeteksi berulang. Pola asli: " + skill.SuggestSkillName(trace)
	}

	// Build the skill using the raw args from the most recent trace.
	steps := make([]skill.Step, 0, len(trace.Steps))
	for _, st := range trace.Steps {
		steps = append(steps, skill.Step{Tool: st.Tool, Args: st.Args})
	}

	// Classify this trace into the skill tree structure so the auto-
	// captured skill ends up in the right branch of the tree instead of
	// piling up under a generic "auto-captured" tag.
	categoryPath, tags := skill.ClassifyTrace(trace)
	categoryPath = skill.PromoteTagsToSubcategory(categoryPath, trace.PromptText)

	// Look for an existing auto-captured skill whose pattern is a strict
	// prefix of this trace. If found, it becomes the parent in the tree.
	parentID, hasParent := skill.FindParentBySubpattern(BuiltinDB, trace)

	sk := &skill.Skill{
		Name:         name,
		Description:  description,
		Steps:        steps,
		Version:      1,
		Tags:         tags,
		CategoryPath: categoryPath,
	}
	if hasParent {
		sk.ParentID = parentID
		// Inherit the parent's category so children stay in the same
		// branch (hierarchy view groups by first category segment).
		if parentSkill, err := skill.Load(parentID); err == nil && len(parentSkill.CategoryPath) > 0 {
			sk.CategoryPath = parentSkill.CategoryPath
		}
	}

	// Deduplicate: if a skill with this name already exists, bail.
	if _, err := skill.Load(name); err == nil {
		log.Printf("[auto-skill] skill '%s' already exists — skipping auto-capture", name)
		_ = skill.MarkPatternCaptured(BuiltinDB, record.Fingerprint, name)
		return
	}

	if err := sk.Validate(); err != nil {
		log.Printf("[auto-skill] validation failed for %s: %v", name, err)
		return
	}
	if err := skill.Save(sk, BuiltinDB); err != nil {
		log.Printf("[auto-skill] save failed for %s: %v", name, err)
		return
	}
	if err := skill.MarkPatternCaptured(BuiltinDB, record.Fingerprint, name); err != nil {
		log.Printf("[auto-skill] mark captured failed: %v", err)
	}
	parentInfo := ""
	if hasParent {
		parentInfo = " (child of " + parentID + ")"
	}
	log.Printf("[auto-skill] ✓ auto-created skill '%s' after %d observations under %v%s",
		name, record.Count, sk.CategoryPath, parentInfo)
}

// generateSkillMeta asks the LLM to propose a name + description for the
// captured trace. Returns ("", "") if the provider is unavailable or LLM
// output cannot be parsed — caller will fall back to deterministic naming.
func (s *Supervisor) generateSkillMeta(trace skill.ExecutionTrace) (string, string) {
	if s.provider == nil {
		return "", ""
	}

	// Compact trace summary to keep prompt short.
	var stepsDesc strings.Builder
	for i, st := range trace.Steps {
		if i > 0 {
			stepsDesc.WriteString(" → ")
		}
		stepsDesc.WriteString(st.Tool)
	}

	// Shorten the prompt example to avoid huge tokens.
	example := trace.PromptText
	if len(example) > 200 {
		example = example[:200] + "…"
	}

	sysPrompt := "Kamu adalah Skill Namer. Diberikan sebuah urutan tool calls yang terbukti berulang, usulkan nama skill (kebab-case, maksimal 40 karakter) dan deskripsi singkat 1 kalimat dalam Bahasa Indonesia. Output WAJIB JSON valid dengan key \"name\" dan \"description\" — tanpa teks lain."

	userPrompt := "Contoh prompt user: \"" + example + "\"\n\n" +
		"Urutan tool: " + stepsDesc.String() + "\n\n" +
		"Balas JSON: {\"name\": \"...\", \"description\": \"...\"}"

	resp, err := s.provider.Chat([]llm.Message{
		{Role: llm.RoleSystem, Content: sysPrompt},
		{Role: llm.RoleUser, Content: userPrompt},
	})
	if err != nil || resp == nil {
		return "", ""
	}

	// Strip DSML / code fences and try to extract JSON.
	content := strings.TrimSpace(resp.Content)
	_, content = llmStripDSML(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	// Find first {...} block.
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return "", ""
	}
	content = content[start : end+1]

	var parsed struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return "", ""
	}

	name := strings.TrimSpace(parsed.Name)
	description := strings.TrimSpace(parsed.Description)

	// Sanitize name: keep only allowed characters.
	var sb strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			sb.WriteRune(r)
		case r == ' ':
			sb.WriteRune('-')
		}
	}
	name = strings.Trim(sb.String(), "-_")
	if len(name) > 40 {
		name = name[:40]
	}
	if name == "" {
		return "", description
	}

	// Prefix so auto-captured skills are easy to identify.
	if !strings.HasPrefix(name, "auto-") {
		name = "auto-" + name
	}
	return name, description
}

// llmStripDSML defers to llm.ExtractToolCallsFromContent to remove any
// stray DSML tags from model output. Returns tool calls (ignored here) and
// cleaned text. Now uses SanitizeForUser for aggressive residual cleaning.
func llmStripDSML(content string) ([]llm.ToolCall, string) {
	calls, cleaned := llm.ExtractToolCallsFromContent(content)
	// Second pass aggressive sanitize to kill truncated/partial leftovers
	cleaned = llm.SanitizeForUser(cleaned)
	return calls, cleaned
}
