package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

type aggressiveMemoryItem struct {
	id         int64
	content    string
	tags       map[string]struct{}
	tokens     map[string]struct{}
	entities   map[string]struct{}
	topics     map[string]struct{}
	source     string
	categoryID int64
	createdAt  time.Time
	embedding  []float32
}

type aggressiveCandidate struct {
	a, b     int64
	score    float64
	relation string
	note     string
}

// AutoLinkAggressive creates permanent auto-linked memory_edges using a low-threshold
// multi-signal scorer: shared tags, extracted entities, topic hubs, lexical overlap,
// optional embeddings, source/category, and recency. It intentionally favors a more
// connected Obsidian-like graph while capping links per node to avoid hairballs.
func (s *SQLiteStore) AutoLinkAggressive(opts AutoLinkOptions) (AutoLinkReport, error) {
	if err := EnsureLinksSchema(s.db); err != nil {
		return AutoLinkReport{}, err
	}
	if opts.Threshold <= 0 {
		opts.Threshold = 0.28
	}
	if opts.MaxPerNode <= 0 {
		opts.MaxPerNode = 10
	}
	if opts.HubThreshold <= 0 {
		opts.HubThreshold = 0.18
	}
	if !opts.HubLinks && !opts.AttachIsolated {
		// Keep aggressive defaults unless caller explicitly configured one of them.
		opts.HubLinks = true
		opts.AttachIsolated = true
	}

	total, withEmb, err := s.countEmbeddingCoverage(opts.WorkspaceID)
	if err != nil {
		return AutoLinkReport{}, err
	}
	report := AutoLinkReport{
		Mode:            AutoLinkModeAggressive,
		MemoriesScanned: total,
		WithEmbedding:   withEmb,
		Threshold:       opts.Threshold,
		TopK:            opts.MaxPerNode,
	}
	if total > 0 {
		report.EmbeddingRatio = float64(withEmb) / float64(total)
	}
	if total < 2 {
		if total == 0 {
			report.Mode = AutoLinkModeNone
		}
		return report, nil
	}
	if opts.Replace {
		if _, err := s.db.Exec(`DELETE FROM memory_links WHERE auto_linked=1`); err != nil {
			return report, err
		}
	}

	items, err := s.loadAggressiveMemoryItems(opts.WorkspaceID)
	if err != nil {
		return report, err
	}
	if len(items) < 2 {
		return report, nil
	}

	candidates := make([]aggressiveCandidate, 0, len(items)*2)
	bestByNode := map[int64]aggressiveCandidate{}
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			cand := scoreAggressivePair(items[i], items[j])
			if cand.score <= 0 {
				continue
			}
			if cand.score >= opts.Threshold {
				candidates = append(candidates, cand)
			}
			rememberBestAggressive(bestByNode, items[i].id, cand)
			rememberBestAggressive(bestByNode, items[j].id, cand)
		}
	}

	selected := selectAggressiveCandidates(candidates, opts.MaxPerNode)
	selectedMap := make(map[string]bool, len(selected))
	for _, c := range selected {
		selectedMap[pairKey(c.a, c.b, c.relation)] = true
	}
	if opts.AttachIsolated {
		degree := map[int64]int{}
		for _, c := range selected {
			degree[c.a]++
			degree[c.b]++
		}
		for _, it := range items {
			if degree[it.id] > 0 {
				continue
			}
			best, ok := bestByNode[it.id]
			if !ok || best.score <= 0 || best.a == best.b {
				continue
			}
			if best.score < opts.HubThreshold {
				best.score = opts.HubThreshold
				best.relation = "auto:hub"
				best.note = "attached isolated node to closest topic/entity neighbor"
			}
			key := pairKey(best.a, best.b, best.relation)
			if !selectedMap[key] {
				selected = append(selected, best)
				selectedMap[key] = true
				report.AttachedIsolated++
			}
		}
	}

	created, err := s.insertAggressiveCandidates(selected)
	if err != nil {
		return report, err
	}
	report.Created = created
	return report, nil
}

