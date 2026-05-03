// Package skill provides a public API for Smara's skill (automation recipe) system.
// Skills are JSON-defined step-by-step recipes that execute tools through a StepExecutor.
package skill

import (
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

// Re-export core types for public consumption.
type (
	// Skill is a reusable automation recipe.
	Skill = skill.Skill
	// Step is one tool call inside a skill recipe.
	Step = skill.Step
	// ParamDef defines a configurable parameter for a skill.
	ParamDef = skill.ParamDef
	// StepResult holds the outcome of one step.
	StepResult = skill.StepResult
	// RunResult holds the full execution outcome.
	RunResult = skill.RunResult
	// StepExecutor is a function that runs a single tool.
	StepExecutor = skill.StepExecutor
	// WorkflowCapture holds tool calls from a successful workflow run.
	WorkflowCapture = skill.WorkflowCapture
	// CapturedStep represents one executed tool call from a workflow.
	CapturedStep = skill.CapturedStep
	// RegistryEntry represents a skill listing in a marketplace manifest.
	RegistryEntry = skill.RegistryEntry
	// RegistryManifest is the top-level marketplace index.
	RegistryManifest = skill.RegistryManifest
	// RegistryConfig defines a configured registry source.
	RegistryConfig = skill.RegistryConfig
	// InstallOptions configures a remote install.
	InstallOptions = skill.InstallOptions
	// ExecutionTracker logs skill runs to SQLite.
	ExecutionTracker = skill.ExecutionTracker
	// SkillExecution tracks one run of a skill.
	SkillExecution = skill.SkillExecution
	// SkillImprovement tracks proposed refinements.
	SkillImprovement = skill.SkillImprovement
	// TreeManager builds and queries the skill tree.
	TreeManager = skill.TreeManager
	// TreeNode represents a skill in the tree.
	TreeNode = skill.TreeNode
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

// InstallFromURL downloads, validates, and saves a skill from a remote URL.
func InstallFromURL(opts InstallOptions) (*Skill, error) {
	return skill.InstallFromURL(opts)
}

// UpdateSkill re-downloads a skill from its SourceURL and saves the new version.
func UpdateSkill(name string) (*Skill, error) {
	return skill.UpdateSkill(name)
}

// GenerateFromWorkflow uses LLM to convert captured steps into a Skill.
func GenerateFromWorkflow(capture WorkflowCapture, provider llm.Provider) (*Skill, error) {
	return skill.GenerateFromWorkflow(capture, provider)
}

// Search searches all configured registries for skills matching query and/or tags.
func Search(query string, registries []RegistryConfig) ([]RegistryEntry, error) {
	return skill.Search(query, registries)
}

// Publish creates a manifest entry for a skill.
func Publish(sk *Skill, registry RegistryConfig) error {
	return skill.Publish(sk, registry)
}

// SyncRegistries fetches manifests from all registries and writes them to local cache.
func SyncRegistries(registries []RegistryConfig) error {
	return skill.SyncRegistries(registries)
}

// ParseMarkdownSkill parses a skill from markdown-with-frontmatter format.
func ParseMarkdownSkill(data []byte) (*Skill, error) {
	return skill.ParseMarkdownSkill(data)
}

// SaveAsMarkdown stores a skill as a markdown-with-frontmatter file.
func SaveAsMarkdown(s *Skill, db interface{}) error {
	return skill.SaveAsMarkdown(s, nil)
}

// IsMarkdownSkill detects if raw data is a markdown skill (starts with ---).
func IsMarkdownSkill(data []byte) bool {
	return skill.IsMarkdownSkill(data)
}
