package graphify

// QueryResult contains matched nodes and connecting edges.
type QueryResult struct {
	Nodes []*Node `json:"nodes"`
	Edges []*Edge `json:"edges"`
}

// Query performs keyword search on nodes and extracts connecting subgraph.
func (g *Graph) Query(text string, depth int) *QueryResult {
	matched := g.SearchNodes(text)
	if len(matched) == 0 {
		return &QueryResult{Nodes: []*Node{}, Edges: []*Edge{}}
	}
	nodeIDs := make([]string, len(matched))
	for i, n := range matched {
		nodeIDs[i] = n.ID
	}
	sub := g.ExtractSubgraph(nodeIDs, depth)
	return &QueryResult{
		Nodes: nodeList(sub.Nodes),
		Edges: sub.Edges,
	}
}

// PathResult contains path and edges between two nodes.
type PathResult struct {
	Path  []string `json:"path"`
	Edges []*Edge  `json:"edges"`
}

// FindPath finds shortest path between two nodes.
func (g *Graph) FindPath(from, to string) *PathResult {
	path := g.ShortestPath(from, to)
	if path == nil {
		return nil
	}
	edges := g.ShortestPathEdges(from, to)
	return &PathResult{Path: path, Edges: edges}
}

// ExplainNode returns detailed view of a node and its neighborhood.
func (g *Graph) ExplainNode(nodeID string, depth int) *QueryResult {
	sub := g.ExtractSubgraph([]string{nodeID}, depth)
	return &QueryResult{
		Nodes: nodeList(sub.Nodes),
		Edges: sub.Edges,
	}
}

func nodeList(m map[string]*Node) []*Node {
	var list []*Node
	for _, n := range m {
		list = append(list, n)
	}
	return list
}
