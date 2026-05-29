package memory

import (
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/session"
)

// StoreOptions configures cloud-aware behaviour for a SQLiteStore.
//
// DeviceID is the cross-device identifier (UUID) loaded from
// `~/.smara/device-id`; it is stamped on every cloud-synced row.
// CloudEnabled toggles cloud-field generation in `Save` and related
// write paths. When false, the store behaves identically to the
// historical local-only implementation.
type StoreOptions struct {
	DeviceID     string
	CloudEnabled bool
}

// SQLiteStore implements MemoryStore using SQLite or libSQL (Turso embedded
// replica). The choice of driver is determined by the DSN passed to
// NewSQLiteStoreWithDSN.
type SQLiteStore struct {
	db     *sql.DB
	dbPath string

	// Cloud Memory: optional cross-device metadata. These fields are zero-valued
	// (empty / false) in the local-only path so existing behaviour is preserved
	// byte-for-byte (Requirement 17.5).
	deviceID     string
	cloudEnabled bool
}

// NewSQLiteStore creates a new SQLite-backed memory store at the given file
// path. This is the historical local-only constructor and is preserved for
// backward compatibility — every existing CLI command path keeps working
// without modification.
//
// It delegates to NewSQLiteStoreWithDSN using a modernc.org/sqlite DSN with
// per-connection PRAGMAs applied before schema initialization.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	return NewSQLiteStoreWithDSN(sqliteDSN(dbPath), StoreOptions{})
}

