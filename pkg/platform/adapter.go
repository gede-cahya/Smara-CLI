package platform

import "context"

// PlatformAdapter is the interface that every messaging platform must implement.
// It provides a uniform way to connect, listen for messages, and send responses.
type PlatformAdapter interface {
	// Name returns the platform identifier (e.g., "telegram", "discord").
	Name() string

	// Connect initializes the connection to the platform using the given config.
	Connect(ctx context.Context, cfg AdapterConfig) error

	// Listen starts listening for incoming messages and dispatches them to the handler.
	// This method should block until ctx is cancelled or an error occurs.
	Listen(ctx context.Context, handler MessageHandler) error

	// SendMessage sends a message to a specific channel/chat on the platform.
	SendMessage(ctx context.Context, channelID string, msg OutgoingMessage) error

	// SendTyping sends a typing indicator to a specific channel/chat.
	SendTyping(ctx context.Context, channelID string) error

	// Close gracefully shuts down the adapter connection.
	Close() error
}

// MessageHandler is a callback function invoked when an incoming message is received.
type MessageHandler func(ctx context.Context, msg IncomingMessage) error

// AdapterConfig holds the configuration for a platform adapter.
type AdapterConfig struct {
	Token        string            // Bot/API token
	WebhookURL   string            // Webhook URL (if applicable)
	AllowedUsers []string          // Whitelisted user IDs (empty = allow all)
	BlockedUsers []string          // Blacklisted user IDs
	GuildIDs     []string          // Discord guild IDs (if applicable)
	AllowedRoles []string          // Discord role-based access (if applicable)
	RateLimit    RateLimitConfig   // Rate limiting settings
	Extra        map[string]string // Platform-specific key-value settings
}

// RateLimitConfig defines rate limiting parameters.
type RateLimitConfig struct {
	RequestsPerMinute int // Maximum requests per minute per user
	BurstSize         int // Maximum burst size
}
