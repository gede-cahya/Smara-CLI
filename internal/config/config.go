// Package config manages Smara CLI configuration via Viper.
// Config is stored at ~/.smara/config.yaml
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

type MCPServer struct {
	Name    string            `mapstructure:"name" yaml:"name"`
	Type    string            `mapstructure:"type" yaml:"type"` // "local" or "remote"
	Command string            `mapstructure:"command" yaml:"command"`
	Args    []string          `mapstructure:"args" yaml:"args"`
	URL     string            `mapstructure:"url" yaml:"url,omitempty"` // for remote servers
	Headers map[string]string `mapstructure:"headers" yaml:"headers,omitempty"`
	Env     map[string]string `mapstructure:"env" yaml:"env,omitempty"`
	Enabled bool              `mapstructure:"enabled" yaml:"enabled"`
}

// PlatformBotConfig holds config for a single platform bot.
type PlatformBotConfig struct {
	Enabled              bool     `mapstructure:"enabled" yaml:"enabled"`
	Token                string   `mapstructure:"token" yaml:"token"`
	AllowedUsers         []string `mapstructure:"allowed_users" yaml:"allowed_users"`
	BlockedUsers         []string `mapstructure:"blocked_users" yaml:"blocked_users"`
	OwnerID              string   `mapstructure:"owner_id" yaml:"owner_id"`
	SensitiveKeywords    []string `mapstructure:"sensitive_keywords" yaml:"sensitive_keywords"`
	SensitiveDenyMessage string   `mapstructure:"sensitive_deny_message" yaml:"sensitive_deny_message"`
	GuildIDs             []string `mapstructure:"guild_ids" yaml:"guild_ids"`         // Discord only
	AllowedRoles         []string `mapstructure:"allowed_roles" yaml:"allowed_roles"` // Discord only
	ReleaseChannelID     string   `mapstructure:"release_channel_id" yaml:"release_channel_id"` // Discord only
	ReleaseWebhookURL    string   `mapstructure:"release_webhook_url" yaml:"release_webhook_url"` // Discord/webhook notification URL
	RateLimit            int      `mapstructure:"rate_limit" yaml:"rate_limit"`       // requests per minute
	RateBurst            int      `mapstructure:"rate_burst" yaml:"rate_burst"`       // burst size
}

// WhatsAppConfig holds config specifically for WhatsApp.
type WhatsAppConfig struct {
	Enabled        bool     `mapstructure:"enabled" yaml:"enabled"`
	SessionDir     string   `mapstructure:"session_dir" yaml:"session_dir"`
	AllowedNumbers []string `mapstructure:"allowed_numbers" yaml:"allowed_numbers"`
	RateLimit      int      `mapstructure:"rate_limit" yaml:"rate_limit"`
	RateBurst      int      `mapstructure:"rate_burst" yaml:"rate_burst"`
}

// CloudMemoryConfig holds configuration for the cloud memory sync feature.
//
// Tokens and other secrets MUST NOT be stored here; they live exclusively in
// the OS-level CredentialStore (keyring with file fallback). Persisting tokens
// in the YAML config would leak them into backups, dotfile syncs, and audit
// logs — precisely what Requirement 13.4 / 16.4 forbid.
type CloudMemoryConfig struct {
	Enabled         bool     `mapstructure:"enabled" yaml:"enabled"`
	Provider        string   `mapstructure:"provider" yaml:"provider"`
	DBNamePattern   string   `mapstructure:"db_name_pattern" yaml:"db_name_pattern"`
	SyncIntervalSec int      `mapstructure:"sync_interval_sec" yaml:"sync_interval_sec"`
	ConflictPolicy  string   `mapstructure:"conflict_policy" yaml:"conflict_policy"`
	OfflineMode     string   `mapstructure:"offline_mode" yaml:"offline_mode"`
	EncryptAtRest   bool     `mapstructure:"encrypt_at_rest" yaml:"encrypt_at_rest"`
	MaxRowsPerHour  int      `mapstructure:"max_rows_per_hour" yaml:"max_rows_per_hour"`
	MaxStorageMB    int      `mapstructure:"max_storage_mb" yaml:"max_storage_mb"`
	EmbeddingsCloud bool     `mapstructure:"embeddings_cloud" yaml:"embeddings_cloud"`
	SyncTables      []string `mapstructure:"sync_tables" yaml:"sync_tables"`
}

