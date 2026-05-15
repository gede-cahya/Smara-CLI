package graphify

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Node represents a graph node (function, type, variable, concept, etc.)
type Node struct {
	ID         string                 `json:"id"`
	Label      string                 `json:"label"`
	Type       string                 `json:"type"` // function, class, type, variable, concept, doc
	SourceFile string                 `json:"source_file"`
	SourceLine int                    `json:"source_line"`
	Language   string                 `json:"language"`
	Content    string                 `json:"content"` // signature, docstring, or summary
	Community  int                    `json:"community"`
	GodScore   float64                `json:"god_score"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// Edge represents a relationship between two nodes
type Edge struct {
	ID              string  `json:"id"`
	Source          string  `json:"source"`
	Target          string  `json:"target"`
	Relation        string  `json:"relation"`   // calls, imports, contains, semantically_similar_to, rationale_for
	Confidence      string  `json:"confidence"` // EXTRACTED, INFERRED, AMBIGUOUS
	ConfidenceScore float64 `json:"confidence_score"`
	SourceFile      string  `json:"source_file"`
	InferredReason  string  `json:"inferred_reason,omitempty"`
}

// Graph is an in-memory knowledge graph
type Graph struct {
	ID        string                 `json:"id"`
	RootPath  string                 `json:"root_path"`
	Nodes     map[string]*Node       `json:"nodes"`
	Edges     []*Edge                `json:"edges"`
	Adjacency map[string][]string    `json:"-"` // nodeID -> list of target nodeIDs
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	mu        sync.RWMutex
}

// NewGraph creates a new empty graph.
func NewGraph(id, rootPath string) *Graph {
	return &Graph{
		ID:        id,
		RootPath:  rootPath,
		Nodes:     make(map[string]*Node),
		Edges:     make([]*Edge, 0),
		Adjacency: make(map[string][]string),
	}
}

// AddNode adds a node to the graph. If a node with the same ID exists, it is overwritten.
func (g *Graph) AddNode(n *Node) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Nodes[n.ID] = n
	if _, ok := g.Adjacency[n.ID]; !ok {
		g.Adjacency[n.ID] = make([]string, 0)
	}
}

// AddEdge adds an edge to the graph.
func (g *Graph) AddEdge(e *Edge) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Edges = append(g.Edges, e)
	if _, ok := g.Adjacency[e.Source]; ok {
		g.Adjacency[e.Source] = append(g.Adjacency[e.Source], e.Target)
	}
}

// GetNode retrieves a node by ID.
func (g *Graph) GetNode(id string) (*Node, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	n, ok := g.Nodes[id]
	return n, ok
}

// HasNode returns true if the node exists.
func (g *Graph) HasNode(id string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, ok := g.Nodes[id]
	return ok
}

// GetNeighbors returns direct neighbors of a node.
func (g *Graph) GetNeighbors(nodeID string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return append([]string{}, g.Adjacency[nodeID]...)
}

// GetNeighborsByDepth returns neighbors up to a given depth using BFS.
func (g *Graph) GetNeighborsByDepth(nodeID string, depth int) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if depth <= 0 {
		return []string{}
	}

	visited := map[string]bool{nodeID: true}
	current := []string{nodeID}

	for i := 0; i < depth; i++ {
		next := []string{}
		for _, n := range current {
			for _, neighbor := range g.Adjacency[n] {
				if !visited[neighbor] {
					visited[neighbor] = true
					next = append(next, neighbor)
				}
			}
		}
		current = next
	}

	delete(visited, nodeID)
	result := make([]string, 0, len(visited))
	for id := range visited {
		result = append(result, id)
	}
	return result
}

// GetEdges returns all edges from a given source node.
func (g *Graph) GetEdges(source string) []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var result []*Edge
	for _, e := range g.Edges {
		if e.Source == source {
			result = append(result, e)
		}
	}
	return result
}

// GetAllEdges returns edges connecting the given nodes (either direction).
func (g *Graph) GetAllEdges(nodeIDs []string) []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	idSet := make(map[string]bool)
	for _, id := range nodeIDs {
		idSet[id] = true
	}
	var result []*Edge
	for _, e := range g.Edges {
		if idSet[e.Source] && idSet[e.Target] {
			result = append(result, e)
		}
	}
	return result
}

// ShortestPath finds the shortest path between two nodes using BFS.
func (g *Graph) ShortestPath(from, to string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, ok := g.Nodes[from]; !ok {
		return nil
	}
	if _, ok := g.Nodes[to]; !ok {
		return nil
	}

	type queueItem struct {
		node string
		path []string
	}

	visited := map[string]bool{from: true}
	queue := []queueItem{{node: from, path: []string{from}}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current.node == to {
			return current.path
		}

		for _, neighbor := range g.Adjacency[current.node] {
			if !visited[neighbor] {
				visited[neighbor] = true
				newPath := append([]string{}, current.path...)
				newPath = append(newPath, neighbor)
				queue = append(queue, queueItem{node: neighbor, path: newPath})
			}
		}
	}

	return nil
}

// ShortestPathEdges returns edges along the shortest path between two nodes.
func (g *Graph) ShortestPathEdges(from, to string) []*Edge {
	path := g.ShortestPath(from, to)
	if len(path) < 2 {
		return nil
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []*Edge
	for i := 0; i < len(path)-1; i++ {
		src := path[i]
		dst := path[i+1]
		for _, e := range g.Edges {
			if e.Source == src && e.Target == dst {
				result = append(result, e)
				break
			}
		}
	}
	return result
}

// SearchNodes finds nodes whose ID, label, or content contains the query.
func (g *Graph) SearchNodes(query string) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	queryLower := strings.ToLower(query)
	var results []*Node
	for _, n := range g.Nodes {
		if strings.Contains(strings.ToLower(n.ID), queryLower) ||
			strings.Contains(strings.ToLower(n.Label), queryLower) ||
			strings.Contains(strings.ToLower(n.Content), queryLower) ||
			strings.Contains(strings.ToLower(n.SourceFile), queryLower) {
			results = append(results, n)
		}
	}
	return results
}

// ExtractSubgraph extracts a subgraph containing the given nodes plus neighbors up to a given depth.
func (g *Graph) ExtractSubgraph(nodeIDs []string, depth int) *Graph {
	g.mu.RLock()
	defer g.mu.RUnlock()

	included := make(map[string]bool)
	queue := make([]string, len(nodeIDs))
	copy(queue, nodeIDs)
	for _, id := range nodeIDs {
		included[id] = true
	}

	for d := 0; d < depth; d++ {
		next := []string{}
		for _, id := range queue {
			for _, neighbor := range g.Adjacency[id] {
				if !included[neighbor] {
					included[neighbor] = true
					next = append(next, neighbor)
				}
			}
		}
		queue = next
	}

	sub := NewGraph(g.ID+"_sub", g.RootPath)
	for id := range included {
		if n, ok := g.Nodes[id]; ok {
			sub.AddNode(copyNode(n))
		}
	}
	for _, e := range g.Edges {
		if included[e.Source] && included[e.Target] {
			sub.AddEdge(copyEdge(e))
		}
	}

	return sub
}

// NodeCount returns the number of nodes.
func (g *Graph) NodeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.Nodes)
}

// EdgeCount returns the number of edges.
func (g *Graph) EdgeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.Edges)
}

// ComputeGodScores computes degree centrality for all nodes.
func (g *Graph) ComputeGodScores() {
	g.mu.Lock()
	defer g.mu.Unlock()

	degree := make(map[string]int)
	for _, e := range g.Edges {
		degree[e.Source]++
		degree[e.Target]++
	}

	maxDeg := 0
	for _, d := range degree {
		if d > maxDeg {
			maxDeg = d
		}
	}

	for id, n := range g.Nodes {
		if maxDeg > 0 {
			n.GodScore = float64(degree[id]) / float64(maxDeg)
		}
	}
}

// TopGodNodes returns the top N nodes by god score.
func (g *Graph) TopGodNodes(n int) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var nodes []*Node
	for _, node := range g.Nodes {
		nodes = append(nodes, node)
	}

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].GodScore > nodes[j].GodScore
	})

	if n > len(nodes) {
		n = len(nodes)
	}
	return nodes[:n]
}

// NodeTypes returns a sorted list of all unique node types.
func (g *Graph) NodeTypes() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	seen := make(map[string]bool)
	for _, n := range g.Nodes {
		seen[n.Type] = true
	}
	var types []string
	for t := range seen {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

// Languages returns a sorted list of all unique languages.
func (g *Graph) Languages() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	seen := make(map[string]bool)
	for _, n := range g.Nodes {
		if n.Language != "" {
			seen[n.Language] = true
		}
	}
	var langs []string
	for l := range seen {
		langs = append(langs, l)
	}
	sort.Strings(langs)
	return langs
}

func copyNode(n *Node) *Node {
	m := &Node{
		ID:         n.ID,
		Label:      n.Label,
		Type:       n.Type,
		SourceFile: n.SourceFile,
		SourceLine: n.SourceLine,
		Language:   n.Language,
		Content:    n.Content,
		Community:  n.Community,
		GodScore:   n.GodScore,
	}
	if n.Metadata != nil {
		m.Metadata = make(map[string]interface{})
		for k, v := range n.Metadata {
			m.Metadata[k] = v
		}
	}
	return m
}

func copyEdge(e *Edge) *Edge {
	return &Edge{
		ID:              e.ID,
		Source:          e.Source,
		Target:          e.Target,
		Relation:        e.Relation,
		Confidence:      e.Confidence,
		ConfidenceScore: e.ConfidenceScore,
		SourceFile:      e.SourceFile,
		InferredReason:  e.InferredReason,
	}
}

// ToJSON serializes the graph to JSON.
func (g *Graph) ToJSON() ([]byte, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return json.Marshal(g)
}

// NodeIDs returns all node IDs.
func (g *Graph) NodeIDs() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// EdgeID generates a deterministic edge ID.
func EdgeID(source, target, relation string) string {
	return fmt.Sprintf("%s--%s--%s", source, relation, target)
}

// NodeID generates a deterministic node ID from file path and name.
func NodeID(filePath, name, nodeType string) string {
	return fmt.Sprintf("%s:%s:%s", filePath, nodeType, name)
}
