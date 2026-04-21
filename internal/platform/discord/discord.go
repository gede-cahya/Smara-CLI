// Package discord provides the Discord Bot adapter for Smara.
package discord

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/gede-cahya/Smara-CLI/internal/platform"
)

// maxEmbedLength is the max length for a Discord embed description.
const maxEmbedLength = 4096

// Adapter implements platform.PlatformAdapter for Discord.
type Adapter struct {
	session *discordgo.Session
	config  platform.AdapterConfig
	handler platform.MessageHandler
	ctx     context.Context
	botID   string
}

// New creates a new Discord adapter.
func New() *Adapter {
	return &Adapter{}
}

// Name returns the platform identifier.
func (a *Adapter) Name() string {
	return "discord"
}

// Connect initializes the Discord bot connection.
func (a *Adapter) Connect(ctx context.Context, cfg platform.AdapterConfig) error {
	if cfg.Token == "" {
		return fmt.Errorf("discord bot token tidak boleh kosong")
	}

	dg, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		return fmt.Errorf("gagal membuat Discord session: %w", err)
	}

	// Set intents
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages | discordgo.IntentMessageContent

	a.session = dg
	a.config = cfg
	a.ctx = ctx

	return nil
}

// Listen starts the Discord bot and dispatches messages to the handler.
func (a *Adapter) Listen(ctx context.Context, handler platform.MessageHandler) error {
	if a.session == nil {
		return fmt.Errorf("session belum terhubung, panggil Connect() dulu")
	}

	a.handler = handler
	a.ctx = ctx

	// Register message handler
	a.session.AddHandler(a.onMessageCreate)

	// Open websocket connection
	if err := a.session.Open(); err != nil {
		return fmt.Errorf("gagal membuka koneksi Discord: %w", err)
	}

	// Store bot's own user ID
	a.botID = a.session.State.User.ID
	log.Printf("[discord] Bot terhubung sebagai %s#%s (%s)", a.session.State.User.Username, a.session.State.User.Discriminator, a.botID)

	// Register slash commands
	a.registerSlashCommands()

	// Block until context is cancelled
	<-ctx.Done()

	return nil
}

// SendMessage sends a message to a Discord channel.
func (a *Adapter) SendMessage(ctx context.Context, channelID string, msg platform.OutgoingMessage) error {
	if a.session == nil {
		return fmt.Errorf("session belum terhubung")
	}

	content := msg.Content

	// If content is short enough, send as plain message
	if len(content) <= 2000 {
		_, err := a.session.ChannelMessageSend(channelID, content)
		if err != nil {
			return fmt.Errorf("gagal mengirim pesan: %w", err)
		}
		return nil
	}

	// For longer messages, use an embed
	if len(content) <= maxEmbedLength {
		embed := &discordgo.MessageEmbed{
			Description: content,
			Color:       0x7D56F4, // Smara purple
			Footer: &discordgo.MessageEmbedFooter{
				Text: "🌀 Smara AI",
			},
		}
		_, err := a.session.ChannelMessageSendEmbed(channelID, embed)
		if err != nil {
			// Fallback to plain text
			_, err = a.session.ChannelMessageSend(channelID, content[:2000])
			return err
		}
		return nil
	}

	// Very long messages: split into multiple embeds
	parts := splitContent(content, maxEmbedLength)
	for i, part := range parts {
		embed := &discordgo.MessageEmbed{
			Description: part,
			Color:       0x7D56F4,
		}
		if i == len(parts)-1 {
			embed.Footer = &discordgo.MessageEmbedFooter{
				Text: "🌀 Smara AI",
			}
		}
		_, err := a.session.ChannelMessageSendEmbed(channelID, embed)
		if err != nil {
			return fmt.Errorf("gagal mengirim embed part %d: %w", i+1, err)
		}
	}

	return nil
}

// SendTyping sends a typing indicator to a Discord channel.
func (a *Adapter) SendTyping(ctx context.Context, channelID string) error {
	if a.session == nil {
		return nil
	}
	_ = a.session.ChannelTyping(channelID)
	return nil
}

// Close shuts down the Discord bot.
func (a *Adapter) Close() error {
	if a.session != nil {
		return a.session.Close()
	}
	return nil
}

// onMessageCreate handles incoming Discord messages.
func (a *Adapter) onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore messages from the bot itself
	if m.Author.ID == a.botID {
		return
	}

	// Ignore bot messages
	if m.Author.Bot {
		return
	}

	msg := a.convertMessage(m)

	// Check if the message mentions the bot or is a DM
	isDM := m.GuildID == ""
	isMentioned := false
	for _, mention := range m.Mentions {
		if mention.ID == a.botID {
			isMentioned = true
			break
		}
	}

	// Check if starts with command prefix (default: "!")
	prefix := "!"
	if extra, ok := a.config.Extra["command_prefix"]; ok && extra != "" {
		prefix = extra
	}
	isCommand := strings.HasPrefix(m.Content, prefix)

	// Only respond if mentioned, DM, or command
	if !isDM && !isMentioned && !isCommand {
		return
	}

	// Strip bot mention from content
	if isMentioned {
		msg.Content = strings.TrimSpace(strings.ReplaceAll(msg.Content, "<@"+a.botID+">", ""))
		msg.Content = strings.TrimSpace(strings.ReplaceAll(msg.Content, "<@!"+a.botID+">", ""))
	}

	// Parse command from prefix
	if isCommand && !msg.IsCommand {
		content := strings.TrimPrefix(m.Content, prefix)
		parts := strings.Fields(content)
		if len(parts) > 0 {
			// Handle "smara" prefix: !smara ask hello → command=ask
			if strings.ToLower(parts[0]) == "smara" && len(parts) > 1 {
				msg.IsCommand = true
				msg.Command = strings.ToLower(parts[1])
				msg.CommandArgs = parts[2:]
				msg.Content = strings.Join(parts[2:], " ")
			} else {
				msg.IsCommand = true
				msg.Command = strings.ToLower(parts[0])
				msg.CommandArgs = parts[1:]
				msg.Content = strings.Join(parts[1:], " ")
			}
		}
	}

	// If mentioned without command, treat as prompt
	if isMentioned && !msg.IsCommand && msg.Content != "" {
		// just pass content through as a prompt
	}

	go func() {
		if err := a.handler(a.ctx, msg); err != nil {
			log.Printf("[discord] Error handling message: %v", err)
		}
	}()
}

