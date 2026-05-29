package agent

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const selfImprovementSource = "agent:self-improvement"

// SelfImprovementMemory captures lessons Smara should auto-apply in later turns.
type SelfImprovementMemory struct {
	Type       string   `json:"type"`
	Scope      string   `json:"scope"`
	Summary    string   `json:"summary"`
	Lesson     string   `json:"lesson"`
	AppliesTo  []string `json:"applies_to,omitempty"`
	Confidence float64  `json:"confidence"`
	AutoApply  bool     `json:"auto_apply"`
}

// SaveSelfImprovementMemory stores or merges a self-improvement lesson in the
// existing memories table. It is intentionally best-effort for agent flows: call
// sites may ignore errors when learning must not block the primary task.
func SaveSelfImprovementMemory(db *sql.DB, lesson SelfImprovementMemory) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("database belum tersedia")
	}
	lesson.Type = strings.TrimSpace(lesson.Type)
	lesson.Scope = strings.TrimSpace(lesson.Scope)
	lesson.Summary = redactSensitive(strings.TrimSpace(lesson.Summary))
	lesson.Lesson = redactSensitive(strings.TrimSpace(lesson.Lesson))
	if lesson.Type == "" {
		lesson.Type = "lesson-learned"
	}
	if lesson.Scope == "" {
		lesson.Scope = "agent"
	}
	if lesson.Summary == "" {
		lesson.Summary = lesson.Lesson
	}
	if lesson.Lesson == "" {
		lesson.Lesson = lesson.Summary
	}
	if lesson.Confidence <= 0 || lesson.Confidence > 1 {
		lesson.Confidence = 0.9
	}
	lesson.AutoApply = true

	fingerprint := selfImprovementFingerprint(lesson.Type, lesson.Scope, lesson.Summary)
	metadata := map[string]interface{}{
		"kind":        "self-improvement",
		"type":        lesson.Type,
		"scope":       lesson.Scope,
		"applies_to":  lesson.AppliesTo,
		"confidence":  lesson.Confidence,
		"auto_apply":  lesson.AutoApply,
		"fingerprint": fingerprint,
	}
	metadataJSON, _ := json.Marshal(metadata)
	tagsJSON, _ := json.Marshal([]string{"self-improvement", lesson.Type, "auto-apply"})
	content := fmt.Sprintf("[%s] %s\nLesson: %s\nScope: %s", lesson.Type, lesson.Summary, lesson.Lesson, lesson.Scope)

	var existingID int64
	var oldVersion int
	err := db.QueryRow(`SELECT id, version FROM memories WHERE source = ? AND metadata LIKE ? ORDER BY updated_at DESC LIMIT 1`, selfImprovementSource, "%"+fingerprint+"%").Scan(&existingID, &oldVersion)
	now := time.Now()
	if err == nil && existingID > 0 {
		_, _ = db.Exec(`INSERT INTO memory_versions (memory_id, content, metadata, changed_by, reason, created_at) SELECT id, content, metadata, ?, ?, ? FROM memories WHERE id = ?`, selfImprovementSource, "self-improvement dedup/merge", now, existingID)
		_, err = db.Exec(`UPDATE memories SET content = ?, tags = ?, metadata = ?, updated_at = ?, version = ? WHERE id = ?`, content, string(tagsJSON), string(metadataJSON), now, oldVersion+1, existingID)
		return existingID, err
	}
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	res, err := db.Exec(`INSERT INTO memories (content, tags, source, metadata, created_at, updated_at, version) VALUES (?, ?, ?, ?, ?, ?, 1)`, content, string(tagsJSON), selfImprovementSource, string(metadataJSON), now, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func selfImprovementFingerprint(parts ...string) string {
	norm := strings.ToLower(strings.Join(parts, "|"))
	norm = strings.Join(strings.Fields(norm), " ")
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])[:16]
}

var sensitiveValueRe = regexp.MustCompile(`(?i)(api[_-]?key|token|password|passwd|secret|authorization|bearer)\s*[:=]\s*[^\s,;]+`)

func redactSensitive(s string) string {
	return sensitiveValueRe.ReplaceAllString(s, "$1=[REDACTED]")
}
