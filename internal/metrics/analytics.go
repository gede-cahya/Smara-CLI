package metrics

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	_ "modernc.org/sqlite"
)

// UsageEvent is one persisted LLM/prompt usage event.
type UsageEvent struct {
	Timestamp    time.Time `json:"timestamp"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	PromptCount  int       `json:"prompt_count"`
	RequestCount int       `json:"request_count"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	TotalTokens  int       `json:"total_tokens"`
	CostUSD      float64   `json:"cost_usd"`
	DurationMs   int64     `json:"duration_ms"`
	Workspace    string    `json:"workspace,omitempty"`
}

type AnalyticsSummary struct {
	GeneratedAt      time.Time    `json:"generated_at"`
	TotalPrompts     int          `json:"total_prompts"`
	TotalRequests    int          `json:"total_requests"`
	InputTokens      int64        `json:"input_tokens"`
	OutputTokens     int64        `json:"output_tokens"`
	TotalTokens      int64        `json:"total_tokens"`
	EstimatedCostUSD float64      `json:"estimated_cost_usd"`
	Models           []ModelUsage `json:"models"`
	Daily            []DailyUsage `json:"daily"`
	TopSkills        []SkillUsage `json:"top_skills"`
	RecentEvents     []UsageEvent `json:"recent_events"`
	DataSource       string       `json:"data_source"`
}

