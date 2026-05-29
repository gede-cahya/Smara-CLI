package skill

import "testing"

func TestLintSkillValid(t *testing.T) {
	s := &Skill{Name: "valid-skill", Description: "Valid skill description", Params: []ParamDef{{Name: "path", Required: true, Description: "Target path"}}, Steps: []Step{{Tool: "run_command", Args: map[string]interface{}{"command": "ls __PARAM__path"}}}}
	r := LintSkillWithOptions(s, LintOptions{KnownTools: map[string]bool{"run_command": true}})
	if r.HasErrors() { t.Fatalf("expected no errors, got %+v", r.Issues) }
}

func TestLintSkillInvalidName(t *testing.T) {
	s := &Skill{Name: "Invalid Name", Description: "Valid skill description", Steps: []Step{{Tool: "run_command"}}}
	r := LintSkill(s, nil)
	if !r.HasErrors() { t.Fatalf("expected invalid name error") }
}

func TestLintSkillMissingDependency(t *testing.T) {
	s := &Skill{Name: "needs-dep", Description: "Valid skill description", Steps: []Step{{Tool: "run_command"}}, Dependencies: []string{"missing-dep"}}
	r := LintSkill(s, map[string]bool{"needs-dep": true})
	if !r.HasErrors() { t.Fatalf("expected missing dependency error") }
}

func TestLintSkillUnknownTool(t *testing.T) {
	s := &Skill{Name: "unknown-tool", Description: "Valid skill description", Steps: []Step{{Tool: "bad_tool"}}}
	r := LintSkillWithOptions(s, LintOptions{KnownTools: map[string]bool{"run_command": true}})
	if !r.HasErrors() { t.Fatalf("expected unknown tool error") }
}

func TestLintSkillUndeclaredPlaceholder(t *testing.T) {
	s := &Skill{Name: "bad-placeholder", Description: "Valid skill description", Steps: []Step{{Tool: "run_command", Args: map[string]interface{}{"command": "cat __PARAM__file"}}}}
	r := LintSkill(s, nil)
	if !r.HasErrors() { t.Fatalf("expected undeclared placeholder error") }
}

func TestLintSkillRequiredParamNeedsDescription(t *testing.T) {
	s := &Skill{Name: "bad-param", Description: "Valid skill description", Params: []ParamDef{{Name: "file", Required: true}}, Steps: []Step{{Tool: "run_command"}}}
	r := LintSkill(s, nil)
	if !r.HasErrors() { t.Fatalf("expected required param description error") }
}
