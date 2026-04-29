// Package mcp - windsurf.go loads MCP server configurations from Windsurf's mcp_config.json.
package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WindsurfConfig represents the relevant parts of Windsurf's mcp_config.json.
type WindsurfConfig struct {
	MCPServers map[string]WindsurfMCPEntry `json:"mcpServers"`
}

// WindsurfMCPEntry represents a single MCP server entry in Windsurf config.
type WindsurfMCPEntry struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"` // for remote HTTP servers
}

// LoadWindsurfMCPServers reads MCP server configs from Windsurf's mcp_config.json.
// Returns a list of MCPServerConfig ready for use by Smara.
func LoadWindsurfMCPServers() ([]MCPServerConfig, error) {
	configPath := findWindsurfConfig()
	if configPath == "" {
		return nil, fmt.Errorf("Windsurf config tidak ditemukan")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("gagal membaca Windsurf config: %w", err)
	}

	var wsConfig WindsurfConfig
	if err := json.Unmarshal(data, &wsConfig); err != nil {
		return nil, fmt.Errorf("gagal parse Windsurf config: %w", err)
	}

	var servers []MCPServerConfig
	for name, entry := range wsConfig.MCPServers {
		cfg := MCPServerConfig{
			Name:    name,
			Enabled: true,
		}

		if entry.URL != "" {
			// Remote HTTP server
			cfg.Type = "remote"
			cfg.URL = entry.URL
		} else {
			// Local stdio server
			cfg.Type = "local"
			cfg.Command = entry.Command
			cfg.Args = entry.Args
			cfg.Env = entry.Env
		}

		servers = append(servers, cfg)
	}

	return servers, nil
}

// findWindsurfConfig searches for the Windsurf config file at known locations.
func findWindsurfConfig() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Standard locations for Windsurf config
	candidates := []string{
		filepath.Join(home, ".codeium", "mcp_config.json"),
		filepath.Join(home, ".codeium", "windsurf", "mcp_config.json"),
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// WindsurfConfigPath returns the path to the detected Windsurf config, or empty string.
func WindsurfConfigPath() string {
	return findWindsurfConfig()
}
