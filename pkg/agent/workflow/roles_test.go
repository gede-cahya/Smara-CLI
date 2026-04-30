package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetRoleDefinition_BuiltInRoles(t *testing.T) {
	roles := []string{"frontend", "backend", "database", "devops", "designer", "qa"}
	for _, role := range roles {
		def, ok := GetRoleDefinition(role)
		assert.True(t, ok, "role %s should exist", role)
		assert.Equal(t, role, def.Name)
		assert.NotEmpty(t, def.Label, "role %s should have label", role)
		assert.NotEmpty(t, def.SystemPrompt, "role %s should have system prompt", role)
		assert.NotEmpty(t, def.KeywordMatches, "role %s should have keyword matches", role)
	}
}

func TestGetRoleDefinition_UnknownRole(t *testing.T) {
	def, ok := GetRoleDefinition("nonexistent")
	assert.False(t, ok)
	assert.Empty(t, def.Name)
}

func TestGenerateDynamicRole(t *testing.T) {
	def := GenerateDynamicRole("ml_engineer", "Machine learning specialist", []string{"terminal"})
	assert.Equal(t, "ml_engineer", def.Name)
	assert.NotEmpty(t, def.Label)
	assert.Contains(t, def.SystemPrompt, "Machine learning specialist")
	assert.Contains(t, def.SystemPrompt, "Ml_engineer")
	assert.Equal(t, []string{"ml_engineer"}, def.KeywordMatches)
	assert.Equal(t, []string{"terminal"}, def.DefaultTools)
}

func TestAllRoleNames(t *testing.T) {
	names := AllRoleNames()
	assert.Len(t, names, 6)
	assert.Contains(t, names, "frontend")
	assert.Contains(t, names, "backend")
	assert.Contains(t, names, "database")
	assert.Contains(t, names, "devops")
	assert.Contains(t, names, "designer")
	assert.Contains(t, names, "qa")
}

func TestRoleDefinition_Fields(t *testing.T) {
	def := RoleDefinition{
		Name:           "test",
		Label:          "Test Role",
		SystemPrompt:   "You are a test role.",
		KeywordMatches: []string{"test", "testing"},
		DefaultTools:   []string{"tool1", "tool2"},
	}
	assert.Equal(t, "test", def.Name)
	assert.Equal(t, "Test Role", def.Label)
	assert.Equal(t, "You are a test role.", def.SystemPrompt)
	assert.Equal(t, []string{"test", "testing"}, def.KeywordMatches)
	assert.Equal(t, []string{"tool1", "tool2"}, def.DefaultTools)
}
