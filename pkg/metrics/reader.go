package metrics

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

// ReadMetrics reads the latest metrics snapshot from a JSON file.
func ReadMetrics(path string) (*Metrics, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("gagal baca metrics file: %w", err)
	}

	var m Metrics
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("gagal parse metrics: %w", err)
	}

	return &m, nil
}

// IsMetricsStale checks if the metrics file is older than the given threshold.
func IsMetricsStale(m *Metrics, threshold time.Duration) bool {
	return time.Since(m.UpdatedAt) > threshold
}

// ReadFromDB reads static metrics from SQLite when serve is not running.
func ReadFromDB(dbPath string) (*Metrics, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&mode=ro")
	if err != nil {
		return nil, fmt.Errorf("gagal buka database: %w", err)
	}
	defer db.Close()

	m := &Metrics{
		Platforms: make(map[string]*PlatformMetrics),
	}

	// Count memories
	var memCount int
	err = db.QueryRow("SELECT COUNT(*) FROM memories").Scan(&memCount)
	if err == nil {
		m.Memory.TotalMemories = memCount
	}

	// Count unsynced
	var unsyncedCount int
	err = db.QueryRow("SELECT COUNT(*) FROM memories WHERE id NOT IN (SELECT memory_id FROM sync_log WHERE status='complete')").Scan(&unsyncedCount)
	if err == nil {
		m.Memory.UnsyncedCount = unsyncedCount
	}

	// DB file size
	if info, err := os.Stat(dbPath); err == nil {
		m.Memory.DBSizeBytes = info.Size()
	}

	// Count active sessions
	var sessCount int
	err = db.QueryRow("SELECT COUNT(*) FROM sessions WHERE state='active'").Scan(&sessCount)
	if err == nil {
		m.ActiveSessions = sessCount
	}

	return m, nil
}
