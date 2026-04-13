// Package memory provides SQLite-backed memory storage with vector search for Smara.
package memory

import "time"

// Memory represents a single piece of stored knowledge.
type Memory struct {
	ID        int64     `json:"id"`
	Content   string    `json:"content"`
	Embedding []float32 `json:"-"` // stored as BLOB
	Tags      string    `json:"tags"`
	Source    string    `json:"source"` // e.g., "agent:worker-1", "user", "sync"
	CreatedAt time.Time `json:"created_at"`
}

// SyncEntry represents a synchronization log entry.
type SyncEntry struct {
	ID        int64     `json:"id"`
	MemoryID  int64     `json:"memory_id"`
	DeltaHash string    `json:"delta_hash"`
	Status    SyncStatus `json:"status"`
	SyncedAt  time.Time `json:"synced_at"`
}

// SyncStatus represents the state of a sync operation.
type SyncStatus string

const (
	SyncPending  SyncStatus = "pending"
	SyncComplete SyncStatus = "complete"
	SyncFailed   SyncStatus = "failed"
)

// SearchResult represents a memory search result with similarity score.
type SearchResult struct {
	Memory     Memory  `json:"memory"`
	Similarity float64 `json:"similarity"`
}

// MemoryStore defines the interface for memory operations.
type MemoryStore interface {
	// Init initializes the database schema.
	Init() error

	// Save stores a new memory entry.
	Save(content, tags, source string, embedding []float32) (*Memory, error)

	// Search finds memories similar to the given embedding.
	Search(embedding []float32, topK int) ([]SearchResult, error)

	// List returns recent memories.
	List(limit int) ([]Memory, error)

	// Delete removes a memory by ID.
	Delete(id int64) error

	// Clear removes all memories.
	Clear() error

	// GetUnsyncedMemories returns memories that haven't been synced yet.
	GetUnsyncedMemories() ([]Memory, error)

	// MarkSynced marks a memory as successfully synced.
	MarkSynced(memoryID int64, deltaHash string) error

	// Close closes the database connection.
	Close() error
}
