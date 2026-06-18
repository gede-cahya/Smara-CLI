package skill

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// SkillExecution tracks one run of a skill.
type SkillExecution struct {
	ID              int64           `json:"id"`
	SkillName       string          `json:"skill_name"`
	RunID           string          `json:"run_id"`
	StartedAt       time.Time       `json:"started_at"`
	DurationMs      int64           `json:"duration_ms"`
	Success         bool            `json:"success"`
	Status          string          `json:"status"`
	ErrorMessage    string          `json:"error_message,omitempty"`
	FailedStep      int             `json:"failed_step,omitempty"`
	ApprovalGranted bool            `json:"approval_granted"`
	VersionID       string          `json:"version_id,omitempty"`
	TriggeredBy     string          `json:"triggered_by"`
	Workspace       string          `json:"workspace,omitempty"`
	Mode            string          `json:"mode,omitempty"`
	StepResults     []StepResultRaw `json:"step_results,omitempty"`
}

type StepResultRaw struct {
	Step          int    `json:"step"`
	Tool          string `json:"tool"`
	Success       bool   `json:"success"`
	Error         string `json:"error,omitempty"`
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
type ExecutionTracker struct{ db *sql.DB }

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
			status TEXT DEFAULT 'success',
			error_message TEXT,
			failed_step INTEGER DEFAULT 0,
			approval_granted INTEGER DEFAULT 0,
			version_id TEXT,
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
	// Best-effort migrations for DBs created by older Phase-6 attempts.
	for _, s := range []string{
		`ALTER TABLE skill_executions ADD COLUMN status TEXT DEFAULT 'success'`,
		`ALTER TABLE skill_executions ADD COLUMN failed_step INTEGER DEFAULT 0`,
		`ALTER TABLE skill_executions ADD COLUMN approval_granted INTEGER DEFAULT 0`,
		`ALTER TABLE skill_executions ADD COLUMN version_id TEXT`,
	} {
		_, _ = db.Exec(s)
	}
	return nil
}

// LogExecution stores a skill run into the database.
func (t *ExecutionTracker) LogExecution(se SkillExecution) error {
	if se.Status == "" {
		if se.Success {
			se.Status = "success"
		} else {
			se.Status = "failed"
		}
	}
	stepJSON, _ := json.Marshal(se.StepResults)
	_, err := t.db.Exec(
		`INSERT INTO skill_executions (skill_name, run_id, started_at, duration_ms, success, status, error_message, failed_step, approval_granted, version_id, triggered_by, workspace, mode, step_results_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		se.SkillName, se.RunID, se.StartedAt, se.DurationMs, boolToInt(se.Success), se.Status, se.ErrorMessage, se.FailedStep, boolToInt(se.ApprovalGranted), se.VersionID, se.TriggeredBy, se.Workspace, se.Mode, string(stepJSON),
	)
	return err
}

// GetStats returns aggregated stats for a skill.
func (t *ExecutionTracker) GetStats(skillName string) (total int, success int, avgMs int64, lastRun *time.Time, err error) {
	row := t.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(success),0), COALESCE(AVG(duration_ms),0), MAX(started_at) FROM skill_executions WHERE skill_name = ?`, skillName)
	var lastStr sql.NullString
	if err := row.Scan(&total, &success, &avgMs, &lastStr); err != nil {
		return 0, 0, 0, nil, err
	}
	if lastStr.Valid {
		// SQLite stores time.Time as RFC3339Nano on direct INSERT, but aggregate
		// functions like MAX() can surface Go's Time.String() layout instead
		// ("2006-01-02 15:04:05.999999999 -0700 MST" plus an "m=+..." monotonic
		// suffix). Strip the monotonic part and try both families of layouts.
		raw := lastStr.String
		if i := strings.Index(raw, " m=+"); i != -1 {
			raw = raw[:i]
		} else if i := strings.Index(raw, " m=-"); i != -1 {
			raw = raw[:i]
		}
		raw = strings.TrimSpace(raw)
		for _, f := range []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05.999999999 -0700 MST",
			"2006-01-02 15:04:05 -0700 MST",
			"2006-01-02 15:04:05.999999999Z07:00",
			"2006-01-02 15:04:05",
		} {
			if parsed, e := time.Parse(f, raw); e == nil {
				lastRun = &parsed
				break
			}
		}
	}
	return
}

