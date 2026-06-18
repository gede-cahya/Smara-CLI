package skill

import (
	"sort"
	"strings"
	"time"
	"unicode"
)

type Recommendation struct {
	SkillName    string   `json:"skill_name"`
	Score        float64  `json:"score"`
	Confidence   string   `json:"confidence"`
	Reasons      []string `json:"reasons"`
	Clarify      bool     `json:"clarify"`
	SuccessRate  float64  `json:"success_rate,omitempty"`
	RecentlyUsed bool     `json:"recently_used,omitempty"`
}

// RecommendationOptions controls scoring and output size.
type RecommendationOptions struct {
	Limit         int
	LowConfidence float64
	StatsProvider RecommendationStatsProvider
}

// RecommendationStatsProvider supplies historical run data for scoring.
type RecommendationStatsProvider interface {
	GetStats(skillName string) (total int, success int, avgMs int64, lastRun *time.Time, err error)
}

// RecommendSkills ranks skills for a free-form user query.
func RecommendSkills(query string, skills []*Skill, opts RecommendationOptions) []Recommendation {
	limit := opts.Limit
	if limit <= 0 {
		limit = 5
	}
	low := opts.LowConfidence
	if low <= 0 {
		low = 25
	}
	terms := tokenize(query)
	if len(terms) == 0 {
		return nil
	}
	out := make([]Recommendation, 0, len(skills))
	for _, sk := range skills {
		if sk == nil {
			continue
		}
		rec := scoreSkillRecommendation(sk, terms, query, opts.StatsProvider)
		if rec.Score > 0 {
			rec.Clarify = rec.Score < low
			if rec.Score >= 55 {
				rec.Confidence = "high"
			} else if rec.Score >= low {
				rec.Confidence = "medium"
			} else {
				rec.Confidence = "low"
			}
			out = append(out, rec)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].SkillName < out[j].SkillName
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > limit {
		out = out[:limit]
	}
	if len(out) > 0 && out[0].Score < low {
		for i := range out {
			out[i].Clarify = true
		}
	}
	return out
}

func scoreSkillRecommendation(sk *Skill, terms []string, rawQuery string, stats RecommendationStatsProvider) Recommendation {
	queryTerms := uniqueStrings(terms)
	query := strings.ToLower(strings.TrimSpace(rawQuery))
	name := strings.ToLower(sk.Name)
	nameText := strings.ReplaceAll(name, "-", " ")
	desc := strings.ToLower(sk.Description)
	trigger := strings.ToLower(sk.Trigger)
	tags := lowerSlice(sk.Tags)
	deps := lowerSlice(sk.Dependencies)
	category := lowerSlice(sk.CategoryPath)
	parent := strings.ToLower(sk.ParentID)
	tools := make([]string, 0, len(sk.Steps))
	for _, st := range sk.Steps {
		tools = append(tools, strings.ToLower(st.Tool))
	}

	score := 0.0
	reasons := []string{}

	if query == name || query == nameText {
		score += 70
		reasons = appendReasonUnique(reasons, "exact name match")
	} else if strings.Contains(query, name) || strings.Contains(query, nameText) {
		score += 45
		reasons = appendReasonUnique(reasons, "strong name phrase match")
	}
	if trigger != "" && phraseMatches(query, trigger) {
		score += 35
		reasons = appendReasonUnique(reasons, "trigger match")
	}

	nameOverlap := tokenOverlapRatio(queryTerms, tokenize(nameText))
	if nameOverlap >= 0.75 {
		score += 34
		reasons = appendReasonUnique(reasons, "name token overlap")
	} else if nameOverlap >= 0.45 {
		score += 22
		reasons = appendReasonUnique(reasons, "partial name token overlap")
	}

	triggerOverlap := tokenOverlapRatio(queryTerms, tokenize(trigger))
	if triggerOverlap >= 0.45 {
		score += 18
		reasons = appendReasonUnique(reasons, "trigger token overlap")
	}
	categoryOverlap := tokenOverlapRatio(queryTerms, category)
	if categoryOverlap > 0 {
		score += 16 * categoryOverlap
		reasons = appendReasonUnique(reasons, "category match")
	}

	for _, term := range queryTerms {
		if term == "" {
			continue
		}
		if strings.Contains(name, term) {
			score += 14
			reasons = appendReasonUnique(reasons, "nama cocok: "+term)
		}
		if strings.Contains(trigger, term) {
			score += 12
			reasons = appendReasonUnique(reasons, "trigger cocok: "+term)
		}
		if containsPart(tags, term) {
			score += 14
			reasons = appendReasonUnique(reasons, "tag cocok: "+term)
		}
		if containsPart(category, term) {
			score += 10
			reasons = appendReasonUnique(reasons, "kategori cocok: "+term)
		}
		if strings.Contains(parent, term) {
			score += 7
			reasons = appendReasonUnique(reasons, "parent cocok: "+term)
		}
		if strings.Contains(desc, term) {
			score += 7
			reasons = appendReasonUnique(reasons, "deskripsi cocok: "+term)
		}
		if containsPart(deps, term) {
			score += 6
			reasons = appendReasonUnique(reasons, "dependency cocok: "+term)
		}
		if containsPart(tools, term) {
			score += 4
			reasons = appendReasonUnique(reasons, "tool cocok: "+term)
		}
	}

	matchedTerms := countMatchedTerms(queryTerms, name, desc, trigger, tags, category, deps, tools, parent)
	if len(queryTerms) >= 3 && matchedTerms == 1 && score < 30 {
		score *= 0.65
		reasons = appendReasonUnique(reasons, "penalti match terlalu sempit")
	}

	rec := Recommendation{SkillName: sk.Name, Reasons: reasons}
	if stats != nil {
		if total, success, _, lastRun, err := stats.GetStats(sk.Name); err == nil && total > 0 {
			rate := float64(success) / float64(total)
			rec.SuccessRate = rate * 100
			if total >= 3 {
				if rate >= 0.85 {
					score += 12
					rec.Reasons = appendReasonUnique(rec.Reasons, "histori sukses tinggi")
				} else if rate < 0.5 {
					score -= 18
					rec.Reasons = appendReasonUnique(rec.Reasons, "penalti histori gagal")
				} else {
					score += rate * 6
					rec.Reasons = appendReasonUnique(rec.Reasons, "histori sukses tersedia")
				}
			} else {
				score += rate * 5
				rec.Reasons = appendReasonUnique(rec.Reasons, "histori sukses tersedia")
			}
			if lastRun != nil && time.Since(*lastRun) <= 7*24*time.Hour {
				score += 5
				rec.RecentlyUsed = true
				rec.Reasons = appendReasonUnique(rec.Reasons, "recently used")
			}
		}
	}
	for _, r := range reasons {
		rec.Reasons = appendReasonUnique(rec.Reasons, r)
	}
	if sk.Risk != nil && (normalizeRiskLevel(sk.Risk.Level) == "high" || normalizeRiskLevel(sk.Risk.Level) == "critical") && !queryExplicitlyRisky(rawQuery) {
		score -= 8
		rec.Reasons = appendReasonUnique(rec.Reasons, "penalti risiko tinggi")
	}
	if score < 0 {
		score = 0
	}
	rec.Score = score
	return rec
}

func phraseMatches(query, phrase string) bool {
	phrase = strings.ToLower(strings.TrimSpace(phrase))
	if phrase == "" {
		return false
	}
	if strings.Contains(query, phrase) || strings.Contains(phrase, query) {
		return true
	}
	return tokenOverlapRatio(tokenize(query), tokenize(phrase)) >= 0.6
}

func countMatchedTerms(terms []string, fields ...interface{}) int {
	count := 0
	for _, term := range terms {
		matched := false
		for _, field := range fields {
			switch v := field.(type) {
			case string:
				matched = strings.Contains(v, term)
			case []string:
				matched = containsPart(v, term)
			}
			if matched {
				break
			}
		}
		if matched {
			count++
		}
	}
	return count
}

func tokenOverlapRatio(queryTerms, candidateTerms []string) float64 {
	queryTerms = uniqueStrings(queryTerms)
	candidateTerms = uniqueStrings(candidateTerms)
	if len(queryTerms) == 0 || len(candidateTerms) == 0 {
		return 0
	}
	candidate := map[string]bool{}
	for _, t := range candidateTerms {
		candidate[t] = true
	}
	matches := 0
	for _, t := range queryTerms {
		if candidate[t] {
			matches++
		}
	}
	return float64(matches) / float64(len(queryTerms))
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	stop := map[string]bool{"ke": true, "di": true, "dan": true, "yang": true, "untuk": true, "the": true, "a": true, "to": true, "saya": true, "tolong": true, "mohon": true, "please": true}
	out := []string{}
	for _, f := range fields {
		if len(f) > 1 && !stop[f] {
			out = append(out, f)
		}
	}
	return out
}
func lowerSlice(in []string) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = strings.ToLower(v)
	}
	return out
}
func containsPart(list []string, term string) bool {
	for _, v := range list {
		if strings.Contains(v, term) {
			return true
		}
	}
	return false
}
func appendReasonUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
func queryExplicitlyRisky(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "deploy") || strings.Contains(q, "hapus") || strings.Contains(q, "delete") || strings.Contains(q, "install") || strings.Contains(q, "update")
}
