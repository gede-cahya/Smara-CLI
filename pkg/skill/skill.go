// Package skill provides a public API for Smara's skill (automation recipe) system.
// Skills are JSON-defined step-by-step recipes that execute tools through a StepExecutor.
package skill

import (
	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

// Re-export core types for public consumption.
type (
	// Skill is a reusable automation recipe.
	Skill = skill.Skill
	// Step is one tool call inside a skill recipe.
	Step = skill.Step
	// StepResult holds the outcome of one step.
	StepResult = skill.StepResult
	// RunResult holds the full execution outcome.
	RunResult = skill.RunResult
	// StepExecutor is a function that runs a single tool.
	StepExecutor = skill.StepExecutor
)

// Load retrieves a skill by name.
func Load(name string) (*Skill, error) {
	return skill.Load(name)
}

// Save stores a skill.
func Save(s *Skill, db interface{}) error {
	return skill.Save(s, nil)
}

// Delete removes a skill by name.
func Delete(name string, db interface{}) error {
	return skill.Delete(name, nil)
}

// List returns all saved skill names.
func List() ([]string, error) {
	return skill.List()
}

// FromJSON deserializes a skill from JSON.
func FromJSON(data []byte) (*Skill, error) {
	return skill.FromJSON(data)
}
