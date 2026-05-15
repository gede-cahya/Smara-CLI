package skill

import (
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
)

// ClassifyTrace derives a category_path and tag list from the tools used
// in an execution trace. This is how auto-captured skills get routed into
// the right branch of the skill tree instead of piling up flat.
//
// Heuristic: we look at the unique tools used and map them to well-known
// categories (remote ops, file editing, web, database, etc). Anything
// uncategorized lands in "general" → "multi-step".
func ClassifyTrace(trace ExecutionTrace) (categoryPath []string, tags []string) {
	toolSet := map[string]bool{}
	for _, s := range trace.Steps {
		toolSet[s.Tool] = true
	}

	// Broad category buckets, ordered: first bucket that matches wins
	// for the *primary* category, but every bucket that matches also
	// contributes tags.
	type bucket struct {
		label    string
		subLabel string
		tools    []string
	}
	buckets := []bucket{
		{"remote", "ssh", []string{"ssh_exec", "ssh_list_dir", "ssh_manage", "ssh_read_file", "ssh_write_file"}},
		{"file", "edit", []string{"edit_file", "write_file", "delete_file"}},
		{"file", "read", []string{"read_file", "view_file", "list_dir", "grep_search"}},
		{"web", "browse", []string{"web_search", "web_fetch"}},
		{"database", "query", []string{"db_query", "sql_exec"}},
		{"skill", "compose", []string{"skill_run", "skill_create"}},
		{"mcp", "integration", []string{"connect_mcp", "disconnect_mcp"}},
		{"memory", "recall", []string{"remember", "search_memories"}},
		{"re", "analysis", []string{"analyze_binary", "extract_strings", "scan_signature"}},
		{"graph", "query", []string{"graphify_init", "graphify_query"}},
		{"shell", "run", []string{"run_command"}},
	}

	matched := []bucket{}
	for _, b := range buckets {
		for _, t := range b.tools {
			if toolSet[t] {
				matched = append(matched, b)
				break
			}
		}
	}

	// Compose tags from every matching bucket, but prefer the *most specific*
	// bucket for the category path. "Most specific" = highest weighted
	// bucket based on tool count match ratio.
	tagSet := map[string]bool{"auto-captured": true}
	for _, b := range matched {
		tagSet[b.label] = true
		if b.subLabel != "" {
			tagSet[b.subLabel] = true
		}
	}
	tags = make([]string, 0, len(tagSet))
	for t := range tagSet {
		tags = append(tags, t)
	}
	sort.Strings(tags)

	if len(matched) == 0 {
		return []string{"general", "multi-step"}, tags
	}

	// Pick the bucket with the highest number of tools actually present
	// in the trace — more specific than just "shell/run" when trace also
	// touches edit_file.
	best := matched[0]
	bestScore := 0
	for _, b := range matched {
		score := 0
		for _, t := range b.tools {
			if toolSet[t] {
				score++
			}
		}
		// Small bias: non-"shell" buckets are preferred when scores tie,
		// because run_command shows up in almost every trace.
		if b.label != "shell" {
			score++
		}
		if score > bestScore {
			best = b
			bestScore = score
		}
	}

	categoryPath = []string{best.label}
	if best.subLabel != "" {
		categoryPath = append(categoryPath, best.subLabel)
	}
	return categoryPath, tags
}

// FindParentBySubpattern looks for an existing skill whose trace is a
// *prefix* (subpattern) of the given trace. If found, that skill is the
// natural parent in the skill tree because the new skill is a
// specialization/extension of it.
//
// Example:
//
//	existing "cek-service-vps"  = [ssh_exec, ssh_exec]
//	new trace                    = [ssh_exec, ssh_exec, run_command]
//	→ "cek-service-vps" becomes the parent of the new skill.
//
// Only captured patterns (those with a captured_skill) are considered so
// we don't parent to a pattern that was never actually saved as a skill.
func FindParentBySubpattern(db *sql.DB, trace ExecutionTrace) (string, bool) {
	if db == nil || len(trace.Steps) < 2 {
		return "", false
	}
	if err := EnsureAutoDetectTable(db); err != nil {
		return "", false
	}

	rows, err := db.Query(`
		SELECT fingerprint, trace_json, captured_skill
		FROM auto_skill_patterns
		WHERE captured_skill != '' AND captured_skill IS NOT NULL
	`)
	if err != nil {
		return "", false
	}
	defer rows.Close()

	// Load all captured patterns, then pick the longest subpattern match.
	type candidate struct {
		skillName string
		steps     []TraceStep
	}
	var candidates []candidate
	for rows.Next() {
		var fp, traceJSON, capturedSkill string
		if err := rows.Scan(&fp, &traceJSON, &capturedSkill); err != nil {
			continue
		}
		var steps []TraceStep
		if err := json.Unmarshal([]byte(traceJSON), &steps); err != nil {
			continue
		}
		candidates = append(candidates, candidate{skillName: capturedSkill, steps: steps})
	}

	// Find the longest candidate whose step sequence is a strict prefix
	// of the new trace (tools + arg keys must match at each position).
	var bestName string
	bestLen := 0
	for _, c := range candidates {
		if len(c.steps) >= len(trace.Steps) {
			// Must be strictly shorter — else it's the same pattern, not a parent.
			continue
		}
		match := true
		for i := range c.steps {
			if c.steps[i].Tool != trace.Steps[i].Tool {
				match = false
				break
			}
			if !sameArgKeys(c.steps[i].Args, trace.Steps[i].Args) {
				match = false
				break
			}
		}
		if match && len(c.steps) > bestLen {
			// Verify the skill still exists before declaring it the parent.
			if _, err := Load(c.skillName); err == nil {
				bestName = c.skillName
				bestLen = len(c.steps)
			}
		}
	}

	return bestName, bestName != ""
}

// sameArgKeys returns true if the two arg maps have the same set of keys
// (values are ignored — same as fingerprinting).
func sameArgKeys(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

// PromoteTagsToSubcategory augments a category path with a secondary
// level derived from user-visible tags (like ssh host names or domain
// keywords). Keeps category paths short and readable.
func PromoteTagsToSubcategory(base []string, promptText string) []string {
	// Currently a simple heuristic: if the prompt mentions a VPS host-like
	// name we append it as a subcategory so all skills for that host live
	// together in the tree.
	lower := strings.ToLower(promptText)
	host := detectHostKeyword(lower)
	if host != "" && len(base) < 3 {
		return append(base, host)
	}
	return base
}

// detectHostKeyword peeks at common patterns like "vps-cahya", "server-x",
// "prod-db". Returns an empty string when nothing obvious shows up.
func detectHostKeyword(s string) string {
	// Simple token scan: look for something that looks like an identifier
	// immediately after "vps", "server", or "host".
	keywords := []string{"vps", "server", "host", "prod", "staging"}
	for _, kw := range keywords {
		idx := strings.Index(s, kw)
		if idx < 0 {
			continue
		}
		// Return the keyword itself as the bucket name. We don't attempt
		// fancy tokenization because wrong guesses pollute the tree.
		return kw
	}
	return ""
}
