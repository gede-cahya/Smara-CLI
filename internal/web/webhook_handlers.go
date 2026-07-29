package web

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gede-cahya/Smara-CLI/pkg/platform/discord"
)

// handleGitHubWebhook processes incoming GitHub webhook events and forwards release notifications to Discord.
func (s *Server) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Gagal membaca body request")
		return
	}
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	event := r.Header.Get("X-GitHub-Event")
	if event == "ping" {
		jsonResponse(w, http.StatusOK, map[string]string{"message": "pong", "status": "active"})
		return
	}

	var payload discord.GitHubReleasePayload
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		errorResponse(w, http.StatusBadRequest, "Payload JSON tidak valid: "+err.Error())
		return
	}

	// Verify that this is a release event with a valid tag
	if payload.Release.TagName == "" {
		jsonResponse(w, http.StatusOK, map[string]string{"status": "ignored", "reason": "Bukan payload GitHub release yang valid (tag_name kosong)"})
		return
	}

	// Only process published/released actions or empty action (for direct test payload)
	action := strings.ToLower(payload.Action)
	if action != "" && action != "published" && action != "released" && action != "created" {
		jsonResponse(w, http.StatusOK, map[string]string{"status": "ignored", "reason": "Action release diabaikan: " + payload.Action})
		return
	}

	releaseChannelID := ""
	releaseWebhookURL := ""

	if s.Cfg != nil {
		releaseChannelID = s.Cfg.Platforms.Discord.ReleaseChannelID
		releaseWebhookURL = s.Cfg.Platforms.Discord.ReleaseWebhookURL
	}

	if releaseWebhookURL == "" {
		releaseWebhookURL = os.Getenv("SMARA_DISCORD_WEBHOOK_URL")
	}
	if releaseChannelID == "" {
		releaseChannelID = os.Getenv("SMARA_DISCORD_RELEASE_CHANNEL_ID")
	}

	sentCount := 0
	var errMsgs []string

	// 1. Send via Discord Webhook URL if configured
	if releaseWebhookURL != "" {
		if err := discord.SendReleaseWebhook(r.Context(), releaseWebhookURL, &payload); err != nil {
			log.Printf("[web] Gagal mengirim release via Discord Webhook: %v", err)
			errMsgs = append(errMsgs, "Webhook: "+err.Error())
		} else {
			sentCount++
			log.Printf("[web] Berhasil mengirim release %s ke Discord Webhook", payload.Release.TagName)
		}
	}

	if sentCount == 0 && len(errMsgs) > 0 {
		errorResponse(w, http.StatusInternalServerError, "Gagal mengirim notifikasi release: "+strings.Join(errMsgs, "; "))
		return
	}

	if sentCount == 0 && releaseWebhookURL == "" && releaseChannelID == "" {
		log.Printf("[web] Webhook release diterima untuk %s tetapi release_webhook_url/release_channel_id belum dikonfigurasi", payload.Release.TagName)
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"status":  "received_but_unconfigured",
			"message": "Pesan release diterima tetapi release_webhook_url atau release_channel_id belum dikonfigurasi.",
			"release": payload.Release.TagName,
		})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "Notifikasi update GitHub Release berhasil dikirim ke Discord",
		"tag":     payload.Release.TagName,
		"name":    payload.Release.Name,
		"sent":    sentCount,
	})
}
