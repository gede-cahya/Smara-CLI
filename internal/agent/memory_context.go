package agent

import (
	"fmt"
	"log"
	"strings"

	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
)

// buildMemoryContext searches the memory store for entries relevant to the
// user's prompt. It first tries semantic (embedding-based) search and
// automatically falls back to full-text search when embeddings are
// unavailable — which is the common case for proxy/custom providers that
// only expose /chat/completions (not /embeddings).
//
// This fallback is what makes long-term memory recall work even when the
// current provider doesn't support embeddings, fixing the issue where
// every memory is saved with a NULL embedding and semantic search returns
// nothing.
//
// When the prompt looks like an identity question ("siapa nama saya",
// "what's my name", etc.), a wider FTS net is cast specifically against
// memories tagged as user_preference / agent so the bot reliably finds
// the user's profile even if the prompt itself doesn't share words with
// the stored memory.
func buildMemoryContext(store memory.MemoryStore, provider llm.Provider, userPrompt string, workspaceID int64) string {
	if store == nil {
		return ""
	}

	// Identity questions warrant pulling every preference-tagged memory
	// since those are the only ones likely to contain the answer.
	if isIdentityQuery(userPrompt) {
		if results, err := store.SearchFullText("nama OR profil OR identitas OR saya OR aku OR user", workspaceID, memory.MemoryFilters{
			SearchFilters: memory.SearchFilters{Sources: []string{"user_preference", "agent", "auto-intro"}},
			Limit:         5,
		}); err == nil && len(results) > 0 {
			var parts []string
			for _, m := range results {
				parts = append(parts, fmt.Sprintf("- %s", truncateForContext(m.Content)))
			}
			return "Konteks identitas dari memori (FTS, source=user_preference):\n" + strings.Join(parts, "\n")
		}
		// Even without preference-tagged memories, scan all sources for a
		// hit on the user-identity keywords. Better than nothing.
		if results, err := store.SearchFullText("nama OR profil OR identitas", workspaceID, memory.MemoryFilters{Limit: 5}); err == nil && len(results) > 0 {
			var parts []string
			for _, m := range results {
				parts = append(parts, fmt.Sprintf("- %s", truncateForContext(m.Content)))
			}
			return "Konteks identitas dari memori (FTS, semua sumber):\n" + strings.Join(parts, "\n")
		}
	}

	// Always pull self-improvement rules first. These are operational lessons
	// (corrections, repeated failures, upgraded workflows) that should be
	// auto-applied before the model decides a fact/path/config is missing.
	if si := buildSelfImprovementContext(store, userPrompt, workspaceID); si != "" {
		return si
	}

	// Try semantic search first.
	if provider != nil {
		if embedding, err := provider.GenerateEmbedding(userPrompt); err == nil && len(embedding) > 0 {
			if results, err := store.Search(embedding, workspaceID, 3); err == nil && len(results) > 0 {
				var parts []string
				for _, r := range results {
					parts = append(parts, fmt.Sprintf("- %s (relevansi: %.2f)", truncateForContext(r.Memory.Content), r.Similarity))
				}
				return "Konteks dari memori (semantic):\n" + strings.Join(parts, "\n")
			}
		} else if err != nil {
			// Soft warn so operators notice but don't crash the flow.
			log.Printf("[memory] embedding unavailable, falling back to FTS: %v", err)
		}
	}

	// Full-text fallback. Keep the query simple — FTS5 supports
	// natural language but some punctuation needs to be sanitized.
	ftsQuery := sanitizeFTSQuery(userPrompt)
	if ftsQuery == "" {
		return ""
	}

	results, err := store.SearchFullText(ftsQuery, workspaceID, memory.MemoryFilters{Limit: 3})
	if err != nil || len(results) == 0 {
		return ""
	}

	var parts []string
	for _, m := range results {
		parts = append(parts, fmt.Sprintf("- %s", truncateForContext(m.Content)))
	}
	return "Konteks dari memori (text):\n" + strings.Join(parts, "\n")
}

// isIdentityQuery returns true when the prompt looks like the user is
// asking who they are / what their name is. The bot must pull profile
// memories before answering these — otherwise it will always say "I don't
// know" even when the answer is sitting right there in the database.
func isIdentityQuery(prompt string) bool {
	p := strings.ToLower(prompt)
	patterns := []string{
		"siapa nama saya", "siapa saya", "saya siapa",
		"nama saya apa", "nama saya siapa", "apa nama saya",
		"siapa aku", "aku siapa", "nama aku",
		"do you know me", "who am i", "whats my name", "what's my name",
		"my name", "tahu saya", "kenal saya", "ingat saya",
		"profil saya", "data saya tentang", "preferensi saya",
	}
	for _, pat := range patterns {
		if strings.Contains(p, pat) {
			return true
		}
	}
	return false
}

