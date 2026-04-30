package platform

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMessageFormat_Constants(t *testing.T) {
	assert.Equal(t, MessageFormat(0), FormatPlain)
	assert.Equal(t, MessageFormat(1), FormatMarkdown)
}

func TestIncomingMessage_Struct(t *testing.T) {
	now := time.Now()
	msg := IncomingMessage{
		ID:          "msg_123",
		Platform:    "telegram",
		ChannelID:   "-100123456",
		UserID:      "user_42",
		Username:    "testuser",
		Content:     "Hello bot",
		Attachments: []Attachment{},
		ReplyTo:     "",
		IsCommand:   true,
		Command:     "ask",
		CommandArgs: []string{"hello"},
		Metadata:    map[string]string{"chat_type": "private"},
		Timestamp:   now,
	}
	assert.Equal(t, "msg_123", msg.ID)
	assert.Equal(t, "telegram", msg.Platform)
	assert.Equal(t, "user_42", msg.UserID)
	assert.True(t, msg.IsCommand)
	assert.Equal(t, "ask", msg.Command)
	assert.Equal(t, []string{"hello"}, msg.CommandArgs)
	assert.Equal(t, "Hello bot", msg.Content)
}

func TestOutgoingMessage_Struct(t *testing.T) {
	msg := OutgoingMessage{
		Content:     "Response",
		Format:      FormatMarkdown,
		Attachments: []Attachment{},
		ReplyTo:     "msg_123",
	}
	assert.Equal(t, "Response", msg.Content)
	assert.Equal(t, FormatMarkdown, msg.Format)
	assert.Equal(t, "msg_123", msg.ReplyTo)
}

func TestAttachment_Struct(t *testing.T) {
	att := Attachment{
		Type:     "image",
		URL:      "https://example.com/img.png",
		FilePath: "",
		FileName: "img.png",
		MimeType: "image/png",
		Size:     1024,
	}
	assert.Equal(t, "image", att.Type)
	assert.Equal(t, "https://example.com/img.png", att.URL)
	assert.Equal(t, "img.png", att.FileName)
	assert.Equal(t, int64(1024), att.Size)
}

func TestPlatformSession_Struct(t *testing.T) {
	now := time.Now()
	ps := PlatformSession{
		Platform:  "discord",
		ChannelID: "123456789",
		UserID:    "987654321",
		Mode:      "ask",
		CreatedAt: now,
		LastMsg:   now,
	}
	assert.Equal(t, "discord", ps.Platform)
	assert.Equal(t, "123456789", ps.ChannelID)
	assert.Equal(t, "ask", ps.Mode)
}
