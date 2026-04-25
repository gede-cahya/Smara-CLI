// Package memory provides SQLite-backed memory storage with vector search for Smara.
package memory

import (
	"encoding/json"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/llm"
)

// Memory represents a single piece of stored knowledge.
type Memory struct {
	ID          int64                  `json:"id"`
	WorkspaceID int64                  `json:"workspace_id"`
	CategoryID  *int64                 `json:"category_id,omitempty"`
	Content     string                 `json:"content"`
	Embedding   []float32              `json:"-"` // stored as BLOB
	Tags        []string               `json:"tags"` // CHANGED: from string to []string
	Source      string                 `json:"source"` // e.g., "agent:worker-1", "user", "sync"
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	ExpiresAt   *time.Time             `json:"expires_at,omitempty"`
	Version     int                    `json:"version"`
}

// Workspace represents a project-specific container.
type Workspace struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
}

// Category represents a memory category/folder within a workspace.
type Category struct {
	ID          int64      `json:"id"`
	WorkspaceID int64      `json:"workspace_id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	ParentID    *int64     `json:"parent_id,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// MemoryVersion represents a historical version of a memory.
type MemoryVersion struct {
	ID        int64     `json:"id"`
	MemoryID  int64     `json:"memory_id"`
	Content   string    `json:"content"`
	Metadata  string    `json:"metadata"`
	ChangedBy string    `json:"changed_by"`
	Reason    string    `json:"reason,omitempty"`
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
	Score      float64 `json:"score,omitempty"` // Combined score for hybrid search
}

// SearchFilters defines filters for searching memories.
type SearchFilters struct {
	Tags       []string
	Sources    []string
	DateFrom   *time.Time
	DateTo     *time.Time
	CategoryID *int64
	MinScore   float64
}

// MemoryFilters defines filters for listing memories with pagination.
type MemoryFilters struct {
	SearchFilters
	Limit  int
	Offset int
	SortBy string // "created_at", "updated_at", "relevance"
	SortDir string // "ASC", "DESC"
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

	// UpdateMemory updates an existing memory.
	UpdateMemory(id int64, updates map[string]interface{}) error

	// GetMemoryByID retrieves a memory by ID.
	GetMemoryByID(id int64) (*Memory, error)

	// Search finds memories similar to the given embedding within a workspace.
	Search(embedding []float32, workspaceID int64, topK int) ([]SearchResult, error)

	// SearchHybrid combines vector and full-text search.
	SearchHybrid(query string, embedding []float32, workspaceID int64, topK int) ([]SearchResult, error)

	// SearchFullText searches memories using full-text search.
	SearchFullText(query string, workspaceID int64, filters MemoryFilters) ([]Memory, error)

	// List returns recent memories for a workspace.
	List(workspaceID int64, limit int) ([]Memory, error)

	// ListMemoriesWithFilters returns memories with advanced filtering.
	ListMemoriesWithFilters(workspaceID int64, filters MemoryFilters) ([]Memory, int, error)

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

	// CreateCategory creates a new category.
	CreateCategory(name, description string, workspaceID int64, parentID *int64) (*Category, error)

	// GetCategory retrieves a category by ID.
	GetCategory(id int64) (*Category, error)

	// ListCategories returns categories for a workspace.
	ListCategories(workspaceID int64, includeSubcategories bool) ([]Category, error)

	// UpdateCategory updates a category.
	UpdateCategory(id int64, updates map[string]interface{}) error

	// DeleteCategory removes a category.
	DeleteCategory(id int64, reassignTo *int64) error

	// DeleteExpiredMemories removes memories past their expiration date.
	DeleteExpiredMemories() (int, error)

	// SetRetentionPolicy sets TTL for a workspace.
	SetRetentionPolicy(workspaceID int64, ttlDays int) error

	// ExportMemories exports memories to JSON or Markdown.
	ExportMemories(workspaceID int64, options ExportOptions) ([]byte, string, error)

	// ImportMemories imports memories from JSON or Markdown.
	ImportMemories(workspaceID int64, data []byte, format ExportFormat, options ImportOptions) (int, error)

	// GetMemoryVersions returns version history for a memory.
	GetMemoryVersions(memoryID int64) ([]MemoryVersion, error)

	// RollbackMemory reverts a memory to a previous version.
	RollbackMemory(memoryID int64, versionID int64) error

	// Close closes the database connection.
	Close() error
}
