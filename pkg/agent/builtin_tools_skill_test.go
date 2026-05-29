package agent

import "testing"

func TestGetBuiltinToolsIncludesAutoSkillTools(t *testing.T) {
	tools := GetBuiltinTools()
	seen := map[string]bool{}
	for _, tool := range tools {
		seen[tool.Name] = true
	}
	for _, name := range []string{"skill_run", "skill_create", "skill_list"} {
		if !seen[name] {
			t.Fatalf("expected builtin tool %q to be exposed", name)
		}
	}
}