type ModelUsage struct {
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	Requests     int     `json:"requests"`
	Prompts      int     `json:"prompts"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TotalTokens  int64   `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

type DailyUsage struct {
	Date         string  `json:"date"`
	Requests     int     `json:"requests"`
	Prompts      int     `json:"prompts"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TotalTokens  int64   `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

type SkillUsage struct {
	Name        string  `json:"name"`
	RunCount    int     `json:"run_count"`
	SuccessRate float64 `json:"success_rate"`
	LastRun     string  `json:"last_run,omitempty"`
}

func DefaultAnalyticsPath(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "usage_analytics.jsonl")
}

func AppendUsageEvent(path string, ev UsageEvent) error {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	if ev.PromptCount <= 0 {
		ev.PromptCount = 1
	}
	if ev.RequestCount <= 0 {
		ev.RequestCount = 1
	}
	if ev.TotalTokens == 0 {
		ev.TotalTokens = ev.InputTokens + ev.OutputTokens
	}
	if ev.CostUSD == 0 && ev.Provider != "" {
		ev.CostUSD = EstimateCost(ev.Provider, ev.Model, int64(ev.InputTokens), int64(ev.OutputTokens))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

func ReadAnalyticsSummary(analyticsPath, dbPath string, days int) (*AnalyticsSummary, error) {
	if days <= 0 {
		days = 30
	}
	s := &AnalyticsSummary{GeneratedAt: time.Now(), DataSource: analyticsPath}
	modelMap := map[string]*ModelUsage{}
	dailyMap := map[string]*DailyUsage{}
	cutoff := time.Now().AddDate(0, 0, -days)

	f, err := os.Open(analyticsPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		for scanner.Scan() {
			var ev UsageEvent
			if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
				continue
			}
			if ev.Timestamp.IsZero() || ev.Timestamp.Before(cutoff) {
				continue
			}
			if ev.PromptCount <= 0 {
				ev.PromptCount = 1
			}
			if ev.RequestCount <= 0 {
				ev.RequestCount = 1
			}
			if ev.TotalTokens == 0 {
				ev.TotalTokens = ev.InputTokens + ev.OutputTokens
			}
			s.TotalPrompts += ev.PromptCount
			s.TotalRequests += ev.RequestCount
			s.InputTokens += int64(ev.InputTokens)
			s.OutputTokens += int64(ev.OutputTokens)
			s.TotalTokens += int64(ev.TotalTokens)
			s.EstimatedCostUSD += ev.CostUSD

			key := ev.Provider + "/" + ev.Model
			mu := modelMap[key]
			if mu == nil {
				mu = &ModelUsage{Provider: ev.Provider, Model: ev.Model}
				modelMap[key] = mu
			}
			mu.Requests += ev.RequestCount
			mu.Prompts += ev.PromptCount
			mu.InputTokens += int64(ev.InputTokens)
			mu.OutputTokens += int64(ev.OutputTokens)
			mu.TotalTokens += int64(ev.TotalTokens)
			mu.CostUSD += ev.CostUSD

			date := ev.Timestamp.Format("2006-01-02")
			du := dailyMap[date]
			if du == nil {
				du = &DailyUsage{Date: date}
				dailyMap[date] = du
			}
			du.Requests += ev.RequestCount
			du.Prompts += ev.PromptCount
			du.InputTokens += int64(ev.InputTokens)
			du.OutputTokens += int64(ev.OutputTokens)
			du.TotalTokens += int64(ev.TotalTokens)
			du.CostUSD += ev.CostUSD

			s.RecentEvents = append(s.RecentEvents, ev)
			if len(s.RecentEvents) > 20 {
				s.RecentEvents = s.RecentEvents[len(s.RecentEvents)-20:]
			}
		}
	}
	for _, v := range modelMap {
		s.Models = append(s.Models, *v)
	}
	sort.Slice(s.Models, func(i, j int) bool { return s.Models[i].TotalTokens > s.Models[j].TotalTokens })
	for _, v := range dailyMap {
		s.Daily = append(s.Daily, *v)
	}
	sort.Slice(s.Daily, func(i, j int) bool { return s.Daily[i].Date < s.Daily[j].Date })

	s.TopSkills = readTopSkillUsage(dbPath, 10)
	return s, nil
}

func readTopSkillUsage(dbPath string, limit int) []SkillUsage {
	if dbPath == "" {
		return nil
	}
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&mode=ro")
	if err != nil {
		return nil
	}
	defer db.Close()
	rows, err := db.Query(`SELECT skill_name, COUNT(*) cnt, AVG(success)*100 rate, MAX(started_at) last_run
		FROM skill_executions GROUP BY skill_name ORDER BY cnt DESC LIMIT ?`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []SkillUsage
	for rows.Next() {
		var it SkillUsage
		var last sql.NullString
		if err := rows.Scan(&it.Name, &it.RunCount, &it.SuccessRate, &last); err == nil {
			if last.Valid {
				it.LastRun = last.String
			}
			out = append(out, it)
		}
	}
	return out
}

func FormatAnalyticsCLI(s *AnalyticsSummary) string {
	if s == nil {
		return "Analytics tidak tersedia"
	}
	out := "📊 Smara Usage Analytics\n"
	out += fmt.Sprintf("Prompts: %d | Requests: %d | Tokens: %d (in %d / out %d) | Cost: $%.6f\n\n", s.TotalPrompts, s.TotalRequests, s.TotalTokens, s.InputTokens, s.OutputTokens, s.EstimatedCostUSD)
	out += "Model yang dipakai:\n"
	if len(s.Models) == 0 {
		out += "  (belum ada data usage)\n"
	}
	for _, m := range s.Models {
		out += fmt.Sprintf("  - %s/%s: %d req, %d token, $%.6f\n", m.Provider, m.Model, m.Requests, m.TotalTokens, m.CostUSD)
	}
	out += "\nGrafik token harian:\n"
	max := int64(1)
	for _, d := range s.Daily {
		if d.TotalTokens > max {
			max = d.TotalTokens
		}
	}
	if len(s.Daily) == 0 {
		out += "  (belum ada data harian)\n"
	}
	for _, d := range s.Daily {
		bars := int((d.TotalTokens * 30) / max)
		if bars == 0 && d.TotalTokens > 0 {
			bars = 1
		}
		out += fmt.Sprintf("  %s | %-30s %d tok $%.5f\n", d.Date, repeatBar(bars), d.TotalTokens, d.CostUSD)
	}
	out += "\nSkill paling sering dipakai:\n"
	if len(s.TopSkills) == 0 {
		out += "  (belum ada data skill execution)\n"
	}
	for _, sk := range s.TopSkills {
		out += fmt.Sprintf("  - %s: %d run, success %.0f%%\n", sk.Name, sk.RunCount, sk.SuccessRate)
	}
	return out
}

func repeatBar(n int) string {
	if n <= 0 {
		return ""
	}
	out := ""
	for i := 0; i < n; i++ {
		out += "█"
	}
	return out
}
