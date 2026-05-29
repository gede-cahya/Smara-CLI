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
	name := strings.ToLower(sk.Name)
	desc := strings.ToLower(sk.Description)
	tags := lowerSlice(sk.Tags)
	deps := lowerSlice(sk.Dependencies)
	tools := make([]string, 0, len(sk.Steps))
	for _, st := range sk.Steps {
		tools = append(tools, strings.ToLower(st.Tool))
	}

	score := 0.0
	reasons := []string{}
	for _, term := range terms {
		if term == "" {
			continue
		}
		if strings.Contains(name, term) {
			score += 18
			reasons = appendReasonUnique(reasons, "nama cocok: "+term)
		}
		if strings.Contains(desc, term) {
			score += 9
			reasons = appendReasonUnique(reasons, "deskripsi cocok: "+term)
		}
		if containsPart(tags, term) {
			score += 14
			reasons = appendReasonUnique(reasons, "tag cocok: "+term)
		}
		if containsPart(deps, term) {
			score += 6
			reasons = appendReasonUnique(reasons, "dependency cocok: "+term)
		}
		if containsPart(tools, term) {
			score += 5
			reasons = appendReasonUnique(reasons, "tool cocok: "+term)
		}
	}
	if stats != nil {
		if total, success, _, _, err := stats.GetStats(sk.Name); err == nil && total > 0 {
			rate := float64(success) / float64(total)
			score += rate * 10
			reasons = append(reasons, "histori sukses tersedia")
		}
	}
	if sk.Risk != nil && (normalizeRiskLevel(sk.Risk.Level) == "high" || normalizeRiskLevel(sk.Risk.Level) == "critical") && !queryExplicitlyRisky(rawQuery) {
		score -= 8
		reasons = append(reasons, "penalti risiko tinggi")
	}
	if score < 0 {
		score = 0
	}
	return Recommendation{SkillName: sk.Name, Score: score, Reasons: reasons}
}

func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	stop := map[string]bool{"ke": true, "di": true, "dan": true, "yang": true, "untuk": true, "the": true, "a": true, "to": true}
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
