package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/pkg/platform/discord"
)

func TestHandleGitHubWebhook_Ping(t *testing.T) {
	srv := NewServer("127.0.0.1:0", nil, nil, nil, &config.SmaraConfig{})
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/github", bytes.NewBufferString("{}"))
	req.Header.Set("X-GitHub-Event", "ping")
	rec := httptest.NewRecorder()

	srv.handleGitHubWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var res map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res["message"] != "pong" {
		t.Errorf("expected message pong, got %s", res["message"])
	}
}

func TestHandleGitHubWebhook_ReleaseWithMockWebhook(t *testing.T) {
	// Create mock Discord webhook server
	var receivedWebhook discord.DiscordWebhookPayload
	mockDiscord := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedWebhook)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer mockDiscord.Close()

	cfg := &config.SmaraConfig{}
	cfg.Platforms.Discord.ReleaseWebhookURL = mockDiscord.URL

	srv := NewServer("127.0.0.1:0", nil, nil, nil, cfg)

	ghPayload := discord.GitHubReleasePayload{
		Action: "published",
		Release: discord.GitHubRelease{
			TagName: "v1.20.67",
			Name:    "v1.20.67 Release",
			Body:    "Test release body",
			HTMLURL: "https://github.com/test/repo/releases/tag/v1.20.67",
		},
		Repository: discord.GitHubRepo{
			FullName: "test/repo",
		},
	}

	payloadBytes, _ := json.Marshal(ghPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/github", bytes.NewBuffer(payloadBytes))
	req.Header.Set("X-GitHub-Event", "release")
	rec := httptest.NewRecorder()

	srv.handleGitHubWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if len(receivedWebhook.Embeds) != 1 {
		t.Fatalf("expected 1 embed sent to Discord webhook, got %d", len(receivedWebhook.Embeds))
	}

	if receivedWebhook.Embeds[0].URL != ghPayload.Release.HTMLURL {
		t.Errorf("expected URL %s, got %s", ghPayload.Release.HTMLURL, receivedWebhook.Embeds[0].URL)
	}
}
