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
	Name    string            `mapstructure:"name" yaml:"name"`
	Command string            `mapstructure:"command" yaml:"command"`
	Args    []string          `mapstructure:"args" yaml:"args"`
	Env     map[string]string `mapstructure:"env" yaml:"env"`
}

// PlatformBotConfig holds config for a single platform bot.
type PlatformBotConfig struct {
	Enabled      bool     `mapstructure:"enabled" yaml:"enabled"`
	Token        string   `mapstructure:"token" yaml:"token"`
	AllowedUsers []string `mapstructure:"allowed_users" yaml:"allowed_users"`
	BlockedUsers []string `mapstructure:"blocked_users" yaml:"blocked_users"`
	GuildIDs     []string `mapstructure:"guild_ids" yaml:"guild_ids"`       // Discord only
	AllowedRoles []string `mapstructure:"allowed_roles" yaml:"allowed_roles"` // Discord only
	RateLimit    int      `mapstructure:"rate_limit" yaml:"rate_limit"`     // requests per minute
	RateBurst    int      `mapstructure:"rate_burst" yaml:"rate_burst"`     // burst size
}

// WhatsAppConfig holds config specifically for WhatsApp.
type WhatsAppConfig struct {
	Enabled        bool     `mapstructure:"enabled" yaml:"enabled"`
	SessionDir     string   `mapstructure:"session_dir" yaml:"session_dir"`
	AllowedNumbers []string `mapstructure:"allowed_numbers" yaml:"allowed_numbers"`
	RateLimit      int      `mapstructure:"rate_limit" yaml:"rate_limit"`
	RateBurst      int      `mapstructure:"rate_burst" yaml:"rate_burst"`
}

// PlatformConfig holds configuration for all platform bots.
type PlatformConfig struct {
	Telegram         PlatformBotConfig `mapstructure:"telegram" yaml:"telegram"`
	Discord          PlatformBotConfig `mapstructure:"discord" yaml:"discord"`
	WhatsApp         WhatsAppConfig    `mapstructure:"whatsapp" yaml:"whatsapp"`
	MaxResponseLen   int               `mapstructure:"max_response_length" yaml:"max_response_length"`
	TypingIndicator  bool              `mapstructure:"typing_indicator" yaml:"typing_indicator"`
	LogConversations bool              `mapstructure:"log_conversations" yaml:"log_conversations"`
}

// SmaraConfig holds all application configuration.
type SmaraConfig struct {
	Provider           string         `mapstructure:"provider" yaml:"provider"`
	Model              string         `mapstructure:"model" yaml:"model"`
	OllamaHost         string         `mapstructure:"ollama_host" yaml:"ollama_host"`
	OpenAIAPIKey       string         `mapstructure:"openai_api_key" yaml:"openai_api_key"`
	OpenAIModel        string         `mapstructure:"openai_model" yaml:"openai_model"`
	OpenAIBaseURL      string         `mapstructure:"openai_base_url" yaml:"openai_base_url"`
	OpenRouterAPIKey   string         `mapstructure:"openrouter_api_key" yaml:"openrouter_api_key"`
	OpenRouterModel    string         `mapstructure:"openrouter_model" yaml:"openrouter_model"`
	AnthropicAPIKey    string         `mapstructure:"anthropic_api_key" yaml:"anthropic_api_key"`
	AnthropicModel     string         `mapstructure:"anthropic_model" yaml:"anthropic_model"`
	CustomProviderName string         `mapstructure:"custom_provider_name" yaml:"custom_provider_name"`
	CustomAPIKey       string         `mapstructure:"custom_api_key" yaml:"custom_api_key"`
	CustomBaseURL      string         `mapstructure:"custom_base_url" yaml:"custom_base_url"`
	CustomModel        string         `mapstructure:"custom_model" yaml:"custom_model"`
	SyncDir            string         `mapstructure:"sync_dir" yaml:"sync_dir"`
	SyncInterval       int            `mapstructure:"sync_interval" yaml:"sync_interval"` // minutes
	MCPServers         []MCPServer    `mapstructure:"mcp_servers" yaml:"mcp_servers"`
	SmaraMCPEnabled    bool           `mapstructure:"smara_mcp_enabled" yaml:"smara_mcp_enabled"`
	SmaraMCPCommand    string         `mapstructure:"smara_mcp_command" yaml:"smara_mcp_command"`
	SmaraMCPArgs       []string       `mapstructure:"smara_mcp_args" yaml:"smara_mcp_args"`
	SmaraMCPAPIKey     string         `mapstructure:"smara_mcp_api_key" yaml:"smara_mcp_api_key"`
	Verbose            bool           `mapstructure:"verbose" yaml:"verbose"`
	DBPath             string         `mapstructure:"db_path" yaml:"db_path"`
	Platforms          PlatformConfig `mapstructure:"platforms" yaml:"platforms"`
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
		Provider:           "ollama",
		Model:              "minimax-m2.5:cloud",
		OllamaHost:         "http://localhost:11434",
		OpenAIAPIKey:       "",
		OpenAIModel:        "gpt-4o",
		OpenAIBaseURL:      "",
		OpenRouterAPIKey:   "",
		OpenRouterModel:    "anthropic/claude-sonnet-4",
		AnthropicAPIKey:    "",
		AnthropicModel:     "claude-sonnet-4-20250514",
		CustomProviderName: "",
		CustomAPIKey:       "",
		CustomBaseURL:      "https://api.openai.com/v1",
		CustomModel:        "",
		SyncDir:            filepath.Join(smaraDir, "sync"),
		SyncInterval:       15,
		MCPServers:         []MCPServer{},
		Verbose:            false,
		DBPath:             filepath.Join(smaraDir, "memory.db"),
		Platforms: PlatformConfig{
			WhatsApp: WhatsAppConfig{
				Enabled:    false,
				SessionDir: filepath.Join(smaraDir, "wa-session"),
				RateLimit:  10,
				RateBurst:  3,
			},
			MaxResponseLen:  4000,
			TypingIndicator: true,
		},
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
	viper.SetDefault("openai_api_key", defaults.OpenAIAPIKey)
	viper.SetDefault("openai_model", defaults.OpenAIModel)
	viper.SetDefault("openai_base_url", defaults.OpenAIBaseURL)
	viper.SetDefault("openrouter_api_key", defaults.OpenRouterAPIKey)
	viper.SetDefault("openrouter_model", defaults.OpenRouterModel)
	viper.SetDefault("anthropic_api_key", defaults.AnthropicAPIKey)
	viper.SetDefault("anthropic_model", defaults.AnthropicModel)
	viper.SetDefault("custom_provider_name", defaults.CustomProviderName)
	viper.SetDefault("custom_api_key", defaults.CustomAPIKey)
	viper.SetDefault("custom_base_url", defaults.CustomBaseURL)
	viper.SetDefault("custom_model", defaults.CustomModel)
	viper.SetDefault("sync_dir", defaults.SyncDir)
	viper.SetDefault("sync_interval", defaults.SyncInterval)
	viper.SetDefault("verbose", defaults.Verbose)
	viper.SetDefault("db_path", defaults.DBPath)
	viper.SetDefault("smara_mcp_enabled", defaults.SmaraMCPEnabled)
	viper.SetDefault("smara_mcp_command", defaults.SmaraMCPCommand)
	viper.SetDefault("smara_mcp_args", defaults.SmaraMCPArgs)
	viper.SetDefault("smara_mcp_api_key", defaults.SmaraMCPAPIKey)

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
