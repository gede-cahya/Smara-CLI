package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cahya/smara/internal/memory"
)

// Daemon is the background sync process that exports/imports memory deltas.
type Daemon struct {
	config   SyncConfig
	memStore memory.MemoryStore
	cancel   context.CancelFunc
	done     chan struct{}
}

// NewDaemon creates a new sync daemon.
func NewDaemon(config SyncConfig, memStore memory.MemoryStore) *Daemon {
	return &Daemon{
		config:   config,
		memStore: memStore,
		done:     make(chan struct{}),
	}
}

// Start begins the background sync loop.
func (d *Daemon) Start(ctx context.Context) {
	ctx, d.cancel = context.WithCancel(ctx)

	go func() {
		defer close(d.done)

		interval := time.Duration(d.config.IntervalMin) * time.Minute
		if interval <= 0 {
			interval = 15 * time.Minute
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Run initial sync
		d.sync()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				d.sync()
			}
		}
	}()
}

// Stop gracefully shuts down the daemon.
func (d *Daemon) Stop() {
	if d.cancel != nil {
		d.cancel()
	}
	<-d.done
}

// sync performs one cycle of export + import.
func (d *Daemon) sync() {
	d.exportDeltas()
	d.importDeltas()
}

// exportDeltas writes unsynced memories as JSON delta files.
func (d *Daemon) exportDeltas() {
	memories, err := d.memStore.GetUnsyncedMemories()
	if err != nil || len(memories) == 0 {
		return
	}

	hostname, _ := os.Hostname()
	delta := SyncDelta{
		ID:        fmt.Sprintf("%s_%d", hostname, time.Now().UnixNano()),
		Source:    hostname,
		CreatedAt: time.Now(),
	}

	for _, m := range memories {
		hash := hashContent(m.Content)
		delta.Memories = append(delta.Memories, DeltaEntry{
			MemoryID: m.ID,
			Content:  m.Content,
			Tags:     m.Tags,
			Hash:     hash,
		})
	}

	// Write delta file
	outDir := filepath.Join(d.config.SyncDir, "outbox")
	os.MkdirAll(outDir, 0o755)

	filename := filepath.Join(outDir, delta.ID+".json")
	data, err := json.MarshalIndent(delta, "", "  ")
	if err != nil {
		return
	}

	if err := os.WriteFile(filename, data, 0o644); err != nil {
		return
	}

	// Mark as synced
	for _, m := range memories {
		d.memStore.MarkSynced(m.ID, hashContent(m.Content))
	}
}

// importDeltas reads delta files from inbox and merges them into local memory.
func (d *Daemon) importDeltas() {
	inboxDir := filepath.Join(d.config.SyncDir, "inbox")
	os.MkdirAll(inboxDir, 0o755)

	entries, err := os.ReadDir(inboxDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		filePath := filepath.Join(inboxDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var delta SyncDelta
		if err := json.Unmarshal(data, &delta); err != nil {
			continue
		}

		// Import each memory entry
		for _, de := range delta.Memories {
			tags := de.Tags
			if tags == "" {
				tags = "synced"
			}
			d.memStore.Save(de.Content, tags, "sync:"+delta.Source, nil)
		}

		// Move processed file to done directory
		doneDir := filepath.Join(d.config.SyncDir, "done")
		os.MkdirAll(doneDir, 0o755)
		os.Rename(filePath, filepath.Join(doneDir, entry.Name()))
	}
}

func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:8])
}
