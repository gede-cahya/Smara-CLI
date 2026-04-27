// Package audit provides structured logging of all agent actions for observability.
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// EntryType represents the type of audit event.
type EntryType string

const (
	EntryPrompt      EntryType = "prompt"
	EntryToolCall    EntryType = "tool_call"
	EntryToolResult  EntryType = "tool_result"
	EntryFileRead    EntryType = "file_read"
	EntryFileWrite   EntryType = "file_write"
	EntryFileDelete  EntryType = "file_delete"
	EntryModeChange  EntryType = "mode_change"
	EntrySession     EntryType = "session"
	EntryMemory      EntryType = "memory"
	EntryError       EntryType = "error"
	EntryDecision    EntryType = "decision"
	EntrySafetyCheck EntryType = "safety_check"
)

// Entry represents a single audit log entry.
type Entry struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Type      EntryType              `json:"type"`
	SessionID string                 `json:"session_id,omitempty"`
	Workspace string                 `json:"workspace,omitempty"`
	Agent     string                 `json:"agent,omitempty"`
	Action    string                 `json:"action"`
	Target    string                 `json:"target,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Success   bool                   `json:"success"`
	Duration  int64                  `json:"duration_ms,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// Logger handles audit log recording.
type Logger struct {
	logPath   string
	mu        sync.Mutex
	entries   []Entry
	maxBuffer int
}

// NewLogger creates a new audit logger.
func NewLogger(logDir string) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("gagal membuat direktori audit: %w", err)
	}

	logPath := filepath.Join(logDir, fmt.Sprintf("audit_%s.jsonl", time.Now().Format("2006-01-02")))

	return &Logger{
		logPath:   logPath,
		entries:   make([]Entry, 0),
		maxBuffer: 100,
	}, nil
}

// Log records an audit entry.
func (l *Logger) Log(entry Entry) error {
	entry.Timestamp = time.Now()
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("audit_%d", time.Now().UnixNano())
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.entries = append(l.entries, entry)

	// Flush to disk if buffer is full
	if len(l.entries) >= l.maxBuffer {
		return l.flush()
	}
	return nil
}

// LogPrompt logs a user prompt.
func (l *Logger) LogPrompt(sessionID, workspace, prompt string) {
	l.Log(Entry{
		Type:      EntryPrompt,
		SessionID: sessionID,
		Workspace: workspace,
		Action:    "user_prompt",
		Target:    truncate(prompt, 200),
		Success:   true,
	})
}

// LogToolCall logs a tool execution.
func (l *Logger) LogToolCall(sessionID, workspace, tool string, args map[string]interface{}, success bool, duration time.Duration) {
	l.Log(Entry{
		Type:      EntryToolCall,
		SessionID: sessionID,
		Workspace: workspace,
		Action:    tool,
		Details:   args,
		Success:   success,
		Duration:  duration.Milliseconds(),
	})
}

// LogFileWrite logs a file modification.
func (l *Logger) LogFileWrite(sessionID, workspace, filePath string, success bool) {
	l.Log(Entry{
		Type:      EntryFileWrite,
		SessionID: sessionID,
		Workspace: workspace,
		Action:    "file_write",
		Target:    filePath,
		Success:   success,
	})
}

// LogFileDelete logs a file deletion.
func (l *Logger) LogFileDelete(sessionID, workspace, filePath string, success bool) {
	l.Log(Entry{
		Type:      EntryFileDelete,
		SessionID: sessionID,
		Workspace: workspace,
		Action:    "file_delete",
		Target:    filePath,
		Success:   success,
	})
}

// LogModeChange logs a mode switch.
func (l *Logger) LogModeChange(sessionID, workspace, from, to string) {
	l.Log(Entry{
		Type:      EntryModeChange,
		SessionID: sessionID,
		Workspace: workspace,
		Action:    "mode_change",
		Details: map[string]interface{}{
			"from": from,
			"to":   to,
		},
		Success: true,
	})
}

// LogError logs an error event.
func (l *Logger) LogError(sessionID, workspace, action, errMsg string) {
	l.Log(Entry{
		Type:      EntryError,
		SessionID: sessionID,
		Workspace: workspace,
		Action:    action,
		Success:   false,
		Error:     errMsg,
	})
}

// LogDecision logs an agentic decision.
func (l *Logger) LogDecision(sessionID, workspace, decision string, context map[string]interface{}) {
	l.Log(Entry{
		Type:      EntryDecision,
		SessionID: sessionID,
		Workspace: workspace,
		Action:    decision,
		Details:   context,
		Success:   true,
	})
}

// LogSafetyCheck logs a safety engine check.
func (l *Logger) LogSafetyCheck(sessionID, workspace, tool, reason string, allowed bool) {
	l.Log(Entry{
		Type:      EntrySafetyCheck,
		SessionID: sessionID,
		Workspace: workspace,
		Action:    "safety_check",
		Target:    tool,
		Details: map[string]interface{}{
			"allowed": allowed,
			"reason":  reason,
		},
		Success: allowed,
	})
}

// flush writes buffered entries to disk.
func (l *Logger) flush() error {
	f, err := os.OpenFile(l.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	for _, entry := range l.entries {
		if err := encoder.Encode(entry); err != nil {
			return err
		}
	}

	l.entries = make([]Entry, 0)
	return nil
}

// Flush forces writing all buffered entries to disk.
func (l *Logger) Flush() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.flush()
}

// Close flushes remaining entries and closes the logger.
func (l *Logger) Close() error {
	return l.Flush()
}

// GetLogPath returns the current log file path.
func (l *Logger) GetLogPath() string {
	return l.logPath
}

// ReadEntries reads all audit entries from the log file.
func (l *Logger) ReadEntries() ([]Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Flush first to ensure all entries are on disk
	if err := l.flush(); err != nil {
		return nil, err
	}

	f, err := os.Open(l.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	decoder := json.NewDecoder(f)
	for {
		var entry Entry
		if err := decoder.Decode(&entry); err != nil {
			if err.Error() == "EOF" {
				break
			}
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// FilterEntries filters audit entries by type and time range.
func FilterEntries(entries []Entry, entryType EntryType, from, to time.Time) []Entry {
	var result []Entry
	for _, e := range entries {
		if entryType != "" && e.Type != entryType {
			continue
		}
		if !from.IsZero() && e.Timestamp.Before(from) {
			continue
		}
		if !to.IsZero() && e.Timestamp.After(to) {
			continue
		}
		result = append(result, e)
	}
	return result
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
