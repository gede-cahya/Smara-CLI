package skill

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveURL_Direct(t *testing.T) {
	input := "https://example.com/skill.json"
	assert.Equal(t, input, resolveURL(input))
}

func TestResolveURL_RawGitHub(t *testing.T) {
	input := "https://raw.githubusercontent.com/user/repo/main/skill.json"
	assert.Equal(t, input, resolveURL(input))
}

func TestResolveURL_GitHubBlob(t *testing.T) {
	input := "https://github.com/user/repo/blob/main/skill.json"
	expected := "https://raw.githubusercontent.com/user/repo/main/skill.json"
	assert.Equal(t, expected, resolveURL(input))
}

func TestResolveURL_GitHubBlobBranchWithPath(t *testing.T) {
	input := "https://github.com/user/repo/blob/develop/skills/deploy.json"
	expected := "https://raw.githubusercontent.com/user/repo/develop/skills/deploy.json"
	assert.Equal(t, expected, resolveURL(input))
}

func TestResolveURL_GitHubShorthand(t *testing.T) {
	input := "user/repo/skill.json"
	expected := "https://raw.githubusercontent.com/user/repo/main/skill.json"
	assert.Equal(t, expected, resolveURL(input))
}

func TestResolveURL_Gist(t *testing.T) {
	input := "https://gist.github.com/user/abc123"
	expected := "https://gist.githubusercontent.com/raw/abc123/skill.json"
	assert.Equal(t, expected, resolveURL(input))
}

func TestResolveURL_GistWithUser(t *testing.T) {
	// gist format can vary; the last segment is the gist ID
	input := "https://gist.github.com/abc123"
	expected := "https://gist.githubusercontent.com/raw/abc123/skill.json"
	assert.Equal(t, expected, resolveURL(input))
}

func TestResolveURL_WithTrailingSlash(t *testing.T) {
	input := "https://raw.githubusercontent.com/user/repo/main/skill.json/"
	// Should pass through since it contains raw.githubusercontent.com
	assert.Equal(t, input, resolveURL(input))
}
