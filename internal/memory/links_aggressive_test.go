package memory

import (
	"path/filepath"
	"testing"
)

func TestAutoLinkAggressive_ConnectsViaEntitiesAndIsolated(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()
	ws, err := store.CreateWorkspace("aggressive-ws", "")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	contents := []struct{ content, tags, source string }{
		{"Smara Web Memory Graph Fast LOD seperti Obsidian graph view", "smara,graph", "chat"},
		{"Node dan edge Memory Graph bisa draggable di Smara Web", "smara,ui", "chat"},
		{"Discord adapter perlu command memory autolink aggressive", "discord,adapter", "chat"},
		{"Telegram adapter juga menjalankan memory autolink aggressive", "telegram,adapter", "chat"},
		{"Preferensi user untuk UI gelap dan tombol approval", "preference,ui", "chat"},
	}
	for _, c := range contents {
		if _, err := store.Save(c.content, c.tags, c.source, ws.ID, nil); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
	report, err := store.AutoLinkAggressive(AutoLinkOptions{WorkspaceID: ws.ID, Replace: true, Threshold: 0.28, MaxPerNode: 10, AttachIsolated: true, HubLinks: true})
	if err != nil {
		t.Fatalf("AutoLinkAggressive: %v", err)
	}
	if report.Mode != AutoLinkModeAggressive {
		t.Fatalf("expected aggressive mode, got %q", report.Mode)
	}
	if report.Created == 0 {
		t.Fatalf("expected created links > 0")
	}
	graph, err := store.BuildGraph(ws.ID, 0)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	isolated := 0
	auto := 0
	for _, n := range graph.Nodes {
		if n.Degree == 0 {
			isolated++
		}
	}
	for _, e := range graph.Edges {
		if e.Auto {
			auto++
		}
		if e.Weight <= 0 || e.Weight > 1 {
			t.Fatalf("edge weight out of range: %.3f", e.Weight)
		}
	}
	if auto == 0 {
		t.Fatalf("expected auto edges")
	}
	if isolated > 1 {
		t.Fatalf("expected aggressive mode to reduce isolated nodes, got %d", isolated)
	}
}
