// Package discord provides the Discord Bot adapter for Smara.
package discord

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
	prd     *prdWizardStore
}

// New creates a new Discord adapter.
func New() *Adapter {
	return &Adapter{prd: newPRDWizardStore()}
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

	// Register message and interaction handlers
	a.session.AddHandler(a.onMessageCreate)
	a.session.AddHandler(a.onInteractionCreate)

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
	if len(msg.Attachments) > 0 {
		return a.sendMessageWithAttachments(channelID, content, msg.Attachments)
	}

	// Short formatted responses look better as rich embeds.
	if msg.Format == platform.FormatMarkdown && len(content) <= maxEmbedLength && !strings.Contains(content, "```") {
		embed := &discordgo.MessageEmbed{
			Description: content,
			Color:       0xBEF264,
			Footer: &discordgo.MessageEmbedFooter{
				Text: "🌀 Smara AI • polished response",
			},
		}
		_, err := a.session.ChannelMessageSendEmbed(channelID, embed)
		if err == nil {
			return nil
		}
		// Fallback to plain content below.
	}

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

func (a *Adapter) sendMessageWithAttachments(channelID, content string, attachments []platform.Attachment) error {
	var files []*discordgo.File
	for _, att := range attachments {
		if att.FilePath == "" {
			continue
		}
		f, err := os.Open(att.FilePath)
		if err != nil {
			return fmt.Errorf("gagal membuka attachment: %w", err)
		}
		defer f.Close()
		name := att.FileName
		if name == "" {
			name = f.Name()
		}
		files = append(files, &discordgo.File{
			Name:        name,
			ContentType: att.MimeType,
			Reader:      f,
		})
	}
	if len(files) == 0 {
		_, err := a.session.ChannelMessageSend(channelID, content)
		if err != nil {
			return fmt.Errorf("gagal mengirim pesan: %w", err)
		}
		return nil
	}
	if len(content) > 2000 {
		parts := splitContent(content, 2000)
		for _, part := range parts[:len(parts)-1] {
			if _, err := a.session.ChannelMessageSend(channelID, part); err != nil {
				return fmt.Errorf("gagal mengirim pesan: %w", err)
			}
		}
		content = parts[len(parts)-1]
	}
	_, err := a.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content: content,
		Files:   files,
	})
	if err != nil {
		return fmt.Errorf("gagal mengirim attachment: %w", err)
	}
	return nil
}

// DownloadAttachment downloads a Discord attachment URL to a local file.
func (a *Adapter) DownloadAttachment(ctx context.Context, id string) (string, error) {
	attachmentURL := strings.TrimSpace(id)
	if attachmentURL == "" {
		return "", fmt.Errorf("attachment URL kosong")
	}
	parsed, err := url.Parse(attachmentURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("attachment URL Discord tidak valid")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, attachmentURL, nil)
	if err != nil {
		return "", fmt.Errorf("gagal membuat request attachment: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("gagal download attachment Discord: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("gagal download attachment Discord: HTTP %d", resp.StatusCode)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	dir := filepath.Join(home, ".smara", "attachments", "discord")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("gagal membuat folder attachment: %w", err)
	}

	ext := discordAttachmentExtension(parsed.Path, resp.Header.Get("Content-Type"))
	file, err := os.CreateTemp(dir, "discord-*"+ext)
	if err != nil {
		return "", fmt.Errorf("gagal membuat file attachment: %w", err)
	}
	defer file.Close()
	if _, err := io.Copy(file, resp.Body); err != nil {
		return "", fmt.Errorf("gagal menyimpan attachment: %w", err)
	}
	return file.Name(), nil
}

func discordAttachmentExtension(pathValue, contentType string) string {
	if ext := filepath.Ext(pathValue); ext != "" {
		return ext
	}
	switch strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0])) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".bin"
	}
}

// SendTyping sends a typing indicator to a Discord channel.
func (a *Adapter) SendTyping(ctx context.Context, channelID string) error {
	if a.session == nil {
		return nil
	}
	_ = a.session.ChannelTyping(channelID)
	return nil
}

// SendMessageWithID sends a message and returns its message ID.
func (a *Adapter) SendMessageWithID(ctx context.Context, channelID string, msg platform.OutgoingMessage) (string, error) {
	if a.session == nil {
		return "", fmt.Errorf("session belum terhubung")
	}

	sent, err := a.session.ChannelMessageSend(channelID, msg.Content)
	if err != nil {
		return "", fmt.Errorf("gagal mengirim pesan: %w", err)
	}

	return sent.ID, nil
}

