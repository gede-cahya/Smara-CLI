package memory

import (
	"regexp"
	"strconv"
	"strings"
)

var wikiLinkPattern = regexp.MustCompile(`\[\[\s*([^\]|#]+)(?:#[^\]|]+)?(?:\|[^\]]+)?\s*\]\]`)

func ExtractWikiLinks(content string) []string {
	matches := wikiLinkPattern.FindAllStringSubmatch(content, -1)
	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		target := normalizeWikiTarget(m[1])
		if target == "" {
			continue
		}
		key := strings.ToLower(target)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, target)
	}
	return out
}

func normalizeWikiTarget(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"'` ")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

// AutoLinkWikiLinks scans memories for Obsidian-style [[Title]] links and turns
// them into bidirectional graph edges. A target matches another memory when it
// equals its short label, full content, or one of its tags (case-insensitive).
func (s *SQLiteStore) AutoLinkWikiLinks(workspaceID int64) (int, error) {
	if err := EnsureLinksSchema(s.db); err != nil {
		return 0, err
	}
	q := `SELECT id, content, tags FROM memories WHERE (workspace_id = ? OR workspace_id IS NULL OR ? = 0)`
	rows, err := s.db.Query(q, workspaceID, workspaceID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type item struct {
		id      int64
		content string
		tags    []string
	}
	items := make([]item, 0, 128)
	index := make(map[string][]int64)
	addIndex := func(key string, id int64) {
		key = strings.ToLower(normalizeWikiTarget(key))
		if key == "" {
			return
		}
		for _, existing := range index[key] {
			if existing == id {
				return
			}
		}
		index[key] = append(index[key], id)
	}

	for rows.Next() {
		var it item
		var tagsJSON string
		if err := rows.Scan(&it.id, &it.content, &tagsJSON); err != nil {
			return 0, err
		}
		it.tags = parseTagsFromJSON(tagsJSON)
		items = append(items, it)
		addIndex(shortLabel(it.content, 80), it.id)
		addIndex(it.content, it.id)
		for _, tag := range it.tags {
			addIndex(tag, it.id)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	created := 0
	seenEdges := map[string]struct{}{}
	for _, it := range items {
		for _, target := range ExtractWikiLinks(it.content) {
			ids := index[strings.ToLower(normalizeWikiTarget(target))]
			for _, targetID := range ids {
				if targetID == it.id {
					continue
				}
				a, b := it.id, targetID
				if a > b {
					a, b = b, a
				}
				key := strings.Join([]string{itoa64(a), itoa64(b), "wikilink"}, ":")
				if _, ok := seenEdges[key]; ok {
					continue
				}
				seenEdges[key] = struct{}{}
				if _, err := s.AddBidirectionalLink(it.id, targetID, "wikilink", 1.0, "created from [["+target+"]]"); err != nil {
					return created, err
				}
				created++
			}
		}
	}
	return created, nil
}

func itoa64(v int64) string {
	return strconv.FormatInt(v, 10)
}