// NewSQLiteStoreWithDSN opens a memory store against a dialect-aware DSN.
//
// Dialect detection rules:
//   - DSN starts with `libsql://` — uses the libSQL driver (registered by the
//     blank import in store_libsql.go).
//   - DSN contains `authToken=` or `syncUrl=` query parameters — also routed
//     to libSQL because these are libSQL embedded-replica markers.
//   - Otherwise — uses the local `sqlite` driver (modernc.org/sqlite).
//
// After opening the connection the constructor runs the standard schema
// initialization (`Init` → `migrate`), which is intentionally additive and
// dialect-agnostic so the same DDL works against both SQLite and libSQL
// (Requirements 6.1, 6.2). Cloud fields from `opts` are stored on the
// returned `*SQLiteStore` for use by cloud-aware write paths.
func NewSQLiteStoreWithDSN(dsn string, opts StoreOptions) (*SQLiteStore, error) {
	driverName := detectDialect(dsn)

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("gagal membuka database (%s): %w", driverName, err)
	}
	configureSQLiteDB(db, driverName)

	store := &SQLiteStore{
		db:           db,
		dbPath:       dsn,
		deviceID:     opts.DeviceID,
		cloudEnabled: opts.CloudEnabled,
	}
	if err := store.Init(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

// detectDialect picks the database/sql driver name based on DSN markers.
// Only the libSQL embedded-replica patterns route to the `libsql` driver;
// every other input falls back to the local `sqlite` driver, matching the
// historical behaviour of NewSQLiteStore. The libSQL driver registers itself
// via the blank import in store_libsql.go.
//
// Markers (any one matches):
//   - prefix `libsql://`
//   - DSN contains `authToken=` (libSQL auth marker, case-sensitive — matches
//     both `?authToken=...` and `&authToken=...`)
//   - DSN contains `syncUrl=` or `syncURL=` (embedded-replica sync URL marker)
func detectDialect(dsn string) string {
	switch {
	case strings.HasPrefix(dsn, "libsql://"):
		return "libsql"
	case strings.Contains(dsn, "authToken="):
		return "libsql"
	case strings.Contains(dsn, "syncUrl="), strings.Contains(dsn, "syncURL="):
		return "libsql"
	default:
		return "sqlite"
	}
}

func sqliteDSN(dbPath string) string {
	separator := "?"
	if strings.Contains(dbPath, "?") {
		separator = "&"
	}
	return dbPath + separator + "_pragma=journal_mode(WAL)&_pragma=busy_timeout(30000)&_pragma=synchronous(NORMAL)"
}

func configureSQLiteDB(db *sql.DB, driverName string) {
	if driverName != "sqlite" {
		return
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	_, _ = db.Exec("PRAGMA journal_mode=WAL")
	_, _ = db.Exec("PRAGMA busy_timeout=30000")
	_, _ = db.Exec("PRAGMA synchronous=NORMAL")
}

func execWithSQLiteRetry(db *sql.DB, query string, args ...interface{}) (sql.Result, error) {
	var err error
	for attempt := 0; attempt < 20; attempt++ {
		var result sql.Result
		result, err = db.Exec(query, args...)
		if err == nil || !isSQLiteBusyErr(err) {
			return result, err
		}
		delay := time.Duration(attempt+1) * 250 * time.Millisecond
		if delay > 2*time.Second {
			delay = 2 * time.Second
		}
		time.Sleep(delay)
	}
	return nil, err
}

func isSQLiteBusyErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "sqlite_busy") || strings.Contains(msg, "sqlite_locked") || strings.Contains(msg, "database is locked") || strings.Contains(msg, "database table is locked") || strings.Contains(msg, "database is busy")
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
			verbosity TEXT DEFAULT 'balanced',
			risk_tolerance TEXT DEFAULT 'balanced',
			primary_domains TEXT DEFAULT '[]',
			preferred_languages TEXT DEFAULT '[]',
			custom_patterns TEXT DEFAULT '{}',
			session_count INTEGER DEFAULT 0,
			total_prompts INTEGER DEFAULT 0,
			last_active DATETIME,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS skills (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			json TEXT NOT NULL,
			version INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS skill_feedback (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			skill_name TEXT NOT NULL,
			run_id TEXT,
			success INTEGER DEFAULT 0,
			notes TEXT,
			proposed_json TEXT,
			approved INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS nudge_schedules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			prompt_text TEXT NOT NULL,
			cron_expr TEXT,
			next_run DATETIME,
			enabled INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS nudge_tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT,
			prompt_text TEXT NOT NULL,
			last_state TEXT DEFAULT '{}',
			dismissed INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_skills_name ON skills(name)`,
		`CREATE INDEX IF NOT EXISTS idx_nudge_next_run ON nudge_schedules(next_run)`,
		`CREATE INDEX IF NOT EXISTS idx_nudge_tasks_created ON nudge_tasks(created_at)`,

		`CREATE TABLE IF NOT EXISTS graph_nodes (
			id INTEGER PRIMARY KEY,
			graph_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			label TEXT,
			type TEXT,
			source_file TEXT,
			source_line INTEGER,
			language TEXT,
			content TEXT,
			community INTEGER DEFAULT 0,
			god_score REAL DEFAULT 0,
			metadata TEXT DEFAULT '{}',
			UNIQUE(graph_id, node_id)
		)`,
		`CREATE TABLE IF NOT EXISTS graph_edges (
			id INTEGER PRIMARY KEY,
			graph_id TEXT NOT NULL,
			source TEXT NOT NULL,
			target TEXT NOT NULL,
			relation TEXT,
			confidence TEXT,
			confidence_score REAL,
			source_file TEXT,
			inferred_reason TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS graph_metadata (
			graph_id TEXT PRIMARY KEY,
			root_path TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			node_count INTEGER DEFAULT 0,
			edge_count INTEGER DEFAULT 0,
			languages TEXT DEFAULT '[]',
			corpus_hash TEXT DEFAULT '',
			version INTEGER DEFAULT 1
		)`,
		`CREATE INDEX IF NOT EXISTS idx_graph_nodes_graph_id ON graph_nodes(graph_id)`,
		`CREATE INDEX IF NOT EXISTS idx_graph_nodes_type ON graph_nodes(type)`,
		`CREATE INDEX IF NOT EXISTS idx_graph_nodes_language ON graph_nodes(language)`,
		`CREATE INDEX IF NOT EXISTS idx_graph_edges_graph_id ON graph_edges(graph_id)`,
		`CREATE INDEX IF NOT EXISTS idx_graph_edges_relation ON graph_edges(relation)`,
		`CREATE INDEX IF NOT EXISTS idx_graph_edges_confidence ON graph_edges(confidence)`,
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

	// Setup memory_links schema (for memory graph)
	if err := EnsureLinksSchema(s.db); err != nil {
		return fmt.Errorf("gagal setup memory_links: %w", err)
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
	cols := []string{"updated_at", "expires_at", "category_id", "metadata", "version", "cloud_id", "device_id", "content_hash"}
	for _, col := range cols {
		exists, err := columnExists("memories", col)
		if err != nil {
			return fmt.Errorf("gagal cek kolom %s: %w", col, err)
		}
		if !exists {
			var stmt string
			switch col {
			case "updated_at":
				// SQLite: ALTER TABLE ADD COLUMN with DEFAULT CURRENT_TIMESTAMP fails
				// if table already has data. Workaround: add without default, then UPDATE.
				if _, err := s.db.Exec("ALTER TABLE memories ADD COLUMN updated_at DATETIME"); err != nil {
					return fmt.Errorf("gagal menambahkan kolom %s ke tabel memories: %w", col, err)
				}
				_, _ = s.db.Exec("UPDATE memories SET updated_at = created_at WHERE updated_at IS NULL")
				stmt = ""
			case "expires_at":
				stmt = "ALTER TABLE memories ADD COLUMN expires_at DATETIME"
			case "category_id":
				stmt = "ALTER TABLE memories ADD COLUMN category_id INTEGER REFERENCES categories(id)"
			case "metadata":
				stmt = "ALTER TABLE memories ADD COLUMN metadata TEXT DEFAULT '{}'"
			case "version":
				stmt = "ALTER TABLE memories ADD COLUMN version INTEGER DEFAULT 1"
			case "cloud_id":
				// Cloud Memory: UUID v7 lintas-device untuk dedup; di-backfill saat enable cloud.
				// Dialect-agnostic DDL — jalan di SQLite (modernc) maupun libSQL (Turso).
				stmt = "ALTER TABLE memories ADD COLUMN cloud_id TEXT"
			case "device_id":
				// Cloud Memory: device asal write (UUID per-install dari ~/.smara/device-id).
				stmt = "ALTER TABLE memories ADD COLUMN device_id TEXT"
			case "content_hash":
				// Cloud Memory: sha256(content) untuk delta detection saat sync.
				stmt = "ALTER TABLE memories ADD COLUMN content_hash TEXT"
			}
			if stmt != "" {
				if _, err := s.db.Exec(stmt); err != nil {
					// Idempotency safety net: tolerate "duplicate column" errors that can
					// occur on libSQL/embedded-replica when the column was already added
					// remotely and synced to the local replica between our existence check
					// and the ALTER. The column-exists check above handles the common case;
					// this catch handles the race.
					if !isDuplicateColumnErr(err) {
						return fmt.Errorf("gagal menambahkan kolom %s ke tabel memories: %w", col, err)
					}
				}
			}
			// Verify the column was actually added
			existsNow, err := columnExists("memories", col)
			if err != nil {
				return fmt.Errorf("gagal verifikasi kolom %s: %w", col, err)
			}
			if !existsNow {
				return fmt.Errorf("kolom %s masih belum ada setelah migrasi", col)
			}
		}
	}

	// 2. Migrate tags from string to JSON array format (for existing records)
	// Check if tags column needs conversion
	rows, err := s.db.Query("SELECT id, tags FROM memories WHERE tags != '[]' AND tags != '' AND (tags NOT LIKE '[%]' OR tags IS NULL)")
	if err == nil {
		type tagUpdate struct {
			id   int64
			tags string
		}
		var updates []tagUpdate
		for rows.Next() {
			var id int64
			var tagsStr string
			if err := rows.Scan(&id, &tagsStr); err != nil {
				continue
			}
			if tagsStr != "" && tagsStr != "[]" {
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
				updates = append(updates, tagUpdate{id: id, tags: jsonTags})
			}
		}
		rows.Close()
		for _, update := range updates {
			_, _ = s.db.Exec("UPDATE memories SET tags = ? WHERE id = ?", update.tags, update.id)
		}
	}

	// 3. Ensure workspace_id exists in memories (for backward compatibility)
	existsWID, err := columnExists("memories", "workspace_id")
	if err != nil {
		return fmt.Errorf("gagal cek kolom workspace_id: %w", err)
	}
	if !existsWID {
		if _, err := s.db.Exec("ALTER TABLE memories ADD COLUMN workspace_id INTEGER DEFAULT 0"); err != nil {
			return fmt.Errorf("gagal menambahkan kolom workspace_id ke tabel memories: %w", err)
		}
	}

	// 4. Ensure missing columns exist in sessions
	sessionCols := map[string]string{
		"workspace_id": "INTEGER DEFAULT 0",
		"mcp_servers":  "TEXT DEFAULT '[]'",
		"memory_ids":   "TEXT DEFAULT '[]'",
		"context":      "TEXT DEFAULT ''",
		"is_agentic":   "INTEGER DEFAULT 0",
		"auto_resume":  "INTEGER DEFAULT 0",
		"created_at":   "DATETIME DEFAULT CURRENT_TIMESTAMP",
		"updated_at":   "DATETIME DEFAULT CURRENT_TIMESTAMP",
	}
	for col, definition := range sessionCols {
		exists, err := columnExists("sessions", col)
		if err != nil {
			return fmt.Errorf("gagal cek kolom sessions.%s: %w", col, err)
		}
		if !exists {
			var stmt string
			if col == "updated_at" || col == "created_at" {
				// SQLite workaround: CURRENT_TIMESTAMP is a non-constant default
				// and cannot be used with ALTER TABLE ADD COLUMN on existing data.
				if _, err := s.db.Exec(fmt.Sprintf("ALTER TABLE sessions ADD COLUMN %s DATETIME", col)); err != nil {
					return fmt.Errorf("gagal menambahkan kolom %s ke tabel sessions: %w", col, err)
				}
				_, _ = s.db.Exec(fmt.Sprintf("UPDATE sessions SET %s = datetime('now') WHERE %s IS NULL", col, col))
			} else {
				stmt = fmt.Sprintf("ALTER TABLE sessions ADD COLUMN %s %s", col, definition)
			}
			if stmt != "" {
				if _, err := s.db.Exec(stmt); err != nil {
					return fmt.Errorf("gagal menambahkan kolom %s ke tabel sessions: %w", col, err)
				}
			}
		}
	}

	// 5. Create indexes if they don't exist (CREATE INDEX IF NOT EXISTS handles this)
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_memories_workspace ON memories(workspace_id)")
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_memories_category ON memories(category_id)")
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_memories_updated ON memories(updated_at DESC)")
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_memories_expires ON memories(expires_at) WHERE expires_at IS NOT NULL")
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_sessions_updated ON sessions(updated_at DESC)")
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_categories_workspace ON categories(workspace_id)")
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_memory_versions_memory ON memory_versions(memory_id)")

	// Cloud Memory: partial unique index pada cloud_id. Partial WHERE clause memastikan
	// banyak row pre-cloud (cloud_id IS NULL) tidak melanggar UNIQUE; sekaligus enforce
	// dedup lintas-device untuk row yang sudah punya cloud_id (Requirements 6.1, 8.1).
	if _, err := s.db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_memories_cloud_id ON memories(cloud_id) WHERE cloud_id IS NOT NULL"); err != nil {
		return fmt.Errorf("gagal membuat unique index idx_memories_cloud_id: %w", err)
	}

	// 6. Cloud Memory: create cloud_databases table for 1:1 mapping workspace ↔ remote DB.
	// UNIQUE constraint on workspace_id enforces one Remote_Database per Workspace
	// (Requirements 5.1, 5.4, 6.3). Strictly additive — CREATE TABLE IF NOT EXISTS is
	// idempotent so re-running migrate() leaves an existing table untouched.
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS cloud_databases (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id INTEGER NOT NULL UNIQUE,
			provider TEXT NOT NULL,
			db_name TEXT NOT NULL,
			db_url TEXT NOT NULL,
			region TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_sync_at DATETIME,
			last_frame_no INTEGER DEFAULT 0,
			FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
		)`); err != nil {
		return fmt.Errorf("gagal membuat tabel cloud_databases: %w", err)
	}

	// 7. Cloud Memory: create cloud_conflicts table to record divergent versions
	// detected during pull. resolved_at IS NULL means the conflict still needs human
	// or policy-driven resolution; the partial index keeps that hot path cheap
	// (Requirements 4.4, 6.3, 12.4). DDL is strictly additive — CREATE TABLE/INDEX
	// IF NOT EXISTS is idempotent so re-running migrate() leaves existing data intact.
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS cloud_conflicts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			memory_id INTEGER NOT NULL,
			local_version INTEGER NOT NULL,
			remote_version INTEGER NOT NULL,
			local_content TEXT NOT NULL,
			remote_content TEXT NOT NULL,
			local_updated_at DATETIME NOT NULL,
			remote_updated_at DATETIME NOT NULL,
			detected_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			resolved_at DATETIME,
			resolution TEXT,
			FOREIGN KEY (memory_id) REFERENCES memories(id) ON DELETE CASCADE
		)`); err != nil {
		return fmt.Errorf("gagal membuat tabel cloud_conflicts: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_cloud_conflicts_unresolved ON cloud_conflicts(resolved_at) WHERE resolved_at IS NULL`); err != nil {
		return fmt.Errorf("gagal membuat index idx_cloud_conflicts_unresolved: %w", err)
	}

	// 8. Cloud Memory: extend sync_log with attempted_at and error columns for delta
	// tracking (Requirements 2.5, 6.3). The base CREATE TABLE in Init() already
	// declares status/synced_at; here we additively add columns the cloud worker
	// needs (attempted_at = first push attempt timestamp, error = last failure
	// message). Strictly additive ALTERs keep the migration idempotent and safe
	// for both fresh installs and pre-existing local-only databases.
	syncLogCols := []struct {
		name string
		ddl  string
	}{
		{"attempted_at", "ALTER TABLE sync_log ADD COLUMN attempted_at DATETIME"},
		{"error", "ALTER TABLE sync_log ADD COLUMN error TEXT"},
	}
	for _, c := range syncLogCols {
		exists, err := columnExists("sync_log", c.name)
		if err != nil {
			return fmt.Errorf("gagal cek kolom sync_log.%s: %w", c.name, err)
		}
		if exists {
			continue
		}
		if _, err := s.db.Exec(c.ddl); err != nil {
			if !isDuplicateColumnErr(err) {
				return fmt.Errorf("gagal menambahkan kolom sync_log.%s: %w", c.name, err)
			}
		}
	}
	// Index status + attempted_at supports the worker poll
	// `WHERE status='pending' ORDER BY attempted_at ASC`. CREATE INDEX IF NOT EXISTS
	// makes the migration idempotent.
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_sync_log_status_attempted ON sync_log(status, attempted_at)`); err != nil {
		return fmt.Errorf("gagal membuat index idx_sync_log_status_attempted: %w", err)
	}

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
//
// When the store was opened with cloud enabled (StoreOptions.CloudEnabled == true),
// SaveWithOptions also stamps the cross-device cloud fields on the new row:
//
//   - cloud_id     — fresh UUID v7 (monotonic) so the row can be deduped across devices
//   - device_id    — s.deviceID, identifying which install authored the row
//   - content_hash — lowercase hex sha256(content), used as the delta marker
//
// In the cloud-enabled path it additionally inserts a `sync_log` row with
// status='pending' and attempted_at=now, so the background sync worker can find
// the new write and replicate it (Requirement 2.5). The local memories INSERT
// commits to the WAL before this function returns, independent of any remote
// acknowledgement (Requirements 2.1, 2.2).
//
// When cloud is disabled the behaviour is identical to the historical local-only
// implementation: cloud columns stay NULL and no sync_log row is written
// (Requirement 17.1, 17.2).
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

	// Cloud Memory: generate per-write metadata only when the store was opened
	// with cloud enabled. Local-only mode leaves these as NULL strings so the
	// existing INSERT shape and behaviour remain identical (Requirement 17.1).
	var cloudID, deviceID, contentHash sql.NullString
	if s.cloudEnabled {
		// UUID v7 carries an embedded timestamp so cloud_ids are monotonic per
		// device, which keeps replication ordering stable across nodes.
		uid, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("gagal generate cloud_id (UUID v7): %w", err)
		}
		sum := sha256.Sum256([]byte(content))
		cloudID = sql.NullString{String: uid.String(), Valid: true}
		deviceID = sql.NullString{String: s.deviceID, Valid: s.deviceID != ""}
		contentHash = sql.NullString{String: hex.EncodeToString(sum[:]), Valid: true}
	}

	result, err := execWithSQLiteRetry(s.db,
		`INSERT INTO memories
		(content, embedding, tags, source, metadata, created_at, updated_at, expires_at, category_id, version, workspace_id, cloud_id, device_id, content_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		content, embBlob, tagsJSON, source, metadataJSON, now, now, expAt, catID, 1, wID, cloudID, deviceID, contentHash,
	)
	if err != nil {
		return nil, fmt.Errorf("gagal menyimpan memory: %w", err)
	}

	id, _ := result.LastInsertId()

	// Create memory version
	_, _ = execWithSQLiteRetry(s.db,
		`INSERT INTO memory_versions (memory_id, content, metadata, changed_by, reason, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, content, metadataJSON, "system", "initial creation", now,
	)

	// Cloud Memory: queue the new row for background replication. We INSERT into
	// sync_log with status='pending' so the sync worker can pick it up on its
	// next tick (Requirement 2.5). delta_hash is set to the row's content_hash
	// so the worker has the marker it needs without recomputing sha256.
	// Failure here is non-fatal: the local commit already succeeded, and the
	// next reconcile pass will re-detect the unsynced row via cloud_id absence
	// from sync_log. Logging keeps surprises visible without breaking writes.
	if s.cloudEnabled {
		if _, syncErr := execWithSQLiteRetry(s.db,
			`INSERT INTO sync_log (memory_id, delta_hash, status, attempted_at) VALUES (?, ?, 'pending', CURRENT_TIMESTAMP)`,
			id, contentHash.String,
		); syncErr != nil {
			fmt.Printf("Warning: gagal menulis sync_log untuk memory id=%d: %v\n", id, syncErr)
		}
	}

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
		var workspaceID sql.NullInt64
		if err := rows.Scan(&m.ID, &workspaceID, &m.Content, &tagsJSON, &m.Source, &m.CreatedAt, &m.UpdatedAt, &expiresAt, &categoryID, &metadataJSON, &m.Version); err != nil {
			return nil, fmt.Errorf("gagal scan memory: %w", err)
		}
		if workspaceID.Valid {
			m.WorkspaceID = workspaceID.Int64
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
	var categoryID, workspaceID sql.NullInt64

	row := s.db.QueryRow(
		`SELECT id, workspace_id, content, embedding, tags, source, created_at, updated_at, expires_at, category_id, metadata, version 
		 FROM memories WHERE id = ?`,
		id,
	)
	var embBlob []byte
	if err := row.Scan(&m.ID, &workspaceID, &m.Content, &embBlob, &tagsJSON, &m.Source, &m.CreatedAt, &m.UpdatedAt, &expiresAt, &categoryID, &metadataJSON, &m.Version); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("gagal membaca memory: %w", err)
	}

	if workspaceID.Valid {
		m.WorkspaceID = workspaceID.Int64
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

// DB returns the underlying sql.DB connection.
func (s *SQLiteStore) DB() *sql.DB {
	return s.db
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
		var workspaceID sql.NullInt64
		var embBlob []byte

		if err := rows.Scan(&m.ID, &workspaceID, &m.Content, &embBlob, &tagsJSON, &m.Source, &m.CreatedAt, &m.UpdatedAt, &expiresAt, &categoryID, &metadataJSON, &m.Version); err != nil {
			return nil, 0, fmt.Errorf("gagal scan memory: %w", err)
		}

		if workspaceID.Valid {
			m.WorkspaceID = workspaceID.Int64
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

// --- Archive Operations ---

// ArchiveMemory soft-archives a memory by ID.
func (s *SQLiteStore) ArchiveMemory(id int64) error {
	now := time.Now()
	result, err := s.db.Exec(
		"UPDATE memories SET is_archived = 1, archived_at = ? WHERE id = ?",
		now, id,
	)
	if err != nil {
		return fmt.Errorf("gagal mengarsipkan memory: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("memory tidak ditemukan: %d", id)
	}
	return nil
}

// UnarchiveMemory restores an archived memory.
func (s *SQLiteStore) UnarchiveMemory(id int64) error {
	result, err := s.db.Exec(
		"UPDATE memories SET is_archived = 0, archived_at = NULL WHERE id = ?",
		id,
	)
	if err != nil {
		return fmt.Errorf("gagal memulihkan memory: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("memory tidak ditemukan: %d", id)
	}
	return nil
}

// ListArchivedMemories returns archived memories for a workspace.
func (s *SQLiteStore) ListArchivedMemories(workspaceID int64, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(
		"SELECT id, workspace_id, content, tags, source, is_archived, archived_at, created_at FROM memories WHERE is_archived = 1 AND (workspace_id = ? OR workspace_id IS NULL) ORDER BY archived_at DESC LIMIT ?",
		workspaceID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("gagal query archived memories: %w", err)
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var m Memory
		var archivedAt sql.NullTime
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.Content, &m.Tags, &m.Source, &m.IsArchived, &archivedAt, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("gagal scan memory: %w", err)
		}
		if archivedAt.Valid {
			m.ArchivedAt = &archivedAt.Time
		}
		memories = append(memories, m)
	}
	return memories, nil
}

// DeleteArchivedMemory permanently deletes an archived memory.
func (s *SQLiteStore) DeleteArchivedMemory(id int64) error {
	result, err := s.db.Exec("DELETE FROM memories WHERE id = ? AND is_archived = 1", id)
	if err != nil {
		return fmt.Errorf("gagal menghapus memory: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("archived memory tidak ditemukan: %d", id)
	}
	return nil
}

// ArchiveWorkspace soft-archives a workspace by ID.
func (s *SQLiteStore) ArchiveWorkspace(id int64) error {
	now := time.Now()
	result, err := s.db.Exec(
		"UPDATE workspaces SET is_archived = 1, archived_at = ? WHERE id = ?",
		now, id,
	)
	if err != nil {
		return fmt.Errorf("gagal mengarsipkan workspace: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("workspace tidak ditemukan: %d", id)
	}
	return nil
}

// UnarchiveWorkspace restores an archived workspace.
func (s *SQLiteStore) UnarchiveWorkspace(id int64) error {
	result, err := s.db.Exec(
		"UPDATE workspaces SET is_archived = 0, archived_at = NULL WHERE id = ?",
		id,
	)
	if err != nil {
		return fmt.Errorf("gagal memulihkan workspace: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("workspace tidak ditemukan: %d", id)
	}
	return nil
}

// ListArchivedWorkspaces returns all archived workspaces.
func (s *SQLiteStore) ListArchivedWorkspaces() ([]Workspace, error) {
	rows, err := s.db.Query("SELECT id, name, path, is_archived, archived_at, created_at FROM workspaces WHERE is_archived = 1 ORDER BY archived_at DESC")
	if err != nil {
		return nil, fmt.Errorf("gagal query archived workspaces: %w", err)
	}
	defer rows.Close()

	var workspaces []Workspace
	for rows.Next() {
		var w Workspace
		var archivedAt sql.NullTime
		if err := rows.Scan(&w.ID, &w.Name, &w.Path, &w.IsArchived, &archivedAt, &w.CreatedAt); err != nil {
			return nil, err
		}
		if archivedAt.Valid {
			w.ArchivedAt = &archivedAt.Time
		}
		workspaces = append(workspaces, w)
	}
	return workspaces, nil
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

// isDuplicateColumnErr reports whether err originates from an ALTER TABLE ADD COLUMN
// that targets a column name already present in the table. SQLite (modernc.org/sqlite)
// surfaces this as "duplicate column name: <name>"; libSQL/Turso replicas surface a
// similar message. We match on the canonical substring so the migration stays idempotent
// even when an ALTER is racing against schema replication on an embedded replica.
func isDuplicateColumnErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column")
}
