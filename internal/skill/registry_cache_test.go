package skill

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteCacheAndReadCache(t *testing.T) {
	// Use a temp home dir to avoid polluting real cache
	testDir := t.TempDir()
	t.Setenv("HOME", testDir)

	manifest := RegistryManifest{
		RegistryURL: "https://example.com/registry",
		Version:     "1.0",
		Skills: []RegistryEntry{
			{
				Name:        "test-skill",
				Description: "A test skill",
				Version:     1,
				Author:      "tester",
				URL:         "https://example.com/skill.json",
				Tags:        []string{"test"},
				Downloads:   42,
				Rating:      4.5,
				UpdatedAt:   time.Now().UTC(),
			},
		},
	}

	require.NoError(t, WriteCache("test-reg", manifest))

	cached, ok := ReadCache("test-reg")
	assert.True(t, ok)
	assert.NotNil(t, cached)
	assert.Equal(t, manifest.RegistryURL, cached.RegistryURL)
	assert.Len(t, cached.Skills, 1)
	assert.Equal(t, "test-skill", cached.Skills[0].Name)
}

func TestReadCache_Miss(t *testing.T) {
	testDir := t.TempDir()
	t.Setenv("HOME", testDir)

	cached, ok := ReadCache("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, cached)
}

func TestReadCache_Expired(t *testing.T) {
	testDir := t.TempDir()
	t.Setenv("HOME", testDir)

	manifest := RegistryManifest{
		RegistryURL: "https://example.com/registry",
		Version:     "1.0",
		Skills:      []RegistryEntry{},
	}

	// Write cache entry with an expired FetchedAt timestamp
	oldTime := time.Now().Add(-25 * time.Hour).UTC()
	entry := CachedManifest{
		Manifest:    manifest,
		FetchedAt:   oldTime,
		RegistryURL: manifest.RegistryURL,
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	require.NoError(t, err)

	cacheDir, err := ensureCacheDir()
	require.NoError(t, err)
	cacheFile := filepath.Join(cacheDir, "expired-reg.json")
	require.NoError(t, os.WriteFile(cacheFile, data, 0644))

	cached, ok := ReadCache("expired-reg")
	assert.False(t, ok)
	assert.Nil(t, cached)
}

func TestClearCache(t *testing.T) {
	testDir := t.TempDir()
	t.Setenv("HOME", testDir)

	manifest := RegistryManifest{
		RegistryURL: "https://example.com/registry",
		Version:     "1.0",
	}

	require.NoError(t, WriteCache("clear-me", manifest))
	_, ok := ReadCache("clear-me")
	assert.True(t, ok)

	require.NoError(t, ClearCache())

	_, ok = ReadCache("clear-me")
	assert.False(t, ok)
}

func TestMatchesQuery(t *testing.T) {
	entry := RegistryEntry{
		Name:        "deploy-site",
		Description: "Deploy static site to hosting",
		Tags:        []string{"deploy", "static", "hosting"},
	}

	assert.True(t, matchesQuery(entry, "deploy"))
	assert.True(t, matchesQuery(entry, "static"))
	assert.True(t, matchesQuery(entry, "hosting"))
	assert.False(t, matchesQuery(entry, "docker"))
}
