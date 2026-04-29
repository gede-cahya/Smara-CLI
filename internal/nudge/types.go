package nudge

import "time"

// Schedule is a recurring reminder set by the user.
type Schedule struct {
	ID         int64
	PromptText string
	CronExpr   string
	NextRun    *time.Time
	Enabled    bool
	CreatedAt  time.Time
}

// Task is an unfinished item detected by the agent.
type Task struct {
	ID         int64
	SessionID  string
	PromptText string
	LastState  string
	Dismissed  bool
	CreatedAt  time.Time
}

// Nudge is a unified pending item for display.
type Nudge struct {
	ID       int64
	Type     string // "schedule" or "task"
	Text     string
	Created  time.Time
	ActionID int64
}
