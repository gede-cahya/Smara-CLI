// Package telegram provides the Telegram Bot API adapter for Smara.
package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/gede-cahya/Smara-CLI/internal/platform"
)

// Adapter implements platform.PlatformAdapter for Telegram.
type Adapter struct {
	bot    *tgbotapi.BotAPI
	config platform.AdapterConfig
}

// New creates a new Telegram adapter.
func New() *Adapter {
	return &Adapter{}
}

// Name returns the platform identifier.
func (a *Adapter) Name() string {
	return "telegram"
}

// Connect initializes the Telegram bot connection.
func (a *Adapter) Connect(ctx context.Context, cfg platform.AdapterConfig) error {
	if cfg.Token == "" {
		return fmt.Errorf("telegram bot token tidak boleh kosong")
	}

	bot, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return fmt.Errorf("gagal menghubungkan ke Telegram: %w", err)
	}

	a.bot = bot
	a.config = cfg
	log.Printf("[telegram] Bot terhubung sebagai @%s", bot.Self.UserName)

	return nil
}

// Listen starts polling for updates and dispatches messages to the handler.
func (a *Adapter) Listen(ctx context.Context, handler platform.MessageHandler) error {
	if a.bot == nil {
		return fmt.Errorf("bot belum terhubung, panggil Connect() dulu")
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30

	updates := a.bot.GetUpdatesChan(u)

	log.Printf("[telegram] Mulai mendengarkan pesan...")

	for {
		select {
		case <-ctx.Done():
			a.bot.StopReceivingUpdates()
			return nil

		case update := <-updates:
			if update.Message == nil {
				continue
			}

			msg := a.convertMessage(update.Message)

			go func() {
				if err := handler(ctx, msg); err != nil {
					log.Printf("[telegram] Error handling message: %v", err)
				}
			}()
		}
	}
}

// SendMessage sends a message to a Telegram chat.
func (a *Adapter) SendMessage(ctx context.Context, channelID string, msg platform.OutgoingMessage) error {
	if a.bot == nil {
		return fmt.Errorf("bot belum terhubung")
	}

	chatID, err := parseChatID(channelID)
	if err != nil {
		return err
	}

	tgMsg := tgbotapi.NewMessage(chatID, msg.Content)

	// Set parse mode based on format
	if msg.Format == platform.FormatMarkdown {
		tgMsg.ParseMode = "Markdown"
	}

	// Disable web page preview for cleaner output
	tgMsg.DisableWebPagePreview = true

	if _, err := a.bot.Send(tgMsg); err != nil {
		// Retry without markdown if parsing fails
		if msg.Format == platform.FormatMarkdown {
			tgMsg.ParseMode = ""
			if _, retryErr := a.bot.Send(tgMsg); retryErr != nil {
				return fmt.Errorf("gagal mengirim pesan: %w", retryErr)
			}
			return nil
		}
		return fmt.Errorf("gagal mengirim pesan: %w", err)
	}

	return nil
}

// SendTyping sends a typing indicator to a Telegram chat.
func (a *Adapter) SendTyping(ctx context.Context, channelID string) error {
	if a.bot == nil {
		return nil
	}

	chatID, err := parseChatID(channelID)
	if err != nil {
		return nil // non-critical, ignore errors
	}

	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	_, _ = a.bot.Send(action)
	return nil
}

// Close shuts down the Telegram bot.
func (a *Adapter) Close() error {
	if a.bot != nil {
		a.bot.StopReceivingUpdates()
	}
	return nil
}

// convertMessage converts a Telegram message to a platform.IncomingMessage.
func (a *Adapter) convertMessage(tgMsg *tgbotapi.Message) platform.IncomingMessage {
	msg := platform.IncomingMessage{
		ID:        fmt.Sprintf("%d", tgMsg.MessageID),
		Platform:  "telegram",
		ChannelID: fmt.Sprintf("%d", tgMsg.Chat.ID),
		UserID:    fmt.Sprintf("%d", tgMsg.From.ID),
		Username:  tgMsg.From.UserName,
		Content:   tgMsg.Text,
		Metadata:  make(map[string]string),
		Timestamp: time.Unix(int64(tgMsg.Date), 0),
	}

	// Use first name if username is empty
	if msg.Username == "" {
		msg.Username = strings.TrimSpace(tgMsg.From.FirstName + " " + tgMsg.From.LastName)
	}

	// Store chat type in metadata
	msg.Metadata["chat_type"] = tgMsg.Chat.Type

	// Parse commands
	if tgMsg.IsCommand() {
		msg.IsCommand = true
		msg.Command = tgMsg.Command()
		argStr := tgMsg.CommandArguments()
		if argStr != "" {
			msg.CommandArgs = strings.Fields(argStr)
		}
		msg.Content = argStr // content = everything after the command
	}

	// Handle reply
	if tgMsg.ReplyToMessage != nil {
		msg.ReplyTo = fmt.Sprintf("%d", tgMsg.ReplyToMessage.MessageID)
	}

	// Handle attachments
	if tgMsg.Photo != nil && len(tgMsg.Photo) > 0 {
		// Get the largest photo
		largest := tgMsg.Photo[len(tgMsg.Photo)-1]
		msg.Attachments = append(msg.Attachments, platform.Attachment{
			Type:     "image",
			FileName: largest.FileID,
			Size:     int64(largest.FileSize),
		})
	}
	if tgMsg.Document != nil {
		msg.Attachments = append(msg.Attachments, platform.Attachment{
			Type:     "file",
			FileName: tgMsg.Document.FileName,
			MimeType: tgMsg.Document.MimeType,
			Size:     int64(tgMsg.Document.FileSize),
		})
	}

	return msg
}

// parseChatID converts a string channel ID to int64 for the Telegram API.
func parseChatID(channelID string) (int64, error) {
	var chatID int64
	_, err := fmt.Sscanf(channelID, "%d", &chatID)
	if err != nil {
		return 0, fmt.Errorf("invalid chat ID: %s", channelID)
	}
	return chatID, nil
}
