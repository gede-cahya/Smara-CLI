package nudge

import (
	"database/sql"
	"fmt"
	"time"
)

// CreateSchedule inserts a new reminder schedule.
func CreateSchedule(db *sql.DB, promptText, cronExpr string, nextRun *time.Time) (*Schedule, error) {
	result, err := db.Exec(
		`INSERT INTO nudge_schedules (prompt_text, cron_expr, next_run, enabled, created_at)
		VALUES (?, ?, ?, 1, ?)`,
		promptText, cronExpr, nextRun, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to create schedule: %w", err)
	}
	id, _ := result.LastInsertId()
	return &Schedule{ID: id, PromptText: promptText, CronExpr: cronExpr, NextRun: nextRun, Enabled: true, CreatedAt: time.Now()}, nil
}

// GetPendingSchedules returns schedules whose next_run is due.
func GetPendingSchedules(db *sql.DB) ([]Schedule, error) {
	rows, err := db.Query(
		`SELECT id, prompt_text, cron_expr, next_run, enabled, created_at FROM nudge_schedules
		WHERE enabled = 1 AND (next_run IS NULL OR next_run <= ?) ORDER BY next_run ASC`,
		time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Schedule
	for rows.Next() {
		var s Schedule
		var next sql.NullTime
		if err := rows.Scan(&s.ID, &s.PromptText, &s.CronExpr, &next, &s.Enabled, &s.CreatedAt); err != nil {
			continue
		}
		if next.Valid {
			s.NextRun = &next.Time
		}
		list = append(list, s)
	}
	return list, nil
}

// UpdateScheduleNextRun sets the next_run time (e.g., after completing a nudge).
func UpdateScheduleNextRun(db *sql.DB, id int64, nextRun *time.Time) error {
	_, err := db.Exec(`UPDATE nudge_schedules SET next_run = ? WHERE id = ?`, nextRun, id)
	return err
}

// DisableSchedule marks a schedule as disabled.
func DisableSchedule(db *sql.DB, id int64) error {
	_, err := db.Exec(`UPDATE nudge_schedules SET enabled = 0 WHERE id = ?`, id)
	return err
}

// CreateTask inserts an unfinished task.
func CreateTask(db *sql.DB, sessionID, promptText, lastState string) (*Task, error) {
	result, err := db.Exec(
		`INSERT INTO nudge_tasks (session_id, prompt_text, last_state, dismissed, created_at)
		VALUES (?, ?, ?, 0, ?)`,
		sessionID, promptText, lastState, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}
	id, _ := result.LastInsertId()
	return &Task{ID: id, SessionID: sessionID, PromptText: promptText, LastState: lastState, CreatedAt: time.Now()}, nil
}

// GetPendingTasks returns non-dismissed tasks.
func GetPendingTasks(db *sql.DB) ([]Task, error) {
	rows, err := db.Query(
		`SELECT id, session_id, prompt_text, last_state, dismissed, created_at FROM nudge_tasks
		WHERE dismissed = 0 ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.SessionID, &t.PromptText, &t.LastState, &t.Dismissed, &t.CreatedAt); err != nil {
			continue
		}
		list = append(list, t)
	}
	return list, nil
}

// DismissTask marks a task as dismissed.
func DismissTask(db *sql.DB, id int64) error {
	_, err := db.Exec(`UPDATE nudge_tasks SET dismissed = 1 WHERE id = ?`, id)
	return err
}

// GetAllPending returns both schedules and tasks as unified Nudges.
func GetAllPending(db *sql.DB) ([]Nudge, error) {
	var nudges []Nudge
	schedules, err := GetPendingSchedules(db)
	if err == nil {
		for _, s := range schedules {
			nudges = append(nudges, Nudge{
				ID:      s.ID,
				Type:    "schedule",
				Text:    s.PromptText,
				Created: s.CreatedAt,
			})
		}
	}
	tasks, err := GetPendingTasks(db)
	if err == nil {
		for _, t := range tasks {
			nudges = append(nudges, Nudge{
				ID:      t.ID,
				Type:    "task",
				Text:    t.PromptText,
				Created: t.CreatedAt,
			})
		}
	}
	return nudges, nil
}
