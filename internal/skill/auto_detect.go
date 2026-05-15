package skill

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ExecutionTrace represents one completed user prompt that caused a sequence
// of tool calls. It is the atomic unit of pattern detection.
type ExecutionTrace struct {
	PromptText  string
	Steps       []TraceStep
	CompletedAt time.Time
}

// TraceStep is a single tool call inside a trace.
type TraceStep struct {
	Tool string                 `json:"tool"`
	Args map[string]interface{} `json:"args"`
}

// Fingerprint returns a stable hash representing the shape of a trace.
// It ignores concrete string/number values but keeps the tool names and
// argument key sets, so two invocations of the same skill with different
// parameters still collide.
func (t *ExecutionTrace) Fingerprint() string {
	h := sha256.New()
	for _, s := range t.Steps {
		h.Write([]byte(s.Tool))
		h.Write([]byte{'|'})

		keys := make([]string, 0, len(s.Args))
		for k := range s.Args {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			h.Write([]byte(k))
			h.Write([]byte{':'})
		}
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// EnsureAutoDetectTable creates the storage table for pattern counts.
func EnsureAutoDetectTable(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS auto_skill_patterns (
			fingerprint TEXT PRIMARY KEY,
			count INTEGER NOT NULL DEFAULT 1,
			first_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
			trace_json TEXT NOT NULL,
			sample_prompt TEXT,
			captured_skill TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_auto_skill_last_seen ON auto_skill_patterns(last_seen);
	`)
	return err
}

// PatternRecord is a row from auto_skill_patterns.
type PatternRecord struct {
	Fingerprint   string
	Count         int
	FirstSeen     time.Time
	LastSeen      time.Time
	Trace         ExecutionTrace
	SamplePrompt  string
	CapturedSkill string
}

// RecordTrace upserts a trace observation and returns the updated record.
// The second return is true if this observation pushed the count up to
// exactly the threshold (crossing event) so the caller can trigger skill
// capture without firing on every subsequent observation.
func RecordTrace(db *sql.DB, t ExecutionTrace, threshold int) (*PatternRecord, bool, error) {
	if db == nil {
		return nil, false, fmt.Errorf("db is nil")
	}
	if len(t.Steps) < 2 {
		// Single-step actions are usually not worth capturing as a skill.
		return nil, false, nil
	}
	if err := EnsureAutoDetectTable(db); err != nil {
		return nil, false, err
	}

	fp := t.Fingerprint()
	traceJSON, err := json.Marshal(t.Steps)
	if err != nil {
		return nil, false, err
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()

	var prevCount int
	var captured string
	err = tx.QueryRow(`SELECT count, COALESCE(captured_skill, '') FROM auto_skill_patterns WHERE fingerprint = ?`, fp).
		Scan(&prevCount, &captured)
	if err != nil && err != sql.ErrNoRows {
		return nil, false, err
	}

	if err == sql.ErrNoRows {
		if _, err := tx.Exec(`
			INSERT INTO auto_skill_patterns (fingerprint, count, trace_json, sample_prompt)
			VALUES (?, 1, ?, ?)
		`, fp, string(traceJSON), truncateStr(t.PromptText, 500)); err != nil {
			return nil, false, err
		}
		if err := tx.Commit(); err != nil {
			return nil, false, err
		}
		return &PatternRecord{
			Fingerprint:  fp,
			Count:        1,
			FirstSeen:    time.Now(),
			LastSeen:     time.Now(),
			Trace:        t,
			SamplePrompt: t.PromptText,
		}, false, nil
	}

	newCount := prevCount + 1
	if _, err := tx.Exec(`
		UPDATE auto_skill_patterns
		SET count = ?, last_seen = CURRENT_TIMESTAMP
		WHERE fingerprint = ?
	`, newCount, fp); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}

	// Only cross the threshold once. After capture, captured_skill is set
	// and subsequent matches will not re-trigger.
	crossed := captured == "" && prevCount < threshold && newCount >= threshold

	return &PatternRecord{
		Fingerprint:   fp,
		Count:         newCount,
		Trace:         t,
		SamplePrompt:  t.PromptText,
		CapturedSkill: captured,
	}, crossed, nil
}

// MarkPatternCaptured records that a skill was created from this fingerprint
// so we do not re-suggest the same pattern again.
func MarkPatternCaptured(db *sql.DB, fingerprint, skillName string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(`UPDATE auto_skill_patterns SET captured_skill = ? WHERE fingerprint = ?`,
		skillName, fingerprint)
	return err
}

// SuggestSkillName generates a reasonable default name from the trace steps.
// Combines the unique tool names and a short hash suffix for uniqueness.
func SuggestSkillName(t ExecutionTrace) string {
	seen := map[string]bool{}
	var parts []string
	for _, s := range t.Steps {
		if !seen[s.Tool] {
			seen[s.Tool] = true
			parts = append(parts, strings.ReplaceAll(s.Tool, "_", "-"))
			if len(parts) >= 2 {
				break
			}
		}
	}
	suffix := t.Fingerprint()[:6]
	base := strings.Join(parts, "-")
	if base == "" {
		base = "auto-skill"
	}
	return "auto-" + base + "-" + suffix
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
