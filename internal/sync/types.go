// Package sync implements local background sync for shared team memory.
package sync

import "time"

// SyncDelta represents a batch of memory changes to sync.
type SyncDelta struct {
	ID        string       `json:"id"`
	Memories  []DeltaEntry `json:"memories"`
	Source    string       `json:"source"` // team member identifier
	CreatedAt time.Time    `json:"created_at"`
}

// DeltaEntry represents a single memory entry in a sync delta.
type DeltaEntry struct {
	MemoryID  int64   `json:"memory_id"`
	Content   string  `json:"content"`
	Tags      string  `json:"tags"`
	Hash      string  `json:"hash"`
}

// SyncConfig configures the sync daemon.
type SyncConfig struct {
	SyncDir      string `json:"sync_dir"`
	IntervalMin  int    `json:"interval_min"`
	Enabled      bool   `json:"enabled"`
}