// convertMessage converts a Discord message to a platform.IncomingMessage.
func (a *Adapter) convertMessage(m *discordgo.MessageCreate) platform.IncomingMessage {
	msg := platform.IncomingMessage{
		ID:        m.ID,
		Platform:  "discord",
		ChannelID: m.ChannelID,
		UserID:    m.Author.ID,
		Username:  m.Author.Username,
		Content:   m.Content,
		Metadata:  make(map[string]string),
		Timestamp: time.Now(),
	}

	// Use message timestamp
	msg.Timestamp = m.Timestamp

	// Store guild info
	msg.Metadata["guild_id"] = m.GuildID

	// Handle attachments
	for _, att := range m.Attachments {
		attType := "file"
		if strings.HasPrefix(att.ContentType, "image/") {
			attType = "image"
		} else if strings.HasPrefix(att.ContentType, "video/") {
			attType = "video"
		}
		msg.Attachments = append(msg.Attachments, platform.Attachment{
			Type:     attType,
			URL:      att.URL,
			FileName: att.Filename,
			MimeType: att.ContentType,
			Size:     int64(att.Size),
		})
	}

	return msg
}

// registerSlashCommands registers Discord slash commands.
func (a *Adapter) registerSlashCommands() {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "smara",
			Description: "Berinteraksi dengan Smara AI",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "ask",
					Description: "Kirim pertanyaan ke Smara",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{
							Name:        "prompt",
							Description: "Pertanyaan atau perintah",
							Type:        discordgo.ApplicationCommandOptionString,
							Required:    true,
						},
					},
				},
				{
					Name:        "mode",
					Description: "Ganti mode agen (ask/rush/plan)",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{
							Name:        "name",
							Description: "Nama mode: ask, rush, plan",
							Type:        discordgo.ApplicationCommandOptionString,
							Required:    false,
						},
					},
				},
				{
					Name:        "help",
					Description: "Tampilkan bantuan",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
			},
		},
	}

	// Register globally (or per guild)
	guildIDs := a.config.GuildIDs
	if len(guildIDs) == 0 {
		// Register globally
		for _, cmd := range commands {
			_, err := a.session.ApplicationCommandCreate(a.session.State.User.ID, "", cmd)
			if err != nil {
				log.Printf("[discord] Gagal mendaftarkan slash command '%s': %v", cmd.Name, err)
			}
		}
	} else {
		for _, guildID := range guildIDs {
			for _, cmd := range commands {
				_, err := a.session.ApplicationCommandCreate(a.session.State.User.ID, guildID, cmd)
				if err != nil {
					log.Printf("[discord] Gagal mendaftarkan slash command '%s' di guild %s: %v", cmd.Name, guildID, err)
				}
			}
		}
	}

	// Handle slash command interactions
	a.session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}

		data := i.ApplicationCommandData()
		if data.Name != "smara" {
			return
		}

		if len(data.Options) == 0 {
			return
		}

		subCmd := data.Options[0]
		msg := platform.IncomingMessage{
			ID:        i.ID,
			Platform:  "discord",
			ChannelID: i.ChannelID,
			UserID:    i.Member.User.ID,
			Username:  i.Member.User.Username,
			IsCommand: true,
			Command:   subCmd.Name,
			Metadata:  map[string]string{"guild_id": i.GuildID, "interaction": "true"},
			Timestamp: time.Now(),
		}

		// Extract options
		for _, opt := range subCmd.Options {
			msg.CommandArgs = append(msg.CommandArgs, opt.StringValue())
		}
		msg.Content = strings.Join(msg.CommandArgs, " ")

		// Acknowledge the interaction
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		})

		// Process in background
		go func() {
			if err := a.handler(a.ctx, msg); err != nil {
				log.Printf("[discord] Error handling slash command: %v", err)
			}
		}()
	})
}

// splitContent splits a long string into chunks.
func splitContent(content string, maxLen int) []string {
	if len(content) <= maxLen {
		return []string{content}
	}
	var parts []string
	for len(content) > 0 {
		if len(content) <= maxLen {
			parts = append(parts, content)
			break
		}
		splitAt := maxLen
		lastNL := strings.LastIndex(content[:maxLen], "\n")
		if lastNL > maxLen/2 {
			splitAt = lastNL + 1
		}
		parts = append(parts, content[:splitAt])
		content = content[splitAt:]
	}
	return parts
}
