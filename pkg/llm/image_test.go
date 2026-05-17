package llm

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomProviderGenerateImage(t *testing.T) {
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/images/generations", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var req openAIImageRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "gpt-image-2", req.Model)
		assert.Equal(t, "robot terminal", req.Prompt)
		assert.Equal(t, "b64_json", req.ResponseFormat)
		assert.Equal(t, "1024x1024", req.Size)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openAIImageResponse{Data: []struct {
			B64JSON       string `json:"b64_json"`
			URL           string `json:"url,omitempty"`
			RevisedPrompt string `json:"revised_prompt,omitempty"`
		}{
			{B64JSON: base64.StdEncoding.EncodeToString(png), RevisedPrompt: "revised"},
		}})
	}))
	defer server.Close()

	provider := NewCustomProvider("custom", "test-key", "deepseek-v4-pro", server.URL)
	result, err := provider.GenerateImage("robot terminal", ImageGenerationOptions{Model: "gpt-image-2", Size: "1024x1024"})
	require.NoError(t, err)
	assert.Equal(t, png, result.Data)
	assert.Equal(t, "gpt-image-2", result.Model)
	assert.Equal(t, "revised", result.RevisedPrompt)
	assert.Equal(t, ".png", result.Extension)
}

func TestCustomProviderGenerateImageEmptyPrompt(t *testing.T) {
	provider := NewCustomProvider("custom", "test-key", "gpt-image-2", "http://example.test")
	_, err := provider.GenerateImage(" ", ImageGenerationOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prompt gambar kosong")
}
