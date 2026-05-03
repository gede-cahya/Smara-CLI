package graphify

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// GraphStore provides SQLite persistence for graphs.
type GraphStore struct {
	db *sql.DB
}

// NewGraphStore creates a graph store backed by a SQLite DB.
func NewGraphStore(db *sql.DB) (*GraphStore, error) {
	gs := &GraphStore{db: db}
	if err := gs.initSchema(); err != nil {
		return nil, err
	}
	return gs, nil
}

func (gs *GraphStore) initSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS graph_nodes (
			id INTEGER PRIMARY KEY,
			graph_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			label TEXT,
			type TEXT,
			source_file TEXT,
			source_line INTEGER,
			language TEXT,
			content TEXT,
			community INTEGER DEFAULT 0,
			god_score REAL DEFAULT 0,
			metadata TEXT DEFAULT '{}',
			UNIQUE(graph_id, node_id)
		)`,
		`CREATE TABLE IF NOT EXISTS graph_edges (
			id INTEGER PRIMARY KEY,
			graph_id TEXT NOT NULL,
			source TEXT NOT NULL,
			target TEXT NOT NULL,
			relation TEXT,
			confidence TEXT,
			confidence_score REAL,
			source_file TEXT,
			inferred_reason TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS graph_metadata (
			graph_id TEXT PRIMARY KEY,
			root_path TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			node_count INTEGER DEFAULT 0,
			edge_count INTEGER DEFAULT 0,
			languages TEXT DEFAULT '[]',
			corpus_hash TEXT DEFAULT '',
			version INTEGER DEFAULT 1
		)`,
		`CREATE INDEX IF NOT EXISTS idx_graph_nodes_graph_id ON graph_nodes(graph_id)`,
		`CREATE INDEX IF NOT EXISTS idx_graph_nodes_type ON graph_nodes(type)`,
		`CREATE INDEX IF NOT EXISTS idx_graph_nodes_language ON graph_nodes(language)`,
		`CREATE INDEX IF NOT EXISTS idx_graph_edges_graph_id ON graph_edges(graph_id)`,
		`CREATE INDEX IF NOT EXISTS idx_graph_edges_relation ON graph_edges(relation)`,
		`CREATE INDEX IF NOT EXISTS idx_graph_edges_confidence ON graph_edges(confidence)`,
	}
	for _, stmt := range stmts {
		if _, err := gs.db.Exec(stmt); err != nil {
			return fmt.Errorf("graph schema init failed: %w", err)
		}
	}
	return nil
}

// SaveGraph persists a graph and its metadata.
func (gs *GraphStore) SaveGraph(g *Graph) error {
	tx, err := gs.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete existing
	_, _ = tx.Exec("DELETE FROM graph_nodes WHERE graph_id = ?", g.ID)
	_, _ = tx.Exec("DELETE FROM graph_edges WHERE graph_id = ?", g.ID)

	// Insert metadata
	langs, _ := json.Marshal(g.Languages())
	_, err = tx.Exec(`
		INSERT OR REPLACE INTO graph_metadata
		(graph_id, root_path, updated_at, node_count, edge_count, languages)
		VALUES (?, ?, ?, ?, ?, ?)`,
		g.ID, g.RootPath, time.Now(), g.NodeCount(), g.EdgeCount(), string(langs))
	if err != nil {
		return fmt.Errorf("save metadata: %w", err)
	}

	// Insert nodes
	nodeStmt, err := tx.Prepare(`
		INSERT INTO graph_nodes (graph_id, node_id, label, type, source_file, source_line, language, content, community, god_score, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer nodeStmt.Close()

	for _, n := range g.Nodes {
		meta, _ := json.Marshal(n.Metadata)
		_, err = nodeStmt.Exec(g.ID, n.ID, n.Label, n.Type, n.SourceFile, n.SourceLine, n.Language, n.Content, n.Community, n.GodScore, string(meta))
		if err != nil {
			return fmt.Errorf("save node %s: %w", n.ID, err)
		}
	}

	// Insert edges
	edgeStmt, err := tx.Prepare(`
		INSERT INTO graph_edges (graph_id, source, target, relation, confidence, confidence_score, source_file, inferred_reason)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer edgeStmt.Close()

	for _, e := range g.Edges {
		_, err = edgeStmt.Exec(g.ID, e.Source, e.Target, e.Relation, e.Confidence, e.ConfidenceScore, e.SourceFile, e.InferredReason)
		if err != nil {
			return fmt.Errorf("save edge %s: %w", e.ID, err)
		}
	}

	return tx.Commit()
}

// LoadGraph loads a graph from the database.
func (gs *GraphStore) LoadGraph(graphID string) (*Graph, error) {
	var rootPath string
	err := gs.db.QueryRow("SELECT root_path FROM graph_metadata WHERE graph_id = ?", graphID).Scan(&rootPath)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("graph not found: %s", graphID)
		}
		return nil, err
	}

	g := NewGraph(graphID, rootPath)

	// Load nodes
	rows, err := gs.db.Query(`
		SELECT node_id, label, type, source_file, source_line, language, content, community, god_score, metadata
		FROM graph_nodes WHERE graph_id = ?`, graphID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var n Node
		var metaJSON string
		err := rows.Scan(&n.ID, &n.Label, &n.Type, &n.SourceFile, &n.SourceLine, &n.Language, &n.Content, &n.Community, &n.GodScore, &metaJSON)
		if err != nil {
			continue
		}
		json.Unmarshal([]byte(metaJSON), &n.Metadata)
		g.AddNode(&n)
	}

	// Load edges
	erows, err := gs.db.Query(`
		SELECT source, target, relation, confidence, confidence_score, source_file, inferred_reason
		FROM graph_edges WHERE graph_id = ?`, graphID)
	if err != nil {
		return nil, err
	}
	defer erows.Close()

	for erows.Next() {
		var e Edge
		err := erows.Scan(&e.Source, &e.Target, &e.Relation, &e.Confidence, &e.ConfidenceScore, &e.SourceFile, &e.InferredReason)
		if err != nil {
			continue
		}
		e.ID = EdgeID(e.Source, e.Target, e.Relation)
		g.AddEdge(&e)
	}

	return g, nil
}

// ListGraphs returns all graph metadata.
func (gs *GraphStore) ListGraphs() ([]map[string]interface{}, error) {
	rows, err := gs.db.Query(`
		SELECT graph_id, root_path, created_at, updated_at, node_count, edge_count, languages, corpus_hash, version
		FROM graph_metadata ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id, root, created, updated, langs, hash string
		var nodes, edges, version int
		if err := rows.Scan(&id, &root, &created, &updated, &nodes, &edges, &langs, &hash, &version); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"graph_id": id, "root_path": root, "created_at": created,
			"updated_at": updated, "node_count": nodes, "edge_count": edges,
			"languages": langs, "corpus_hash": hash, "version": version,
		})
	}
	return results, nil
}

// DeleteGraph removes a graph and all its data.
func (gs *GraphStore) DeleteGraph(graphID string) error {
	_, err := gs.db.Exec("DELETE FROM graph_nodes WHERE graph_id = ?", graphID)
	if err != nil {
		return err
	}
	_, err = gs.db.Exec("DELETE FROM graph_edges WHERE graph_id = ?", graphID)
	if err != nil {
		return err
	}
	_, err = gs.db.Exec("DELETE FROM graph_metadata WHERE graph_id = ?", graphID)
	return err
}

// GraphExists checks if a graph exists.
func (gs *GraphStore) GraphExists(graphID string) bool {
	var count int
	err := gs.db.QueryRow("SELECT COUNT(*) FROM graph_metadata WHERE graph_id = ?", graphID).Scan(&count)
	return err == nil && count > 0
}
