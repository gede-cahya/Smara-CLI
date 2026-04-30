package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapRoleToMCP(t *testing.T) {
	servers := []string{"stitch", "figma", "terminal", "sql", "deploy", "docker"}

	frontendMatches := MapRoleToMCP("frontend", servers)
	assert.Contains(t, frontendMatches, "stitch")
	assert.Contains(t, frontendMatches, "figma")

	backendMatches := MapRoleToMCP("backend", servers)
	assert.Contains(t, backendMatches, "terminal")

	dbMatches := MapRoleToMCP("database", servers)
	assert.Contains(t, dbMatches, "sql")

	devopsMatches := MapRoleToMCP("devops", servers)
	assert.Contains(t, devopsMatches, "deploy")
	assert.Contains(t, devopsMatches, "docker")
}

func TestMapRoleToMCP_NoMatches(t *testing.T) {
	servers := []string{"stitch", "figma"}
	backendMatches := MapRoleToMCP("backend", servers)
	assert.Empty(t, backendMatches)
}

func TestFilterToolsByRole(t *testing.T) {
	tools := []string{"stitch_generate", "figma_create", "write_file", "run_command", "sql_query"}

	frontendTools := FilterToolsByRole("frontend", tools)
	assert.Contains(t, frontendTools, "stitch_generate")
	assert.Contains(t, frontendTools, "figma_create")
	assert.NotContains(t, frontendTools, "run_command")

	backendTools := FilterToolsByRole("backend", tools)
	assert.Contains(t, backendTools, "write_file")
	assert.Contains(t, backendTools, "run_command")

	dbTools := FilterToolsByRole("database", tools)
	assert.Contains(t, dbTools, "sql_query")
	assert.Contains(t, dbTools, "run_command")
}

func TestContainsAny(t *testing.T) {
	assert.True(t, containsAny("stitch", []string{"stitch", "figma"}))
	assert.True(t, containsAny("figma", []string{"stitch", "figma"}))
	assert.False(t, containsAny("terminal", []string{"stitch", "figma"}))
	assert.False(t, containsAny("", []string{"a", "b"}))
}
