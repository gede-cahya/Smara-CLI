package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectLibrariesFromPrompt(t *testing.T) {
	tests := []struct {
		name           string
		prompt         string
		expectCount    int
		expectContains []string // expected library names
	}{
		{
			name:           "Next.js keyword",
			prompt:         "buatkan komponen Next.js dengan app router dan next/image",
			expectCount:    1,
			expectContains: []string{"nextjs"},
		},
		{
			name:           "React hooks",
			prompt:         "buatkan custom hook useEffect untuk fetch data",
			expectCount:    1,
			expectContains: []string{"react"},
		},
		{
			name:           "Docker and Kubernetes",
			prompt:         "buatkan dockerfile dan deployment k8s untuk service ini",
			expectCount:    2,
			expectContains: []string{"docker", "kubernetes"},
		},
		{
			name:           "Multiple matches - Go + Docker",
			prompt:         "deploy aplikasi golang menggunakan docker container",
			expectCount:    2,
			expectContains: []string{"go", "docker"},
		},
		{
			name:           "Tailwind classes",
			prompt:         "buatkan responsive grid dengan tailwind utility class",
			expectCount:    1,
			expectContains: []string{"tailwindcss"},
		},
		{
			name:        "No matches",
			prompt:      "halo apa kabar hari ini",
			expectCount: 0,
		},
		{
			name:           "PostgreSQL query",
			prompt:         "optimasi query postgres dengan index",
			expectCount:    1,
			expectContains: []string{"postgres"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := DetectLibrariesFromPrompt(tt.prompt)
			require.NoError(t, err)
			assert.Len(t, results, tt.expectCount)

			for _, expected := range tt.expectContains {
				found := false
				for _, r := range results {
					if r.Name == expected {
						found = true
						break
					}
				}
				assert.True(t, found, "expected library '%s' not found in results", expected)
			}
		})
	}
}

func TestSearchContext7Registry(t *testing.T) {
	results, err := SearchContext7Registry("react")
	require.NoError(t, err)
	require.NotEmpty(t, results)

	found := false
	for _, r := range results {
		if r.Name == "react" || r.Name == "nextjs" {
			found = true
		}
	}
	assert.True(t, found, "should find react or nextjs when searching 'react'")
}

func TestSearchContext7Registry_NoResults(t *testing.T) {
	results, err := SearchContext7Registry("xyznonexistent")
	require.NoError(t, err)
	assert.Empty(t, results)
}
