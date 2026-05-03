package skill

import (
	"fmt"
	"sort"
)

// TreeNode represents a skill in the tree with resolved edges.
type TreeNode struct {
	Skill        Skill    `json:"skill"`
	Children     []string `json:"children"`
	Dependencies []string `json:"dependencies"`
}

// TreeManager builds and queries the skill dependency/parent tree.
type TreeManager struct {
	nodes map[string]TreeNode
}

// BuildTree constructs the tree from all saved skills.
func BuildTree() (*TreeManager, error) {
	names, err := List()
	if err != nil {
		return nil, err
	}
	nodes := make(map[string]TreeNode, len(names))
	for _, n := range names {
		sk, err := Load(n)
		if err != nil {
			continue
		}
		nodes[n] = TreeNode{Skill: *sk}
	}

	// Resolve children (reverse of parent_id)
	for name, node := range nodes {
		if node.Skill.ParentID != "" {
			if parent, ok := nodes[node.Skill.ParentID]; ok {
				parent.Children = append(parent.Children, name)
				nodes[node.Skill.ParentID] = parent
			}
		}
	}

	// Normalize dependency references
	for name, node := range nodes {
		seen := make(map[string]bool)
		var deps []string
		for _, d := range node.Skill.Dependencies {
			if _, ok := nodes[d]; ok && d != name && !seen[d] {
				seen[d] = true
				deps = append(deps, d)
			}
		}
		node.Dependencies = deps
		nodes[name] = node
	}

	return &TreeManager{nodes: nodes}, nil
}

// GetSubtree returns the node and all descendants by name.
func (tm *TreeManager) GetSubtree(root string) ([]TreeNode, error) {
	_, ok := tm.nodes[root]
	if !ok {
		return nil, fmt.Errorf("skill '%s' not found", root)
	}
	var out []TreeNode
	var visit func(string)
	visit = func(name string) {
		n, ok := tm.nodes[name]
		if !ok {
			return
		}
		out = append(out, n)
		for _, child := range n.Children {
			visit(child)
		}
	}
	visit(root)
	return out, nil
}

// GetDependencies resolves transitive dependencies for a skill.
func (tm *TreeManager) GetDependencies(name string) ([]string, error) {
	if _, ok := tm.nodes[name]; !ok {
		return nil, fmt.Errorf("skill '%s' not found", name)
	}
	var result []string
	seen := make(map[string]bool)
	var resolve func(string)
	resolve = func(n string) {
		node, ok := tm.nodes[n]
		if !ok {
			return
		}
		for _, d := range node.Dependencies {
			if !seen[d] {
				seen[d] = true
				result = append(result, d)
				resolve(d)
			}
		}
	}
	resolve(name)
	return result, nil
}

// ValidateTree checks for circular dependencies and orphan nodes.
func (tm *TreeManager) ValidateTree() []string {
	var issues []string
	for name := range tm.nodes {
		if _, err := tm.GetDependencies(name); err != nil {
			issues = append(issues, fmt.Sprintf("%s: dependency resolution error", name))
			continue
		}
		// Detect self-cycle via DFS
		path := make(map[string]bool)
		var dfs func(string) bool
		dfs = func(n string) bool {
			if path[n] {
				return true
			}
			node, ok := tm.nodes[n]
			if !ok {
				return false
			}
			path[n] = true
			for _, d := range node.Dependencies {
				if dfs(d) {
					return true
				}
			}
			delete(path, n)
			return false
		}
		if dfs(name) {
			issues = append(issues, fmt.Sprintf("%s: circular dependency detected", name))
		}
	}

	// Check parent_id references
	for name, node := range tm.nodes {
		if node.Skill.ParentID != "" {
			if _, ok := tm.nodes[node.Skill.ParentID]; !ok {
				issues = append(issues, fmt.Sprintf("%s: orphan parent '%s'", name, node.Skill.ParentID))
			}
		}
	}
	return issues
}

// SuggestNextSkills returns skills that depend on or are children of the given skill.
func (tm *TreeManager) SuggestNextSkills(name string) []string {
	var suggestions []string
	seen := make(map[string]bool)
	node, ok := tm.nodes[name]
	if !ok {
		return nil
	}
	for _, child := range node.Children {
		if !seen[child] {
			seen[child] = true
			suggestions = append(suggestions, child)
		}
	}
	// Also include skills that list this skill as a dependency
	for otherName, other := range tm.nodes {
		for _, dep := range other.Dependencies {
			if dep == name && !seen[otherName] {
				seen[otherName] = true
				suggestions = append(suggestions, otherName)
			}
		}
	}
	sort.Strings(suggestions)
	return suggestions
}

// AllNodes returns the full map of nodes.
func (tm *TreeManager) AllNodes() map[string]TreeNode {
	return tm.nodes
}

// ToGraphJSON returns nodes and edges for graph visualization.
func (tm *TreeManager) ToGraphJSON() (nodes []map[string]interface{}, edges []map[string]interface{}) {
	idx := make(map[string]int)
	i := 0
	for name, node := range tm.nodes {
		idx[name] = i
		category := ""
		if len(node.Skill.CategoryPath) > 0 {
			category = node.Skill.CategoryPath[0]
		}
		nodes = append(nodes, map[string]interface{}{
			"id":       name,
			"label":    name,
			"category": category,
			"version":  node.Skill.Version,
		})
		i++
	}
	for name, node := range tm.nodes {
		src := idx[name]
		for _, dep := range node.Dependencies {
			if dst, ok := idx[dep]; ok {
				edges = append(edges, map[string]interface{}{
					"source": dst,
					"target": src,
					"type":   "dependency",
				})
			}
		}
		if node.Skill.ParentID != "" {
			if dst, ok := idx[node.Skill.ParentID]; ok {
				edges = append(edges, map[string]interface{}{
					"source": dst,
					"target": src,
					"type":   "parent",
				})
			}
		}
	}
	return nodes, edges
}
