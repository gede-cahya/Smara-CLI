package skill

import (
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseCodexSkillMarkdown converts a Codex-style folder skill
// (`<skill>/SKILL.md`) into a Smara skill. The markdown body is preserved as a
// single instruction step so the agent can read the workflow and continue with
// normal tools.
func ParseCodexSkillMarkdown(data []byte, fallbackName, skillDir string) (*Skill, error) {
	content := strings.TrimSpace(string(data))
	if !strings.HasPrefix(content, "---") {
		return nil, fmt.Errorf("codex skill must start with frontmatter delimiter ---")
	}

	endIdx := strings.Index(content[3:], "---")
	if endIdx < 0 {
		return nil, fmt.Errorf("codex skill missing closing frontmatter delimiter ---")
	}
	endIdx += 3

	var fm markdownFrontmatter
	if err := yaml.Unmarshal([]byte(strings.TrimSpace(content[3:endIdx])), &fm); err != nil {
		return nil, fmt.Errorf("failed to parse codex skill frontmatter: %w", err)
	}

	body := strings.TrimSpace(content[endIdx+3:])
	name := strings.TrimSpace(fm.Name)
	if name == "" {
		name = strings.TrimSpace(fallbackName)
	}
	if name == "" && skillDir != "" {
		name = filepath.Base(skillDir)
	}
	if name == "" {
		return nil, fmt.Errorf("codex skill name is required")
	}

	description := strings.TrimSpace(fm.Description)
	if description == "" {
		description = extractFirstParagraph(body)
	}
	version := fm.Version
	if version == 0 {
		version = 1
	}

	instructions := strings.TrimSpace(body)
	if instructions == "" {
		instructions = description
	}

	sk := &Skill{
		Name:        name,
		Description: description,
		Version:     version,
		Tags:        fm.Tags,
		Author:      fm.Author,
		SourceURL:   fm.SourceURL,
		Trigger:     fm.Trigger,
		Params:      fm.Params,
		Steps: []Step{{
			Tool: "skill_instructions",
			Args: map[string]interface{}{
				"skill_name":   name,
				"skill_dir":    skillDir,
				"trigger":      fm.Trigger,
				"instructions": instructions,
			},
		}},
	}
	if err := sk.Validate(); err != nil {
		return nil, fmt.Errorf("parsed codex skill invalid: %w", err)
	}
	return sk, nil
}

// ParseExternalInstructionMarkdown wraps a generic external markdown file
// (Claude Code command/agent, Antigravity rule, plain prompt workflow, etc.)
// as a Codex-style instruction skill. If YAML frontmatter exists, name,
// description, trigger and tags are reused when present.
func ParseExternalInstructionMarkdown(data []byte, fallbackName, sourcePath string) (*Skill, error) {
	content := strings.TrimSpace(string(data))
	if strings.HasPrefix(content, "---") {
		if sk, err := ParseCodexSkillMarkdown(data, fallbackName, filepath.Dir(sourcePath)); err == nil {
			return sk, nil
		}
	}

	name := sanitizeSkillName(fallbackName)
	if name == "" {
		name = sanitizeSkillName(strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath)))
	}
	if name == "" {
		return nil, fmt.Errorf("external markdown skill name is required")
	}
	description := extractFirstParagraph(content)
	if description == "" {
		description = "External markdown instruction skill imported into Smara."
	}
	trigger := ""
	if strings.HasSuffix(strings.ToLower(sourcePath), ".md") {
		base := strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
		if base != "" {
			trigger = "/" + base
		}
	}

	sk := &Skill{
		Name:        name,
		Description: description,
		Version:     1,
		Trigger:     trigger,
		Tags:        []string{"external", "markdown"},
		SourceURL:   sourcePath,
		Steps: []Step{{
			Tool: "skill_instructions",
			Args: map[string]interface{}{
				"skill_name":   name,
				"skill_dir":    filepath.Dir(sourcePath),
				"trigger":      trigger,
				"instructions": content,
			},
		}},
	}
	if err := sk.Validate(); err != nil {
		return nil, fmt.Errorf("parsed external markdown skill invalid: %w", err)
	}
	return sk, nil
}

func sanitizeSkillName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if ok {
			b.WriteRune(r)
			lastDash = r == '-'
			continue
		}
		if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-_")
}
