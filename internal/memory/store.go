package memory

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/session"
)

// SQLiteStore implements MemoryStore using SQLite.
type SQLiteStore struct {
	db     *sql.DB
	dbPath string
}

// NewSQLiteStore creates a new SQLite-backed memory store.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("gagal membuka database: %w", err)
	}

	store := &SQLiteStore{db: db, dbPath: dbPath}
	if err := store.Init(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

// Init creates the database schema if it doesn't exist.
func (s *SQLiteStore) Init() error {
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
			synced_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (memory_id) REFERENCES memories(id) ON DELETE CASCADE
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
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
			FOREIGN KEY (parent_id) REFERENCES categories(id) ON DELETE SET NULL
		)`,
		`CREATE TABLE IF NOT EXISTS memory_versions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			memory_id INTEGER NOT NULL,
			content TEXT NOT NULL,
			metadata TEXT DEFAULT '{}',
			changed_by TEXT DEFAULT '',
			reason TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (memory_id) REFERENCES memories(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_workspace ON memories(workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_category ON memories(category_id)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_updated ON memories(updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_expires ON memories(expires_at) WHERE expires_at IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_memories_tags ON memories(tags)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_source ON memories(source)`,
		`CREATE INDEX IF NOT EXISTS idx_sync_status ON sync_log(status)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_updated ON sessions(updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_categories_workspace ON categories(workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_versions_memory ON memory_versions(memory_id)`,
	}

	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("gagal eksekusi schema statement: %w", err)
		}
	}

	// Migrasi: Tambahkan kolom baru jika belum ada
	if err := s.migrate(); err != nil {
		return fmt.Errorf("gagal migrasi database: %w", err)
	}

	// Setup FTS5 virtual table
	if err := s.setupFTS5(); err != nil {
		return fmt.Errorf("gagal setup FTS5: %w", err)
	}

	return nil
}

func (s *SQLiteStore) migrate() error {
	// Helper function to check if column exists
	columnExists := func(table, column string) (bool, error) {
		var count int
		err := s.db.QueryRow(
			"SELECT count(*) FROM pragma_table_info(?) WHERE name=?",
			table, column,
		).Scan(&count)
		if err != nil {
			return false, err
		}
		return count > 0, nil
	}

	// 1. Migrate memories table - add new columns if they don't exist
	cols := []string{"updated_at", "expires_at", "category_id", "metadata", "version"}
	for _, col := range cols {
		exists, err := columnExists("memories", col)
		if err != nil {
			return fmt.Errorf("gagal cek kolom %s: %w", col, err)
		}
		if !exists {
			var stmt string
			switch col {
			case "updated_at":
				stmt = "ALTER TABLE memories ADD COLUMN updated_at DATETIME DEFAULT CURRENT_TIMESTAMP"
			case "expires_at":
				stmt = "ALTER TABLE memories ADD COLUMN expires_at DATETIME"
			case "category_id":
				stmt = "ALTER TABLE memories ADD COLUMN category_id INTEGER REFERENCES categories(id)"
			case "metadata":
				stmt = "ALTER TABLE memories ADD COLUMN metadata TEXT DEFAULT '{}'"
			case "version":
				stmt = "ALTER TABLE memories ADD COLUMN version INTEGER DEFAULT 1"
			}
			if stmt != "" {
				_, _ = s.db.Exec(stmt)
			}
		}
	}

	// 2. Migrate tags from string to JSON array format (for existing records)
	// Check if tags column needs conversion
	rows, err := s.db.Query("SELECT id, tags FROM memories WHERE tags != '[]' AND tags != '' AND (tags NOT LIKE '[%]' OR tags IS NULL)")
	if err == nil {
		for rows.Next() {
			var id int64
			var tagsStr string
			if err := rows.Scan(&id, &tagsStr); err != nil {
				continue
			}
			// Convert comma-separated string to JSON array
			if tagsStr != "" && tagsStr != "[]" {
				// Simple conversion: split by comma and create JSON array
				jsonTags := "["
				first := true
				start := 0
				for i, ch := range tagsStr {
					if ch == ',' {
						if !first {
							jsonTags += ","
						}
						jsonTags += "\"" + tagsStr[start:i] + "\""
						first = false
						start = i + 1
					}
				}
				if start < len(tagsStr) {
					if !first {
						jsonTags += ","
					}
					jsonTags += "\"" + tagsStr[start:] + "\""
				}
				jsonTags += "]"
				_, _ = s.db.Exec("UPDATE memories SET tags = ? WHERE id = ?", jsonTags, id)
			}
		}
		rows.Close()
	}

	// 3. Ensure workspace_id exists in memories (for backward compatibility)
	exists, _ := columnExists("memories", "workspace_id")
	if !exists {
		_, _ = s.db.Exec("ALTER TABLE memories ADD COLUMN workspace_id INTEGER DEFAULT 0")
	}

	// 4. Ensure workspace_id exists in sessions
	exists, _ = columnExists("sessions", "workspace_id")
	if !exists {
		_, _ = s.db.Exec("ALTER TABLE sessions ADD COLUMN workspace_id INTEGER DEFAULT 0")
	}

	// 5. Create indexes if they don't exist (CREATE INDEX IF NOT EXISTS handles this)
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_memories_workspace ON memories(workspace_id)")
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_memories_category ON memories(category_id)")
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_memories_updated ON memories(updated_at DESC)")
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_memories_expires ON memories(expires_at) WHERE expires_at IS NOT NULL")
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_categories_workspace ON categories(workspace_id)")
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_memory_versions_memory ON memory_versions(memory_id)")

	return nil
}

// setupFTS5 creates the FTS5 virtual table and triggers for full-text search.
// Note: modernc.org/sqlite may not support FTS5. If it fails, we'll fall back to LIKE queries.
func (s *SQLiteStore) setupFTS5() error {
	// Try to create FTS5 virtual table
	_, err := s.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
			content,
			tags,
			source,
			content='memories',
			content_rowid='id'
		)`)
	if err != nil {
		// FTS5 not supported, will use fallback search
		return nil
	}

	// Create triggers to keep FTS table in sync
	triggers := []string{
		`CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
			INSERT INTO memories_fts(rowid, content, tags, source) VALUES (new.id, new.content, new.tags, new.source);
		END`,
		`CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
			INSERT INTO memories_fts(memories_fts, rowid, content, tags, source) VALUES ('delete', old.id, old.content, old.tags, old.source);
			INSERT INTO memories_fts(rowid, content, tags, source) VALUES (new.id, new.content, new.tags, new.source);
		END`,
		`CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
			INSERT INTO memories_fts(memories_fts, rowid, content, tags, source) VALUES ('delete', old.id, old.content, old.tags, old.source);
		END`,
	}

	for _, trigger := range triggers {
		_, err = s.db.Exec(trigger)
		if err != nil {
			// If trigger creation fails, FTS5 might not be fully supported
			// Continue anyway - we'll use fallback search
		}
	}

	return nil
}

// Save stores a new memory with optional embedding and workspace scoping.
func (s *SQLiteStore) Save(content, tags, source string, workspaceID int64, embedding []float32) (*Memory, error) {
	return s.SaveWithOptions(content, tags, source, workspaceID, embedding, nil, nil, nil)
}

// SaveWithOptions stores a new memory with full options including category, metadata, and TTL.
func (s *SQLiteStore) SaveWithOptions(content, tags, source string, workspaceID int64, embedding []float32, categoryID *int64, metadata map[string]interface{}, expiresAt *time.Time) (*Memory, error) {
	var embBlob []byte
	if len(embedding) > 0 {
		embBlob = float32ToBytes(embedding)
	}

	// Convert tags to JSON array
	tagsJSON := "[]"
	if tags != "" {
		// Simple conversion - in production, use proper JSON marshaling
		tagsJSON = "["
		first := true
		start := 0
		for i, ch := range tags {
			if ch == ',' {
				if !first {
					tagsJSON += ","
				}
				tagsJSON += "\"" + tags[start:i] + "\""
				first = false
				start = i + 1
			}
		}
		if start < len(tags) {
			if !first {
				tagsJSON += ","
			}
			tagsJSON += "\"" + tags[start:] + "\""
		}
		tagsJSON += "]"
	}

	// Convert metadata to JSON
	metadataJSON := "{}"
	if metadata != nil {
		if data, err := json.Marshal(metadata); err == nil {
			metadataJSON = string(data)
		}
	}

	var wID interface{} = workspaceID
	if workspaceID <= 0 {
		wID = nil
	}

	var catID interface{} = categoryID
	if categoryID != nil && *categoryID <= 0 {
		catID = nil
	}

	var expAt interface{} = expiresAt
	if expiresAt != nil && expiresAt.IsZero() {
		expAt = nil
	}

	now := time.Now()

	result, err := s.db.Exec(
		`INSERT INTO memories 
		(content, embedding, tags, source, metadata, created_at, updated_at, expires_at, category_id, version, workspace_id) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		content, embBlob, tagsJSON, source, metadataJSON, now, now, expAt, catID, 1, wID,
	)
	if err != nil {
		return nil, fmt.Errorf("gagal menyimpan memory: %w", err)
	}

	id, _ := result.LastInsertId()

	// Create memory version
	_, _ = s.db.Exec(
		`INSERT INTO memory_versions (memory_id, content, metadata, changed_by, reason, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, content, metadataJSON, "system", "initial creation", now,
	)

	return &Memory{
		ID:          id,
		WorkspaceID: workspaceID,
		CategoryID:  categoryID,
		Content:     content,
		Embedding:   embedding,
		Tags:        parseTagsFromJSON(tagsJSON),
		Source:      source,
		Metadata:    metadata,
		CreatedAt:   now,
		UpdatedAt:   now,
		ExpiresAt:   expiresAt,
		Version:     1,
	}, nil
}

// List returns the most recent memories for a specific workspace.
func (s *SQLiteStore) List(workspaceID int64, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.Query(
		`SELECT id, workspace_id, content, tags, source, created_at, updated_at, expires_at, category_id, metadata, version 
		 FROM memories WHERE workspace_id = ? OR workspace_id IS NULL OR ? = 0 ORDER BY created_at DESC LIMIT ?`,
		workspaceID, workspaceID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("gagal query memories: %w", err)
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var m Memory
		var tagsJSON, metadataJSON sql.NullString
		var expiresAt sql.NullTime
		var categoryID sql.NullInt64
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.Content, &tagsJSON, &m.Source, &m.CreatedAt, &m.UpdatedAt, &expiresAt, &categoryID, &metadataJSON, &m.Version); err != nil {
			return nil, fmt.Errorf("gagal scan memory: %w", err)
		}
		m.Tags = parseTagsFromJSON(tagsJSON.String)
		if expiresAt.Valid {
			m.ExpiresAt = &expiresAt.Time
		}
		if categoryID.Valid {
			m.CategoryID = &categoryID.Int64
		}
		if metadataJSON.Valid {
			var metadata map[string]interface{}
			if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err == nil {
				m.Metadata = metadata
			}
		}
		memories = append(memories, m)
	}
	return memories, nil
}

// Delete removes a memory by ID.
func (s *SQLiteStore) Delete(id int64) error {
	_, err := s.db.Exec("DELETE FROM memories WHERE id = ?", id)
	return err
}

// Clear removes all memories and sync logs.
func (s *SQLiteStore) Clear() error {
	_, err := s.db.Exec("DELETE FROM memories; DELETE FROM sync_log;")
	return err
}

// GetUnsyncedMemories returns memories without a successful sync entry.
func (s *SQLiteStore) GetUnsyncedMemories() ([]Memory, error) {
	rows, err := s.db.Query(`
		SELECT m.id, m.workspace_id, m.content, m.embedding, m.tags, m.source, m.created_at, m.updated_at, m.expires_at, m.category_id, m.metadata, m.version
		FROM memories m
		LEFT JOIN sync_log sl ON m.id = sl.memory_id AND sl.status = 'complete'
		WHERE sl.id IS NULL
		ORDER BY m.created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("gagal query unsynced memories: %w", err)
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var m Memory
		var embBlob []byte
		var tagsJSON, metadataJSON sql.NullString
		var expiresAt sql.NullTime
		var categoryID sql.NullInt64
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.Content, &embBlob, &tagsJSON, &m.Source, &m.CreatedAt, &m.UpdatedAt, &expiresAt, &categoryID, &metadataJSON, &m.Version); err != nil {
			return nil, fmt.Errorf("gagal scan memory: %w", err)
		}
		if len(embBlob) > 0 {
			m.Embedding = bytesToFloat32(embBlob)
		}
		m.Tags = parseTagsFromJSON(tagsJSON.String)
		if expiresAt.Valid {
			m.ExpiresAt = &expiresAt.Time
		}
		if categoryID.Valid {
			m.CategoryID = &categoryID.Int64
		}
		if metadataJSON.Valid {
			var metadata map[string]interface{}
			if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err == nil {
				m.Metadata = metadata
			}
		}
		memories = append(memories, m)
	}
	return memories, nil
}

// MarkSynced records a successful sync for a memory.
func (s *SQLiteStore) MarkSynced(memoryID int64, deltaHash string) error {
	_, err := s.db.Exec(
		"INSERT INTO sync_log (memory_id, delta_hash, status) VALUES (?, ?, 'complete')",
		memoryID, deltaHash,
	)
	return err
}

// UpdateMemory updates an existing memory.
func (s *SQLiteStore) UpdateMemory(id int64, updates map[string]interface{}) error {
	// First, get current memory for versioning
	var current Memory
	var tagsJSON, metadataJSON string
	var err error
	row := s.db.QueryRow(
		`SELECT content, tags, metadata, category_id, expires_at, version FROM memories WHERE id = ?`,
		id,
	)
	if err := row.Scan(&current.Content, &tagsJSON, &metadataJSON, &current.CategoryID, &current.ExpiresAt, &current.Version); err != nil {
		return fmt.Errorf("gagal membaca memory saat ini: %w", err)
	}

	// Build update query dynamically
	var setClauses []string
	var args []interface{}
	argCount := 1

	if content, ok := updates["content"].(string); ok {
		setClauses = append(setClauses, fmt.Sprintf("content = $%d", argCount))
		args = append(args, content)
		argCount++
		current.Content = content
	}

	if tags, ok := updates["tags"].([]string); ok {
		setClauses = append(setClauses, fmt.Sprintf("tags = $%d", argCount))
		args = append(args, formatTagsToJSON(tags))
		argCount++
		current.Tags = tags
	}

	if source, ok := updates["source"].(string); ok {
		setClauses = append(setClauses, fmt.Sprintf("source = $%d", argCount))
		args = append(args, source)
		argCount++
		current.Source = source
	}

	if categoryID, ok := updates["category_id"].(*int64); ok {
		setClauses = append(setClauses, fmt.Sprintf("category_id = $%d", argCount))
		args = append(args, categoryID)
		argCount++
		current.CategoryID = categoryID
	}

	if expiresAt, ok := updates["expires_at"].(*time.Time); ok {
		setClauses = append(setClauses, fmt.Sprintf("expires_at = $%d", argCount))
		args = append(args, expiresAt)
		argCount++
		current.ExpiresAt = expiresAt
	}

	if metadata, ok := updates["metadata"].(map[string]interface{}); ok {
		metadataJSON, _ := json.Marshal(metadata)
		setClauses = append(setClauses, fmt.Sprintf("metadata = $%d", argCount))
		args = append(args, string(metadataJSON))
		argCount++
		current.Metadata = metadata
	}

	if embedding, ok := updates["embedding"].([]float32); ok {
		setClauses = append(setClauses, fmt.Sprintf("embedding = $%d", argCount))
		args = append(args, float32ToBytes(embedding))
		argCount++
		current.Embedding = embedding
	}

	// Always update updated_at
	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argCount))
	args = append(args, time.Now())
	argCount++

	// Increment version
	setClauses = append(setClauses, fmt.Sprintf("version = $%d", argCount))
	args = append(args, current.Version+1)
	argCount++
	current.Version = current.Version + 1

	if len(setClauses) == 0 {
		return fmt.Errorf("tidak ada field yang diupdate")
	}

	args = append(args, id)
	query := fmt.Sprintf("UPDATE memories SET %s WHERE id = $%d", 
		strings.Join(setClauses, ", "), argCount)

	_, err = s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("gagal update memory: %w", err)
	}

	// Create version record
	_, err = s.db.Exec(
		`INSERT INTO memory_versions (memory_id, content, metadata, changed_by, reason, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, current.Content, metadataJSON, "system", "update", time.Now(),
	)
	if err != nil {
		// Non-critical error, log but don't fail
		fmt.Printf("Warning: gagal menyimpan versi memory: %v\n", err)
	}

	return nil
}

// GetMemoryByID retrieves a memory by ID.
func (s *SQLiteStore) GetMemoryByID(id int64) (*Memory, error) {
	var m Memory
	var tagsJSON, metadataJSON sql.NullString
	var expiresAt sql.NullTime
	var categoryID sql.NullInt64

	row := s.db.QueryRow(
		`SELECT id, workspace_id, content, embedding, tags, source, created_at, updated_at, expires_at, category_id, metadata, version 
		 FROM memories WHERE id = ?`,
		id,
	)
	var embBlob []byte
	if err := row.Scan(&m.ID, &m.WorkspaceID, &m.Content, &embBlob, &tagsJSON, &m.Source, &m.CreatedAt, &m.UpdatedAt, &expiresAt, &categoryID, &metadataJSON, &m.Version); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("gagal membaca memory: %w", err)
	}

	if len(embBlob) > 0 {
		m.Embedding = bytesToFloat32(embBlob)
	}
	m.Tags = parseTagsFromJSON(tagsJSON.String)
	if expiresAt.Valid {
		m.ExpiresAt = &expiresAt.Time
	}
	if categoryID.Valid {
		m.CategoryID = &categoryID.Int64
	}
	if metadataJSON.Valid {
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err == nil {
			m.Metadata = metadata
		}
	}

	return &m, nil
}


// CreateSession stores a new session.
func (s *SQLiteStore) CreateSession(session *session.Session) error {
	mcpServersJSON, _ := json.Marshal(session.MCPServers)
	historyJSON, _ := json.Marshal(session.History)
	tasksJSON, _ := json.Marshal(session.Tasks)

	var wID interface{} = session.WorkspaceID
	if session.WorkspaceID <= 0 {
		wID = nil
	}

	_, err := s.db.Exec(
		`INSERT INTO sessions (id, workspace_id, name, state, mode, mcp_servers, history, tasks, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID, wID, session.Name, string(session.State), string(session.Mode),
		string(mcpServersJSON), string(historyJSON), string(tasksJSON),
		session.CreatedAt, session.UpdatedAt,
	)
	return err
}

// GetSession retrieves a session by ID.
func (s *SQLiteStore) GetSession(id string) (*session.Session, error) {
	row := s.db.QueryRow(
		`SELECT id, workspace_id, name, state, mode, mcp_servers, history, tasks, created_at, updated_at
		 FROM sessions WHERE id = ?`, id,
	)

	var sess session.Session
	var mcpServersJSON, historyJSON, tasksJSON string

	err := row.Scan(&sess.ID, &sess.WorkspaceID, &sess.Name, &sess.State, &sess.Mode,
		&mcpServersJSON, &historyJSON, &tasksJSON, &sess.CreatedAt, &sess.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("gagal scan session: %w", err)
	}

	if err := json.Unmarshal([]byte(mcpServersJSON), &sess.MCPServers); err != nil {
		sess.MCPServers = []string{}
	}
	if err := json.Unmarshal([]byte(historyJSON), &sess.History); err != nil {
		sess.History = []llm.Message{}
	}
	if err := json.Unmarshal([]byte(tasksJSON), &sess.Tasks); err != nil {
		sess.Tasks = []session.Task{}
	}

	return &sess, nil
}

// UpdateSession updates an existing session.
func (s *SQLiteStore) UpdateSession(session *session.Session) error {
	mcpServersJSON, _ := json.Marshal(session.MCPServers)
	historyJSON, _ := json.Marshal(session.History)
	tasksJSON, _ := json.Marshal(session.Tasks)

	_, err := s.db.Exec(
		`UPDATE sessions SET name=?, state=?, mode=?, mcp_servers=?, history=?, tasks=?, updated_at=?
		 WHERE id=?`,
		session.Name, string(session.State), string(session.Mode),
		string(mcpServersJSON), string(historyJSON), string(tasksJSON),
		time.Now(), session.ID,
	)
	return err
}

// DeleteSession removes a session by ID.
func (s *SQLiteStore) DeleteSession(id string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE id = ?", id)
	return err
}

// ListSessions returns all sessions for a workspace ordered by updated_at DESC.
func (s *SQLiteStore) ListSessions(workspaceID int64) ([]session.Session, error) {
	rows, err := s.db.Query(
		`SELECT id, workspace_id, name, state, mode, mcp_servers, history, tasks, created_at, updated_at
		 FROM sessions WHERE workspace_id = ? OR workspace_id IS NULL ORDER BY updated_at DESC`,
		workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("gagal query sessions: %w", err)
	}
	defer rows.Close()

	var sessions []session.Session
	for rows.Next() {
		var session session.Session
		var mcpServersJSON, historyJSON, tasksJSON string

		if err := rows.Scan(&session.ID, &session.WorkspaceID, &session.Name, &session.State, &session.Mode,
			&mcpServersJSON, &historyJSON, &tasksJSON, &session.CreatedAt, &session.UpdatedAt); err != nil {
			return nil, fmt.Errorf("gagal scan session: %w", err)
		}

		json.Unmarshal([]byte(mcpServersJSON), &session.MCPServers)
		json.Unmarshal([]byte(historyJSON), &session.History)
		json.Unmarshal([]byte(tasksJSON), &session.Tasks)

		sessions = append(sessions, session)
	}

	return sessions, nil
}

// GetLastActiveSession returns the most recently updated session for a workspace.
func (s *SQLiteStore) GetLastActiveSession(workspaceID int64) (*session.Session, error) {
	row := s.db.QueryRow(
		`SELECT id, workspace_id, name, state, mode, mcp_servers, history, tasks, created_at, updated_at
		 FROM sessions WHERE workspace_id = ? OR workspace_id IS NULL ORDER BY updated_at DESC LIMIT 1`,
		workspaceID,
	)

	var sess session.Session
	var mcpServersJSON, historyJSON, tasksJSON string

	err := row.Scan(&sess.ID, &sess.WorkspaceID, &sess.Name, &sess.State, &sess.Mode,
		&mcpServersJSON, &historyJSON, &tasksJSON, &sess.CreatedAt, &sess.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	json.Unmarshal([]byte(mcpServersJSON), &sess.MCPServers)
	json.Unmarshal([]byte(historyJSON), &sess.History)
	json.Unmarshal([]byte(tasksJSON), &sess.Tasks)

	return &sess, nil
}

// --- Workspace Operations ---

// CreateWorkspace creates a new workspace.
func (s *SQLiteStore) CreateWorkspace(name, path string) (*Workspace, error) {
	result, err := s.db.Exec(
		"INSERT INTO workspaces (name, path) VALUES (?, ?)",
		name, path,
	)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat workspace: %w", err)
	}

	id, _ := result.LastInsertId()
	return &Workspace{
		ID:        id,
		Name:      name,
		Path:      path,
		CreatedAt: time.Now(),
	}, nil
}

// GetWorkspace retrieves a workspace by ID.
func (s *SQLiteStore) GetWorkspace(id int64) (*Workspace, error) {
	var w Workspace
	err := s.db.QueryRow("SELECT id, name, path, created_at FROM workspaces WHERE id = ?", id).
		Scan(&w.ID, &w.Name, &w.Path, &w.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &w, nil
}

// GetWorkspaceByName retrieves a workspace by name.
func (s *SQLiteStore) GetWorkspaceByName(name string) (*Workspace, error) {
	var w Workspace
	err := s.db.QueryRow("SELECT id, name, path, created_at FROM workspaces WHERE name = ?", name).
		Scan(&w.ID, &w.Name, &w.Path, &w.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &w, nil
}

// ListWorkspaces returns all available workspaces.
func (s *SQLiteStore) ListWorkspaces() ([]Workspace, error) {
	rows, err := s.db.Query("SELECT id, name, path, created_at FROM workspaces ORDER BY name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workspaces []Workspace
	for rows.Next() {
		var w Workspace
		if err := rows.Scan(&w.ID, &w.Name, &w.Path, &w.CreatedAt); err != nil {
			return nil, err
		}
		workspaces = append(workspaces, w)
	}
	return workspaces, nil
}

// DeleteWorkspace removes a workspace and all its associated data.
func (s *SQLiteStore) DeleteWorkspace(id int64) error {
	_, err := s.db.Exec("DELETE FROM workspaces WHERE id = ?", id)
	return err
}

// Search is implemented in search.go
// Included here to satisfy the MemoryStore interface check.
func (s *SQLiteStore) Search(embedding []float32, workspaceID int64, topK int) ([]SearchResult, error) {
	return searchByEmbedding(s.db, embedding, workspaceID, topK)
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// ListMemoriesWithFilters returns memories with advanced filtering.
func (s *SQLiteStore) ListMemoriesWithFilters(workspaceID int64, filters MemoryFilters) ([]Memory, int, error) {
	if filters.Limit <= 0 {
		filters.Limit = 20
	}
	if filters.SortBy == "" {
		filters.SortBy = "created_at"
	}
	if filters.SortDir == "" {
		filters.SortDir = "DESC"
	}

	// Validate sort field
	sortField := "created_at"
	switch filters.SortBy {
	case "created_at", "updated_at":
		sortField = filters.SortBy
	case "relevance":
		sortField = "created_at" // Default for now
	}

	// Validate sort direction
	sortDir := "DESC"
	if filters.SortDir == "ASC" {
		sortDir = "ASC"
	}

	query := `SELECT id, workspace_id, content, embedding, tags, source, created_at, updated_at, expires_at, category_id, metadata, version FROM memories WHERE 1=1`
	countQuery := `SELECT count(*) FROM memories WHERE 1=1`
	var args []interface{}
	argCount := 1

	if workspaceID > 0 {
		query += fmt.Sprintf(" AND (workspace_id = $%d OR workspace_id IS NULL)", argCount)
		countQuery += fmt.Sprintf(" AND (workspace_id = $%d OR workspace_id IS NULL)", argCount)
		args = append(args, workspaceID)
		argCount++
	}

	if len(filters.Tags) > 0 {
		// For each tag, check if it exists in the tags array
		for _, tag := range filters.Tags {
			query += fmt.Sprintf(" AND (tags LIKE $%d OR tags LIKE $%d OR tags LIKE $%d OR tags LIKE $%d OR tags = $%d)", 
				argCount, argCount+1, argCount+2, argCount+3, argCount+4)
			countQuery += fmt.Sprintf(" AND (tags LIKE $%d OR tags LIKE $%d OR tags LIKE $%d OR tags LIKE $%d OR tags = $%d)", 
				argCount, argCount+1, argCount+2, argCount+3, argCount+4)
			args = append(args, "%[\""+tag+"\"]%", "%\""+tag+"%%", "%,"+tag+",%", "%,"+tag+"]%", "[\""+tag+"\"]")
			argCount += 5
		}
	}

	if len(filters.Sources) > 0 {
		placeholders := ""
		for i, src := range filters.Sources {
			if i > 0 {
				placeholders += ","
			}
			placeholders += fmt.Sprintf("$%d", argCount)
			args = append(args, src)
			argCount++
		}
		query += fmt.Sprintf(" AND source IN (%s)", placeholders)
		countQuery += fmt.Sprintf(" AND source IN (%s)", placeholders)
	}

	if filters.DateFrom != nil {
		query += fmt.Sprintf(" AND created_at >= $%d", argCount)
		countQuery += fmt.Sprintf(" AND created_at >= $%d", argCount)
		args = append(args, *filters.DateFrom)
		argCount++
	}

	if filters.DateTo != nil {
		query += fmt.Sprintf(" AND created_at <= $%d", argCount)
		countQuery += fmt.Sprintf(" AND created_at <= $%d", argCount)
		args = append(args, *filters.DateTo)
		argCount++
	}

	if filters.CategoryID != nil {
		query += fmt.Sprintf(" AND category_id = $%d", argCount)
		countQuery += fmt.Sprintf(" AND category_id = $%d", argCount)
		args = append(args, *filters.CategoryID)
		argCount++
	}

	// Get total count
	var total int
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("gagal hitung total: %w", err)
	}

	// Add pagination
	if filters.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argCount)
		args = append(args, filters.Offset)
		argCount++
	}

	query += fmt.Sprintf(" ORDER BY %s %s LIMIT $%d", sortField, sortDir, argCount)
	args = append(args, filters.Limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("gagal query dengan filter: %w", err)
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var m Memory
		var tagsJSON, metadataJSON sql.NullString
		var expiresAt sql.NullTime
		var categoryID sql.NullInt64
		var embBlob []byte

		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.Content, &embBlob, &tagsJSON, &m.Source, &m.CreatedAt, &m.UpdatedAt, &expiresAt, &categoryID, &metadataJSON, &m.Version); err != nil {
			return nil, 0, fmt.Errorf("gagal scan memory: %w", err)
		}

		if len(embBlob) > 0 {
			m.Embedding = bytesToFloat32(embBlob)
		}
		m.Tags = parseTagsFromJSON(tagsJSON.String)
		if expiresAt.Valid {
			m.ExpiresAt = &expiresAt.Time
		}
		if categoryID.Valid {
			m.CategoryID = &categoryID.Int64
		}
		if metadataJSON.Valid {
			var metadata map[string]interface{}
			if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err == nil {
				m.Metadata = metadata
			}
		}
		memories = append(memories, m)
	}

	return memories, total, nil
}

// --- Helpers ---

func float32ToBytes(floats []float32) []byte {
	buf := make([]byte, len(floats)*4)
	for i, f := range floats {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

func bytesToFloat32(data []byte) []float32 {
	floats := make([]float32, len(data)/4)
	for i := range floats {
		floats[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return floats
}

// parseTagsFromJSON parses tags from JSON array string to []string
func parseTagsFromJSON(tagsJSON string) []string {
	if tagsJSON == "" || tagsJSON == "[]" {
		return []string{}
	}
	var tags []string
	if err := json.Unmarshal([]byte(tagsJSON), &tags); err != nil {
		// Fallback: try to parse as comma-separated string
		if tagsJSON != "" {
			// Check if it looks like a JSON array (starts with [ and ends with ])
			if len(tagsJSON) >= 2 && tagsJSON[0] == '[' && tagsJSON[len(tagsJSON)-1] == ']' {
				// Try to parse as comma-separated values inside brackets
				inner := tagsJSON[1 : len(tagsJSON)-1]
				if inner != "" {
					parts := strings.Split(inner, ",")
					for _, part := range parts {
						tag := strings.TrimSpace(part)
						// Remove quotes if present
						tag = strings.Trim(tag, "\"")
						tag = strings.Trim(tag, "'")
						if tag != "" {
							tags = append(tags, tag)
						}
					}
				}
				if len(tags) > 0 {
					return tags
				}
			}
			// Try to parse as plain comma-separated string
			parts := strings.Split(tagsJSON, ",")
			for _, part := range parts {
				tag := strings.TrimSpace(part)
				tag = strings.Trim(tag, "\"")
				tag = strings.Trim(tag, "'")
				// Skip if it looks like a JSON object or invalid JSON
				if strings.Contains(tag, "{") || strings.Contains(tag, "}") || 
				   strings.Contains(tag, "[") || strings.Contains(tag, "]") {
					continue
				}
				if tag != "" && tag != "[]" && tag != "[" && tag != "]" && tag != "{" && tag != "}" {
					tags = append(tags, tag)
				}
			}
			if len(tags) > 0 {
				return tags
			}
		}
		return []string{}
	}
	return tags
}

// formatTagsToJSON formats tags from []string to JSON array string
func formatTagsToJSON(tags []string) string {
	if len(tags) == 0 {
		return "[]"
	}
	data, err := json.Marshal(tags)
	if err != nil {
		return "[]"
	}
	return string(data)
}