type ChangeJournalConfig struct {
	Enabled         bool   `mapstructure:"enabled" yaml:"enabled"`
	MemoryEnabled   bool   `mapstructure:"memory_enabled" yaml:"memory_enabled"`
	ObsidianEnabled bool   `mapstructure:"obsidian_enabled" yaml:"obsidian_enabled"`
	ObsidianServer  string `mapstructure:"obsidian_server" yaml:"obsidian_server"`
	ObsidianNote    string `mapstructure:"obsidian_note" yaml:"obsidian_note"`
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

// ParallelOrchestrationConfig controls the parallel task orchestration runtime
// and its web dashboard/API behavior.
type ParallelOrchestrationConfig struct {
	Enabled               bool   `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	MaxConcurrency        int    `mapstructure:"max_concurrency" yaml:"max_concurrency" json:"max_concurrency"`
	RequireApprovalHigh   bool   `mapstructure:"require_approval_high" yaml:"require_approval_high" json:"require_approval_high"`
	RequireApprovalRemote bool   `mapstructure:"require_approval_remote" yaml:"require_approval_remote" json:"require_approval_remote"`
	DryRun                bool   `mapstructure:"dry_run" yaml:"dry_run" json:"dry_run"`
	SerialFallback        bool   `mapstructure:"serial_fallback" yaml:"serial_fallback" json:"serial_fallback"`
	AutoThreshold         string `mapstructure:"auto_threshold" yaml:"auto_threshold" json:"auto_threshold"`
}

// DefaultParallelOrchestrationConfig returns conservative defaults for safe
// parallel execution.
func DefaultParallelOrchestrationConfig() ParallelOrchestrationConfig {
	return ParallelOrchestrationConfig{
		Enabled:               true,
		MaxConcurrency:        4,
		RequireApprovalHigh:   true,
		RequireApprovalRemote: true,
		DryRun:                false,
		SerialFallback:        true,
		AutoThreshold:         "complex",
	}
}

// SetParallelOrchestration updates the in-memory and Viper-backed parallel
// orchestration config. It intentionally does not call Save so API callers can
// change runtime behavior without unexpectedly rewriting the user's config file.
func SetParallelOrchestration(next ParallelOrchestrationConfig) {
	next = normalizeParallelOrchestrationConfig(next)
	if cfg == nil {
		cfg = DefaultConfig()
	}
	cfg.ParallelOrchestration = next
	viper.Set("parallel_orchestration", next)
}

func normalizeParallelOrchestrationConfig(next ParallelOrchestrationConfig) ParallelOrchestrationConfig {
	if next.MaxConcurrency < 1 {
		next.MaxConcurrency = 1
	}
	switch strings.ToLower(strings.TrimSpace(next.AutoThreshold)) {
	case "conservative", "balanced", "complex", "aggressive":
		next.AutoThreshold = strings.ToLower(strings.TrimSpace(next.AutoThreshold))
	default:
		next.AutoThreshold = "complex"
	}
	return next
}

// SmaraConfig holds all application configuration.
type SmaraConfig struct {
	Provider              string                      `mapstructure:"provider" yaml:"provider"`
	Model                 string                      `mapstructure:"model" yaml:"model"`
	ReasoningEffort       string                      `mapstructure:"reasoning_effort" yaml:"reasoning_effort"`
	OllamaHost            string                      `mapstructure:"ollama_host" yaml:"ollama_host"`
	OpenAIAPIKey          string                      `mapstructure:"openai_api_key" yaml:"openai_api_key"`
	OpenAIModel           string                      `mapstructure:"openai_model" yaml:"openai_model"`
	OpenAIBaseURL         string                      `mapstructure:"openai_base_url" yaml:"openai_base_url"`
	OpenRouterAPIKey      string                      `mapstructure:"openrouter_api_key" yaml:"openrouter_api_key"`
	OpenRouterModel       string                      `mapstructure:"openrouter_model" yaml:"openrouter_model"`
	AnthropicAPIKey       string                      `mapstructure:"anthropic_api_key" yaml:"anthropic_api_key"`
	AnthropicModel        string                      `mapstructure:"anthropic_model" yaml:"anthropic_model"`
	CustomProviderName    string                      `mapstructure:"custom_provider_name" yaml:"custom_provider_name"`
	CustomAPIKey          string                      `mapstructure:"custom_api_key" yaml:"custom_api_key"`
	CustomBaseURL         string                      `mapstructure:"custom_base_url" yaml:"custom_base_url"`
	CustomModel           string                      `mapstructure:"custom_model" yaml:"custom_model"`
	CustomDisableStream   bool                        `mapstructure:"custom_disable_stream" yaml:"custom_disable_stream"`
	SyncDir               string                      `mapstructure:"sync_dir" yaml:"sync_dir"`
	SyncInterval          int                         `mapstructure:"sync_interval" yaml:"sync_interval"` // minutes
	MCPServers            []MCPServer                 `mapstructure:"mcp_servers" yaml:"mcp_servers"`
	SmaraMCPEnabled       bool                        `mapstructure:"smara_mcp_enabled" yaml:"smara_mcp_enabled"`
	SmaraMCPCommand       string                      `mapstructure:"smara_mcp_command" yaml:"smara_mcp_command"`
	SmaraMCPArgs          []string                    `mapstructure:"smara_mcp_args" yaml:"smara_mcp_args"`
	SmaraMCPAPIKey        string                      `mapstructure:"smara_mcp_api_key" yaml:"smara_mcp_api_key"`
	ImageModel            string                      `mapstructure:"image_model" yaml:"image_model"`
	ImageBaseURL          string                      `mapstructure:"image_base_url" yaml:"image_base_url"`
	ImageAPIKey           string                      `mapstructure:"image_api_key" yaml:"image_api_key"`
	ImageOutputDir        string                      `mapstructure:"image_output_dir" yaml:"image_output_dir"`
	NineDriveEnabled      bool                        `mapstructure:"ninedrive_enabled" yaml:"ninedrive_enabled"`
	NineDriveBaseURL      string                      `mapstructure:"ninedrive_base_url" yaml:"ninedrive_base_url"`
	NineDriveAPIKey       string                      `mapstructure:"ninedrive_api_key" yaml:"ninedrive_api_key"`
	VoiceProvider         string                      `mapstructure:"voice_provider" yaml:"voice_provider"`
	VoiceAPIKey           string                      `mapstructure:"voice_api_key" yaml:"voice_api_key"`
	VoiceBaseURL          string                      `mapstructure:"voice_base_url" yaml:"voice_base_url"`
	VoiceCharacter        string                      `mapstructure:"voice_character" yaml:"voice_character"`
	VoiceModelID          string                      `mapstructure:"voice_model_id" yaml:"voice_model_id"`
	VoiceLanguage         string                      `mapstructure:"voice_language" yaml:"voice_language"`
	VoiceSpeed            float64                     `mapstructure:"voice_speed" yaml:"voice_speed"`
	VoiceVolume           float64                     `mapstructure:"voice_volume" yaml:"voice_volume"`
	VoiceStreaming        bool                        `mapstructure:"voice_streaming" yaml:"voice_streaming"`
	Verbose               bool                        `mapstructure:"verbose" yaml:"verbose"`
	DBPath                string                      `mapstructure:"db_path" yaml:"db_path"`
	ActiveWorkspace       string                      `mapstructure:"active_workspace" yaml:"active_workspace"`
	ActiveWorkspaceID     int64                       `mapstructure:"-"` // runtime only
	Platforms             PlatformConfig              `mapstructure:"platforms" yaml:"platforms"`
	SkillRegistries       []RegistryConfig            `mapstructure:"skill_registries" yaml:"skill_registries"`
	CloudMemory           CloudMemoryConfig           `mapstructure:"cloud_memory" yaml:"cloud_memory"`
	ChangeJournal         ChangeJournalConfig         `mapstructure:"change_journal" yaml:"change_journal"`
	ParallelOrchestration ParallelOrchestrationConfig `mapstructure:"parallel_orchestration" yaml:"parallel_orchestration"`
	// AutoSkillDetect enables automatic skill capture: when the same tool-call
	// pattern is observed repeatedly, Smara creates a skill without being asked.
	// Default: true. Disable with `auto_skill_detect: false` in config.
	AutoSkillDetect bool `mapstructure:"auto_skill_detect" yaml:"auto_skill_detect"`

	// AutoSkillThreshold is the minimum number of times a tool-call pattern
	// must be observed before being auto-captured as a skill. Default: 3.
	AutoSkillThreshold int `mapstructure:"auto_skill_threshold" yaml:"auto_skill_threshold"`

	// AutoSkillRefine enables automatic refinement for repeatedly failing skills.
	AutoSkillRefine bool `mapstructure:"auto_skill_refine" yaml:"auto_skill_refine"`

	// AutoSkillRefineApply applies valid LLM refinement proposals automatically.
	// When false, proposals are still generated and stored for review.
	AutoSkillRefineApply bool `mapstructure:"auto_skill_refine_apply" yaml:"auto_skill_refine_apply"`

	// PlatformPromptTimeout limits how long a single Telegram/Discord/WhatsApp
	// prompt may run, in seconds. Long-running workflow tasks (multi-step
	// SSH, file edits, deep reasoning) often exceed the previous 300s default.
	// Set 0 to use the built-in default (10 minutes).
	PlatformPromptTimeout int `mapstructure:"platform_prompt_timeout" yaml:"platform_prompt_timeout"`

	// AgentMaxIterations caps the agentic loop (think → tool → observe) per
	// prompt. Default: 30. Increase if your tasks chain many tool calls
	// (e.g. multi-host SSH + service restart + verification). Set 0 to use
	// the built-in default.
	AgentMaxIterations int `mapstructure:"agent_max_iterations" yaml:"agent_max_iterations"`

	// AgentRequestTimeoutSec is the wall-clock cap for a single web/TUI
	// agentic turn (from prompt submit to final answer). Default: 1800
	// (30 min). Long roadmap-style chains with many `go test/build`
	// invocations regularly exceed the old 5 min cap. Set 0 for the
	// built-in default.
	AgentRequestTimeoutSec int `mapstructure:"agent_request_timeout_sec" yaml:"agent_request_timeout_sec"`

	// DisabledToolGroups lists tool group names to exclude from the LLM
	// request, reducing input token count and improving latency.
	// Available groups: ssh, lsp, binary, graphify, skill, image, planning, export
	// Example: ["ssh", "lsp", "binary", "graphify"] saves ~3000 tokens.
	DisabledToolGroups []string `mapstructure:"disabled_tool_groups" yaml:"disabled_tool_groups"`
}

// RegistryConfig defines a configured skill marketplace/registry source.
type RegistryConfig struct {
	Name      string `mapstructure:"name" yaml:"name"`
	URL       string `mapstructure:"url" yaml:"url"`
	AuthToken string `mapstructure:"auth_token,omitempty" yaml:"auth_token,omitempty"`
}

var (
	cfg          *SmaraConfig
	cfgDir       string
	cfgFile      string
	onReloadFuncs []func(*SmaraConfig)
)

// OnReload registers a callback fired when the config file changes on disk.
func OnReload(fn func(*SmaraConfig)) {
	onReloadFuncs = append(onReloadFuncs, fn)
}

// DefaultConfig returns sensible defaults for MVP.
func DefaultConfig() *SmaraConfig {
	home, _ := os.UserHomeDir()
	smaraDir := filepath.Join(home, ".smara")
	return &SmaraConfig{
		Provider:            "custom",
		Model:               "cx/gpt-5.5",
		ReasoningEffort:     "",
		OllamaHost:          "http://localhost:11434",
		OpenAIAPIKey:        "",
		OpenAIModel:         "gpt-4o",
		OpenAIBaseURL:       "",
		OpenRouterAPIKey:    "",
		OpenRouterModel:     "anthropic/claude-sonnet-4",
		AnthropicAPIKey:     "",
		AnthropicModel:      "claude-sonnet-4-20250514",
		CustomProviderName:  "9router",
		CustomAPIKey:        "sk-63a768fa898cb6e0-9r8iio-0a23c1a1",
		CustomBaseURL:       "http://localhost:20128/v1",
		CustomModel:         "cx/gpt-5.5",
		CustomDisableStream: false,
		SyncDir:             filepath.Join(smaraDir, "sync"),
		SyncInterval:        15,
		MCPServers:          []MCPServer{},
		ImageModel:          "gpt-image-2",
		ImageBaseURL:        "", // empty = use custom_base_url
		ImageAPIKey:         "", // empty = use custom_api_key
		ImageOutputDir:      filepath.Join(smaraDir, "images"),
		NineDriveEnabled:    false,
		NineDriveBaseURL:    "http://localhost:4000",
		NineDriveAPIKey:     "",
		VoiceProvider:       "browser",
		VoiceAPIKey:         "", // empty = use ELEVENLABS_API_KEY env
		VoiceBaseURL:        "https://api.elevenlabs.io",
		VoiceCharacter:      "ngvNHfiCrXLPAHcTrZK1",
		VoiceModelID:        "eleven_multilingual_v2",
		VoiceLanguage:       "id-ID",
		VoiceSpeed:          1,
		VoiceVolume:         1,
		VoiceStreaming:      true,
		Verbose:             false,
		DBPath:              filepath.Join(smaraDir, "memory.db"),
		ActiveWorkspace:     "default",
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
		SkillRegistries: []RegistryConfig{
			{
				Name: "smara-official",
				URL:  "https://raw.githubusercontent.com/gede-cahya/smara-skills/main/skill-registry.json",
			},
		},
		AutoSkillDetect:        true,
		AutoSkillThreshold:     3,
		AutoSkillRefine:        true,
		AutoSkillRefineApply:   true,
		PlatformPromptTimeout:  600,        // 10 minutes; was hardcoded 300s
		AgentMaxIterations:     80,         // long roadmap chains routinely exceed 30
		AgentRequestTimeoutSec: 3600,       // 60 min; web/TUI was hardcoded 5 min
		DisabledToolGroups:     []string{}, // empty = all tools enabled
		ChangeJournal: ChangeJournalConfig{
			Enabled:         true,
			MemoryEnabled:   true,
			ObsidianEnabled: true,
			ObsidianServer:  "obsidian",
			ObsidianNote:    "Second Brain/Smara/Change Log.md",
		},
		CloudMemory: CloudMemoryConfig{
			Enabled:         false,
			Provider:        "turso",
			DBNamePattern:   "smara-{workspace}",
			SyncIntervalSec: 30,
			ConflictPolicy:  "lww",
			OfflineMode:     "auto",
			EncryptAtRest:   false,
			MaxRowsPerHour:  50000,
			MaxStorageMB:    8000,
			EmbeddingsCloud: false,
			SyncTables: []string{
				"memories",
				"memory_links",
				"memory_versions",
				"categories",
				"workspaces",
				"sync_log",
			},
		},
		ParallelOrchestration: DefaultParallelOrchestrationConfig(),
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
	viper.SetDefault("reasoning_effort", defaults.ReasoningEffort)
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
	viper.SetDefault("custom_disable_stream", defaults.CustomDisableStream)
	viper.SetDefault("sync_dir", defaults.SyncDir)
	viper.SetDefault("sync_interval", defaults.SyncInterval)
	viper.SetDefault("verbose", defaults.Verbose)
	viper.SetDefault("db_path", defaults.DBPath)
	viper.SetDefault("active_workspace", defaults.ActiveWorkspace)
	viper.SetDefault("smara_mcp_enabled", defaults.SmaraMCPEnabled)
	viper.SetDefault("smara_mcp_command", defaults.SmaraMCPCommand)
	viper.SetDefault("smara_mcp_args", defaults.SmaraMCPArgs)
	viper.SetDefault("smara_mcp_api_key", defaults.SmaraMCPAPIKey)
	viper.SetDefault("image_model", defaults.ImageModel)
	viper.SetDefault("image_base_url", defaults.ImageBaseURL)
	viper.SetDefault("image_api_key", defaults.ImageAPIKey)
	viper.SetDefault("image_output_dir", defaults.ImageOutputDir)
	viper.SetDefault("ninedrive_enabled", defaults.NineDriveEnabled)
	viper.SetDefault("ninedrive_base_url", defaults.NineDriveBaseURL)
	viper.SetDefault("ninedrive_api_key", defaults.NineDriveAPIKey)
	viper.SetDefault("voice_provider", defaults.VoiceProvider)
	viper.SetDefault("voice_api_key", defaults.VoiceAPIKey)
	viper.SetDefault("voice_base_url", defaults.VoiceBaseURL)
	viper.SetDefault("voice_character", defaults.VoiceCharacter)
	viper.SetDefault("voice_model_id", defaults.VoiceModelID)
	viper.SetDefault("voice_language", defaults.VoiceLanguage)
	viper.SetDefault("voice_speed", defaults.VoiceSpeed)
	viper.SetDefault("voice_volume", defaults.VoiceVolume)
	viper.SetDefault("voice_streaming", defaults.VoiceStreaming)
	viper.SetDefault("skill_registries", defaults.SkillRegistries)
	viper.SetDefault("auto_skill_detect", defaults.AutoSkillDetect)
	viper.SetDefault("auto_skill_threshold", defaults.AutoSkillThreshold)
	viper.SetDefault("auto_skill_refine", defaults.AutoSkillRefine)
	viper.SetDefault("auto_skill_refine_apply", defaults.AutoSkillRefineApply)
	viper.SetDefault("platform_prompt_timeout", defaults.PlatformPromptTimeout)
	viper.SetDefault("agent_max_iterations", defaults.AgentMaxIterations)
	viper.SetDefault("agent_request_timeout_sec", defaults.AgentRequestTimeoutSec)
	viper.SetDefault("disabled_tool_groups", defaults.DisabledToolGroups)
	viper.SetDefault("change_journal.enabled", defaults.ChangeJournal.Enabled)
	viper.SetDefault("change_journal.memory_enabled", defaults.ChangeJournal.MemoryEnabled)
	viper.SetDefault("change_journal.obsidian_enabled", defaults.ChangeJournal.ObsidianEnabled)
	viper.SetDefault("change_journal.obsidian_server", defaults.ChangeJournal.ObsidianServer)
	viper.SetDefault("change_journal.obsidian_note", defaults.ChangeJournal.ObsidianNote)
	viper.SetDefault("cloud_memory.enabled", defaults.CloudMemory.Enabled)
	viper.SetDefault("cloud_memory.provider", defaults.CloudMemory.Provider)
	viper.SetDefault("cloud_memory.db_name_pattern", defaults.CloudMemory.DBNamePattern)
	viper.SetDefault("cloud_memory.sync_interval_sec", defaults.CloudMemory.SyncIntervalSec)
	viper.SetDefault("cloud_memory.conflict_policy", defaults.CloudMemory.ConflictPolicy)
	viper.SetDefault("cloud_memory.offline_mode", defaults.CloudMemory.OfflineMode)
	viper.SetDefault("cloud_memory.encrypt_at_rest", defaults.CloudMemory.EncryptAtRest)
	viper.SetDefault("cloud_memory.max_rows_per_hour", defaults.CloudMemory.MaxRowsPerHour)
	viper.SetDefault("cloud_memory.max_storage_mb", defaults.CloudMemory.MaxStorageMB)
	viper.SetDefault("cloud_memory.embeddings_cloud", defaults.CloudMemory.EmbeddingsCloud)
	viper.SetDefault("cloud_memory.sync_tables", defaults.CloudMemory.SyncTables)
	viper.SetDefault("parallel_orchestration.enabled", defaults.ParallelOrchestration.Enabled)
	viper.SetDefault("parallel_orchestration.max_concurrency", defaults.ParallelOrchestration.MaxConcurrency)
	viper.SetDefault("parallel_orchestration.require_approval_high", defaults.ParallelOrchestration.RequireApprovalHigh)
	viper.SetDefault("parallel_orchestration.require_approval_remote", defaults.ParallelOrchestration.RequireApprovalRemote)
	viper.SetDefault("parallel_orchestration.dry_run", defaults.ParallelOrchestration.DryRun)
	viper.SetDefault("parallel_orchestration.serial_fallback", defaults.ParallelOrchestration.SerialFallback)
	viper.SetDefault("parallel_orchestration.auto_threshold", defaults.ParallelOrchestration.AutoThreshold)

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

	// Watch config file for external changes and hot-reload.
	viper.WatchConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		newCfg := &SmaraConfig{}
		if err := viper.Unmarshal(newCfg); err != nil {
			return
		}
		cfg = newCfg
		for _, fn := range onReloadFuncs {
			fn(newCfg)
		}
	})

	// Initialize database and workspace ID
	return initWorkspaceID()
}

func initWorkspaceID() error {
	fromMemory := true
	// We need memory.NewSQLiteStore but that creates a circular dependency if called here.
	// We'll handle this in the main.go or root.go by calling a separate InitWorkspace function
	// or we can just let the commands handle it.
	// For now, let's keep it simple.
	_ = fromMemory
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
//
// Type coercion: viper.Set stores whatever value is passed in. The CLI
// `smara config set <key> <value>` always passes a string, but our schema
// has int/bool fields too. If we store everything as a string, the next
// load with mapstructure may fail to decode (numeric fields end up as 0
// → default fallback). Coerce here based on the existing default value's
// type, falling back to string when unknown.
func Set(key, value string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("config key kosong")
	}
	trimmed := strings.TrimSpace(value)
	var parsed interface{}
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil && (strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") || trimmed == "true" || trimmed == "false" || strings.HasPrefix(trimmed, `"`)) {
		viper.Set(key, parsed)
		applyRuntimeConfigValue()
		return Save()
	}
	defaults := DefaultConfig()
	defaultsAll := allSettingsFromStruct(defaults)
	if existing, ok := defaultsAll[key]; ok {
		switch existing.(type) {
		case int, int32, int64:
			if n, err := strconv.Atoi(value); err == nil {
				viper.Set(key, n)
				applyRuntimeConfigValue()
				return Save()
			}
		case float32, float64:
			if f, err := strconv.ParseFloat(value, 64); err == nil {
				viper.Set(key, f)
				applyRuntimeConfigValue()
				return Save()
			}
		case bool:
			if b, err := strconv.ParseBool(value); err == nil {
				viper.Set(key, b)
				applyRuntimeConfigValue()
				return Save()
			}
		}
	}
	viper.Set(key, value)
	applyRuntimeConfigValue()
	return Save()
}

