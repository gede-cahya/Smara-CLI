package skill

import (
	"encoding/json"
	"fmt"
)

// Step is one tool call inside a skill recipe.
type Step struct {
	Tool string                 `json:"tool"`
	Args map[string]interface{} `json:"args"`
}

// Skill is a reusable automation recipe.
type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Steps       []Step `json:"steps"`
	Version     int    `json:"version"`
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
