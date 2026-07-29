package discord

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildReleaseEmbed(t *testing.T) {
	payload := &GitHubReleasePayload{
		Action: "published",
		Release: GitHubRelease{
			ID:          12345,
			TagName:     "v1.20.67",
			Name:        "Smara CLI v1.20.67",
			Body:        "## Feature\n- Added GitHub Release Discord notification",
			HTMLURL:     "https://github.com/gede-cahya/Smara-CLI/releases/tag/v1.20.67",
			PublishedAt: "2026-07-29T12:00:00Z",
			Author: GitHubUser{
				Login:     "cahya",
				AvatarURL: "https://github.com/cahya.png",
			},
			Assets: []GitHubAsset{
				{
					Name:               "smara-v1.20.67-linux-amd64.tar.gz",
					Size:               15432100,
					BrowserDownloadURL: "https://github.com/gede-cahya/Smara-CLI/releases/download/v1.20.67/smara-v1.20.67-linux-amd64.tar.gz",
				},
			},
		},
		Repository: GitHubRepo{
			Name:     "Smara-CLI",
			FullName: "gede-cahya/Smara-CLI",
			HTMLURL:  "https://github.com/gede-cahya/Smara-CLI",
		},
	}

	embed := BuildReleaseEmbed(payload)
	if embed == nil {
		t.Fatalf("BuildReleaseEmbed returned nil")
	}

	if embed.Title == "" {
		t.Errorf("expected non-empty title")
	}

	if embed.URL != payload.Release.HTMLURL {
		t.Errorf("expected URL %s, got %s", payload.Release.HTMLURL, embed.URL)
	}

	if len(embed.Fields) < 2 {
		t.Errorf("expected at least 2 fields, got %d", len(embed.Fields))
	}
}

func TestSendReleaseWebhook(t *testing.T) {
	var receivedPayload DiscordWebhookPayload

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedPayload); err != nil {
			t.Errorf("failed to decode webhook body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	payload := &GitHubReleasePayload{
		Action: "published",
		Release: GitHubRelease{
			TagName: "v1.0.0",
			Name:    "v1.0.0 Release",
			HTMLURL: "https://github.com/test/repo/releases/v1.0.0",
		},
	}

	err := SendReleaseWebhook(context.Background(), ts.URL, payload)
	if err != nil {
		t.Fatalf("SendReleaseWebhook failed: %v", err)
	}

	if len(receivedPayload.Embeds) != 1 {
		t.Fatalf("expected 1 embed, got %d", len(receivedPayload.Embeds))
	}
}