func applyRuntimeConfigValue() {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	_ = viper.Unmarshal(cfg)
}

// allSettingsFromStruct flattens scalar settings for type coercion in Set.
func allSettingsFromStruct(c *SmaraConfig) map[string]interface{} {
	if c == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"agent_max_iterations":                           c.AgentMaxIterations,
		"agent_request_timeout_sec":                      c.AgentRequestTimeoutSec,
		"reasoning_effort":                               c.ReasoningEffort,
		"auto_skill_detect":                              c.AutoSkillDetect,
		"auto_skill_threshold":                           c.AutoSkillThreshold,
		"auto_skill_refine":                              c.AutoSkillRefine,
		"auto_skill_refine_apply":                        c.AutoSkillRefineApply,
		"platform_prompt_timeout":                        c.PlatformPromptTimeout,
		"image_model":                                    c.ImageModel,
		"image_base_url":                                 c.ImageBaseURL,
		"image_api_key":                                  c.ImageAPIKey,
		"image_output_dir":                               c.ImageOutputDir,
		"ninedrive_enabled":                              c.NineDriveEnabled,
		"ninedrive_base_url":                             c.NineDriveBaseURL,
		"ninedrive_api_key":                              c.NineDriveAPIKey,
		"voice_provider":                                 c.VoiceProvider,
		"voice_api_key":                                  c.VoiceAPIKey,
		"voice_base_url":                                 c.VoiceBaseURL,
		"voice_character":                                c.VoiceCharacter,
		"voice_model_id":                                 c.VoiceModelID,
		"voice_language":                                 c.VoiceLanguage,
		"voice_speed":                                    c.VoiceSpeed,
		"voice_volume":                                   c.VoiceVolume,
		"voice_streaming":                                c.VoiceStreaming,
		"sync_interval":                                  c.SyncInterval,
		"cloud_memory.sync_interval_sec":                 c.CloudMemory.SyncIntervalSec,
		"cloud_memory.encrypt_at_rest":                   c.CloudMemory.EncryptAtRest,
		"cloud_memory.max_rows_per_hour":                 c.CloudMemory.MaxRowsPerHour,
		"cloud_memory.max_storage_mb":                    c.CloudMemory.MaxStorageMB,
		"cloud_memory.embeddings_cloud":                  c.CloudMemory.EmbeddingsCloud,
		"parallel_orchestration.enabled":                 c.ParallelOrchestration.Enabled,
		"parallel_orchestration.max_concurrency":         c.ParallelOrchestration.MaxConcurrency,
		"parallel_orchestration.require_approval_high":   c.ParallelOrchestration.RequireApprovalHigh,
		"parallel_orchestration.require_approval_remote": c.ParallelOrchestration.RequireApprovalRemote,
		"parallel_orchestration.dry_run":                 c.ParallelOrchestration.DryRun,
		"parallel_orchestration.serial_fallback":         c.ParallelOrchestration.SerialFallback,
		"parallel_orchestration.auto_threshold":          c.ParallelOrchestration.AutoThreshold,
	}
}

