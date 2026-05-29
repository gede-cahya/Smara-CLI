package imageflow

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateRejectsOversizedImageNode(t *testing.T) {
	wf := &Workflow{
		Version: "1.0",
		Name:    "oversized",
		Nodes: []Node{{
			ID:   "gen1",
			Type: "generate_image",
			Config: map[string]interface{}{
				"width":  4096,
				"height": 1024,
			},
		}},
	}
	if err := Validate(wf); err == nil {
		t.Fatalf("expected validation error for oversized image node")
	}
}

func TestCleanupAssetsRemovesOldArchivedOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldTime := time.Now().Add(-48 * time.Hour).Format(time.RFC3339)
	newTime := time.Now().Format(time.RFC3339)
	oldPath := filepath.Join(home, ".smara", "images", "old.png")
	newPath := filepath.Join(home, ".smara", "images", "new.png")
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(oldPath, []byte("png"), 0o644)
	_ = os.WriteFile(newPath, []byte("png"), 0o644)
	if err := writeAssets([]Asset{
		{ID: "old", Path: oldPath, Archived: true, CreatedAt: oldTime},
		{ID: "new", Path: newPath, Archived: false, CreatedAt: newTime},
	}); err != nil {
		t.Fatal(err)
	}
	removed, err := CleanupAssets(24*time.Hour, true)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed=%d, want 1", removed)
	}
	assets, err := ListAssets()
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].ID != "new" {
		t.Fatalf("unexpected assets after cleanup: %+v", assets)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old file should be removed, stat err=%v", err)
	}
}

func TestStartNextQueuedJobPicksHighestPriority(t *testing.T) {
	jobs.Lock()
	jobs.items = map[string]*jobRecord{
		"done": {job: Job{ID: "done", Status: "running", CreatedAt: "2026-01-01T00:00:00Z"}},
		"low":  {job: Job{ID: "low", Status: "queued", CreatedAt: "2026-01-01T00:00:01Z", Priority: 0}},
		"high": {job: Job{ID: "high", Status: "queued", CreatedAt: "2026-01-01T00:00:02Z", Priority: 10}},
	}
	jobs.running = map[string]bool{"done": true}
	jobs.Unlock()

	startNextQueuedJob("done")

	jobs.Lock()
	defer jobs.Unlock()
	if !jobs.running["high"] {
		t.Fatalf("expected high priority job to be scheduled, running=%v", jobs.running)
	}
	delete(jobs.items, "high")
	delete(jobs.items, "low")
	delete(jobs.items, "done")
	jobs.running = map[string]bool{}
}
