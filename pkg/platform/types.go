// Package platform provides the multi-platform integration layer for Smara CLI.
// It defines shared types, interfaces, and routing logic that allow Smara to
// operate as a bot on various messaging platforms (Telegram, Discord, etc.).
package platform

import "time"

// MessageFormat represents the format of an outgoing message.
type MessageFormat int

const (
	// FormatPlain sends the message as plain text.
	FormatPlain MessageFormat = iota
	// FormatMarkdown sends the message with markdown formatting.
	FormatMarkdown
)

// IncomingMessage represents a message received from any platform.
type IncomingMessage struct {
	ID          string            // Platform-specific message ID
	Platform    string            // Source platform: "telegram", "discord", etc.
	ChannelID   string            // Chat/channel/group ID
	UserID      string            // Platform-specific user ID
	Username    string            // Display name of the sender
	Content     string            // Text content of the message
	Attachments []Attachment      // Files, images, voice notes
	ReplyTo     string            // Message ID this is replying to (if any)
	IsCommand   bool              // Whether the message starts with a command prefix
	Command     string            // Parsed command name (e.g., "ask", "mode")
	CommandArgs []string          // Parsed command arguments
	Metadata    map[string]string // Platform-specific metadata
	Timestamp   time.Time
}

// OutgoingMessage represents a message to be sent to a platform.
type OutgoingMessage struct {
	Content     string        // Text content
	Format      MessageFormat // FormatPlain or FormatMarkdown
	Attachments []Attachment  // Files to send
	ReplyTo     string        // Message ID to reply to
}

// Attachment represents a file or media attachment.
type Attachment struct {
	Type     string // "image", "file", "voice", "video"
	URL      string // Remote URL
	FilePath string // Local file path
	FileName string // Display name
	MimeType string // MIME type
	Size     int64  // File size in bytes
}

// PlatformSession tracks a conversation session on a specific platform channel.
type PlatformSession struct {
	Platform  string    // Platform name
	ChannelID string    // Channel/chat ID
	UserID    string    // Primary user ID
	Mode      string    // Current agent mode (ask, rush, plan)
	CreatedAt time.Time // Session creation time
	LastMsg   time.Time // Last message time
}