// GetValue returns a config value by key.
func GetValue(key string) interface{} {
	return viper.Get(key)
}

func AllSettings() map[string]interface{} {
	return viper.AllSettings()
}

// AddMCPServer adds or replaces an MCP server in the config and persists it.
func AddMCPServer(srv MCPServer) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	// Deduplicate: replace if exists
	found := false
	for i, existing := range cfg.MCPServers {
		if existing.Name == srv.Name {
			cfg.MCPServers[i] = srv
			found = true
			break
		}
	}
	if !found {
		cfg.MCPServers = append(cfg.MCPServers, srv)
	}
	viper.Set("mcp_servers", cfg.MCPServers)
	return Save()
}

// RemoveMCPServer removes an MCP server from config by name and persists it.
func RemoveMCPServer(name string) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	var filtered []MCPServer
	for _, s := range cfg.MCPServers {
		if s.Name != name {
			filtered = append(filtered, s)
		}
	}
	cfg.MCPServers = filtered
	viper.Set("mcp_servers", cfg.MCPServers)
	return Save()
}

// ListMCPServers returns the list of configured MCP servers.
func ListMCPServers() []MCPServer {
	if cfg == nil {
		return []MCPServer{}
	}
	result := make([]MCPServer, len(cfg.MCPServers))
	copy(result, cfg.MCPServers)
	return result
}

// Save writes the current configuration to the config file.
func Save() error {
	if cfgFile == "" {
		cfgFile = filepath.Join(SmaraDir(), "config.yaml")
	}
	return viper.WriteConfigAs(cfgFile)
}