// GetTimeline returns recent executions for a skill, or all skills if skillName is empty.
func (t *ExecutionTracker) GetTimeline(skillName string, limit int) ([]SkillExecution, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `SELECT id, skill_name, run_id, started_at, duration_ms, success, status, error_message, failed_step, approval_granted, version_id, triggered_by, workspace, mode, step_results_json FROM skill_executions`
	var rows *sql.Rows
	var err error
	if skillName != "" {
		rows, err = t.db.Query(query+` WHERE skill_name = ? ORDER BY started_at DESC LIMIT ?`, skillName, limit)
	} else {
		rows, err = t.db.Query(query+` ORDER BY started_at DESC LIMIT ?`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []SkillExecution
	for rows.Next() {
		var se SkillExecution
		var stepJSON string
		if err := rows.Scan(&se.ID, &se.SkillName, &se.RunID, &se.StartedAt, &se.DurationMs, &se.Success, &se.Status, &se.ErrorMessage, &se.FailedStep, &se.ApprovalGranted, &se.VersionID, &se.TriggeredBy, &se.Workspace, &se.Mode, &stepJSON); err != nil {
			continue
		}
		_ = json.Unmarshal([]byte(stepJSON), &se.StepResults)
		list = append(list, se)
	}
	return list, nil
}

// GetTopSkills returns the most-used skills by run count.
func (t *ExecutionTracker) GetTopSkills(n int) ([]struct {
	Name        string  `json:"name"`
	RunCount    int     `json:"run_count"`
	SuccessRate float64 `json:"success_rate"`
}, error) {
	if n <= 0 {
		n = 10
	}
	rows, err := t.db.Query(`SELECT skill_name, COUNT(*) as cnt, AVG(success)*100 as rate FROM skill_executions GROUP BY skill_name ORDER BY cnt DESC LIMIT ?`, n)
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

func (t *ExecutionTracker) GetSkillsNeedingAttention() ([]struct {
	Name        string  `json:"name"`
	RunCount    int     `json:"run_count"`
	SuccessRate float64 `json:"success_rate"`
}, error) {
	rows, err := t.db.Query(`SELECT skill_name, COUNT(*) as cnt, AVG(success)*100 as rate FROM skill_executions GROUP BY skill_name HAVING cnt >= 3 AND rate < 70 ORDER BY rate ASC`)
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

func (t *ExecutionTracker) RecordImprovement(si SkillImprovement) error {
	_, err := t.db.Exec(`INSERT INTO skill_improvements (skill_name, version, triggered_at, trigger, change_summary, success_rate_before, success_rate_after, applied, proposed_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, si.SkillName, si.Version, si.TriggeredAt, si.Trigger, si.ChangeSummary, si.SuccessRateBefore, si.SuccessRateAfter, boolToInt(si.Applied), si.ProposedJSON)
	return err
}

func (t *ExecutionTracker) GetImprovements(skillName string, limit int) ([]SkillImprovement, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := t.db.Query(`SELECT id, skill_name, version, triggered_at, trigger, change_summary, success_rate_before, success_rate_after, applied, proposed_json
		FROM skill_improvements WHERE skill_name = ? ORDER BY triggered_at DESC LIMIT ?`, skillName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var improvements []SkillImprovement
	for rows.Next() {
		var item SkillImprovement
		var applied int
		if err := rows.Scan(
			&item.ID,
			&item.SkillName,
			&item.Version,
			&item.TriggeredAt,
			&item.Trigger,
			&item.ChangeSummary,
			&item.SuccessRateBefore,
			&item.SuccessRateAfter,
			&applied,
			&item.ProposedJSON,
		); err != nil {
			return nil, err
		}
		item.Applied = applied != 0
		improvements = append(improvements, item)
	}
	return improvements, rows.Err()
}

func (t *ExecutionTracker) GlobalAnalytics() (map[string]interface{}, error) {
	var totalRuns, successfulRuns, failedRuns, last24hRuns int
	var avgDuration float64
	_ = t.db.QueryRow(`SELECT COUNT(*) FROM skill_executions`).Scan(&totalRuns)
	_ = t.db.QueryRow(`SELECT COUNT(*) FROM skill_executions WHERE success = 1`).Scan(&successfulRuns)
	_ = t.db.QueryRow(`SELECT COUNT(*) FROM skill_executions WHERE success = 0`).Scan(&failedRuns)
	_ = t.db.QueryRow(`SELECT COALESCE(AVG(duration_ms),0) FROM skill_executions`).Scan(&avgDuration)
	_ = t.db.QueryRow(`SELECT COUNT(*) FROM skill_executions WHERE started_at >= datetime('now', '-1 day')`).Scan(&last24hRuns)
	top, _ := t.GetTopSkills(5)
	struggling, _ := t.GetSkillsNeedingAttention()
	recent, _ := t.GetTimeline("", 50)
	recentFailures := make([]SkillExecution, 0, 5)
	for _, item := range recent {
		if !item.Success {
			recentFailures = append(recentFailures, item)
			if len(recentFailures) >= 5 {
				break
			}
		}
	}
	triggeredBy := map[string]int{}
	rows, err := t.db.Query(`SELECT COALESCE(NULLIF(triggered_by,''),'unknown'), COUNT(*) FROM skill_executions GROUP BY COALESCE(NULLIF(triggered_by,''),'unknown')`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			var count int
			if rows.Scan(&name, &count) == nil {
				triggeredBy[name] = count
			}
		}
	}
	rate := 0.0
	if totalRuns > 0 {
		rate = float64(successfulRuns) / float64(totalRuns) * 100
	}
	return map[string]interface{}{
		"total_runs":          totalRuns,
		"successful_runs":     successfulRuns,
		"failed_runs":         failedRuns,
		"overall_rate":        rate,
		"avg_duration_ms":     avgDuration,
		"last_24h_runs":       last24hRuns,
		"triggered_by":        triggeredBy,
		"top_skills":          top,
		"struggling":          struggling,
		"recent_failures":     recentFailures,
	}, nil
}

// LogRun records a skill execution from a RunResult and timing.
func (t *ExecutionTracker) LogRun(skillName, runID, triggeredBy, workspace, mode string, result *RunResult, start time.Time) error {
	return t.LogRunWithMetadata(skillName, "", false, runID, triggeredBy, workspace, mode, result, start)
}

func (t *ExecutionTracker) LogRunWithMetadata(skillName, versionID string, approvalGranted bool, runID, triggeredBy, workspace, mode string, result *RunResult, start time.Time) error {
	var stepResults []StepResultRaw
	failedStep := 0
	errMsg := ""
	for i, sr := range result.StepResults {
		preview := sr.Output
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		stepErr := ""
		if sr.Error != nil {
			stepErr = sr.Error.Error()
			if failedStep == 0 {
				failedStep = i + 1
				errMsg = stepErr
			}
		}
		stepResults = append(stepResults, StepResultRaw{Step: i + 1, Tool: sr.Tool, Success: sr.Error == nil, Error: stepErr, OutputPreview: preview})
	}
	if errMsg == "" && !result.Success {
		errMsg = result.Summary
	}
	status := "success"
	if !result.Success {
		status = "failed"
	}
	return t.LogExecution(SkillExecution{SkillName: skillName, RunID: runID, StartedAt: start, DurationMs: time.Since(start).Milliseconds(), Success: result.Success, Status: status, ErrorMessage: errMsg, FailedStep: failedStep, ApprovalGranted: approvalGranted, VersionID: versionID, TriggeredBy: triggeredBy, Workspace: workspace, Mode: mode, StepResults: stepResults})
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
