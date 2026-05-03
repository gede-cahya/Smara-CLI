package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	cacheDirName = ".smara/registry-cache"
	cacheTTL     = 24 * time.Hour
)

// CachedManifest wraps a manifest with metadata for TTL checking.
type CachedManifest struct {
	Manifest    RegistryManifest `json:"manifest"`
	FetchedAt   time.Time        `json:"fetched_at"`
	RegistryURL string           `json:"registry_url"`
}

// ensureCacheDir creates the registry cache directory.
func ensureCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, cacheDirName)
	return dir, os.MkdirAll(dir, 0755)
}

// cachePath returns the full path for a cached registry file.
func cachePath(registryName string) (string, error) {
	dir, err := ensureCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, registryName+".json"), nil
}

// WriteCache stores a manifest to the local cache.
func WriteCache(registryName string, manifest RegistryManifest) error {
	path, err := cachePath(registryName)
	if err != nil {
		return err
	}

	entry := CachedManifest{
		Manifest:    manifest,
		FetchedAt:   time.Now().UTC(),
		RegistryURL: manifest.RegistryURL,
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// ReadCache reads a cached manifest if it exists and is not expired.
func ReadCache(registryName string) (*RegistryManifest, bool) {
	path, err := cachePath(registryName)
	if err != nil {
		return nil, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var entry CachedManifest
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}

	if time.Since(entry.FetchedAt) > cacheTTL {
		return nil, false
	}

	return &entry.Manifest, true
}

// ClearCache removes all cached registry manifests.
func ClearCache() error {
	dir, err := ensureCacheDir()
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	return nil
}

// SyncRegistries fetches manifests from all registries and writes them to local cache.
// The built-in embedded registry is always synced so search works offline.
func SyncRegistries(registries []RegistryConfig) error {
	var errs []string
	for _, reg := range registries {
		manifest, err := FetchManifest(reg.URL, reg.AuthToken)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", reg.Name, err))
			continue
		}
		if err := WriteCache(reg.Name, *manifest); err != nil {
			errs = append(errs, fmt.Sprintf("%s cache: %v", reg.Name, err))
		}
	}

	// Always sync built-in embedded registry to cache for offline fallback
	if builtinManifest != nil {
		if err := WriteCache("builtin", *builtinManifest); err != nil {
			errs = append(errs, fmt.Sprintf("builtin cache: %v", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("sync errors: %s", errs[0])
	}
	return nil
}
