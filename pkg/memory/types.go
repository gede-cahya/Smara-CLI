// Package memory provides SQLite-backed memory storage with vector search for Smara.
package memory

import (
	"encoding/json"
	"time"

	"github.com/gede-cahya/Smara-CLI/pkg/llm"
)

// Memory represents a single piece of stored knowledge.
type Memory struct {
	ID          int64     `json:"id"`
	WorkspaceID int64     `json:"workspace_id"`
	Content     string    `json:"content"`
	Embedding   []float32 `json:"-"` // stored as BLOB
	Tags        string    `json:"tags"`
	Source      string    `json:"source"` // e.g., "agent:worker-1", "user", "sync"
	CreatedAt   time.Time `json:"created_at"`
}

// Workspace represents a project-specific container.
type Workspace struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
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

// MessageToJSON serializes an llm.Message to JSON string for storage.
func MessageToJSON(m llm.Message) string {
	data, _ := json.Marshal(m)
	return string(data)
}

// MessageFromJSON deserializes an llm.Message from JSON string.
func MessageFromJSON(s string) (llm.Message, error) {
	var m llm.Message
	err := json.Unmarshal([]byte(s), &m)
	return m, err
}

// MemoryStore defines the interface for memory operations.
type MemoryStore interface {
	// Init initializes the database schema.
	Init() error

	// Save stores a new memory entry.
	Save(content, tags, source string, workspaceID int64, embedding []float32) (*Memory, error)

	// Search finds memories similar to the given embedding within a workspace.
	Search(embedding []float32, workspaceID int64, topK int) ([]SearchResult, error)

	// List returns recent memories for a workspace.
	List(workspaceID int64, limit int) ([]Memory, error)

	// Workspace Operations
	CreateWorkspace(name, path string) (*Workspace, error)
	GetWorkspace(id int64) (*Workspace, error)
	GetWorkspaceByName(name string) (*Workspace, error)
	ListWorkspaces() ([]Workspace, error)
	DeleteWorkspace(id int64) error

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
