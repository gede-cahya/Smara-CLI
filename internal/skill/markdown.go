package skill

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// markdownFrontmatter is the YAML structure for skill metadata in markdown files.
// It mirrors the Skill struct for easy unmarshaling.
type markdownFrontmatter struct {
	Name        string     `yaml:"name"`
	Description string     `yaml:"description"`
	Version     int        `yaml:"version"`
	Tags        []string   `yaml:"tags,omitempty"`
	Author      string     `yaml:"author,omitempty"`
	SourceURL   string     `yaml:"source_url,omitempty"`
	Trigger     string     `yaml:"trigger,omitempty"`
	Params      []ParamDef `yaml:"params,omitempty"`
	Steps       []Step     `yaml:"steps"`
}

// ParseMarkdownSkill parses a skill from markdown-with-frontmatter format.
// The format is:
//
//	---
//	name: skill-name
//	version: 1
//	steps:
//	  - tool: echo
//	    args:
//	      msg: hello
//	---
//
//	# Skill Title
//
//	Description in markdown...
func ParseMarkdownSkill(data []byte) (*Skill, error) {
	content := string(data)
	content = strings.TrimSpace(content)

	// Must start with ---
	if !strings.HasPrefix(content, "---") {
		return nil, fmt.Errorf("markdown skill must start with frontmatter delimiter ---")
	}

	// Find end of frontmatter
	endIdx := strings.Index(content[3:], "---")
	if endIdx == -1 {
		return nil, fmt.Errorf("markdown skill missing closing frontmatter delimiter ---")
	}
	endIdx += 3 // account for the initial ---

	frontmatterStr := content[3:endIdx]
	frontmatterStr = strings.TrimSpace(frontmatterStr)

	var fm markdownFrontmatter
	if err := yaml.Unmarshal([]byte(frontmatterStr), &fm); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter YAML: %w", err)
	}

	// If description is empty, try to extract from markdown body (first paragraph after # heading)
	if fm.Description == "" {
		body := strings.TrimSpace(content[endIdx+3:])
		fm.Description = extractFirstParagraph(body)
	}

	sk := &Skill{
		Name:        fm.Name,
		Description: fm.Description,
		Steps:       fm.Steps,
		Version:     fm.Version,
		Tags:        fm.Tags,
		Author:      fm.Author,
		SourceURL:   fm.SourceURL,
		Trigger:     fm.Trigger,
		Params:      fm.Params,
	}

	if err := sk.Validate(); err != nil {
		return nil, fmt.Errorf("parsed markdown skill invalid: %w", err)
	}

	return sk, nil
}

// ToMarkdown serializes a skill to markdown-with-frontmatter format.
// The body contains a title and description for human readability.
func (s *Skill) ToMarkdown() ([]byte, error) {
	fm := markdownFrontmatter{
		Name:        s.Name,
		Description: s.Description,
		Version:     s.Version,
		Tags:        s.Tags,
		Author:      s.Author,
		SourceURL:   s.SourceURL,
		Trigger:     s.Trigger,
		Params:      s.Params,
		Steps:       s.Steps,
	}

	yamlBytes, err := yaml.Marshal(fm)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal frontmatter: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(yamlBytes)
	buf.WriteString("---\n\n")

	// Human-readable body
	buf.WriteString(fmt.Sprintf("# %s\n\n", s.Name))
	if s.Description != "" {
		buf.WriteString(s.Description)
		buf.WriteString("\n\n")
	}

	if len(s.Steps) > 0 {
		buf.WriteString("## Steps\n\n")
		for i, st := range s.Steps {
			buf.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, st.Tool))
			if len(st.Args) > 0 {
				for k, v := range st.Args {
					buf.WriteString(fmt.Sprintf("   - `%s`: `%v`\n", k, v))
				}
			}
			buf.WriteString("\n")
		}
	}

	return buf.Bytes(), nil
}

// IsMarkdownSkill detects if raw data is a markdown skill (starts with ---).
func IsMarkdownSkill(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	return bytes.HasPrefix(trimmed, []byte("---"))
}

// extractFirstParagraph pulls the first non-heading paragraph from markdown body.
func extractFirstParagraph(body string) string {
	lines := strings.Split(body, "\n")
	var para strings.Builder
	inPara := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if inPara {
				break
			}
			continue
		}
		// Skip markdown headings
		if strings.HasPrefix(line, "#") {
			continue
		}
		inPara = true
		if para.Len() > 0 {
			para.WriteString(" ")
		}
		para.WriteString(line)
	}
	result := strings.TrimSpace(para.String())
	if len(result) > 200 {
		result = result[:200] + "..."
	}
	return result
}