// sanitizeFTSQuery strips characters that FTS5 treats specially so a raw
// user prompt can be passed through. Empty result means no usable query.
func sanitizeFTSQuery(prompt string) string {
	// Keep alphanumerics, spaces, and a few harmless punctuation marks.
	var b strings.Builder
	for _, r := range prompt {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}
	// Collapse whitespace.
	fields := strings.Fields(b.String())
	// Drop ultra-short fragments that pollute FTS ranking.
	var kept []string
	for _, f := range fields {
		if len(f) >= 2 {
			kept = append(kept, f)
		}
	}
	if len(kept) == 0 {
		return ""
	}
	// FTS5 MATCH expects whitespace-separated terms.
	return strings.Join(kept, " ")
}

func truncateForContext(s string) string {
	const max = 200
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// buildSelfImprovementContext retrieves auto-apply lessons that match the
// current prompt. It deliberately uses FTS/source filtering (not embeddings) so
// corrections such as "I already told you the VPS key path" are found even when
// embeddings are unavailable.
func buildSelfImprovementContext(store memory.MemoryStore, userPrompt string, workspaceID int64) string {
	ftsQuery := sanitizeFTSQuery(userPrompt)
	queries := []string{"self improvement auto apply correction lesson workflow rule memory"}
	if ftsQuery != "" {
		queries = append([]string{ftsQuery}, queries...)
	}
	seen := map[int64]bool{}
	var parts []string
	for _, q := range queries {
		results, err := store.SearchFullText(q, workspaceID, memory.MemoryFilters{
			SearchFilters: memory.SearchFilters{Sources: []string{selfImprovementSource}},
			Limit:         5,
		})
		if err != nil || len(results) == 0 {
			continue
		}
		for _, m := range results {
			if seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			parts = append(parts, fmt.Sprintf("- %s", truncateForContext(m.Content)))
			if len(parts) >= 5 {
				break
			}
		}
		if len(parts) >= 5 {
			break
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "Self-improvement memory (AUTO-APPLY sebelum menjawab/menjalankan tool):\n" + strings.Join(parts, "\n")
}

// detectIntroduction returns the inferred user statement when the prompt
// looks like a self-introduction. Returns ("", false) if no clear intro
// pattern matches.
//
// Patterns supported:
//   - "nama saya <X>"  → "User nama: <X>"
//   - "saya bernama <X>" → same
//   - "panggil saya <X>" → "Panggil user: <X>"
//   - "saya <X>" (loose, only after "halo"/"hai"/"hi" + comma)
//
// The LLM is still expected to call `remember` for richer info, but this
// regex-level safety net guarantees a name is captured even when the
// model forgets.
func detectIntroduction(prompt string) (string, bool) {
	p := strings.TrimSpace(prompt)
	if p == "" {
		return "", false
	}
	lower := strings.ToLower(p)

	patterns := []struct {
		marker string
		label  string
	}{
		{"nama saya ", "User nama: "},
		{"nama gue ", "User nama: "},
		{"nama aku ", "User nama: "},
		{"saya bernama ", "User nama: "},
		{"aku bernama ", "User nama: "},
		{"panggil saya ", "Panggil user: "},
		{"panggil aku ", "Panggil user: "},
		{"panggil gue ", "Panggil user: "},
		{"my name is ", "User name: "},
		{"i am called ", "User name: "},
		{"call me ", "Call user: "},
	}
	for _, pat := range patterns {
		idx := strings.Index(lower, pat.marker)
		if idx < 0 {
			continue
		}
		// Slice the original prompt at the same offset to preserve the
		// user's casing for the name itself.
		start := idx + len(pat.marker)
		if start >= len(p) {
			continue
		}
		name := strings.TrimSpace(p[start:])
		// Stop at the first sentence break so multi-sentence prompts don't
		// drag the entire paragraph into the memory entry.
		for _, stop := range []string{".", ",", "\n", " dan ", " lalu ", "?"} {
			if cut := strings.Index(strings.ToLower(name), stop); cut > 0 {
				name = strings.TrimSpace(name[:cut])
			}
		}
		if name == "" || len(name) > 60 {
			continue
		}
		return pat.label + name, true
	}
	return "", false
}

// detectSelfImprovementCorrection identifies explicit operational corrections
// from the user and returns a compact lesson for durable auto-apply memory.
func detectSelfImprovementCorrection(prompt string) (string, bool) {
	p := strings.TrimSpace(prompt)
	if p == "" {
		return "", false
	}
	lower := strings.ToLower(p)
	markers := []string{
		"mulai sekarang", "ingat", "simpan", "rule", "aturan", "koreksi",
		"saya sudah", "sudah saya kasih tau", "jangan", "harus", "selalu",
		"update caranya", "perbaiki kesalahan", "self improvement",
	}
	for _, m := range markers {
		if strings.Contains(lower, m) {
			if len(p) > 500 {
				p = p[:500] + "…"
			}
			return p, true
		}
	}
	return "", false
}
