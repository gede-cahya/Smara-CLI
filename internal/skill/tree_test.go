package skill

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildTree_Empty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tm, err := BuildTree()
	require.NoError(t, err)
	assert.Empty(t, tm.AllNodes())
}

func TestBuildTree_WithDependencies(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tm, err := BuildTree()
	require.NoError(t, err)
	// Should not panic even if no skills exist
	_ = tm.ValidateTree()
}

func TestTreeManager_SuggestNextSkills(t *testing.T) {
	tm := &TreeManager{nodes: map[string]TreeNode{
		"deploy-backend": {
			Skill:        Skill{Name: "deploy-backend"},
			Dependencies: []string{"build-image"},
			Children:     []string{"deploy-frontend"},
		},
		"build-image": {
			Skill:    Skill{Name: "build-image"},
			Children: []string{"deploy-backend"},
		},
		"deploy-frontend": {
			Skill:        Skill{Name: "deploy-frontend"},
			Dependencies: []string{"deploy-backend"},
		},
	}}

	// build-image unlocks deploy-backend
	suggest := tm.SuggestNextSkills("build-image")
	assert.Contains(t, suggest, "deploy-backend")

	// deploy-backend unlocks deploy-frontend
	suggest = tm.SuggestNextSkills("deploy-backend")
	assert.Contains(t, suggest, "deploy-frontend")

	// leaf node has no unlocks
	suggest = tm.SuggestNextSkills("deploy-frontend")
	assert.Empty(t, suggest)
}

func TestTreeManager_GetDependencies(t *testing.T) {
	tm := &TreeManager{nodes: map[string]TreeNode{
		"deploy": {
			Skill:        Skill{Name: "deploy"},
			Dependencies: []string{"build", "test"},
		},
		"build": {Skill: Skill{Name: "build"}},
		"test":  {Skill: Skill{Name: "test"}},
	}}

	deps, err := tm.GetDependencies("deploy")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"build", "test"}, deps)

	// missing node
	_, err = tm.GetDependencies("nonexistent")
	assert.Error(t, err)
}

func TestTreeManager_GetSubtree(t *testing.T) {
	tm := &TreeManager{nodes: map[string]TreeNode{
		"root":   {Skill: Skill{Name: "root"}, Children: []string{"child1"}},
		"child1": {Skill: Skill{Name: "child1"}, Children: []string{"child2"}},
		"child2": {Skill: Skill{Name: "child2"}},
	}}

	subtree, err := tm.GetSubtree("root")
	require.NoError(t, err)
	assert.Len(t, subtree, 3)

	// missing root
	_, err = tm.GetSubtree("nonexistent")
	assert.Error(t, err)
}

func TestTreeManager_ToGraphJSON(t *testing.T) {
	tm := &TreeManager{nodes: map[string]TreeNode{
		"parent": {Skill: Skill{Name: "parent", CategoryPath: []string{"deploy"}}, Children: []string{"child"}},
		"child":  {Skill: Skill{Name: "child", CategoryPath: []string{"deploy"}, ParentID: "parent"}, Dependencies: []string{"dep"}},
		"dep":    {Skill: Skill{Name: "dep", CategoryPath: []string{"test"}}},
	}}

	nodes, edges := tm.ToGraphJSON()
	assert.Len(t, nodes, 3)
	assert.Len(t, edges, 2) // parent→child, dep→child

	// Verify edge types
	foundParent := false
	foundDep := false
	for _, e := range edges {
		if e["type"] == "parent" {
			foundParent = true
		}
		if e["type"] == "dependency" {
			foundDep = true
		}
	}
	assert.True(t, foundParent, "expected a parent edge")
	assert.True(t, foundDep, "expected a dependency edge")
}

func TestTreeManager_ValidateTree_Circular(t *testing.T) {
	tm := &TreeManager{nodes: map[string]TreeNode{
		"a": {Skill: Skill{Name: "a"}, Dependencies: []string{"b"}},
		"b": {Skill: Skill{Name: "b"}, Dependencies: []string{"a"}},
	}}

	issues := tm.ValidateTree()
	assert.NotEmpty(t, issues, "circular dependency should be reported")
}

func TestTreeManager_ValidateTree_MissingDep(t *testing.T) {
	// ValidateTree reports circular deps and orphan parents, but not missing
	// dependencies that are simply not loaded into the tree manager.
	tm := &TreeManager{nodes: map[string]TreeNode{
		"a": {Skill: Skill{Name: "a", ParentID: "missing"}},
	}}

	issues := tm.ValidateTree()
	assert.NotEmpty(t, issues, "orphan parent should be reported")
}
