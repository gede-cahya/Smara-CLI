package web

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const processLogFileName = "web-process.log"

var processLogMu sync.Mutex

type processLogEvent struct {
	Timestamp time.Time              `json:"timestamp"`
	SessionID string                 `json:"session_id,omitempty"`
	RunID     string                 `json:"run_id,omitempty"`
	Level     string                 `json:"level"`
	Event     string                 `json:"event"`
	Message   string                 `json:"message"`
	Mode      string                 `json:"mode,omitempty"`
	Tool      string                 `json:"tool,omitempty"`
	ElapsedMs int64                  `json:"elapsed_ms,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

func (s *Server) processLogPath() string {
	if s != nil && s.Cfg != nil && strings.TrimSpace(s.Cfg.DBPath) != "" {
		return filepath.Join(filepath.Dir(s.Cfg.DBPath), processLogFileName)
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join(".", processLogFileName)
	}
	return filepath.Join(home, ".smara", processLogFileName)
}

func (s *Server) appendProcessLog(event processLogEvent) string {
	path := s.processLogPath()
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.Level == "" {
		event.Level = "info"
	}
	processLogMu.Lock()
	defer processLogMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return path
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return path
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	_ = enc.Encode(event)
	return path
}

func formatProcessLogLine(event processLogEvent, path string) string {
	level := strings.ToUpper(strings.TrimSpace(event.Level))
	if level == "" {
		level = "INFO"
	}
	prefix := event.Timestamp.Format("15:04:05")
	elapsed := ""
	if event.ElapsedMs > 0 {
		elapsed = fmt.Sprintf(" +%s", time.Duration(event.ElapsedMs)*time.Millisecond)
	}
	msg := fmt.Sprintf("[%s%s] %s %s: %s", prefix, elapsed, level, event.Event, event.Message)
	if event.Tool != "" {
		msg += " tool=" + event.Tool
	}
	if event.RunID != "" {
		msg += " run=" + event.RunID
	}
	if path != "" && event.Event == "run_start" {
		msg += " log=" + path
	}
	return msg
}

func redactProcessDetails(details map[string]interface{}) map[string]interface{} {
	if len(details) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(details))
	for k, v := range details {
		lower := strings.ToLower(k)
		if strings.Contains(lower, "token") ||
			strings.Contains(lower, "secret") ||
			strings.Contains(lower, "password") ||
			strings.Contains(lower, "api_key") ||
			strings.Contains(lower, "apikey") {
			out[k] = "[redacted]"
			continue
		}
		out[k] = compactProcessValue(v)
	}
	return out
}

func compactProcessValue(v interface{}) interface{} {
	switch x := v.(type) {
	case string:
		if len(x) > 500 {
			return x[:500] + "...[truncated]"
		}
		return x
	case map[string]interface{}:
		return redactProcessDetails(x)
	default:
		return x
	}
}
