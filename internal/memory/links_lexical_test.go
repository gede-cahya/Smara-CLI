package memory

import (
	"path/filepath"
	"testing"
)

// TestAutoLinkSmart_FallsBackToLexical verifies that when no memories have
// embeddings, AutoLinkSmart falls back to Jaccard token overlap and produces
// links between obviously related memories.
func TestAutoLinkSmart_FallsBackToLexical(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ws, err := store.CreateWorkspace("test-ws", "")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	// 4 memories: 2 about Go SQLite, 2 about React UI.
	contents := []string{
		"Setup project Go dengan cobra CLI dan SQLite untuk storage memori",
		"Database SQLite WAL mode untuk konkurensi penyimpanan memori Go",
		"React component design dengan tailwind CSS untuk halaman dashboard",
		"Tailwind CSS dan React UI pattern untuk dashboard frontend",
	}
	for _, c := range contents {
		if _, err := store.Save(c, "", "test", ws.ID, nil); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	report, err := store.AutoLinkSmart(AutoLinkOptions{
		WorkspaceID: ws.ID,
		Threshold:   0.78,
		MaxPerNode:  3,
		Replace:     true,
	})
	if err != nil {
		t.Fatalf("AutoLinkSmart: %v", err)
	}

	if report.Mode != AutoLinkModeLexical {
		t.Errorf("expected mode=lexical, got %q", report.Mode)
	}
	if !report.FellBackToLexical {
		t.Error("expected fell_back_to_lexical=true")
	}
	if report.WithEmbedding != 0 {
		t.Errorf("expected with_embedding=0, got %d", report.WithEmbedding)
	}
	if report.MemoriesScanned != 4 {
		t.Errorf("expected memories_scanned=4, got %d", report.MemoriesScanned)
	}
	if report.Created == 0 {
		t.Error("expected lexical engine to produce at least one link, got 0")
	}

	// Verify graph contains some auto-linked edges.
	graph, err := store.BuildGraph(ws.ID, 0)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	autoCount := 0
	for _, e := range graph.Edges {
		if e.Auto {
			autoCount++
		}
	}
	if autoCount == 0 {
		t.Errorf("expected at least one auto edge in graph, got 0 (created=%d)", report.Created)
	}
}

func TestJaccard(t *testing.T) {
	a := tokenize("Setup project Go dengan cobra CLI dan SQLite")
	b := tokenize("Database SQLite WAL mode untuk Go memori")
	sim := jaccard(a, b)
	if sim <= 0 {
		t.Errorf("expected jaccard > 0 for overlapping content, got %f", sim)
	}
	if sim >= 1 {
		t.Errorf("expected jaccard < 1 for partial overlap, got %f", sim)
	}

	c := tokenize("totally different topic about cooking pasta tomato")
	sim2 := jaccard(a, c)
	if sim2 >= sim {
		t.Errorf("expected unrelated text to score lower than related text (%.3f vs %.3f)", sim2, sim)
	}
}

func TestTokenize_FiltersStopwords(t *testing.T) {
	tokens := tokenize("the quick brown fox jumps over yang dan untuk")
	if _, ok := tokens["the"]; ok {
		t.Error("expected 'the' to be filtered as stopword")
	}
	if _, ok := tokens["yang"]; ok {
		t.Error("expected 'yang' (Bahasa stopword) to be filtered")
	}
	if _, ok := tokens["quick"]; !ok {
		t.Error("expected 'quick' to be retained")
	}
}
