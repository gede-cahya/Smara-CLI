package session

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("gagal membuka database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("gagal koneksi database: %w", err)
	}

	store := &SQLiteStore{db: db}
	if err := store.init(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *SQLiteStore) init() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			workspace_id INTEGER,
			name TEXT NOT NULL,
			state TEXT NOT NULL,
			mode TEXT NOT NULL,
			mcp_servers TEXT,
			history TEXT,
			tasks TEXT,
			memory_ids TEXT,
			context TEXT,
			is_agentic INTEGER DEFAULT 0,
			auto_resume INTEGER DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_workspace ON sessions(workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_state ON sessions(state)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_updated ON sessions(updated_at)`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("gagal inisialisasi tabel: %w", err)
		}
	}

	// Migrasi: Tambahkan kolom yang mungkin hilang di versi lama
	columns := []struct {
		name string
		typ  string
	}{
		{"memory_ids", "TEXT"},
		{"context", "TEXT"},
		{"is_agentic", "INTEGER DEFAULT 0"},
		{"auto_resume", "INTEGER DEFAULT 0"},
		{"is_archived", "INTEGER DEFAULT 0"},
		{"archived_at", "TEXT"},
	}

	for _, col := range columns {
		query := fmt.Sprintf("ALTER TABLE sessions ADD COLUMN %s %s", col.name, col.typ)
		// Kita abaikan error "duplicate column name"
		_, _ = s.db.Exec(query)
	}

	// Index untuk archive queries
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_sessions_archived ON sessions(is_archived, workspace_id)")

	return nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) CreateSession(session *Session) error {
	return s.saveSession(session)
}

func (s *SQLiteStore) GetSession(id string) (*Session, error) {
	var session Session
	var mcpServers, history, tasks, memoryIDs []byte
	var createdAtStr, updatedAtStr string

	err := s.db.QueryRow(
		`SELECT id, workspace_id, name, state, mode, mcp_servers, history, tasks, memory_ids, context, is_agentic, auto_resume, created_at, updated_at
		 FROM sessions WHERE id = ?`,
		id,
	).Scan(
		&session.ID, &session.WorkspaceID, &session.Name, &session.State, &session.Mode,
		&mcpServers, &history, &tasks, &memoryIDs,
		&session.Context, &session.IsAgentic, &session.AutoResume,
		&createdAtStr, &updatedAtStr,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session tidak ditemukan: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("gagal membaca session: %w", err)
	}

	if len(mcpServers) > 0 {
		json.Unmarshal(mcpServers, &session.MCPServers)
	}
	if len(history) > 0 {
		json.Unmarshal(history, &session.History)
	}
	if len(tasks) > 0 {
		json.Unmarshal(tasks, &session.Tasks)
	}
	if len(memoryIDs) > 0 {
		json.Unmarshal(memoryIDs, &session.MemoryIDs)
	}

	session.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	session.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
	return &session, nil
}

func (s *SQLiteStore) UpdateSession(session *Session) error {
	return s.saveSession(session)
}

func (s *SQLiteStore) DeleteSession(id string) error {
	result, err := s.db.Exec("DELETE FROM sessions WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("gagal menghapus session: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session tidak ditemukan: %s", id)
	}

	return nil
}

func (s *SQLiteStore) ListSessions() ([]Session, error) {
	return s.ListSessionsByWorkspace(0)
}

func (s *SQLiteStore) ListSessionsByWorkspace(workspaceID int64) ([]Session, error) {
	query := `SELECT id, workspace_id, name, state, mode, mcp_servers, history, tasks, memory_ids, context, is_agentic, auto_resume, created_at, updated_at
		 FROM sessions`
	var args []interface{}
	if workspaceID > 0 {
		query += " WHERE workspace_id = ?"
		args = append(args, workspaceID)
	}
	query += " ORDER BY updated_at DESC LIMIT 50"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("gagal daftar session: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var session Session
		var mcpServers, history, tasks, memoryIDs []byte
		var createdAtStr, updatedAtStr string

		err := rows.Scan(
			&session.ID, &session.WorkspaceID, &session.Name, &session.State, &session.Mode,
			&mcpServers, &history, &tasks, &memoryIDs,
			&session.Context, &session.IsAgentic, &session.AutoResume,
			&createdAtStr, &updatedAtStr,
		)
		if err != nil {
			return nil, err
		}

		if len(mcpServers) > 0 {
			json.Unmarshal(mcpServers, &session.MCPServers)
		}
		if len(history) > 0 {
			json.Unmarshal(history, &session.History)
		}
		if len(tasks) > 0 {
			json.Unmarshal(tasks, &session.Tasks)
		}
		if len(memoryIDs) > 0 {
			json.Unmarshal(memoryIDs, &session.MemoryIDs)
		}

		session.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		session.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
		sessions = append(sessions, session)
	}

	return sessions, nil
}

func (s *SQLiteStore) ListActiveSessions() ([]Session, error) {
	rows, err := s.db.Query(
		`SELECT id, workspace_id, name, state, mode, mcp_servers, history, tasks, memory_ids, context, is_agentic, auto_resume, created_at, updated_at
		 FROM sessions WHERE state = 'active' ORDER BY updated_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("gagal daftar session aktif: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var session Session
		var mcpServers, history, tasks, memoryIDs []byte
		var createdAtStr, updatedAtStr string

		err := rows.Scan(
			&session.ID, &session.WorkspaceID, &session.Name, &session.State, &session.Mode,
			&mcpServers, &history, &tasks, &memoryIDs,
			&session.Context, &session.IsAgentic, &session.AutoResume,
			&createdAtStr, &updatedAtStr,
		)
		if err != nil {
			return nil, err
		}

		if len(mcpServers) > 0 {
			json.Unmarshal(mcpServers, &session.MCPServers)
		}
		if len(history) > 0 {
			json.Unmarshal(history, &session.History)
		}
		if len(tasks) > 0 {
			json.Unmarshal(tasks, &session.Tasks)
		}
		if len(memoryIDs) > 0 {
			json.Unmarshal(memoryIDs, &session.MemoryIDs)
		}

		session.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		session.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
		sessions = append(sessions, session)
	}

	return sessions, nil
}

