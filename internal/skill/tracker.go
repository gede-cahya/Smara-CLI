package skill

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// SkillExecution tracks one run of a skill.
type SkillExecution struct {
	ID             int64           `json:"id"`
	SkillName      string          `json:"skill_name"`
	RunID          string          `json:"run_id"`
	StartedAt      time.Time       `json:"started_at"`
	DurationMs     int64           `json:"duration_ms"`
	Success        bool            `json:"success"`
	ErrorMessage   string          `json:"error_message,omitempty"`
	TriggeredBy    string          `json:"triggered_by"`
	Workspace      string          `json:"workspace,omitempty"`
	Mode           string          `json:"mode,omitempty"`
	StepResults    []StepResultRaw `json:"step_results,omitempty"`
}

type StepResultRaw struct {
	Step         int    `json:"step"`
	Tool         string `json:"tool"`
	Success      bool   `json:"success"`
	OutputPreview string `json:"output_preview,omitempty"`
}

// SkillImprovement tracks proposed and applied refinements.
type SkillImprovement struct {
	ID                int64     `json:"id"`
	SkillName         string    `json:"skill_name"`
	Version           int       `json:"version"`
	TriggeredAt       time.Time `json:"triggered_at"`
	Trigger           string    `json:"trigger"` // auto-refine | feedback | manual
	ChangeSummary     string    `json:"change_summary,omitempty"`
	SuccessRateBefore float64   `json:"success_rate_before"`
	SuccessRateAfter  float64   `json:"success_rate_after"`
	Applied           bool      `json:"applied"`
	ProposedJSON      string    `json:"proposed_json,omitempty"`
}

// ExecutionTracker logs and queries skill executions.
type ExecutionTracker struct {
	db *sql.DB
}

// DB returns the underlying *sql.DB so adjacent packages (export,
// auto-detect) can share the same sqlite handle without opening a
// second connection.
func (t *ExecutionTracker) DB() *sql.DB {
	if t == nil {
		return nil
	}
	return t.db
}

// NewExecutionTracker creates a tracker backed by the given SQLite DB.
func NewExecutionTracker(db *sql.DB) (*ExecutionTracker, error) {
	if err := initTrackerSchema(db); err != nil {
		return nil, fmt.Errorf("tracker schema init failed: %w", err)
	}
	return &ExecutionTracker{db: db}, nil
}

func initTrackerSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS skill_executions (
			id INTEGER PRIMARY KEY,
			skill_name TEXT NOT NULL,
			run_id TEXT NOT NULL,
			started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			duration_ms INTEGER,
			success INTEGER DEFAULT 0,
			error_message TEXT,
			triggered_by TEXT DEFAULT 'manual',
			workspace TEXT,
			mode TEXT,
			step_results_json TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_skill_exec_name ON skill_executions(skill_name)`,
		`CREATE INDEX IF NOT EXISTS idx_skill_exec_time ON skill_executions(started_at)`,
		`CREATE TABLE IF NOT EXISTS skill_improvements (
			id INTEGER PRIMARY KEY,
			skill_name TEXT NOT NULL,
			version INTEGER NOT NULL,
			triggered_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			trigger TEXT NOT NULL,
			change_summary TEXT,
			success_rate_before REAL,
			success_rate_after REAL,
			applied INTEGER DEFAULT 0,
			proposed_json TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_skill_imp_name ON skill_improvements(skill_name)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

// LogExecution stores a skill run into the database.
func (t *ExecutionTracker) LogExecution(se SkillExecution) error {
	stepJSON, _ := json.Marshal(se.StepResults)
	_, err := t.db.Exec(
		`INSERT INTO skill_executions (skill_name, run_id, started_at, duration_ms, success, error_message, triggered_by, workspace, mode, step_results_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		se.SkillName, se.RunID, se.StartedAt, se.DurationMs, boolToInt(se.Success), se.ErrorMessage, se.TriggeredBy, se.Workspace, se.Mode, string(stepJSON),
	)
	return err
}

// GetStats returns aggregated stats for a skill.
func (t *ExecutionTracker) GetStats(skillName string) (total int, success int, avgMs int64, lastRun *time.Time, err error) {
	row := t.db.QueryRow(
		`SELECT COUNT(*), SUM(success), AVG(duration_ms), MAX(started_at) FROM skill_executions WHERE skill_name = ?`,
		skillName,
	)
	var totalInt, successInt sql.NullInt64
	var avg sql.NullFloat64
	var lastStr sql.NullString
	if err := row.Scan(&totalInt, &successInt, &avg, &lastStr); err != nil {
		return 0, 0, 0, nil, err
	}
	if lastStr.Valid {
		formats := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05.999999999Z07:00",
			"2006-01-02T15:04:05.999999999Z07:00",
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
			"2006-01-02 15:04",
			"2006-01-02T15:04",
			"2006-01-02",
		}
		var parsed time.Time
		for _, f := range formats {
			var err error
			parsed, err = time.Parse(f, lastStr.String)
			if err == nil {
				break
			}
		}
		if !parsed.IsZero() {
			return int(totalInt.Int64), int(successInt.Int64), int64(avg.Float64), &parsed, nil
		}
	}
	return int(totalInt.Int64), int(successInt.Int64), int64(avg.Float64), nil, nil
}

// GetTimeline returns recent executions for a skill.
func (t *ExecutionTracker) GetTimeline(skillName string, limit int) ([]SkillExecution, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := t.db.Query(
		`SELECT id, skill_name, run_id, started_at, duration_ms, success, error_message, triggered_by, workspace, mode, step_results_json
		FROM skill_executions WHERE skill_name = ? ORDER BY started_at DESC LIMIT ?`,
		skillName, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []SkillExecution
	for rows.Next() {
		var se SkillExecution
		var stepJSON string
		var started sql.NullTime
		err := rows.Scan(&se.ID, &se.SkillName, &se.RunID, &started, &se.DurationMs, &se.Success, &se.ErrorMessage, &se.TriggeredBy, &se.Workspace, &se.Mode, &stepJSON)
		if err != nil {
			continue
		}
		if started.Valid {
			se.StartedAt = started.Time
		}
		se.Success = se.Success // sqlite bool scan
		_ = json.Unmarshal([]byte(stepJSON), &se.StepResults)
		list = append(list, se)
	}
	return list, nil
}

// GetTopSkills returns the most-used skills by run count.
func (t *ExecutionTracker) GetTopSkills(n int) ([]struct {
	Name       string `json:"name"`
	RunCount   int    `json:"run_count"`
	SuccessRate float64 `json:"success_rate"`
}, error) {
	if n <= 0 {
		n = 10
	}
	rows, err := t.db.Query(
		`SELECT skill_name, COUNT(*) as cnt, AVG(success)*100 as rate
		FROM skill_executions GROUP BY skill_name ORDER BY cnt DESC LIMIT ?`, n,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		Name       string `json:"name"`
		RunCount   int    `json:"run_count"`
		SuccessRate float64 `json:"success_rate"`
	}
	for rows.Next() {
		var item struct {
			Name       string `json:"name"`
			RunCount   int    `json:"run_count"`
			SuccessRate float64 `json:"success_rate"`
		}
		if err := rows.Scan(&item.Name, &item.RunCount, &item.SuccessRate); err == nil {
			out = append(out, item)
		}
	}
	return out, nil
}

// GetSkillsNeedingAttention returns skills with <70% success rate and >=3 runs.
func (t *ExecutionTracker) GetSkillsNeedingAttention() ([]struct {
	Name        string  `json:"name"`
	RunCount    int     `json:"run_count"`
	SuccessRate float64 `json:"success_rate"`
}, error) {
	rows, err := t.db.Query(
		`SELECT skill_name, COUNT(*) as cnt, AVG(success)*100 as rate
		FROM skill_executions GROUP BY skill_name HAVING cnt >= 3 AND rate < 70
		ORDER BY rate ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		Name        string  `json:"name"`
		RunCount    int     `json:"run_count"`
		SuccessRate float64 `json:"success_rate"`
	}
	for rows.Next() {
		var item struct {
			Name        string  `json:"name"`
			RunCount    int     `json:"run_count"`
			SuccessRate float64 `json:"success_rate"`
		}
		if err := rows.Scan(&item.Name, &item.RunCount, &item.SuccessRate); err == nil {
			out = append(out, item)
		}
	}
	return out, nil
}

// RecordImprovement saves a refinement proposal.
func (t *ExecutionTracker) RecordImprovement(si SkillImprovement) error {
	_, err := t.db.Exec(
		`INSERT INTO skill_improvements (skill_name, version, triggered_at, trigger, change_summary, success_rate_before, success_rate_after, applied, proposed_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		si.SkillName, si.Version, si.TriggeredAt, si.Trigger, si.ChangeSummary, si.SuccessRateBefore, si.SuccessRateAfter, boolToInt(si.Applied), si.ProposedJSON,
	)
	return err
}

// GetImprovements returns refinement history for a skill.
func (t *ExecutionTracker) GetImprovements(skillName string, limit int) ([]SkillImprovement, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := t.db.Query(
		`SELECT id, skill_name, version, triggered_at, trigger, change_summary, success_rate_before, success_rate_after, applied, proposed_json
		FROM skill_improvements WHERE skill_name = ? ORDER BY triggered_at DESC LIMIT ?`,
		skillName, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []SkillImprovement
	for rows.Next() {
		var si SkillImprovement
		var triggered sql.NullTime
		err := rows.Scan(&si.ID, &si.SkillName, &si.Version, &triggered, &si.Trigger, &si.ChangeSummary, &si.SuccessRateBefore, &si.SuccessRateAfter, &si.Applied, &si.ProposedJSON)
		if err != nil {
			continue
		}
		if triggered.Valid {
			si.TriggeredAt = triggered.Time
		}
		list = append(list, si)
	}
	return list, nil
}

// GlobalAnalytics returns aggregated data across all skills.
func (t *ExecutionTracker) GlobalAnalytics() (map[string]interface{}, error) {
	var totalRuns int
	_ = t.db.QueryRow(`SELECT COUNT(*) FROM skill_executions`).Scan(&totalRuns)

	var successfulRuns int
	_ = t.db.QueryRow(`SELECT COUNT(*) FROM skill_executions WHERE success = 1`).Scan(&successfulRuns)

	top, _ := t.GetTopSkills(5)
	struggling, _ := t.GetSkillsNeedingAttention()

	return map[string]interface{}{
		"total_runs":      totalRuns,
		"successful_runs": successfulRuns,
		"overall_rate":    float64(0),
		"top_skills":      top,
		"struggling":      struggling,
	}, nil
}

// LogRun records a skill execution from a RunResult and timing.
func (t *ExecutionTracker) LogRun(skillName, runID, triggeredBy, workspace, mode string, result *RunResult, start time.Time) error {
	var stepResults []StepResultRaw
	for i, sr := range result.StepResults {
		preview := sr.Output
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		errMsg := ""
		if sr.Error != nil {
			errMsg = sr.Error.Error()
		}
		stepResults = append(stepResults, StepResultRaw{
			Step:          i + 1,
			Tool:          sr.Tool,
			Success:       sr.Error == nil,
			OutputPreview: preview,
		})
		_ = errMsg
	}
	return t.LogExecution(SkillExecution{
		SkillName:   skillName,
		RunID:       runID,
		StartedAt:   start,
		DurationMs:  time.Since(start).Milliseconds(),
		Success:     result.Success,
		ErrorMessage: func() string { if result.Success { return "" } else { return result.Summary } }(),
		TriggeredBy: triggeredBy,
		Workspace:   workspace,
		Mode:        mode,
		StepResults: stepResults,
	})
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
