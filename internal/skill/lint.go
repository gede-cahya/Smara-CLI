package skill

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type LintIssue struct {
	Severity string `json:"severity"`
	Skill    string `json:"skill"`
	Field    string `json:"field,omitempty"`
	Message  string `json:"message"`
}

type LintReport struct {
	Issues []LintIssue `json:"issues"`
}

type LintOptions struct {
	Existing   map[string]bool
	KnownTools map[string]bool
}

func (r LintReport) HasErrors() bool {
	for _, i := range r.Issues {
		if i.Severity == "error" {
			return true
		}
	}
	return false
}

var skillNameRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func LintSkill(s *Skill, existing map[string]bool) LintReport {
	return LintSkillWithOptions(s, LintOptions{Existing: existing})
}

func LintSkillWithOptions(s *Skill, opts LintOptions) LintReport {
	if s == nil {
		return LintReport{Issues: []LintIssue{{Severity: "error", Field: "skill", Message: "skill is nil"}}}
	}
	var issues []LintIssue
	add := func(sev, field, msg string) {
		issues = append(issues, LintIssue{Severity: sev, Skill: s.Name, Field: field, Message: msg})
	}
	if strings.TrimSpace(s.Name) == "" {
		add("error", "name", "name is required")
	} else if !skillNameRe.MatchString(s.Name) {
		add("error", "name", "name must be kebab-case")
	}
	if strings.TrimSpace(s.Description) == "" {
		add("warning", "description", "description is empty")
	} else if len(strings.TrimSpace(s.Description)) < 12 {
		add("warning", "description", "description is too short")
	}
	if len(s.Steps) == 0 {
		add("error", "steps", "at least one step is required")
	}

	params := map[string]bool{}
	for _, p := range s.Params {
		if strings.TrimSpace(p.Name) == "" {
			add("error", "params", "parameter with empty name")
			continue
		}
		if params[p.Name] {
			add("error", "params."+p.Name, "duplicate parameter name")
		}
		params[p.Name] = true
		if p.Required && strings.TrimSpace(p.Description) == "" {
			add("error", "params."+p.Name, "required parameter must have description")
		}
	}

	for i, st := range s.Steps {
		field := fmt.Sprintf("steps[%d]", i)
		tool := strings.TrimSpace(st.Tool)
		if tool == "" {
			add("error", field+".tool", "tool is empty")
		} else if opts.KnownTools != nil && !opts.KnownTools[tool] {
			add("error", field+".tool", "unknown tool: "+tool)
		}
		for _, ph := range placeholders(st.Args) {
			if !params[ph] {
				add("error", field, "placeholder __PARAM__"+ph+" has no declared param")
			}
		}
	}
	for _, dep := range s.Dependencies {
		dep = strings.TrimSpace(dep)
		if dep == "" {
			add("error", "dependencies", "dependency name is empty")
			continue
		}
		if opts.Existing != nil && !opts.Existing[dep] {
			add("error", "dependencies", "missing dependency: "+dep)
		}
		if dep == s.Name {
			add("error", "dependencies", "skill cannot depend on itself")
		}
	}
	return LintReport{Issues: issues}
}

func LintAll() (LintReport, error) { return LintAllWithKnownTools(nil) }

func LintAllWithKnownTools(knownTools map[string]bool) (LintReport, error) {
	names, err := List()
	if err != nil {
		return LintReport{}, err
	}
	seen := map[string]bool{}
	var all []LintIssue
	for _, n := range names {
		if seen[n] {
			all = append(all, LintIssue{Severity: "error", Skill: n, Field: "name", Message: "duplicate skill name"})
		}
		seen[n] = true
	}
	for _, n := range names {
		s, err := Load(n)
		if err != nil {
			all = append(all, LintIssue{Severity: "error", Skill: n, Message: err.Error()})
			continue
		}
		all = append(all, LintSkillWithOptions(s, LintOptions{Existing: seen, KnownTools: knownTools}).Issues...)
	}
	if _, err := BuildTree(); err != nil {
		all = append(all, LintIssue{Severity: "error", Field: "tree", Message: err.Error()})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Skill == all[j].Skill {
			return all[i].Field < all[j].Field
		}
		return all[i].Skill < all[j].Skill
	})
	return LintReport{Issues: all}, nil
}

func placeholders(v interface{}) []string {
	var out []string
	var walk func(interface{})
	walk = func(x interface{}) {
		switch t := x.(type) {
		case string:
			parts := strings.Split(t, "__PARAM__")
			for _, p := range parts[1:] {
				name := ""
				for _, r := range p {
					if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
						name += string(r)
					} else {
						break
					}
				}
				if name != "" {
					out = append(out, name)
				}
			}
		case map[string]interface{}:
			for _, v := range t {
				walk(v)
			}
		case []interface{}:
			for _, v := range t {
				walk(v)
			}
		}
	}
	walk(v)
	return out
}
