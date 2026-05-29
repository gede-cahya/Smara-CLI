package workflow

import (
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunner_buildWaves_NoDeps(t *testing.T) {
	bp := Blueprint{
		Agents: []AgentSpec{
			{Role: "frontend", DependsOn: []string{}},
			{Role: "backend", DependsOn: []string{}},
			{Role: "database", DependsOn: []string{}},
		},
	}

	r := NewRunner(bp, nil, nil)
	waves := r.buildWaves()
	assert.Len(t, waves, 1)
	assert.Len(t, waves[0], 3)
}

func TestRunner_buildWaves_LinearDeps(t *testing.T) {
	bp := Blueprint{
		Agents: []AgentSpec{
			{Role: "database", DependsOn: []string{}},
			{Role: "backend", DependsOn: []string{"database"}},
			{Role: "frontend", DependsOn: []string{"backend"}},
		},
	}

	r := NewRunner(bp, nil, nil)
	waves := r.buildWaves()
	assert.Len(t, waves, 3)
	assert.Equal(t, []string{"database"}, waves[0])
	assert.Equal(t, []string{"backend"}, waves[1])
	assert.Equal(t, []string{"frontend"}, waves[2])
}

func TestRunner_buildWaves_MixedDeps(t *testing.T) {
	bp := Blueprint{
		Agents: []AgentSpec{
			{Role: "database", DependsOn: []string{}},
			{Role: "backend", DependsOn: []string{"database"}},
			{Role: "frontend", DependsOn: []string{"backend"}},
			{Role: "devops", DependsOn: []string{"backend"}},
			{Role: "qa", DependsOn: []string{"frontend", "devops"}},
		},
	}

	r := NewRunner(bp, nil, nil)
	waves := r.buildWaves()
	assert.Len(t, waves, 4)
	assert.Equal(t, []string{"database"}, waves[0])
	assert.Equal(t, []string{"backend"}, waves[1])
	assert.Len(t, waves[2], 2) // frontend and devops parallel
	assert.Equal(t, []string{"qa"}, waves[3])
}

func TestRunner_buildWaves_Circular(t *testing.T) {
	bp := Blueprint{
		Agents: []AgentSpec{
			{Role: "a", DependsOn: []string{"b"}},
			{Role: "b", DependsOn: []string{"a"}},
		},
	}

	r := NewRunner(bp, nil, nil)
	waves, err := r.BuildWaves()
	require.Error(t, err)
	assert.Nil(t, waves)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestRunner_BuildWaves_UnknownDependency(t *testing.T) {
	bp := Blueprint{Agents: []AgentSpec{{Role: "frontend", DependsOn: []string{"backend"}}}}
	r := NewRunner(bp, nil, nil)
	waves, err := r.BuildWaves()
	require.Error(t, err)
	assert.Nil(t, waves)
	assert.Contains(t, err.Error(), "unknown role")
}

func TestRunner_BuildWaves_DuplicateRole(t *testing.T) {
	bp := Blueprint{Agents: []AgentSpec{{Role: "backend"}, {Role: "backend"}}}
	r := NewRunner(bp, nil, nil)
	waves, err := r.BuildWaves()
	require.Error(t, err)
	assert.Nil(t, waves)
	assert.Contains(t, err.Error(), "duplicate agent role")
}

func TestBuildRoleTasks(t *testing.T) {
	state := NewSharedState("/tmp/test")
	spec := AgentSpec{
		Role: "backend",
		Tasks: []Task{
			{ID: "t1", Description: "Create API"},
		},
	}

	tasks := BuildRoleTasks(spec, state)
	require.Len(t, tasks, 1)
	assert.Equal(t, "backend-t1", tasks[0].ID)
	assert.Equal(t, "Create API", tasks[0].Description)
	assert.Equal(t, "backend", tasks[0].AssignedTo)
}

func TestInjectDependencies(t *testing.T) {
	tasks := []agent.Task{
		{ID: "t1", Description: "Initial"},
	}
	completed := map[string][]agent.TaskResult{
		"backend": {
			{TaskID: "b1", Output: "API done"},
		},
	}

	result := injectDependencies(tasks, completed)
	assert.Contains(t, result[0].Description, "API done")
	assert.Contains(t, result[0].Description, "BACKEND")
}

func TestNewRunner(t *testing.T) {
	bp := Blueprint{ProjectName: "Test"}
	r := NewRunner(bp, nil, nil)
	assert.Equal(t, bp, r.Blueprint)
	assert.Nil(t, r.Workers)
	assert.Nil(t, r.SharedState)
	assert.Equal(t, 4, r.MaxConcurrency)
}
