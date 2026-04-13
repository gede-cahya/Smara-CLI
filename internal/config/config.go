// Package config manages Smara CLI configuration via Viper.
// Config is stored at ~/.smara/config.yaml
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// MCPServer represents a configured MCP server endpoint.
type MCPServer struct {
	Name    string   `mapstructure:"name" yaml:"name"`
	Command string   `mapstructure:"command" yaml:"command"`
	Args    []string `mapstructure:"args" yaml:"args"`
	Env     map[string]string `mapstructure:"env" yaml:"env"`
}

// SmaraConfig holds all application configuration.
type SmaraConfig struct {
	Provider   string            `mapstructure:"provider" yaml:"provider"`
	Model      string            `mapstructure:"model" yaml:"model"`
	OllamaHost string           `mapstructure:"ollama_host" yaml:"ollama_host"`
	SyncDir    string            `mapstructure:"sync_dir" yaml:"sync_dir"`
	SyncInterval int            `mapstructure:"sync_interval" yaml:"sync_interval"` // minutes
	MCPServers []MCPServer       `mapstructure:"mcp_servers" yaml:"mcp_servers"`
	Verbose    bool              `mapstructure:"verbose" yaml:"verbose"`
	DBPath     string            `mapstructure:"db_path" yaml:"db_path"`
}

var (
	cfg     *SmaraConfig
	cfgDir  string
	cfgFile string
)

// DefaultConfig returns sensible defaults for MVP.
func DefaultConfig() *SmaraConfig {
	home, _ := os.UserHomeDir()
	smaraDir := filepath.Join(home, ".smara")
	return &SmaraConfig{
		Provider:     "ollama",
		Model:        "minimax-m2.5:cloud",
		OllamaHost:   "http://localhost:11434",
		SyncDir:      filepath.Join(smaraDir, "sync"),
		SyncInterval: 15,
		MCPServers:   []MCPServer{},
		Verbose:      false,
		DBPath:       filepath.Join(smaraDir, "memory.db"),
	}
}

// SmaraDir returns the path to ~/.smara/
func SmaraDir() string {
	if cfgDir != "" {
		return cfgDir
	}
	home, _ := os.UserHomeDir()
	cfgDir = filepath.Join(home, ".smara")
	return cfgDir
}

// Init initializes the configuration system.
// If configPath is empty, uses ~/.smara/config.yaml
func Init(configPath string) error {
	dir := SmaraDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("gagal membuat direktori config: %w", err)
	}

	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		cfgFile = filepath.Join(dir, "config.yaml")
		viper.SetConfigFile(cfgFile)
	}

	viper.SetConfigType("yaml")

	// Set defaults
	defaults := DefaultConfig()
	viper.SetDefault("provider", defaults.Provider)
	viper.SetDefault("model", defaults.Model)
	viper.SetDefault("ollama_host", defaults.OllamaHost)
	viper.SetDefault("sync_dir", defaults.SyncDir)
	viper.SetDefault("sync_interval", defaults.SyncInterval)
	viper.SetDefault("verbose", defaults.Verbose)
	viper.SetDefault("db_path", defaults.DBPath)

	// Environment variable overrides
	viper.SetEnvPrefix("SMARA")
	viper.AutomaticEnv()

	// Read config file (ignore error if not found)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// Only ignore "file not found", not other errors
			if !os.IsNotExist(err) {
				return fmt.Errorf("gagal membaca config: %w", err)
			}
		}
	}

	cfg = &SmaraConfig{}
	if err := viper.Unmarshal(cfg); err != nil {
		return fmt.Errorf("gagal parse config: %w", err)
	}

	// Ensure sync directory exists
	if err := os.MkdirAll(cfg.SyncDir, 0o755); err != nil {
		return fmt.Errorf("gagal membuat sync dir: %w", err)
	}

	return nil
}

// Get returns the current loaded configuration.
func Get() *SmaraConfig {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return cfg
}

// Set sets a configuration value and saves to file.
func Set(key, value string) error {
	viper.Set(key, value)
	return Save()
}

// GetValue returns a config value by key.
func GetValue(key string) interface{} {
	return viper.Get(key)
}

// AllSettings returns all current settings as a map.
func AllSettings() map[string]interface{} {
	return viper.AllSettings()
}

// Save writes the current configuration to the config file.
func Save() error {
	if cfgFile == "" {
		cfgFile = filepath.Join(SmaraDir(), "config.yaml")
	}
	return viper.WriteConfigAs(cfgFile)
}
