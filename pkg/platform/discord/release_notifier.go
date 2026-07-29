package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// GitHubReleasePayload represents the webhook payload sent by GitHub when a release event occurs.
type GitHubReleasePayload struct {
	Action     string        `json:"action"`
	Release    GitHubRelease `json:"release"`
	Repository GitHubRepo    `json:"repository"`
	Sender     GitHubUser    `json:"sender"`
}

// GitHubRelease contains details about the published GitHub release.
type GitHubRelease struct {
	ID          int64         `json:"id"`
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	HTMLURL     string        `json:"html_url"`
	Prerelease  bool          `json:"prerelease"`
	Draft       bool          `json:"draft"`
	PublishedAt string        `json:"published_at"`
	Author      GitHubUser    `json:"author"`
	Assets      []GitHubAsset `json:"assets"`
}

// GitHubRepo contains repository info from GitHub webhook.
type GitHubRepo struct {
	Name     string     `json:"name"`
	FullName string     `json:"full_name"`
	HTMLURL  string     `json:"html_url"`
	Owner    GitHubUser `json:"owner"`
}

// GitHubUser contains user/author details.
type GitHubUser struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
	HTMLURL   string `json:"html_url"`
}

// GitHubAsset contains release binary asset details.
type GitHubAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	DownloadCount      int    `json:"download_count"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// BuildReleaseEmbed formats a GitHub release event into a Discord MessageEmbed.
func BuildReleaseEmbed(payload *GitHubReleasePayload) *discordgo.MessageEmbed {
	rel := payload.Release
	repo := payload.Repository

	releaseName := rel.Name
	if releaseName == "" {
		releaseName = rel.TagName
	}

	embedTitle := fmt.Sprintf("🚀 New Release: %s", releaseName)
	if repo.FullName != "" && !strings.Contains(releaseName, repo.Name) {
		embedTitle = fmt.Sprintf("🚀 New Release: [%s] %s", repo.FullName, releaseName)
	}

	// Description Header + Body
	var descBuilder strings.Builder
	descBuilder.WriteString(fmt.Sprintf("# 🌀 %s Released!\n\n", releaseName))

	bodyText := strings.TrimSpace(rel.Body)
	if bodyText != "" {
		if len(bodyText) > 1600 {
			bodyText = bodyText[:1597] + "..."
		}
		descBuilder.WriteString(bodyText)
	} else {
		descBuilder.WriteString("_Tidak ada release notes yang disertakan._")
	}

	embed := &discordgo.MessageEmbed{
		Title:       embedTitle,
		URL:         rel.HTMLURL,
		Description: descBuilder.String(),
		Color:       8214260, // 0x7D56F4
		Timestamp:   rel.PublishedAt,
		Footer: &discordgo.MessageEmbedFooter{
			Text: "🌀 Smara Discord Integration • GitHub Release Notification",
		},
	}

	if payload.Sender.AvatarURL != "" {
		embed.Author = &discordgo.MessageEmbedAuthor{
			Name:    payload.Sender.Login,
			IconURL: payload.Sender.AvatarURL,
			URL:     payload.Sender.HTMLURL,
		}
	} else if rel.Author.AvatarURL != "" {
		embed.Author = &discordgo.MessageEmbedAuthor{
			Name:    rel.Author.Login,
			IconURL: rel.Author.AvatarURL,
			URL:     rel.Author.HTMLURL,
		}
	}

	// Add Fields for Version & Assets
	fields := []*discordgo.MessageEmbedField{
		{
			Name:   "📌 Version Tag",
			Value:  fmt.Sprintf("`%s`", rel.TagName),
			Inline: true,
		},
	}

	if repo.FullName != "" {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "📦 Repository",
			Value:  fmt.Sprintf("[%s](%s)", repo.FullName, repo.HTMLURL),
			Inline: true,
		})
	}

	if len(rel.Assets) > 0 {
		var assetSb strings.Builder
		for i, asset := range rel.Assets {
			if i >= 6 {
				assetSb.WriteString(fmt.Sprintf("\n_...and %d more assets_", len(rel.Assets)-6))
				break
			}
			sizeMB := float64(asset.Size) / (1024 * 1024)
			assetSb.WriteString(fmt.Sprintf("• [%s](%s) (%.1f MB)\n", asset.Name, asset.BrowserDownloadURL, sizeMB))
		}
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "💾 Downloads / Assets",
			Value:  assetSb.String(),
			Inline: false,
		})
	}

	embed.Fields = fields
	return embed
}

// SendReleaseToChannel sends a GitHub release notification using the bot session to a Discord channel.
func (a *Adapter) SendReleaseToChannel(ctx context.Context, channelID string, payload *GitHubReleasePayload) error {
	if a.session == nil {
		return fmt.Errorf("discord session belum terhubung")
	}
	embed := BuildReleaseEmbed(payload)
	_, err := a.session.ChannelMessageSendEmbed(channelID, embed)
	if err != nil {
		return fmt.Errorf("gagal mengirim release embed ke channel %s: %w", channelID, err)
	}
	return nil
}

// DiscordWebhookPayload represents the JSON body sent to a Discord Webhook.
type DiscordWebhookPayload struct {
	Username  string                  `json:"username,omitempty"`
	AvatarURL string                  `json:"avatar_url,omitempty"`
	Embeds    []*discordgo.MessageEmbed `json:"embeds"`
}

// SendReleaseWebhook sends a GitHub release notification directly to a Discord Webhook URL.
func SendReleaseWebhook(ctx context.Context, webhookURL string, payload *GitHubReleasePayload) error {
	webhookURL = strings.TrimSpace(webhookURL)
	if webhookURL == "" {
		return fmt.Errorf("discord webhook URL kosong")
	}

	embed := BuildReleaseEmbed(payload)
	body := DiscordWebhookPayload{
		Username: "Smara Release Bot",
		Embeds:   []*discordgo.MessageEmbed{embed},
	}

	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("gagal marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return fmt.Errorf("gagal membuat request webhook: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("gagal melempar webhook ke Discord: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord webhook merespon dengan status HTTP %d", resp.StatusCode)
	}

	return nil
}
