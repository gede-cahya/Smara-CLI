package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/autonomy"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/gede-cahya/Smara-CLI/internal/ninedrive"
)

// Daemon is the background sync process that runs an autonomy loop
// for memory delta synchronization.
type Daemon struct {
	config   SyncConfig
	memStore memory.MemoryStore
	engine   *autonomy.Engine
	cancel   context.CancelFunc
	done     chan struct{}
}

// NewDaemon creates a new sync daemon using an autonomy loop.
func NewDaemon(config SyncConfig, memStore memory.MemoryStore) *Daemon {
	return &Daemon{
		config:   config,
		memStore: memStore,
		engine:   autonomy.NewEngine(),
		done:     make(chan struct{}),
	}
}

// Start begins the background autonomy-driven sync loop.
func (d *Daemon) Start(ctx context.Context) {
	ctx, d.cancel = context.WithCancel(ctx)

	interval := time.Duration(d.config.IntervalMin) * time.Minute
	if interval <= 0 {
		interval = 15 * time.Minute
	}

	// Register sync-specific timeframes
	d.engine.AddTimeframe(autonomy.Timeframe{
		Name:        "memory_export",
		Interval:    interval,
		Description: "Export unsynced memory deltas",
		Enabled:     true,
	})
	d.engine.AddTimeframe(autonomy.Timeframe{
		Name:        "memory_import",
		Interval:    interval,
		Description: "Import incoming memory deltas",
		Enabled:     true,
	})

	if d.config.NineDriveEnabled {
		d.engine.AddTimeframe(autonomy.Timeframe{
			Name:        "ninedrive_push",
			Interval:    interval,
			Description: "Push exported deltas to 9Drive",
			Enabled:     true,
		})
		d.engine.AddTimeframe(autonomy.Timeframe{
			Name:        "ninedrive_pull",
			Interval:    interval,
			Description: "Pull new deltas from 9Drive",
			Enabled:     true,
		})
	}

	// Register checkers (Observe → Think)
	d.engine.RegisterChecker("memory_export", func() (bool, map[string]interface{}) {
		memories, err := d.memStore.GetUnsyncedMemories()
		if err != nil || len(memories) == 0 {
			return false, nil
		}
		return true, map[string]interface{}{
			"count": len(memories),
		}
	})
	d.engine.RegisterChecker("memory_import", func() (bool, map[string]interface{}) {
		inboxDir := filepath.Join(d.config.SyncDir, "inbox")
		entries, err := os.ReadDir(inboxDir)
		if err != nil {
			return false, nil
		}
		count := 0
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
				count++
			}
		}
		if count == 0 {
			return false, nil
		}
		return true, map[string]interface{}{
			"count": count,
		}
	})

	if d.config.NineDriveEnabled {
		d.engine.RegisterChecker("ninedrive_push", func() (bool, map[string]interface{}) {
			outDir := filepath.Join(d.config.SyncDir, "outbox")
			entries, err := os.ReadDir(outDir)
			if err != nil {
				return false, nil
			}
			count := 0
			for _, e := range entries {
				if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
					count++
				}
			}
			if count == 0 {
				return false, nil
			}
			return true, map[string]interface{}{"count": count}
		})
		d.engine.RegisterChecker("ninedrive_pull", func() (bool, map[string]interface{}) {
			return true, nil
		})
	}

	// Register executors (Act)
	d.engine.RegisterExecutor("memory_export", func(ctx context.Context, context map[string]interface{}) error {
		return d.exportDeltas()
	})
	d.engine.RegisterExecutor("memory_import", func(ctx context.Context, context map[string]interface{}) error {
		return d.importDeltas()
	})

	if d.config.NineDriveEnabled {
		d.engine.RegisterExecutor("ninedrive_push", func(ctx context.Context, context map[string]interface{}) error {
			return d.ninedrivePushDeltas(ctx)
		})
		d.engine.RegisterExecutor("ninedrive_pull", func(ctx context.Context, context map[string]interface{}) error {
			return d.ninedrivePullDeltas(ctx)
		})
	}

	go func() {
		defer close(d.done)
		d.engine.Start(ctx)
		<-ctx.Done()
	}()
}

// Stop gracefully shuts down the daemon.
func (d *Daemon) Stop() {
	if d.cancel != nil {
		d.cancel()
	}
	if d.engine != nil {
		d.engine.Stop()
	}
	<-d.done
}

// GetState returns the current autonomy loop state.
func (d *Daemon) GetState() autonomy.LoopState {
	if d.engine != nil {
		return d.engine.GetState()
	}
	return autonomy.StateIdle
}

// GetMetrics returns execution metrics.
func (d *Daemon) GetMetrics() map[string]int {
	if d.engine != nil {
		return d.engine.GetMetrics()
	}
	return nil
}

