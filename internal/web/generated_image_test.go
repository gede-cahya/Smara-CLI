package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/imageflow"
	"github.com/stretchr/testify/require"
)

func TestRewriteGeneratedImageMarkdownRewritesAnyGeneratedImageMarkdown(t *testing.T) {
	input := strings.Join([]string{
		"Gambar berhasil dibuat.",
		"Path: /home/cahya/.smara/images/smara-image.png",
		"![generated image](/home/cahya/.smara/images/smara-image.png)",
	}, "\n")

	got := rewriteGeneratedImageMarkdown(input, func(path string) (string, error) {
		require.Equal(t, "/home/cahya/.smara/images/smara-image.png", path)
		return "/api/generated-image?path=%2Fhome%2Fcahya%2F.smara%2Fimages%2Fsmara-image.png", nil
	})

	require.Contains(t, got, "![generated image](/api/generated-image?path=%2Fhome%2Fcahya%2F.smara%2Fimages%2Fsmara-image.png)")
}

func TestRewriteGeneratedImageMarkdownStillSupportsMarkdownPrefix(t *testing.T) {
	input := "Markdown: ![generated image](/tmp/smara-image.png)"

	got := rewriteGeneratedImageMarkdown(input, func(path string) (string, error) {
		require.Equal(t, "/tmp/smara-image.png", path)
		return "/api/generated-image?path=%2Ftmp%2Fsmara-image.png", nil
	})

	require.Equal(t, "Markdown: ![generated image](/api/generated-image?path=%2Ftmp%2Fsmara-image.png)", got)
}

func TestRewriteGeneratedImageMarkdownLeavesDisallowedPathUntouched(t *testing.T) {
	input := "![external image](/etc/passwd)"

	got := rewriteGeneratedImageMarkdown(input, func(path string) (string, error) {
		return "", errors.New("not allowed")
	})

	require.Equal(t, input, got)
}

func TestImageFlowAssetsSkipsStaleDisallowedAssets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	imgDir := filepath.Join(home, ".smara", "images")
	require.NoError(t, os.MkdirAll(imgDir, 0o755))
	validPath := filepath.Join(imgDir, "valid.png")
	require.NoError(t, os.WriteFile(validPath, []byte("png"), 0o644))

	cfg := &config.SmaraConfig{ImageOutputDir: imgDir}
	srv := &Server{Cfg: cfg}
	staleTempPath := filepath.Join(os.TempDir(), "TestCleanupAssetsRemovesOldArchivedOnly", "001", ".smara", "images", "new.png")
	require.NoError(t, imageflow.SaveAsset(imageflow.Asset{ID: "stale", Path: staleTempPath, Workflow: "stale test"}))
	require.NoError(t, imageflow.SaveAsset(imageflow.Asset{ID: "valid", Path: validPath, Workflow: "valid image"}))

	req := httptest.NewRequest(http.MethodGet, "/api/image-flow/assets", nil)
	res := httptest.NewRecorder()
	srv.handleImageFlowAssets(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	require.Contains(t, res.Body.String(), "valid")
	require.NotContains(t, res.Body.String(), "stale")
	require.NotContains(t, res.Body.String(), "TestCleanupAssetsRemovesOldArchivedOnly")
}
