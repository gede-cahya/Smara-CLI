package ssh

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// LogEntry represents a single SSH execution log.
type LogEntry struct {
	ID        int64     `json:"id"`
	HostName  string    `json:"host_name"`
	Address   string    `json:"address"`
	Command   string    `json:"command"`
	Stdout    string    `json:"stdout"`
	Stderr    string    `json:"stderr"`
	Status    string    `json:"status"` // success / error
	Duration  int64     `json:"duration_ms"`
	CreatedAt time.Time `json:"created_at"`
}

// TransferLogEntry represents a single file transfer log.
type TransferLogEntry struct {
	ID         int64     `json:"id"`
	HostName   string    `json:"host_name"`
	Address    string    `json:"address"`
	LocalPath  string    `json:"local_path"`
	RemotePath string    `json:"remote_path"`
	Direction  string    `json:"direction"` // upload / download
	Bytes      int64     `json:"bytes"`
	Method     string    `json:"method"`      // sftp / scp
	Status     string    `json:"status"`      // success / error
	ErrorMsg   string    `json:"error_msg"`
	Duration   int64     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

// Store provides SQLite persistence for SSH logs.
type Store struct {
	db *sql.DB
}

// NewStore creates a new SSH log store backed by SQLite.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("gagal membuka database: %w", err)
	}

	store := &Store{db: db}
	if err := store.Init(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

// Init creates the SSH tables if they don't exist.
func (s *Store) Init() error {
	statements := []string{
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
		`CREATE TABLE IF NOT EXISTS ssh_transfer_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			host_name TEXT NOT NULL,
			address TEXT NOT NULL,
			local_path TEXT,
			remote_path TEXT,
			direction TEXT NOT NULL,
			bytes INTEGER DEFAULT 0,
			method TEXT DEFAULT 'sftp',
			status TEXT DEFAULT 'success',
			error_msg TEXT,
			duration_ms INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ssh_logs_host ON ssh_logs(host_name)`,
		`CREATE INDEX IF NOT EXISTS idx_ssh_logs_created ON ssh_logs(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_ssh_transfer_logs_host ON ssh_transfer_logs(host_name)`,
		`CREATE INDEX IF NOT EXISTS idx_ssh_transfer_logs_created ON ssh_transfer_logs(created_at)`,
	}

	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("gagal eksekusi schema statement: %w", err)
		}
	}
	return nil
}

// SaveLog inserts an execution log entry.
func (s *Store) SaveLog(entry LogEntry) error {
	_, err := s.db.Exec(
		`INSERT INTO ssh_logs (host_name, address, command, stdout, stderr, status, duration_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.HostName, entry.Address, entry.Command, entry.Stdout, entry.Stderr, entry.Status, entry.Duration,
	)
	return err
}

// ListLogs returns recent SSH execution logs.
func (s *Store) ListLogs(limit int) ([]LogEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.Query(
		`SELECT id, host_name, address, command, stdout, stderr, status, duration_ms, created_at
		 FROM ssh_logs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("gagal query logs: %w", err)
	}
	defer rows.Close()

	var logs []LogEntry
	for rows.Next() {
		var e LogEntry
		err := rows.Scan(&e.ID, &e.HostName, &e.Address, &e.Command, &e.Stdout, &e.Stderr, &e.Status, &e.Duration, &e.CreatedAt)
		if err != nil {
			continue
		}
		logs = append(logs, e)
	}
	return logs, rows.Err()
}

// SaveTransferLog inserts a file transfer log entry.
func (s *Store) SaveTransferLog(entry TransferLogEntry) error {
	_, err := s.db.Exec(
		`INSERT INTO ssh_transfer_logs (host_name, address, local_path, remote_path, direction, bytes, method, status, error_msg, duration_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.HostName, entry.Address, entry.LocalPath, entry.RemotePath,
		entry.Direction, entry.Bytes, entry.Method, entry.Status, entry.ErrorMsg, entry.Duration,
	)
	return err
}

// ListTransferLogs returns recent SSH file transfer logs.
func (s *Store) ListTransferLogs(limit int) ([]TransferLogEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.Query(
		`SELECT id, host_name, address, local_path, remote_path, direction, bytes, method, status, error_msg, duration_ms, created_at
		 FROM ssh_transfer_logs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("gagal query transfer logs: %w", err)
	}
	defer rows.Close()

	var logs []TransferLogEntry
	for rows.Next() {
		var e TransferLogEntry
		err := rows.Scan(&e.ID, &e.HostName, &e.Address, &e.LocalPath, &e.RemotePath,
			&e.Direction, &e.Bytes, &e.Method, &e.Status, &e.ErrorMsg, &e.Duration, &e.CreatedAt)
		if err != nil {
			continue
		}
		logs = append(logs, e)
	}
	return logs, rows.Err()
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}
