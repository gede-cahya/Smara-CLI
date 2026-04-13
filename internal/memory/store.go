package memory

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteStore implements MemoryStore using SQLite.
type SQLiteStore struct {
	db     *sql.DB
	dbPath string
}

// NewSQLiteStore creates a new SQLite-backed memory store.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
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
	schema := `
	CREATE TABLE IF NOT EXISTS memories (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		content TEXT NOT NULL,
		embedding BLOB,
		tags TEXT DEFAULT '',
		source TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS sync_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		memory_id INTEGER NOT NULL,
		delta_hash TEXT NOT NULL,
		status TEXT DEFAULT 'pending',
		synced_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (memory_id) REFERENCES memories(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_memories_tags ON memories(tags);
	CREATE INDEX IF NOT EXISTS idx_memories_source ON memories(source);
	CREATE INDEX IF NOT EXISTS idx_sync_status ON sync_log(status);
	`

	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("gagal membuat schema: %w", err)
	}
	return nil
}

// Save stores a new memory with optional embedding.
func (s *SQLiteStore) Save(content, tags, source string, embedding []float32) (*Memory, error) {
	var embBlob []byte
	if len(embedding) > 0 {
		embBlob = float32ToBytes(embedding)
	}

	result, err := s.db.Exec(
		"INSERT INTO memories (content, embedding, tags, source) VALUES (?, ?, ?, ?)",
		content, embBlob, tags, source,
	)
	if err != nil {
		return nil, fmt.Errorf("gagal menyimpan memory: %w", err)
	}

	id, _ := result.LastInsertId()
	return &Memory{
		ID:        id,
		Content:   content,
		Embedding: embedding,
		Tags:      tags,
		Source:    source,
		CreatedAt: time.Now(),
	}, nil
}

// List returns the most recent memories.
func (s *SQLiteStore) List(limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.Query(
		"SELECT id, content, tags, source, created_at FROM memories ORDER BY created_at DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("gagal query memories: %w", err)
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var m Memory
		if err := rows.Scan(&m.ID, &m.Content, &m.Tags, &m.Source, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("gagal scan memory: %w", err)
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
		SELECT m.id, m.content, m.embedding, m.tags, m.source, m.created_at
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
		if err := rows.Scan(&m.ID, &m.Content, &embBlob, &m.Tags, &m.Source, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("gagal scan memory: %w", err)
		}
		if len(embBlob) > 0 {
			m.Embedding = bytesToFloat32(embBlob)
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

// Search is implemented in search.go
// Included here to satisfy the MemoryStore interface check.
func (s *SQLiteStore) Search(embedding []float32, topK int) ([]SearchResult, error) {
	return searchByEmbedding(s.db, embedding, topK)
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
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
