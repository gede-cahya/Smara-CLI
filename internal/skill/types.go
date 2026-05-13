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
	Type        string      `json:"type"`        // string, number, boolean
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
// When a skill is refined, the prior version's description, tags and a
// short summary of the steps are appended to Lineage so the history is
// preserved even though the JSON file is overwritten.
type LineageEntry struct {
	Version     int       `json:"version"`
	Description string    `json:"description,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	StepCount   int       `json:"step_count"`
	RefinedAt   time.Time `json:"refined_at"`
	RefinedFrom string    `json:"refined_from,omitempty"` // "auto", "manual", "feedback"
}

// Skill is a reusable automation recipe.
type Skill struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Steps        []Step         `json:"steps"`
	Version      int            `json:"version"`
	Tags         []string       `json:"tags,omitempty"`
	Author       string         `json:"author,omitempty"`
	SourceURL    string         `json:"source_url,omitempty"`
	Params       []ParamDef     `json:"params,omitempty"`
	ParentID     string         `json:"parent_id,omitempty"`
	CategoryPath []string       `json:"category_path,omitempty"`
	Dependencies []string       `json:"dependencies,omitempty"`
	Lineage      []LineageEntry `json:"lineage,omitempty"` // history of prior versions
	Children     []string       `json:"children,omitempty"` // derived, not persisted
}

// WithArgs returns a copy of the skill with parameter substitution applied.
// Runtime args override default skill args, and param defaults are applied.
func (s *Skill) WithArgs(runtimeArgs map[string]interface{}) *Skill {
	if len(runtimeArgs) == 0 && len(s.Params) == 0 {
		return s
	}

	// Build merged args: param defaults -> skill step args -> runtime args
	merged := make(map[string]interface{})

	// Apply param defaults
	for _, p := range s.Params {
		if p.Default != nil {
			merged[p.Name] = p.Default
		}
	}

	// Override with runtime args
	for k, v := range runtimeArgs {
		merged[k] = v
	}

	// Create a deep copy of the skill with substituted args
	newSkill := &Skill{
		Name:         s.Name,
		Description:  s.Description,
		Steps:        make([]Step, len(s.Steps)),
		Version:      s.Version,
		Tags:         append([]string(nil), s.Tags...),
		Author:       s.Author,
		SourceURL:    s.SourceURL,
		Params:       append([]ParamDef(nil), s.Params...),
		ParentID:     s.ParentID,
		CategoryPath: append([]string(nil), s.CategoryPath...),
		Dependencies: append([]string(nil), s.Dependencies...),
		Lineage:      append([]LineageEntry(nil), s.Lineage...),
	}

	for i, step := range s.Steps {
		newArgs := make(map[string]interface{})
		for k, v := range step.Args {
			newArgs[k] = substituteParamValue(v, merged)
		}
		// Override with merged runtime args where keys match (exact key replacement)
		for k, v := range merged {
			if _, exists := newArgs[k]; exists {
				newArgs[k] = substituteParamValue(v, merged)
			}
		}
		newSkill.Steps[i] = Step{Tool: step.Tool, Args: newArgs}
	}

	return newSkill
}

// substituteParamValue replaces __PARAM__name placeholders recursively in strings, maps, and slices.
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

// ToJSON serializes the skill.
func (s *Skill) ToJSON() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// FromJSON deserializes a skill.
func FromJSON(data []byte) (*Skill, error) {
	var s Skill
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("invalid skill JSON: %w", err)
	}
	return &s, nil
}

// Validate checks basic correctness.
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
