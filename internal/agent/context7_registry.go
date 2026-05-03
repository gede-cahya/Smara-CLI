package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

// Context7RegistryEntry maps keywords to a Context7 library for auto-resolve.
type Context7RegistryEntry struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Tags           []string `json:"tags"`
	Keywords       []string `json:"keywords"`
	Context7Library string  `json:"context7_library"`
	SkillFile      string   `json:"skill_file,omitempty"`
}

// Context7RegistryManifest is the top-level curated library index.
type Context7RegistryManifest struct {
	Version int                     `json:"version"`
	Skills  []Context7RegistryEntry `json:"skills"`
}

// loadContext7Registry loads the embedded registry index from the skills directory.
// It searches in the following order:
// 1. Working directory: skills/registry/index.json
// 2. Executable directory: skills/registry/index.json (for standalone binary)
// 3. Repository root: skills/registry/index.json (for dev mode)
func loadContext7Registry() (*Context7RegistryManifest, error) {
	// 1. Try embedded registry first (works for standalone binaries)
	if manifest, err := loadEmbeddedContext7Registry(); err == nil {
		return manifest, nil
	}

	candidates := []string{
		"skills/registry/index.json",
	}

	// Try executable directory (for installed binary)
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates, filepath.Join(exeDir, "skills", "registry", "index.json"))
	}

	// Try repository root relative to this file (for dev)
	_, thisFile, _, ok := runtime.Caller(0)
	if ok {
		repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
		candidates = append(candidates, filepath.Join(repoRoot, "skills", "registry", "index.json"))
	}

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var manifest Context7RegistryManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue
		}
		return &manifest, nil
	}

	return nil, fmt.Errorf("context7 registry index not found (tried %d paths)", len(candidates))
}

// SearchContext7Registry searches the registry for entries matching query.
// Matches against name, description, tags, and keywords.
func SearchContext7Registry(query string) ([]Context7RegistryEntry, error) {
	manifest, err := loadContext7Registry()
	if err != nil {
		return nil, err
	}

	queryLower := strings.ToLower(query)
	var results []Context7RegistryEntry

	for _, entry := range manifest.Skills {
		if matchesContext7Entry(entry, queryLower) {
			results = append(results, entry)
		}
	}

	return results, nil
}

func matchesContext7Entry(entry Context7RegistryEntry, queryLower string) bool {
	if strings.Contains(strings.ToLower(entry.Name), queryLower) {
		return true
	}
	if strings.Contains(strings.ToLower(entry.Description), queryLower) {
		return true
	}
	for _, kw := range entry.Keywords {
		if strings.Contains(strings.ToLower(kw), queryLower) {
			return true
		}
	}
	for _, tag := range entry.Tags {
		if strings.Contains(strings.ToLower(tag), queryLower) {
			return true
		}
	}
	return false
}

// DetectLibrariesFromPrompt scans a user prompt and returns matching registry entries.
// Keywords are matched with word boundaries to avoid false positives (e.g., "image" in
// "next/image" matching docker, "class" in "utility class" matching python).
func DetectLibrariesFromPrompt(prompt string) ([]Context7RegistryEntry, error) {
	manifest, err := loadContext7Registry()
	if err != nil {
		return nil, err
	}

	promptLower := strings.ToLower(prompt)
	var results []Context7RegistryEntry
	seen := make(map[string]bool)

	for _, entry := range manifest.Skills {
		if seen[entry.Name] {
			continue
		}
		for _, kw := range entry.Keywords {
			if keywordMatchesPrompt(kw, promptLower) {
				results = append(results, entry)
				seen[entry.Name] = true
				break
			}
		}
	}

	return results, nil
}

// keywordMatchesPrompt checks if a keyword appears as a whole word/phrase in the prompt.
// Multi-word keywords are matched as phrases; single-word keywords require word boundaries.
func keywordMatchesPrompt(keyword, promptLower string) bool {
	kwLower := strings.ToLower(keyword)
	// For multi-word keywords, require exact phrase match with word boundaries on both ends
	if strings.Contains(kwLower, " ") {
		return strings.Contains(promptLower, kwLower)
	}
	// For single-word keywords, use a simple boundary check:
	// the keyword must not be immediately preceded or followed by [a-z0-9_]
	idx := strings.Index(promptLower, kwLower)
	for idx != -1 {
		beforeOK := idx == 0 || !isWordChar(promptLower[idx-1])
		afterOK := idx+len(kwLower) == len(promptLower) || !isWordChar(promptLower[idx+len(kwLower)])
		if beforeOK && afterOK {
			return true
		}
		idx = strings.Index(promptLower[idx+1:], kwLower)
		if idx != -1 {
			idx++ // adjust for the +1 offset
		}
	}
	return false
}

func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_'
}

// InstallContext7Skill creates a skill recipe for a Context7 library entry and saves it.
// It first tries to load a predefined skill file from the skills/ directory.
// If not found, it generates a generic context7-help wrapper skill.
func InstallContext7Skill(entry Context7RegistryEntry) (*skill.Skill, error) {
	// 1. Try to load a predefined skill file from the repo skills/ directory
	candidates := []string{
		filepath.Join("skills", entry.SkillFile),
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates, filepath.Join(exeDir, "skills", entry.SkillFile))
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if ok {
		repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
		candidates = append(candidates, filepath.Join(repoRoot, "skills", entry.SkillFile))
	}

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var sk skill.Skill
		if err := json.Unmarshal(data, &sk); err == nil {
			if err := skill.Save(&sk, nil); err != nil {
				return nil, fmt.Errorf("failed to save predefined skill '%s': %w", entry.Name, err)
			}
			return &sk, nil
		}
	}

	// 2. Generate a generic context7-help wrapper skill
	sk := &skill.Skill{
		Name:        entry.Name,
		Description: entry.Description,
		Version:     1,
		Tags:        append([]string{"context7", "auto-generated"}, entry.Tags...),
		Params: []skill.ParamDef{
			{
				Name:        "library",
				Type:        "string",
				Description: fmt.Sprintf("Library name (default: %s)", entry.Context7Library),
				Required:    false,
				Default:     entry.Context7Library,
			},
			{
				Name:        "topic",
				Type:        "string",
				Description: "Specific topic to look up",
				Required:    false,
			},
		},
		Steps: []skill.Step{
			{
				Tool: "resolve",
				Args: map[string]interface{}{
					"libraryName": "__PARAM__library",
					"version":     "",
				},
			},
			{
				Tool: "get-library-documentation",
				Args: map[string]interface{}{
					"uri":   "{{step_0.result.uri}}",
					"topic": "__PARAM__topic",
				},
			},
		},
	}

	if err := sk.Validate(); err != nil {
		return nil, fmt.Errorf("generated skill validation failed: %w", err)
	}
	if err := skill.Save(sk, nil); err != nil {
		return nil, fmt.Errorf("failed to save generated skill '%s': %w", entry.Name, err)
	}
	return sk, nil
}
