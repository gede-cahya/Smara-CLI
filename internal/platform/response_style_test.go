package platform

import (
	"strings"
	"testing"
)

func TestPolishMarkdown_PrettyPrintsJSONFences(t *testing.T) {
	got := polishMarkdown("```json\n{\"b\":2,\"a\":1}\n```")

	if !strings.Contains(got, "  \"b\": 2") || !strings.Contains(got, "  \"a\": 1") {
		t.Fatalf("expected pretty JSON, got:\n%s", got)
	}
}

func TestPolishMarkdown_CompactsBlankLinesOutsideCode(t *testing.T) {
	got := polishMarkdown("a\n\n\n\n```text\nx\n\n\ny\n```\n\n\n\nb")

	if strings.Contains(got, "a\n\n\n") {
		t.Fatalf("expected compact blank lines outside code, got:\n%s", got)
	}
	if !strings.Contains(got, "x\n\n\ny") {
		t.Fatalf("expected code block blank lines preserved, got:\n%s", got)
	}
}

func TestRenderTelegramResponse_UsesCompactHeader(t *testing.T) {
	got := RenderPlatformResponse("telegram", "Selesai.", "", 0, 0, 0, 0)

	if strings.Contains(got, "━━━━━━━━") {
		t.Fatalf("expected compact header without separator, got:\n%s", got)
	}
	if !strings.HasPrefix(got, "🌀 *Smara Response*\n\n") {
		t.Fatalf("expected compact response header, got:\n%s", got)
	}
}