// EditMessage edits an existing Discord message.
func (a *Adapter) EditMessage(ctx context.Context, channelID string, messageID string, msg platform.OutgoingMessage) error {
	if a.session == nil {
		return fmt.Errorf("session belum terhubung")
	}

	_, err := a.session.ChannelMessageEdit(channelID, messageID, msg.Content)
	if err != nil {
		return fmt.Errorf("gagal mengedit pesan: %w", err)
	}

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
	if len(msg.Attachments) > 0 || msg.ReplyTo != "" || msg.Metadata["reply_message_id"] != "" {
		log.Printf("[discord] converted message attachments=%d reply=%s referenced=%s", len(msg.Attachments), msg.ReplyTo, msg.Metadata["reply_message_id"])
	}

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

	a.appendDiscordMessageImages(&msg, m.Message)
	if m.ReferencedMessage != nil {
		msg.Metadata["reply_message_id"] = m.ReferencedMessage.ID
		a.appendDiscordMessageImages(&msg, m.ReferencedMessage)
	} else if m.MessageReference != nil && m.MessageReference.MessageID != "" && a.session != nil {
		msg.Metadata["reply_message_id"] = m.MessageReference.MessageID
		channelID := m.ChannelID
		if m.MessageReference.ChannelID != "" {
			channelID = m.MessageReference.ChannelID
		}
		ref, err := a.session.ChannelMessage(channelID, m.MessageReference.MessageID)
		if err != nil {
			log.Printf("[discord] gagal mengambil referenced message %s/%s: %v", channelID, m.MessageReference.MessageID, err)
		} else {
			a.appendDiscordMessageImages(&msg, ref)
		}
	}

	return msg
}

func (a *Adapter) appendDiscordMessageImages(msg *platform.IncomingMessage, message *discordgo.Message) {
	if message == nil {
		return
	}
	a.appendDiscordAttachments(msg, message.Attachments)
	a.appendDiscordEmbeds(msg, message.Embeds)
}

func (a *Adapter) appendDiscordAttachments(msg *platform.IncomingMessage, attachments []*discordgo.MessageAttachment) {
	for _, att := range attachments {
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
}

func (a *Adapter) appendDiscordEmbeds(msg *platform.IncomingMessage, embeds []*discordgo.MessageEmbed) {
	for _, embed := range embeds {
		if embed == nil {
			continue
		}
		if embed.Image != nil {
			a.appendDiscordEmbedImage(msg, embed.Image.URL, embed.Image.ProxyURL)
		}
		if embed.Thumbnail != nil {
			a.appendDiscordEmbedImage(msg, embed.Thumbnail.URL, embed.Thumbnail.ProxyURL)
		}
	}
}

func (a *Adapter) appendDiscordEmbedImage(msg *platform.IncomingMessage, imageURL, proxyURL string) {
	url := imageURL
	if proxyURL != "" {
		url = proxyURL
	}
	if url == "" {
		return
	}
	msg.Attachments = append(msg.Attachments, platform.Attachment{
		Type:     "image",
		URL:      url,
		FileName: filepath.Base(strings.Split(imageURL, "?")[0]),
	})
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
						{Name: "prompt", Description: "Pertanyaan atau perintah", Type: discordgo.ApplicationCommandOptionString, Required: true},
					},
				},
				{
					Name:        "mode",
					Description: "Ganti mode agen (ask/rush/plan)",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Name: "name", Description: "Nama mode: ask, rush, plan", Type: discordgo.ApplicationCommandOptionString, Required: false},
					},
				},
				{Name: "help", Description: "Tampilkan bantuan", Type: discordgo.ApplicationCommandOptionSubCommand},
				{
					Name:        "prd",
					Description: "Buat PRD interaktif dengan quest dan button",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Name: "idea", Description: "Ide produk singkat untuk PRD", Type: discordgo.ApplicationCommandOptionString, Required: false},
					},
				},
			},
		},
	}

	// Register globally (or per guild)
	guildIDs := a.config.GuildIDs
	if len(guildIDs) == 0 {
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

func (a *Adapter) onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		a.onSlashCommand(s, i)
	case discordgo.InteractionMessageComponent:
		a.onComponentInteraction(s, i)
	}
}

