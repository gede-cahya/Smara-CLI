package repair

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

// requiredTables are tables that must exist for a healthy memory DB.
var requiredTables = []string{
	"workspaces", "memories", "sync_log", "sessions", "categories",
	"memory_versions", "ssh_hosts", "ssh_logs", "user_profile",
}

// CheckDBHealth runs SQLite diagnostics.
func CheckDBHealth(dbPath string) CheckResult {
	res := CheckResult{
		Module: "db",
		Status: StatusOK,
	}

	// 1. File existence and size
	info, err := os.Stat(dbPath)
	if err != nil {
		if os.IsNotExist(err) {
			res.Status = StatusFail
			res.Message = fmt.Sprintf("DB file tidak ditemukan: %s", dbPath)
			res.Fixable = true
			res.Suggestion = "Inisialisasi ulang database"
			return res
		}
		res.Status = StatusFail
		res.Message = fmt.Sprintf("Gagal membaca DB file: %v", err)
		res.Fixable = false
		return res
	}

	if info.Size() == 0 {
		res.Status = StatusFail
		res.Message = fmt.Sprintf("DB file 0 byte (corrupt): %s", dbPath)
		res.Fixable = true
		res.Suggestion = "Backup & recreate database"
		return res
	}

	// 2. Open and integrity check
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		res.Status = StatusFail
		res.Message = fmt.Sprintf("Gagal membuka DB: %v", err)
		res.Fixable = true
		res.Suggestion = "Backup & recreate database"
		return res
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		res.Status = StatusFail
		res.Message = fmt.Sprintf("Gagal ping DB: %v", err)
		res.Fixable = true
		res.Suggestion = "Backup & recreate database"
		return res
	}

	var integrity string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
		res.Status = StatusFail
		res.Message = fmt.Sprintf("Gagal integrity check: %v", err)
		res.Fixable = true
		res.Suggestion = "Backup & recreate database"
		return res
	}
	if integrity != "ok" {
		res.Status = StatusFail
		res.Message = fmt.Sprintf("Integrity check gagal: %s", integrity)
		res.Fixable = true
		res.Suggestion = "Backup & recreate database"
		return res
	}

	// 3. Check required tables
	for _, table := range requiredTables {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
			table,
		).Scan(&name)
		if err != nil {
			res.Status = StatusWarn
			res.Message = fmt.Sprintf("Tabel '%s' tidak ditemukan", table)
			res.Fixable = true
			res.Suggestion = "Re-initialize schema"
			return res
		}
	}

	res.Message = fmt.Sprintf("DB OK (%s, %d bytes)", dbPath, info.Size())
	return res
}

// RepairDB backs up a corrupt DB and recreates it with the full schema.
func RepairDB(dbPath string) error {
	// Backup existing file (even if 0 bytes or corrupt)
	if _, err := os.Stat(dbPath); err == nil {
		backupPath, err := BackupFile(dbPath)
		if err != nil {
			return fmt.Errorf("gagal backup DB: %w", err)
		}
		_ = backupPath
	}

	// Remove old DB
	_ = os.Remove(dbPath)
	// Remove WAL/shm if they exist
	_ = os.Remove(dbPath + "-shm")
	_ = os.Remove(dbPath + "-wal")

	// Create new DB with full schema
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("gagal create DB baru: %w", err)
	}
	defer db.Close()

	statements := []string{
		`CREATE TABLE IF NOT EXISTS workspaces (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			path TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS memories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content TEXT NOT NULL,
			embedding BLOB,
			tags TEXT DEFAULT '[]',
			source TEXT DEFAULT '',
			metadata TEXT DEFAULT '{}',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME,
			category_id INTEGER,
			version INTEGER DEFAULT 1,
			workspace_id INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS sync_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			memory_id INTEGER NOT NULL,
			delta_hash TEXT NOT NULL,
			status TEXT DEFAULT 'pending',
			synced_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			name TEXT DEFAULT '',
			state TEXT DEFAULT 'active',
			mode TEXT DEFAULT 'ask',
			mcp_servers TEXT DEFAULT '[]',
			history TEXT DEFAULT '[]',
			tasks TEXT DEFAULT '[]',
			memory_ids TEXT DEFAULT '[]',
			context TEXT DEFAULT '',
			is_agentic INTEGER DEFAULT 0,
			auto_resume INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS categories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			parent_id INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS memory_versions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			memory_id INTEGER NOT NULL,
			content TEXT NOT NULL,
			metadata TEXT DEFAULT '{}',
			changed_by TEXT DEFAULT '',
			reason TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_tags ON memories(tags)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_source ON memories(source)`,
		`CREATE INDEX IF NOT EXISTS idx_sync_status ON sync_log(status)`,
		`CREATE INDEX IF NOT EXISTS idx_categories_workspace ON categories(workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_versions_memory ON memory_versions(memory_id)`,
		`CREATE TABLE IF NOT EXISTS ssh_hosts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			address TEXT NOT NULL,
			port TEXT DEFAULT '22',
			user TEXT NOT NULL,
			key_path TEXT,
			password TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS ssh_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			host_name TEXT NOT NULL,
			address TEXT NOT NULL,
			command TEXT NOT NULL,
			stdout TEXT,
			stderr TEXT,
			status TEXT DEFAULT 'success',
			duration_ms INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ssh_logs_host ON ssh_logs(host_name)`,
		`CREATE INDEX IF NOT EXISTS idx_ssh_logs_created ON ssh_logs(created_at)`,
		`CREATE TABLE IF NOT EXISTS user_profile (
			id INTEGER PRIMARY KEY,
			name TEXT,
			preferences TEXT DEFAULT '{}',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("gagal inisialisasi schema: %w", err)
		}
	}

	return nil
}

// VacuumDB runs VACUUM on the database.
func VacuumDB(dbPath string) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec("VACUUM")
	return err
}

// PurgeOldSessions removes ended sessions older than retentionDays.
func PurgeOldSessions(dbPath string, retentionDays int) (int64, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays).Format(time.RFC3339)
	res, err := db.Exec(
		"DELETE FROM sessions WHERE state = 'ended' AND updated_at < ?",
		cutoff,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
