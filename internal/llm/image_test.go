package llm

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
		_ = json.NewEncoder(w).Encode(openAIImageResponse{Data: []openAIImageDataItem{
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

func TestCustomProviderGenerateImageProviderErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "stream error: stream ID 1; INTERNAL_ERROR; received from peer",
				"type":    "server_error",
				"code":    "internal_server_error",
			},
		})
	}))
	defer server.Close()

	provider := NewCustomProvider("custom", "test-key", "gpt-image-2", server.URL)
	_, err := provider.GenerateImage("robot terminal", ImageGenerationOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image provider error")
	assert.Contains(t, err.Error(), "stream error")
	assert.Contains(t, err.Error(), "type=server_error")
	assert.Contains(t, err.Error(), "code=internal_server_error")
	assert.NotContains(t, err.Error(), "image response kosong")
}

func TestCustomProviderGenerateImageEmptyPrompt(t *testing.T) {
	provider := NewCustomProvider("custom", "test-key", "gpt-image-2", "http://example.test")
	_, err := provider.GenerateImage(" ", ImageGenerationOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prompt gambar kosong")
}

func TestCustomProviderEditImage(t *testing.T) {
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52}
	input := t.TempDir() + "/input.png"
	require.NoError(t, os.WriteFile(input, png, 0o644))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/images/edits", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		require.NoError(t, r.ParseMultipartForm(10<<20))
		assert.Equal(t, "gpt-image-2", r.FormValue("model"))
		assert.Equal(t, "make it cyberpunk", r.FormValue("prompt"))
		assert.Equal(t, "b64_json", r.FormValue("response_format"))
		assert.Equal(t, "1024x1024", r.FormValue("size"))
		file, _, err := r.FormFile("image")
		require.NoError(t, err)
		defer file.Close()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openAIImageResponse{Data: []openAIImageDataItem{
			{B64JSON: base64.StdEncoding.EncodeToString(png), RevisedPrompt: "edited"},
		}})
	}))
	defer server.Close()

	provider := NewCustomProvider("custom", "test-key", "deepseek-v4-pro", server.URL)
	result, err := provider.EditImage(input, "make it cyberpunk", ImageEditOptions{Model: "gpt-image-2", Size: "1024x1024"})
	require.NoError(t, err)
	assert.Equal(t, png, result.Data)
	assert.Equal(t, "gpt-image-2", result.Model)
	assert.Equal(t, "edited", result.RevisedPrompt)
	assert.Equal(t, ".png", result.Extension)
}
