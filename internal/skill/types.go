package skill

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ParamDef defines a configurable parameter for a skill.
type ParamDef struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"` // string, number, boolean
	Description string      `json:"description"`
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
}

// Step is one tool call inside a skill recipe.
type Step struct {
	Tool string                 `json:"tool"`
	Args map[string]interface{} `json:"args"`
}

// LineageEntry records a previous version of a skill after refinement.
type LineageEntry struct {
	Version     int       `json:"version"`
	Description string    `json:"description,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	StepCount   int       `json:"step_count"`
	RefinedAt   time.Time `json:"refined_at"`
	RefinedFrom string    `json:"refined_from,omitempty"`
	Snapshot    string    `json:"snapshot,omitempty"`
}

type AutoSkillMetadata struct {
	Enabled                  bool   `json:"enabled"`
	Mode                     string `json:"mode,omitempty"`
	CreatePolicy             string `json:"create_policy,omitempty"`
	MinimumToolActions       int    `json:"minimum_tool_actions"`
	RepeatedWorkflowRequired bool   `json:"repeated_workflow_required"`
	UpgradePolicy            string `json:"upgrade_policy,omitempty"`
	ExecuteAfterCreate       bool   `json:"execute_after_create"`
	ExecuteAfterUpgrade      bool   `json:"execute_after_upgrade"`
	ApprovalRequired         bool   `json:"approval_required"`
	LineageBackup            bool   `json:"lineage_backup"`
	MaxAutoUpgradeRetries    int    `json:"max_auto_upgrade_retries,omitempty"`
}

// Skill is a reusable automation recipe.
type Skill struct {
	Name               string              `json:"name"`
	Description        string              `json:"description"`
	Steps              []Step              `json:"steps"`
	Version            int                 `json:"version"`
	Tags               []string            `json:"tags,omitempty"`
	Author             string              `json:"author,omitempty"`
	SourceURL          string              `json:"source_url,omitempty"`
	Trigger            string              `json:"trigger,omitempty"`
	Params             []ParamDef          `json:"params,omitempty"`
	ParentID           string              `json:"parent_id,omitempty"`
	CategoryPath       []string            `json:"category_path,omitempty"`
	Dependencies       []string            `json:"dependencies,omitempty"`
	DependencyWorkflow *DependencyWorkflow `json:"dependency_workflow,omitempty"`
	Risk               *RiskMetadata       `json:"risk,omitempty"`
	AutoSkill          *AutoSkillMetadata  `json:"auto_skill,omitempty"`
	Lineage            []LineageEntry      `json:"lineage,omitempty"`
	Children           []string            `json:"children,omitempty"`
}

func cloneAutoSkillMetadata(a *AutoSkillMetadata) *AutoSkillMetadata {
	if a == nil {
		return nil
	}
	return &AutoSkillMetadata{
		Enabled:                  a.Enabled,
		Mode:                     a.Mode,
		CreatePolicy:             a.CreatePolicy,
		MinimumToolActions:       a.MinimumToolActions,
		RepeatedWorkflowRequired: a.RepeatedWorkflowRequired,
		UpgradePolicy:            a.UpgradePolicy,
		ExecuteAfterCreate:       a.ExecuteAfterCreate,
		ExecuteAfterUpgrade:      a.ExecuteAfterUpgrade,
		ApprovalRequired:         a.ApprovalRequired,
		LineageBackup:            a.LineageBackup,
		MaxAutoUpgradeRetries:    a.MaxAutoUpgradeRetries,
	}
}

func cloneRiskMetadata(r *RiskMetadata) *RiskMetadata {
	if r == nil {
		return nil
	}
	return &RiskMetadata{
		Level:            r.Level,
		Categories:       append([]string(nil), r.Categories...),
		Reasons:          append([]string(nil), r.Reasons...),
		RequiresApproval: r.RequiresApproval,
	}
}

func cloneDependencyWorkflow(w *DependencyWorkflow) *DependencyWorkflow {
	if w == nil {
		return nil
	}
	return &DependencyWorkflow{
		Requires:  append([]string(nil), w.Requires...),
		Suggests:  append([]string(nil), w.Suggests...),
		Precheck:  append([]string(nil), w.Precheck...),
		Postcheck: append([]string(nil), w.Postcheck...),
	}
}

// WithArgs returns a copy of the skill with parameter substitution applied.
func (s *Skill) WithArgs(runtimeArgs map[string]interface{}) *Skill {
	if len(runtimeArgs) == 0 && len(s.Params) == 0 {
		return s
	}
	merged := make(map[string]interface{})
	for _, p := range s.Params {
		if p.Default != nil {
			merged[p.Name] = p.Default
		}
	}
	for k, v := range runtimeArgs {
		merged[k] = v
	}
	newSkill := &Skill{
		Name: s.Name, Description: s.Description, Steps: make([]Step, len(s.Steps)), Version: s.Version,
		Tags: append([]string(nil), s.Tags...), Author: s.Author, SourceURL: s.SourceURL, Trigger: s.Trigger,
		Params: append([]ParamDef(nil), s.Params...), ParentID: s.ParentID, CategoryPath: append([]string(nil), s.CategoryPath...),
		Dependencies: append([]string(nil), s.Dependencies...), DependencyWorkflow: cloneDependencyWorkflow(s.DependencyWorkflow), Risk: cloneRiskMetadata(s.Risk), AutoSkill: cloneAutoSkillMetadata(s.AutoSkill), Lineage: append([]LineageEntry(nil), s.Lineage...),
		Children: append([]string(nil), s.Children...),
	}
	for i, step := range s.Steps {
		newArgs := make(map[string]interface{})
		for k, v := range step.Args {
			newArgs[k] = substituteParamValue(v, merged)
		}
		for k, v := range merged {
			if _, exists := newArgs[k]; exists {
				newArgs[k] = substituteParamValue(v, merged)
			}
		}
		newSkill.Steps[i] = Step{Tool: step.Tool, Args: newArgs}
	}
	return newSkill
}

func substituteParamValue(v interface{}, params map[string]interface{}) interface{} {
	switch val := v.(type) {
	case string:
		result := val
		for k, pv := range params {
			placeholder := "__PARAM__" + k
			if strings.Contains(result, placeholder) {
				var replacement string
				switch rv := pv.(type) {
				case string:
					replacement = rv
				case fmt.Stringer:
					replacement = rv.String()
				default:
					replacement = fmt.Sprintf("%v", pv)
				}
				result = strings.ReplaceAll(result, placeholder, replacement)
			}
		}
		return result
	case map[string]interface{}:
		newMap := make(map[string]interface{}, len(val))
		for mk, mv := range val {
			newMap[mk] = substituteParamValue(mv, params)
		}
		return newMap
	case []interface{}:
		newSlice := make([]interface{}, len(val))
		for i, sv := range val {
			newSlice[i] = substituteParamValue(sv, params)
		}
		return newSlice
	default:
		return v
	}
}

func (s *Skill) ToJSON() ([]byte, error) { return json.MarshalIndent(s, "", "  ") }

func FromJSON(data []byte) (*Skill, error) {
	var s Skill
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("invalid skill JSON: %w", err)
	}
	return &s, nil
}

func (s *Skill) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("skill name is required")
	}
	if len(s.Steps) == 0 {
		return fmt.Errorf("skill '%s' has no steps", s.Name)
	}
	for i, st := range s.Steps {
		if st.Tool == "" {
			return fmt.Errorf("skill '%s' step %d has empty tool", s.Name, i)
		}
	}
	return nil
}
