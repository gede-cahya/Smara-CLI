package memory

import (
	"database/sql"
	"sort"
	"strings"
	"time"
	"unicode"
)

// AutoLinkMode reports which similarity engine ran.
type AutoLinkMode string

const (
	AutoLinkModeSemantic AutoLinkMode = "semantic" // cosine similarity on embeddings
	AutoLinkModeLexical  AutoLinkMode = "lexical"  // jaccard on token sets
	AutoLinkModeNone     AutoLinkMode = "none"     // not enough data
)

// AutoLinkReport describes the outcome of an auto-link run.
type AutoLinkReport struct {
	Mode              AutoLinkMode `json:"mode"`
	Created           int          `json:"created"`
	MemoriesScanned   int          `json:"memories_scanned"`
	WithEmbedding     int          `json:"with_embedding"`
	EmbeddingRatio    float64      `json:"embedding_ratio"`
	Threshold         float64      `json:"threshold"`
	TopK              int          `json:"top_k"`
	FellBackToLexical bool         `json:"fell_back_to_lexical"`
}

// AutoLinkSmart runs the best available similarity engine:
//  1. If ≥ minEmbeddingRatio (default 30%) of memories have embeddings → semantic.
//  2. Otherwise → lexical (Jaccard on tokenized content+tags).
//
// Threshold is applied per-engine appropriately. The default 0.78 is reasonable
// for embeddings; for Jaccard we lower it automatically since lexical scores
// are typically smaller (clamp to opts.Threshold * 0.4 if user kept default).
func (s *SQLiteStore) AutoLinkSmart(opts AutoLinkOptions) (AutoLinkReport, error) {
	if err := EnsureLinksSchema(s.db); err != nil {
		return AutoLinkReport{}, err
	}
	if opts.Threshold <= 0 {
		opts.Threshold = 0.78
	}
	if opts.MaxPerNode <= 0 {
		opts.MaxPerNode = 5
	}

	report := AutoLinkReport{
		Threshold: opts.Threshold,
		TopK:      opts.MaxPerNode,
	}

	// Decide engine.
	total, withEmb, err := s.countEmbeddingCoverage(opts.WorkspaceID)
	if err != nil {
		return report, err
	}
	report.MemoriesScanned = total
	report.WithEmbedding = withEmb
	if total > 0 {
		report.EmbeddingRatio = float64(withEmb) / float64(total)
	}

	if total < 2 {
		report.Mode = AutoLinkModeNone
		return report, nil
	}

	if opts.Replace {
		if _, err := s.db.Exec(`DELETE FROM memory_links WHERE auto_linked=1`); err != nil {
			return report, err
		}
	}

	const minEmbeddingRatio = 0.30
	if report.EmbeddingRatio >= minEmbeddingRatio {
		n, err := s.AutoLink(AutoLinkOptions{
			WorkspaceID: opts.WorkspaceID,
			Threshold:   opts.Threshold,
			MaxPerNode:  opts.MaxPerNode,
			Replace:     false, // already cleared above
		})
		if err != nil {
			return report, err
		}
		report.Mode = AutoLinkModeSemantic
		report.Created = n
		return report, nil
	}

	// Lexical fallback. Auto-relax threshold if user kept the default semantic
	// value (0.78), which is far too high for token-set Jaccard scores.
	threshold := opts.Threshold
	if threshold >= 0.6 {
		threshold = threshold * 0.4 // 0.78 → ~0.31
	}
	n, err := s.autoLinkLexical(opts.WorkspaceID, threshold, opts.MaxPerNode)
	if err != nil {
		return report, err
	}
	report.Mode = AutoLinkModeLexical
	report.Created = n
	report.FellBackToLexical = true
	return report, nil
}

// countEmbeddingCoverage returns (total, withEmbedding) for the workspace.
func (s *SQLiteStore) countEmbeddingCoverage(workspaceID int64) (int, int, error) {
	q := `SELECT
			COUNT(*),
			SUM(CASE WHEN embedding IS NOT NULL AND length(embedding) > 0 THEN 1 ELSE 0 END)
		  FROM memories`
	args := []interface{}{}
	if workspaceID > 0 {
		q += ` WHERE (workspace_id = ? OR workspace_id IS NULL)`
		args = append(args, workspaceID)
	}
	var total int
	var withEmb sql.NullInt64
	if err := s.db.QueryRow(q, args...).Scan(&total, &withEmb); err != nil {
		return 0, 0, err
	}
	return total, int(withEmb.Int64), nil
}

