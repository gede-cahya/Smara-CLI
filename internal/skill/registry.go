package skill

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	registryTimeout = 30 * time.Second
	maxRegistrySize = 1 << 20 // 1 MB
)

// RegistryEntry represents a skill listing in a marketplace manifest.
type RegistryEntry struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Version     int       `json:"version"`
	Author      string    `json:"author"`
	URL         string    `json:"url"`
	Tags        []string  `json:"tags"`
	Downloads   int       `json:"downloads"`
	Rating      float64   `json:"rating"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RegistryManifest is the top-level marketplace index.
type RegistryManifest struct {
	RegistryURL string          `json:"registry_url"`
	Version     string          `json:"version"`
	Skills      []RegistryEntry `json:"skills"`
}

// RegistryConfig defines a configured registry source (duplicated here to avoid import cycle).
type RegistryConfig struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	AuthToken string `json:"auth_token,omitempty"`
}

// FetchManifest downloads a registry manifest from a URL.
func FetchManifest(url string, authToken string) (*RegistryManifest, error) {
	client := &http.Client{Timeout: registryTimeout}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry fetch failed: %s", resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRegistrySize))
	if err != nil {
		return nil, err
	}

	var manifest RegistryManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("invalid registry manifest JSON: %w", err)
	}

	return &manifest, nil
}

// Search searches all configured registries for skills matching query and/or tags.
// If query is empty, returns all skills. query is matched against name, description, and tags.
// Falls back to the built-in embedded registry when all external sources fail.
func Search(query string, registries []RegistryConfig) ([]RegistryEntry, error) {
	var results []RegistryEntry
	queryLower := strings.ToLower(query)
	allFailed := true

	for _, regCfg := range registries {
		manifest, err := FetchManifest(regCfg.URL, regCfg.AuthToken)
		if err != nil {
			// Log but don't fail — try other registries
			continue
		}
		allFailed = false

		for _, entry := range manifest.Skills {
			if query == "" || matchesQuery(entry, queryLower) {
				results = append(results, entry)
			}
		}
	}

	// Fallback to built-in embedded registry if all external sources failed or no results
	if (allFailed || len(results) == 0) && builtinManifest != nil {
		for _, entry := range builtinManifest.Skills {
			if query == "" || matchesQuery(entry, queryLower) {
				results = append(results, entry)
			}
		}
	}

	return results, nil
}

func matchesQuery(entry RegistryEntry, queryLower string) bool {
	if strings.Contains(strings.ToLower(entry.Name), queryLower) {
		return true
	}
	if strings.Contains(strings.ToLower(entry.Description), queryLower) {
		return true
	}
	for _, tag := range entry.Tags {
		if strings.Contains(strings.ToLower(tag), queryLower) {
			return true
		}
	}
	return false
}

// Publish creates a manifest entry for a skill and uploads it to a registry.
// This is a placeholder — actual publish depends on registry backend (GitHub PR, API, etc.)
func Publish(sk *Skill, registry RegistryConfig) error {
	if sk.SourceURL == "" {
		return fmt.Errorf("skill '%s' has no source_url — install from remote or set source_url first", sk.Name)
	}

	entry := RegistryEntry{
		Name:        sk.Name,
		Description: sk.Description,
		Version:     sk.Version,
		Author:      sk.Author,
		URL:         sk.SourceURL,
		Tags:        sk.Tags,
		UpdatedAt:   time.Now().UTC(),
	}

	// In a real implementation, this would POST to the registry API
	// or create a PR against a GitHub repo. For now, we serialize and
	// instruct the user to submit manually.
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}

	fmt.Printf("Registry entry untuk '%s':\n%s\n", sk.Name, string(data))
	fmt.Println("\nSilakan submit entry ini ke registry di atas. Auto-publish belum diimplementasikan.")
	return nil
}