func (s *SQLiteStore) loadAggressiveMemoryItems(workspaceID int64) ([]aggressiveMemoryItem, error) {
	q := `SELECT id, content, tags, source, category_id, created_at, embedding FROM memories WHERE (workspace_id = ? OR workspace_id IS NULL OR ? = 0) ORDER BY created_at DESC`
	rows, err := s.db.Query(q, workspaceID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]aggressiveMemoryItem, 0, 128)
	for rows.Next() {
		var it aggressiveMemoryItem
		var tagsJSON sql.NullString
		var source sql.NullString
		var cat sql.NullInt64
		var created sql.NullTime
		var emb []byte
		if err := rows.Scan(&it.id, &it.content, &tagsJSON, &source, &cat, &created, &emb); err != nil {
			return nil, err
		}
		it.source = strings.ToLower(strings.TrimSpace(source.String))
		if cat.Valid {
			it.categoryID = cat.Int64
		}
		if created.Valid {
			it.createdAt = created.Time
		}
		it.tags = make(map[string]struct{})
		for _, t := range parseTagsFromJSON(tagsJSON.String) {
			for _, tok := range splitToken(strings.ToLower(t)) {
				if len(tok) >= 2 {
					it.tags[tok] = struct{}{}
				}
			}
		}
		it.tokens = tokenize(it.content)
		for t := range it.tags {
			it.tokens[t] = struct{}{}
		}
		it.entities = extractAggressiveEntities(it.content, it.tags)
		it.topics = inferAggressiveTopics(it.content, it.tags, it.entities)
		if len(emb) > 0 {
			it.embedding = blobToFloat32(emb)
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func scoreAggressivePair(a, b aggressiveMemoryItem) aggressiveCandidate {
	score := 0.0
	reasons := []string{}
	relation := "auto:mixed"
	if n := intersectionCount(a.tags, b.tags); n > 0 {
		score += math.Min(0.36, float64(n)*0.18)
		reasons = append(reasons, fmt.Sprintf("tags:%d", n))
		relation = "auto:tag"
	}
	if n := intersectionCount(a.entities, b.entities); n > 0 {
		score += math.Min(0.44, float64(n)*0.22)
		reasons = append(reasons, fmt.Sprintf("entities:%d", n))
		relation = "auto:entity"
	}
	if n := intersectionCount(a.topics, b.topics); n > 0 {
		score += math.Min(0.25, float64(n)*0.12)
		reasons = append(reasons, fmt.Sprintf("topics:%d", n))
		if relation == "auto:mixed" {
			relation = "auto:hub"
		}
	}
	if sim := jaccard(a.tokens, b.tokens); sim > 0 {
		score += sim * 0.25
		reasons = append(reasons, fmt.Sprintf("keyword:%.2f", sim))
		if relation == "auto:mixed" {
			relation = "auto:keyword"
		}
	}
	if a.source != "" && a.source == b.source {
		score += 0.05
		reasons = append(reasons, "source")
	}
	if a.categoryID > 0 && a.categoryID == b.categoryID {
		score += 0.12
		reasons = append(reasons, "category")
	}
	if !a.createdAt.IsZero() && !b.createdAt.IsZero() {
		d := a.createdAt.Sub(b.createdAt)
		if d < 0 {
			d = -d
		}
		if d <= 24*time.Hour {
			score += 0.08
			reasons = append(reasons, "within24h")
		} else if d <= 7*24*time.Hour {
			score += 0.03
			reasons = append(reasons, "within7d")
		}
	}
	if len(a.embedding) > 0 && len(b.embedding) > 0 {
		sim := cosineSimilarity(a.embedding, b.embedding)
		switch {
		case sim >= 0.85:
			score += 0.45
		case sim >= 0.75:
			score += 0.35
		case sim >= 0.65:
			score += 0.25
		case sim >= 0.55:
			score += 0.15
		}
		if sim >= 0.55 {
			reasons = append(reasons, fmt.Sprintf("semantic:%.2f", sim))
			relation = "auto:semantic"
		}
	}
	if score > 1 {
		score = 1
	}
	return aggressiveCandidate{a: minInt64(a.id, b.id), b: maxInt64(a.id, b.id), score: score, relation: relation, note: strings.Join(reasons, ", ")}
}

func selectAggressiveCandidates(cands []aggressiveCandidate, maxPerNode int) []aggressiveCandidate {
	sort.Slice(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	degree := map[int64]int{}
	seen := map[string]bool{}
	out := make([]aggressiveCandidate, 0, len(cands))
	for _, c := range cands {
		key := pairKey(c.a, c.b, c.relation)
		if seen[key] || c.a == c.b {
			continue
		}
		if maxPerNode > 0 && (degree[c.a] >= maxPerNode || degree[c.b] >= maxPerNode) {
			continue
		}
		seen[key] = true
		degree[c.a]++
		degree[c.b]++
		out = append(out, c)
	}
	return out
}

func (s *SQLiteStore) insertAggressiveCandidates(cands []aggressiveCandidate) (int, error) {
	if len(cands) == 0 {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO memory_links (source_id, target_id, relation, weight, auto_linked, note, created_at)
		VALUES (?, ?, ?, ?, 1, ?, ?)
		ON CONFLICT(source_id, target_id, relation) DO UPDATE SET weight=excluded.weight, auto_linked=1, note=excluded.note`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	now := time.Now()
	created := 0
	for _, c := range cands {
		if c.a == c.b {
			continue
		}
		if c.score <= 0 {
			continue
		}
		if _, err := stmt.Exec(c.a, c.b, c.relation, c.score, c.note, now); err != nil {
			return 0, err
		}
		created++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return created, nil
}

func rememberBestAggressive(m map[int64]aggressiveCandidate, id int64, cand aggressiveCandidate) {
	if cand.score <= 0 {
		return
	}
	if old, ok := m[id]; !ok || cand.score > old.score {
		m[id] = cand
	}
}

var aggressiveEntityPhrases = []string{
	"smara web", "memory graph", "fast lod", "graph view", "openai api", "user profile",
	"discord", "telegram", "obsidian", "smara", "vps", "ssh", "ollama", "openai", "mcp", "cli", "web", "graph", "node", "edge", "adapter", "memory", "workspace", "server", "react", "golang", "sqlite",
}

var pathOrHostRE = regexp.MustCompile(`(?i)([a-z0-9_./-]+\.(go|ts|tsx|js|jsx|json|md|yaml|yml)|\b\d{1,3}(?:\.\d{1,3}){3}\b|[a-z0-9_-]+/[a-z0-9_./-]+)`)

func extractAggressiveEntities(content string, tags map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{})
	lower := strings.ToLower(content)
	for _, phrase := range aggressiveEntityPhrases {
		if strings.Contains(lower, phrase) {
			out[phrase] = struct{}{}
		}
	}
	for _, m := range pathOrHostRE.FindAllString(lower, -1) {
		out[m] = struct{}{}
	}
	for t := range tags {
		out[t] = struct{}{}
	}
	return out
}

func inferAggressiveTopics(content string, tags, entities map[string]struct{}) map[string]struct{} {
	out := map[string]struct{}{}
	lower := strings.ToLower(content)
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(lower, w) {
				return true
			}
			if _, ok := tags[w]; ok {
				return true
			}
			if _, ok := entities[w]; ok {
				return true
			}
		}
		return false
	}
	if has("smara", "smara web") {
		out["Smara"] = struct{}{}
	}
	if has("memory", "memory graph", "graph", "node", "edge", "lod") {
		out["Memory Graph"] = struct{}{}
	}
	if has("obsidian", "graph view", "vault", "wikilink") {
		out["Obsidian"] = struct{}{}
	}
	if has("cli", "command", "cobra") {
		out["CLI"] = struct{}{}
	}
	if has("adapter", "discord", "telegram", "whatsapp") {
		out["Adapters"] = struct{}{}
	}
	if has("vps", "server", "ssh", "ubuntu", "nginx") {
		out["Infrastructure"] = struct{}{}
	}
	if has("ui", "react", "frontend", "web", "button") {
		out["Web UI"] = struct{}{}
	}
	if has("preference", "profile", "suka", "preferensi") {
		out["Preferences"] = struct{}{}
	}
	if has("ai", "llm", "openai", "ollama", "model", "mcp") {
		out["AI Tools"] = struct{}{}
	}
	return out
}

func intersectionCount(a, b map[string]struct{}) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	if len(b) < len(a) {
		a, b = b, a
	}
	n := 0
	for k := range a {
		if _, ok := b[k]; ok {
			n++
		}
	}
	return n
}

func pairKey(a, b int64, rel string) string {
	return fmt.Sprintf("%d:%d:%s", minInt64(a, b), maxInt64(a, b), rel)
}
func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// jsonCompact is useful in tests/debug logs without pulling in UI code.
func jsonCompact(v interface{}) string { b, _ := json.Marshal(v); return string(b) }