// autoLinkLexical computes Jaccard similarity on tokenized memory content+tags.
// It writes weighted edges with relation='related' and auto_linked=1, top-K per node.
func (s *SQLiteStore) autoLinkLexical(workspaceID int64, threshold float64, topK int) (int, error) {
	if threshold <= 0 {
		threshold = 0.25
	}
	if topK <= 0 {
		topK = 5
	}

	q := `SELECT id, content, tags FROM memories`
	args := []interface{}{}
	if workspaceID > 0 {
		q += ` WHERE (workspace_id = ? OR workspace_id IS NULL)`
		args = append(args, workspaceID)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return 0, err
	}

	type item struct {
		id     int64
		tokens map[string]struct{}
	}
	items := make([]item, 0, 128)
	for rows.Next() {
		var id int64
		var content string
		var tagsJSON sql.NullString
		if err := rows.Scan(&id, &content, &tagsJSON); err != nil {
			rows.Close()
			return 0, err
		}
		tokens := tokenize(content)
		// Boost tags by adding them to the token set (already lowercased).
		for _, t := range parseTagsFromJSON(tagsJSON.String) {
			for _, tok := range splitToken(strings.ToLower(t)) {
				tokens[tok] = struct{}{}
			}
		}
		if len(tokens) >= 2 {
			items = append(items, item{id: id, tokens: tokens})
		}
	}
	rows.Close()

	if len(items) < 2 {
		return 0, nil
	}

	type cand struct {
		other int64
		sim   float64
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT INTO memory_links (source_id, target_id, relation, weight, auto_linked, note, created_at)
		 VALUES (?, ?, 'related', ?, 1, '', ?)
		 ON CONFLICT(source_id, target_id, relation) DO UPDATE SET
			weight = excluded.weight,
			auto_linked = 1`,
	)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	now := time.Now()
	inserted := 0
	for i := range items {
		var cands []cand
		for j := range items {
			if i == j {
				continue
			}
			sim := jaccard(items[i].tokens, items[j].tokens)
			if sim >= threshold {
				cands = append(cands, cand{other: items[j].id, sim: sim})
			}
		}
		if len(cands) == 0 {
			continue
		}
		sort.Slice(cands, func(a, b int) bool { return cands[a].sim > cands[b].sim })
		if len(cands) > topK {
			cands = cands[:topK]
		}
		for _, c := range cands {
			a, b := items[i].id, c.other
			if a > b {
				a, b = b, a
			}
			if _, err := stmt.Exec(a, b, c.sim, now); err != nil {
				return 0, err
			}
			inserted++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return inserted, nil
}

// jaccard returns |A ∩ B| / |A ∪ B|.
func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	// Iterate over the smaller set for the intersection.
	small, large := a, b
	if len(b) < len(a) {
		small, large = b, a
	}
	inter := 0
	for k := range small {
		if _, ok := large[k]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// stopwords filters very common words that add noise to similarity.
var stopwords = map[string]struct{}{
	// English
	"the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "but": {}, "if": {}, "of": {}, "to": {}, "in": {},
	"on": {}, "at": {}, "by": {}, "for": {}, "with": {}, "as": {}, "is": {}, "are": {}, "was": {}, "were": {},
	"be": {}, "been": {}, "being": {}, "this": {}, "that": {}, "these": {}, "those": {}, "it": {}, "its": {},
	"i": {}, "you": {}, "he": {}, "she": {}, "we": {}, "they": {}, "them": {}, "his": {}, "her": {}, "their": {},
	"my": {}, "our": {}, "your": {}, "from": {}, "not": {}, "no": {}, "do": {}, "does": {}, "did": {},
	"have": {}, "has": {}, "had": {}, "can": {}, "could": {}, "would": {}, "should": {}, "may": {}, "might": {},
	"will": {}, "shall": {}, "than": {}, "then": {}, "so": {}, "such": {},
	// Bahasa Indonesia
	"yang": {}, "dan": {}, "atau": {}, "dengan": {}, "untuk": {}, "dari": {}, "di": {}, "ke": {}, "pada": {},
	"adalah": {}, "ini": {}, "itu": {}, "ada": {}, "tidak": {}, "akan": {}, "saya": {}, "kamu": {}, "kita": {},
	"mereka": {}, "juga": {}, "sudah": {}, "lebih": {}, "agar": {}, "supaya": {}, "bisa": {}, "harus": {},
	"jadi": {}, "kalau": {}, "saat": {}, "dalam": {}, "sebagai": {}, "oleh": {}, "tapi": {}, "tetapi": {},
}

// tokenize lowercases and splits on non-alphanumeric runes, dropping stopwords
// and tokens shorter than 3 chars (except numbers >= 4 digits which often carry meaning).
func tokenize(text string) map[string]struct{} {
	out := make(map[string]struct{}, 64)
	for _, tok := range splitToken(strings.ToLower(text)) {
		if _, isStop := stopwords[tok]; isStop {
			continue
		}
		if len(tok) < 3 {
			continue
		}
		out[tok] = struct{}{}
	}
	return out
}

// splitToken splits a string on any rune that is not a letter or digit.
// Returns lowercased segments.
func splitToken(s string) []string {
	out := make([]string, 0, 16)
	cur := strings.Builder{}
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}
