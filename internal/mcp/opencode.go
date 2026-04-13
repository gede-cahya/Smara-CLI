// Package mcp - opencode.go loads MCP server configurations from OpenCode's config file.
package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// OpenCodeConfig represents the relevant parts of OpenCode's config JSON.
type OpenCodeConfig struct {
	MCP map[string]OpenCodeMCPEntry `json:"mcp"`
}

// OpenCodeMCPEntry represents a single MCP server entry in OpenCode config.
type OpenCodeMCPEntry struct {
	Type        string            `json:"type"`                  // "local" or "remote"
	Command     []string          `json:"command,omitempty"`     // for local servers
	URL         string            `json:"url,omitempty"`         // for remote servers
	Headers     map[string]string `json:"headers,omitempty"`     // for remote servers
	Environment map[string]string `json:"environment,omitempty"` // env vars
	Enabled     *bool             `json:"enabled,omitempty"`
}

// LoadOpenCodeMCPServers reads MCP server configs from OpenCode's opencode.json.
// Returns a list of MCPServerConfig ready for use by Smara.
func LoadOpenCodeMCPServers() ([]MCPServerConfig, error) {
	configPath := findOpenCodeConfig()
	if configPath == "" {
		return nil, fmt.Errorf("OpenCode config tidak ditemukan")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("gagal membaca OpenCode config: %w", err)
	}

	var ocConfig OpenCodeConfig
	if err := json.Unmarshal(data, &ocConfig); err != nil {
		return nil, fmt.Errorf("gagal parse OpenCode config: %w", err)
	}

	var servers []MCPServerConfig
	for name, entry := range ocConfig.MCP {
		// Check if enabled (default: true)
		enabled := true
		if entry.Enabled != nil {
			enabled = *entry.Enabled
		}
		if !enabled {
			continue
		}

		cfg := MCPServerConfig{
			Name:    name,
			Type:    entry.Type,
			Enabled: enabled,
		}

		switch entry.Type {
		case "local":
			if len(entry.Command) > 0 {
				cfg.Command = entry.Command[0]
				if len(entry.Command) > 1 {
					cfg.Args = entry.Command[1:]
				}
			}
			cfg.Env = entry.Environment

		case "remote":
			cfg.URL = entry.URL
			cfg.Headers = entry.Headers
		}

		servers = append(servers, cfg)
	}

	return servers, nil
}

// findOpenCodeConfig searches for the OpenCode config file at known locations.
func findOpenCodeConfig() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Standard locations for OpenCode config
	candidates := []string{
		filepath.Join(home, ".config", "opencode", "opencode.json"),
		filepath.Join(home, ".opencode", "opencode.json"),
		filepath.Join(home, ".opencode", "config.json"),
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// OpenCodeConfigPath returns the path to the detected OpenCode config, or empty string.
func OpenCodeConfigPath() string {
	return findOpenCodeConfig()
}
