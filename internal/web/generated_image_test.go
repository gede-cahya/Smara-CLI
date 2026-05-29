package web

import (
	"errors"
	"strings"
	"testing"

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
