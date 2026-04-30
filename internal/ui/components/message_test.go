package components

import (
	"strings"
	"testing"
	"time"
)

func TestRenderMessage_CollapsedThinking(t *testing.T) {
	r := NewMessageRenderer(80)

	thinking := "User mengatakan...\nMari saya cari...\nIni adalah reasoning panjang dari LLM."
	rendered := r.RenderMessage(
		"Agent", "Halo!", thinking,
		nil, nil,
		0, 0, time.Second,
		"ask", "test-model", false,
	)

	// Should contain collapsed indicator, NOT raw thinking content
	if !strings.Contains(rendered, "▼") {
		t.Errorf("Expected collapsed thinking indicator ▼, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "Mari saya cari") {
		t.Errorf("Thinking content should be hidden when collapsed, got:\n%s", rendered)
	}
}

func TestRenderMessage_NoThinking(t *testing.T) {
	r := NewMessageRenderer(80)

	rendered := r.RenderMessage(
		"Agent", "Halo!", "",
		nil, nil,
		0, 0, 0,
		"ask", "", false,
	)

	if strings.Contains(rendered, "Thinking") {
		t.Errorf("Should not contain thinking block when thinking is empty, got:\n%s", rendered)
	}
}