func (a *Adapter) onSlashCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	if data.Name != "smara" || len(data.Options) == 0 {
		return
	}

	subCmd := data.Options[0]
	user := interactionUser(i)
	if user == nil {
		return
	}

	if subCmd.Name == "prd" {
		idea := ""
		for _, opt := range subCmd.Options {
			if opt.Name == "idea" {
				idea = opt.StringValue()
			}
		}
		a.startPRDWizardInteraction(s, i, user, idea)
		return
	}

	msg := platform.IncomingMessage{
		ID:        i.ID,
		Platform:  "discord",
		ChannelID: i.ChannelID,
		UserID:    user.ID,
		Username:  user.Username,
		IsCommand: true,
		Command:   subCmd.Name,
		Metadata:  map[string]string{"guild_id": i.GuildID, "interaction": "true"},
		Timestamp: time.Now(),
	}
	for _, opt := range subCmd.Options {
		msg.CommandArgs = append(msg.CommandArgs, opt.StringValue())
	}
	msg.Content = strings.Join(msg.CommandArgs, " ")

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource})
	go func() {
		if err := a.handler(a.ctx, msg); err != nil {
			log.Printf("[discord] Error handling slash command: %v", err)
		}
	}()
}

func (a *Adapter) startPRDWizardInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, user *discordgo.User, idea string) {
	if a.prd == nil {
		a.prd = newPRDWizardStore()
	}
	a.prd.cleanup(2 * time.Hour)
	sess := a.prd.start(i.GuildID, i.ChannelID, user.ID, user.Username, idea)
	resp := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:    renderPRDWizardMessage(sess),
			Components: renderPRDComponents(sess),
		},
	}
	if err := s.InteractionRespond(i.Interaction, resp); err != nil {
		log.Printf("[discord] gagal memulai PRD wizard: %v", err)
	}
}

func (a *Adapter) onComponentInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	sessionID, value, ok := parsePRDButtonID(data.CustomID)
	if !ok {
		return
	}
	user := interactionUser(i)
	if user == nil {
		return
	}
	if a.prd == nil {
		a.prd = newPRDWizardStore()
	}

	sess, allowed, err := a.prd.apply(sessionID, user.ID, value)
	if err != nil || !allowed {
		content := "⚠️ " + err.Error()
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: content, Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	if sess.Step == prdStepDone {
		a.prd.delete(sessionID)
		a.finishPRDWizard(s, i, sess)
		return
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    renderPRDWizardMessage(sess),
			Components: renderPRDComponents(sess),
		},
	})
}

func (a *Adapter) finishPRDWizard(s *discordgo.Session, i *discordgo.InteractionCreate, sess *prdWizardSession) {
	prd := GeneratePRDMarkdown(sess.Answers)
	fileName := PRDFileName(sess.Answers.ProductName)
	file, err := os.CreateTemp(os.TempDir(), "smara-prd-*.md")
	if err != nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Content: "❌ Gagal membuat file PRD."}})
		return
	}
	_, writeErr := file.WriteString(prd)
	_ = file.Close()
	if writeErr != nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Content: "❌ Gagal menulis file PRD."}})
		return
	}
	defer os.Remove(file.Name())

	preview := prd
	if len(preview) > 1400 {
		preview = preview[:1400] + "\n\n...\n"
	}
	content := "✅ **PRD selesai dibuat!**\nFile Markdown terlampir untuk download. Preview copy-paste:\n\n```markdown\n" + preview + "```"
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseUpdateMessage, Data: &discordgo.InteractionResponseData{Content: "✅ PRD selesai dibuat. Mengirim file Markdown...", Components: []discordgo.MessageComponent{}}})
	f, err := os.Open(file.Name())
	if err != nil {
		return
	}
	defer f.Close()
	_, err = s.ChannelMessageSendComplex(i.ChannelID, &discordgo.MessageSend{
		Content: content,
		Files:   []*discordgo.File{{Name: fileName, ContentType: "text/markdown", Reader: f}},
	})
	if err != nil {
		log.Printf("[discord] gagal mengirim PRD file: %v", err)
	}
}

func interactionUser(i *discordgo.InteractionCreate) *discordgo.User {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User
	}
	return i.User
}
