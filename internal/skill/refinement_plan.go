package skill

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

type SkillDiff struct {
	NameChanged         bool     `json:"name_changed"`
	OldName             string   `json:"old_name,omitempty"`
	NewName             string   `json:"new_name,omitempty"`
	DescriptionChanged  bool     `json:"description_changed"`
	OldDescription      string   `json:"old_description,omitempty"`
	NewDescription      string   `json:"new_description,omitempty"`
	StepsAdded          []int    `json:"steps_added,omitempty"`
	StepsRemoved        []int    `json:"steps_removed,omitempty"`
	StepsChanged        []int    `json:"steps_changed,omitempty"`
	ParamsAdded         []string `json:"params_added,omitempty"`
	ParamsRemoved       []string `json:"params_removed,omitempty"`
	DependenciesAdded   []string `json:"dependencies_added,omitempty"`
	DependenciesRemoved []string `json:"dependencies_removed,omitempty"`
}

type RefinementPreview struct {
	OriginalName string     `json:"original_name"`
	ProposedName string     `json:"proposed_name"`
	Diff         SkillDiff  `json:"diff"`
	Summary      []string   `json:"summary"`
	Lint         LintReport `json:"lint"`
}

func BuildRefinementPreview(original, proposed *Skill, opts LintOptions) RefinementPreview {
	if original == nil || proposed == nil {
		return RefinementPreview{Lint: LintReport{Issues: []LintIssue{{Severity: "error", Field: "skill", Message: "original/proposed skill is nil"}}}}
	}
	diff := DiffSkills(original, proposed)
	return RefinementPreview{
		OriginalName: original.Name,
		ProposedName: proposed.Name,
		Diff:         diff,
		Summary:      summarizeDiff(diff),
		Lint:         LintSkillWithOptions(proposed, opts),
	}
}

func NormalizeRefinementProposal(original, proposed *Skill) *Skill {
	if original == nil || proposed == nil {
		return proposed
	}
	normalized := *proposed
	normalized.Name = original.Name
	return &normalized
}

func DiffSkills(oldSkill, newSkill *Skill) SkillDiff {
	var diff SkillDiff
	if oldSkill == nil || newSkill == nil {
		return diff
	}
	if oldSkill.Name != newSkill.Name {
		diff.NameChanged = true
		diff.OldName = oldSkill.Name
		diff.NewName = newSkill.Name
	}
	if oldSkill.Description != newSkill.Description {
		diff.DescriptionChanged = true
		diff.OldDescription = oldSkill.Description
		diff.NewDescription = newSkill.Description
	}
	oldLen, newLen := len(oldSkill.Steps), len(newSkill.Steps)
	minLen := oldLen
	if newLen < minLen {
		minLen = newLen
	}
	for i := 0; i < minLen; i++ {
		if !reflect.DeepEqual(oldSkill.Steps[i], newSkill.Steps[i]) {
			diff.StepsChanged = append(diff.StepsChanged, i)
		}
	}
	for i := minLen; i < newLen; i++ {
		diff.StepsAdded = append(diff.StepsAdded, i)
	}
	for i := minLen; i < oldLen; i++ {
		diff.StepsRemoved = append(diff.StepsRemoved, i)
	}
	diff.ParamsAdded, diff.ParamsRemoved = diffNames(paramNames(oldSkill.Params), paramNames(newSkill.Params))
	diff.DependenciesAdded, diff.DependenciesRemoved = diffNames(oldSkill.Dependencies, newSkill.Dependencies)
	return diff
}

func ApplyRefinementWithLint(name string, proposedJSON []byte, db *sql.DB, refinedFrom string, opts LintOptions, allowInvalid bool) (*Skill, RefinementPreview, error) {
	prior, err := Load(name)
	if err != nil {
		return nil, RefinementPreview{}, err
	}
	proposed, err := FromJSON(proposedJSON)
	if err != nil {
		return nil, RefinementPreview{}, fmt.Errorf("proposed skill invalid: %w", err)
	}
	proposed = NormalizeRefinementProposal(prior, proposed)
	preview := BuildRefinementPreview(prior, proposed, opts)
	if preview.Lint.HasErrors() && !allowInvalid {
		return nil, preview, fmt.Errorf("refined skill lint failed: %d error(s)", len(preview.Lint.Issues))
	}
	if proposed.Version <= prior.Version {
		proposed.Version = prior.Version + 1
	}
	AttachLineage(proposed, prior, refinedFrom)
	if err := Save(proposed, db); err != nil {
		return nil, preview, fmt.Errorf("failed to save refined skill: %w", err)
	}
	return proposed, preview, nil
}

func MarshalSkillDiff(oldSkill, newSkill *Skill) string {
	oldJSON, _ := oldSkill.ToJSON()
	newJSON, _ := newSkill.ToJSON()
	return fmt.Sprintf("--- %s@v%d\n%s\n+++ %s@v%d\n%s", oldSkill.Name, oldSkill.Version, string(oldJSON), newSkill.Name, newSkill.Version, string(newJSON))
}

func summarizeDiff(diff SkillDiff) []string {
	var out []string
	if diff.NameChanged {
		out = append(out, fmt.Sprintf("name: %s -> %s", diff.OldName, diff.NewName))
	}
	if diff.DescriptionChanged {
		out = append(out, "description changed")
	}
	appendCount := func(label string, n int) {
		if n > 0 {
			out = append(out, fmt.Sprintf("%s: %d", label, n))
		}
	}
	appendCount("steps added", len(diff.StepsAdded))
	appendCount("steps removed", len(diff.StepsRemoved))
	appendCount("steps changed", len(diff.StepsChanged))
	appendCount("params added", len(diff.ParamsAdded))
	appendCount("params removed", len(diff.ParamsRemoved))
	appendCount("dependencies added", len(diff.DependenciesAdded))
	appendCount("dependencies removed", len(diff.DependenciesRemoved))
	if len(out) == 0 {
		out = append(out, "no changes")
	}
	return out
}

func paramNames(params []ParamDef) []string {
	names := make([]string, 0, len(params))
	for _, p := range params {
		names = append(names, p.Name)
	}
	return names
}

func diffNames(oldNames, newNames []string) ([]string, []string) {
	oldSet := map[string]bool{}
	newSet := map[string]bool{}
	for _, name := range oldNames {
		oldSet[name] = true
	}
	for _, name := range newNames {
		newSet[name] = true
	}
	var added, removed []string
	for name := range newSet {
		if !oldSet[name] {
			added = append(added, name)
		}
	}
	for name := range oldSet {
		if !newSet[name] {
			removed = append(removed, name)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

func (p RefinementPreview) ToJSON() ([]byte, error) {
	return json.MarshalIndent(p, "", "  ")
}