// exportDeltas writes unsynced memories as JSON delta files.
func (d *Daemon) exportDeltas() error {
	memories, err := d.memStore.GetUnsyncedMemories()
	if err != nil || len(memories) == 0 {
		return nil
	}

	hostname, _ := os.Hostname()
	delta := SyncDelta{
		ID:        fmt.Sprintf("%s_%d", hostname, time.Now().UnixNano()),
		Source:    hostname,
		CreatedAt: time.Now(),
	}

	for _, m := range memories {
		hash := hashContent(m.Content)
		tagsJSON, _ := json.Marshal(m.Tags)
		delta.Memories = append(delta.Memories, DeltaEntry{
			MemoryID: m.ID,
			Content:  m.Content,
			Tags:     string(tagsJSON),
			Hash:     hash,
		})
	}

	outDir := filepath.Join(d.config.SyncDir, "outbox")
	os.MkdirAll(outDir, 0o755)

	filename := filepath.Join(outDir, delta.ID+".json")
	data, err := json.MarshalIndent(delta, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(filename, data, 0o644); err != nil {
		return err
	}

	for _, m := range memories {
		d.memStore.MarkSynced(m.ID, hashContent(m.Content))
	}
	return nil
}

// importDeltas reads delta files from inbox and merges them into local memory.
func (d *Daemon) importDeltas() error {
	inboxDir := filepath.Join(d.config.SyncDir, "inbox")
	os.MkdirAll(inboxDir, 0o755)

	entries, err := os.ReadDir(inboxDir)
	if err != nil {
		return err
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

		for _, de := range delta.Memories {
			tags := de.Tags
			if tags == "" {
				tags = "synced"
			}
			d.memStore.Save(de.Content, tags, "sync:"+delta.Source, 0, nil)
		}

		doneDir := filepath.Join(d.config.SyncDir, "done")
		os.MkdirAll(doneDir, 0o755)
		os.Rename(filePath, filepath.Join(doneDir, entry.Name()))
	}
	return nil
}

func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:8])
}

// ninedrivePushDeltas uploads exported JSON delta files from outbox to 9Drive.
func (d *Daemon) ninedrivePushDeltas(ctx context.Context) error {
	if !d.config.NineDriveEnabled || d.config.NineDriveAPIKey == "" {
		return nil
	}

	outDir := filepath.Join(d.config.SyncDir, "outbox")
	entries, err := os.ReadDir(outDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	client := ninedrive.NewClient(d.config.NineDriveBaseURL, d.config.NineDriveAPIKey)
	doneDir := filepath.Join(d.config.SyncDir, "done")
	os.MkdirAll(doneDir, 0o755)

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		filePath := filepath.Join(outDir, entry.Name())
		_, err := client.UploadFile(ctx, filePath, "application/json")
		if err != nil {
			fmt.Fprintf(os.Stderr, "[9DRIVE-SYNC] Gagal upload %s: %v\n", entry.Name(), err)
			continue
		}

		destPath := filepath.Join(doneDir, entry.Name())
		os.Remove(destPath)
		if err := os.Rename(filePath, destPath); err != nil {
			data, err2 := os.ReadFile(filePath)
			if err2 == nil {
				if err3 := os.WriteFile(destPath, data, 0o644); err3 == nil {
					os.Remove(filePath)
				}
			}
		}
	}
	return nil
}

// ninedrivePullDeltas downloads new delta files from 9Drive to the local inbox.
func (d *Daemon) ninedrivePullDeltas(ctx context.Context) error {
	if !d.config.NineDriveEnabled || d.config.NineDriveAPIKey == "" {
		return nil
	}

	client := ninedrive.NewClient(d.config.NineDriveBaseURL, d.config.NineDriveAPIKey)
	files, err := client.ListFiles(ctx, "", "")
	if err != nil {
		return err
	}

	inboxDir := filepath.Join(d.config.SyncDir, "inbox")
	outDir := filepath.Join(d.config.SyncDir, "outbox")
	doneDir := filepath.Join(d.config.SyncDir, "done")
	os.MkdirAll(inboxDir, 0o755)
	os.MkdirAll(outDir, 0o755)
	os.MkdirAll(doneDir, 0o755)

	for _, f := range files {
		if !strings.HasSuffix(strings.ToLower(f.Name), ".json") {
			continue
		}

		inboxPath := filepath.Join(inboxDir, f.Name)
		outboxPath := filepath.Join(outDir, f.Name)
		donePath := filepath.Join(doneDir, f.Name)

		if fileExists(inboxPath) || fileExists(outboxPath) || fileExists(donePath) {
			continue
		}

		err := client.DownloadFile(ctx, f.ID, inboxPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[9DRIVE-SYNC] Gagal download %s: %v\n", f.Name, err)
			continue
		}
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

