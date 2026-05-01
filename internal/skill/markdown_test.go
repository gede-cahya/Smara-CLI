package skill

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMarkdownSkill_Valid(t *testing.T) {
	input := `---
name: deploy-site
version: 2
description: Deploy static site
tags: [deploy, frontend]
author: tester
source_url: https://example.com
params:
  - name: env
    type: string
    default: production
steps:
  - tool: build
    args:
      cmd: npm run build
  - tool: deploy
    args:
      provider: netlify
---

# Deploy Site

Deploy static site ke hosting.
`
	sk, err := ParseMarkdownSkill([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, "deploy-site", sk.Name)
	assert.Equal(t, 2, sk.Version)
	assert.Equal(t, "Deploy static site", sk.Description)
	assert.Equal(t, []string{"deploy", "frontend"}, sk.Tags)
	assert.Equal(t, "tester", sk.Author)
	assert.Equal(t, "https://example.com", sk.SourceURL)
	require.Len(t, sk.Params, 1)
	assert.Equal(t, "env", sk.Params[0].Name)
	assert.Equal(t, "string", sk.Params[0].Type)
	assert.Equal(t, "production", sk.Params[0].Default)
	require.Len(t, sk.Steps, 2)
	assert.Equal(t, "build", sk.Steps[0].Tool)
	assert.Equal(t, "npm run build", sk.Steps[0].Args["cmd"])
	assert.Equal(t, "deploy", sk.Steps[1].Tool)
	assert.Equal(t, "netlify", sk.Steps[1].Args["provider"])
}

func TestParseMarkdownSkill_MissingFrontmatterDelimiter(t *testing.T) {
	input := `name: deploy-site
version: 1
---
`
	_, err := ParseMarkdownSkill([]byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must start with frontmatter")
}

func TestParseMarkdownSkill_MissingClosingDelimiter(t *testing.T) {
	input := `---
name: deploy-site
`
	_, err := ParseMarkdownSkill([]byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing closing")
}

func TestParseMarkdownSkill_InvalidYAML(t *testing.T) {
	input := `---
name: [invalid
---
`
	_, err := ParseMarkdownSkill([]byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse frontmatter")
}

func TestParseMarkdownSkill_EmptyDescriptionFallsBackToBody(t *testing.T) {
	input := `---
name: test-skill
version: 1
steps:
  - tool: echo
---

# Test Skill

This is the fallback description from the body paragraph.
`
	sk, err := ParseMarkdownSkill([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, "test-skill", sk.Name)
	assert.Equal(t, "This is the fallback description from the body paragraph.", sk.Description)
}

func TestSkill_ToMarkdown(t *testing.T) {
	sk := &Skill{
		Name:        "deploy-site",
		Description: "Deploy static site",
		Version:     3,
		Tags:        []string{"deploy", "frontend"},
		Author:      "tester",
		SourceURL:   "https://example.com",
		Params: []ParamDef{
			{Name: "env", Type: "string", Default: "production"},
		},
		Steps: []Step{
			{Tool: "build", Args: map[string]interface{}{"cmd": "npm run build"}},
			{Tool: "deploy", Args: map[string]interface{}{"provider": "netlify"}},
		},
	}
	data, err := sk.ToMarkdown()
	require.NoError(t, err)
	output := string(data)
	assert.Contains(t, output, "---")
	assert.Contains(t, output, "name: deploy-site")
	assert.Contains(t, output, "version: 3")
	assert.Contains(t, output, "# deploy-site")
	assert.Contains(t, output, "## Steps")
	assert.Contains(t, output, "1. **build**")
	assert.Contains(t, output, "2. **deploy**")
}

func TestIsMarkdownSkill_True(t *testing.T) {
	assert.True(t, IsMarkdownSkill([]byte("---\nname: test\n---\n")))
}

func TestIsMarkdownSkill_TrueWithWhitespace(t *testing.T) {
	assert.True(t, IsMarkdownSkill([]byte("  \n  ---\nname: test\n")))
}

func TestIsMarkdownSkill_False(t *testing.T) {
	assert.False(t, IsMarkdownSkill([]byte(`{"name":"test"}`)))
}

func TestIsMarkdownSkill_Empty(t *testing.T) {
	assert.False(t, IsMarkdownSkill([]byte("")))
}

func TestExtractFirstParagraph(t *testing.T) {
	body := "# Title\n\nFirst paragraph line one.\nLine two.\n\nSecond paragraph."
	result := extractFirstParagraph(body)
	assert.Equal(t, "First paragraph line one. Line two.", result)
}

func TestExtractFirstParagraph_NoHeading(t *testing.T) {
	body := "Just a paragraph.\nWith more text."
	result := extractFirstParagraph(body)
	assert.Equal(t, "Just a paragraph. With more text.", result)
}