func (s *SQLiteStore) GetLastActiveSession() (*Session, error) {
	return s.GetLastActiveSessionByWorkspace(0)
}

func (s *SQLiteStore) GetLastActiveSessionByWorkspace(workspaceID int64) (*Session, error) {
	var session Session
	var mcpServers, history, tasks, memoryIDs []byte

	query := `SELECT id, workspace_id, name, state, mode, mcp_servers, history, tasks, memory_ids, context, is_agentic, auto_resume, is_archived, archived_at, created_at, updated_at
		 FROM sessions WHERE state = 'active' AND is_archived = 0`
	var args []interface{}
	if workspaceID > 0 {
		query += " AND workspace_id = ?"
		args = append(args, workspaceID)
	}
	query += " ORDER BY updated_at DESC LIMIT 1"

	var createdAtStr, updatedAtStr string
	var archivedAt sql.NullString
	err := s.db.QueryRow(query, args...).Scan(
		&session.ID, &session.WorkspaceID, &session.Name, &session.State, &session.Mode,
		&mcpServers, &history, &tasks, &memoryIDs,
		&session.Context, &session.IsAgentic, &session.AutoResume,
		&session.IsArchived, &archivedAt,
		&createdAtStr, &updatedAtStr,
	)
	if archivedAt.Valid && archivedAt.String != "" {
		session.ArchivedAt = new(time.Time)
		*session.ArchivedAt, _ = time.Parse(time.RFC3339, archivedAt.String)
	}

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("gagal membaca session terakhir: %w", err)
	}

	if len(mcpServers) > 0 {
		json.Unmarshal(mcpServers, &session.MCPServers)
	}
	if len(history) > 0 {
		json.Unmarshal(history, &session.History)
	}
	if len(tasks) > 0 {
		json.Unmarshal(tasks, &session.Tasks)
	}
	if len(memoryIDs) > 0 {
		json.Unmarshal(memoryIDs, &session.MemoryIDs)
	}

	return &session, nil
}

