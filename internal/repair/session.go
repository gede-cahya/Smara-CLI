package repair

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/gede-cahya/Smara-CLI/internal/config"
)

// CheckSessionHealth looks for orphaned active sessions and stale lock files.
func CheckSessionHealth() []CheckResult {
	var results []CheckResult

	dbPath := config.Get().DBPath

	// Check orphaned sessions in memory DB
	res := checkOrphanedSessions(dbPath)
	results = append(results, res)

	// Check stale lock files
	res2 := checkStaleLockFiles()
	results = append(results, res2)

	return results
}

func checkOrphanedSessions(dbPath string) CheckResult {
	res := CheckResult{
		Module:  "session",
		Status:  StatusOK,
		Message: "Tidak ada session bermasalah",
		Fixable: true,
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		res.Status = StatusWarn
		res.Message = fmt.Sprintf("Gagal buka DB untuk cek session: %v", err)
		res.Fixable = false
		return res
	}
	defer db.Close()

	// Sessions active but updated > 24 hours ago
	cutoff := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	var count int
	err = db.QueryRow(
		"SELECT COUNT(*) FROM sessions WHERE state = 'active' AND updated_at < ?",
		cutoff,
	).Scan(&count)
	if err != nil {
		res.Status = StatusWarn
		res.Message = fmt.Sprintf("Gagal query session: %v", err)
		res.Fixable = false
		return res
	}

	if count > 0 {
		res.Status = StatusWarn
		res.Message = fmt.Sprintf("%d session aktif yang terlalu lama (>24h)", count)
		res.Suggestion = "Mark sebagai ended"
	}

	return res
}

func checkStaleLockFiles() CheckResult {
	res := CheckResult{
		Module:  "session",
		Status:  StatusOK,
		Message: "Tidak ada stale lock file",
		Fixable: true,
	}

	smaraDir := config.SmaraDir()
	lockPattern := filepath.Join(smaraDir, "*.lock")

	// Simple glob check
	matches, err := filepath.Glob(lockPattern)
	if err != nil {
		res.Status = StatusWarn
		res.Message = fmt.Sprintf("Gagal scan lock file: %v", err)
		res.Fixable = false
		return res
	}

	staleCount := 0
	for _, f := range matches {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		// Lock older than 1 hour = stale
		if time.Since(info.ModTime()) > time.Hour {
			staleCount++
		}
	}

	if staleCount > 0 {
		res.Status = StatusWarn
		res.Message = fmt.Sprintf("%d stale lock file ditemukan", staleCount)
		res.Suggestion = "Hapus lock file lama"
	}

	return res
}

// RepairSessions marks orphaned active sessions as ended and removes stale locks.
func RepairSessions(dbPath string) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	cutoff := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	_, err = db.Exec(
		"UPDATE sessions SET state = 'ended', updated_at = ? WHERE state = 'active' AND updated_at < ?",
		time.Now().UTC().Format(time.RFC3339),
		cutoff,
	)
	if err != nil {
		return fmt.Errorf("gagal update orphaned sessions: %w", err)
	}

	// Remove stale lock files
	smaraDir := config.SmaraDir()
	matches, _ := filepath.Glob(filepath.Join(smaraDir, "*.lock"))
	for _, f := range matches {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		if time.Since(info.ModTime()) > time.Hour {
			_ = os.Remove(f)
		}
	}

	return nil
}

// CountSessionsByState returns session counts grouped by state.
func CountSessionsByState(dbPath string) (map[string]int, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query("SELECT state, COUNT(*) FROM sessions GROUP BY state")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var state string
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			continue
		}
		counts[state] = count
	}
	return counts, rows.Err()
}
