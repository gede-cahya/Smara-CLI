package graphify

// ConnectedComponents finds connected components using DFS.
// Returns map from node ID to component index.
func (g *Graph) ConnectedComponents() map[string]int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	visited := make(map[string]bool)
	component := make(map[string]int)
	compID := 0

	var dfs func(node string)
	dfs = func(node string) {
		visited[node] = true
		component[node] = compID
		for _, neighbor := range g.Adjacency[node] {
			if !visited[neighbor] {
				dfs(neighbor)
			}
		}
	}

	for id := range g.Nodes {
		if !visited[id] {
			dfs(id)
			compID++
		}
	}
	return component
}

// AssignCommunities assigns community IDs using connected components.
func (g *Graph) AssignCommunities() {
	comps := g.ConnectedComponents()
	g.mu.Lock()
	defer g.mu.Unlock()
	for id, n := range g.Nodes {
		n.Community = comps[id]
	}
}