// --- Archive Operations ---

// ArchiveSession soft-archives a session by ID.
func (s *SQLiteStore) ArchiveSession(id string) error {
	now := time.Now().Format(time.RFC3339)
	result, err := s.db.Exec(
		"UPDATE sessions SET is_archived = 1, archived_at = ? WHERE id = ?",
		now, id,
	)
	if err != nil {
		return fmt.Errorf("gagal mengarsipkan session: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session tidak ditemukan: %s", id)
	}
	return nil
}

// UnarchiveSession restores an archived session.
func (s *SQLiteStore) UnarchiveSession(id string) error {
	result, err := s.db.Exec(
		"UPDATE sessions SET is_archived = 0, archived_at = NULL WHERE id = ?",
		id,
	)
	if err != nil {
		return fmt.Errorf("gagal memulihkan session: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session tidak ditemukan: %s", id)
	}
	return nil
}

// ListArchivedSessions returns archived sessions for a workspace.
func (s *SQLiteStore) ListArchivedSessions(workspaceID int64) ([]Session, error) {
	query := `SELECT id, workspace_id, name, state, mode, mcp_servers, history, tasks, memory_ids, context, is_agentic, auto_resume, is_archived, archived_at, created_at, updated_at
		 FROM sessions WHERE is_archived = 1`
	var args []interface{}
	if workspaceID > 0 {
		query += " AND workspace_id = ?"
		args = append(args, workspaceID)
	}
	query += " ORDER BY archived_at DESC LIMIT 50"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("gagal daftar archived session: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var session Session
		var mcpServers, history, tasks, memoryIDs []byte
		var archivedAt sql.NullString

		err := rows.Scan(
			&session.ID, &session.WorkspaceID, &session.Name, &session.State, &session.Mode,
			&mcpServers, &history, &tasks, &memoryIDs,
			&session.Context, &session.IsAgentic, &session.AutoResume,
			&session.IsArchived, &archivedAt,
			&session.CreatedAt, &session.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if archivedAt.Valid && archivedAt.String != "" {
			t, _ := time.Parse(time.RFC3339, archivedAt.String)
			session.ArchivedAt = &t
		}
		if len(mcpServers) > 0 {
			json.Unmarshal(mcpServers, &session.MCPServers)
		}
		if len(history) > 0 {
			json.Unmarshal(history, &session.History)
		}
		if len(tasks) > 0 {
			json.Unmarshal(tasks, &session.Tasks)
		}
		if len(memoryIDs) > 0 {
			json.Unmarshal(memoryIDs, &session.MemoryIDs)
		}

		sessions = append(sessions, session)
	}
	return sessions, nil
}

// DeleteArchivedSession permanently deletes an archived session.
func (s *SQLiteStore) DeleteArchivedSession(id string) error {
	result, err := s.db.Exec("DELETE FROM sessions WHERE id = ? AND is_archived = 1", id)
	if err != nil {
		return fmt.Errorf("gagal menghapus session: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("archived session tidak ditemukan: %s", id)
	}
	return nil
}

func (s *SQLiteStore) saveSession(session *Session) error {
	mcpServers, _ := json.Marshal(session.MCPServers)
	history, _ := json.Marshal(session.History)
	tasks, _ := json.Marshal(session.Tasks)
	memoryIDs, _ := json.Marshal(session.MemoryIDs)

	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO sessions 
		 (id, workspace_id, name, state, mode, mcp_servers, history, tasks, memory_ids, context, is_agentic, auto_resume, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID,
		session.WorkspaceID,
		session.Name,
		session.State,
		session.Mode,
		mcpServers,
		history,
		tasks,
		memoryIDs,
		session.Context,
		session.IsAgentic,
		session.AutoResume,
		session.CreatedAt.Format(time.RFC3339),
		session.UpdatedAt.Format(time.RFC3339),
	)

	if err != nil {
		return fmt.Errorf("gagal menyimpan session: %w", err)
	}

	return nil
}
