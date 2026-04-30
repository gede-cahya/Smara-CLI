package components

import (
	"strings"
	"testing"
)

func TestNewNodeGraphDefaults(t *testing.T) {
	ng := NewNodeGraph()
	if len(ng.nodes) != 5 {
		t.Fatalf("expected 5 default nodes, got %d", len(ng.nodes))
	}
	for _, role := range []string{"orchestrator", "frontend", "backend", "database", "qa"} {
		if _, ok := ng.nodes[role]; !ok {
			t.Fatalf("expected node %q not found", role)
		}
	}
}

func TestNodeGraphRender(t *testing.T) {
	ng := NewNodeGraph()
	ng.SetSize(120, 40)
	out := stripANSI(ng.View())

	// Should contain title
	if !strings.Contains(out, "NODE GRAPH") {
		t.Error("expected 'NODE GRAPH' in title")
	}

	// Should contain all nodes (Orchestrator truncated to 10 runes)
	for _, label := range []string{"Orchestra", "Frontend", "Backend", "Database", "QA"} {
		if !strings.Contains(out, label) {
			t.Errorf("expected node label %q in output", label)
		}
	}

	// Should contain footer hints
	if !strings.Contains(out, "[Tab]") {
		t.Error("expected footer hint [Tab] in output")
	}

	// Popup should not be visible by default
	if ng.IsPopupOpen() {
		t.Error("popup should be closed by default")
	}
}

func TestNodeGraphNavigation(t *testing.T) {
	ng := NewNodeGraph()
	ng.SetSize(120, 40)

	initFocus := ng.FocusedRole()
	ng.FocusNext()
	if ng.FocusedRole() == initFocus {
		t.Error("focus should have moved after FocusNext")
	}
	ng.FocusPrev()
	if ng.FocusedRole() != initFocus {
		t.Error("focus should have returned after FocusPrev")
	}
}

func TestNodeGraphPopup(t *testing.T) {
	ng := NewNodeGraph()
	ng.SetSize(120, 40)

	ng.TogglePopup()
	if !ng.IsPopupOpen() {
		t.Error("popup should be open after TogglePopup")
	}
	ng.ClosePopup()
	if ng.IsPopupOpen() {
		t.Error("popup should be closed after ClosePopup")
	}
}

func TestNodeGraphUpdateAgent(t *testing.T) {
	ng := NewNodeGraph()
	ng.SetSize(120, 40)

	ng.UpdateAgent("backend-2", "backend", StatusWorking, "Writing API handlers")
	ng.SetNodeProgress("backend", 0.75)

	node := ng.nodes["backend"]
	if node.Status != StatusWorking {
		t.Errorf("expected status work, got %s", node.Status)
	}
	if node.Progress != 0.75 {
		t.Errorf("expected progress 0.75, got %f", node.Progress)
	}
}

func TestRenderProgressBar(t *testing.T) {
	tests := []struct {
		p      float64
		w      int
		expect string
	}{
		{0, 10, "░░░░░░░░░░"},
		{1, 10, "▓▓▓▓▓▓▓▓▓▓"},
		{0.5, 10, "▓▓▓▓▓░░░░░"},
	}
	for _, tc := range tests {
		got := renderProgressBar(tc.p, tc.w)
		if got != tc.expect {
			t.Errorf("progressBar(%v,%d) = %q, want %q", tc.p, tc.w, got, tc.expect)
		}
	}
}

func TestAvatarForRole(t *testing.T) {
	if avatarForRole("orchestrator") != "🧠" {
		t.Error("expected 🧠 for orchestrator")
	}
	if avatarForRole("unknown") != "🤖" {
		t.Error("expected 🤖 for unknown role")
	}
}

func TestStatusIndicator(t *testing.T) {
	if statusIndicator(StatusDone) != "✓" {
		t.Error("expected ✓ for done status")
	}
	if statusIndicator(StatusError) != "✕" {
		t.Error("expected ✕ for error status")
	}
}

func TestNodeGraphSmallTerminal(t *testing.T) {
	ng := NewNodeGraph()
	ng.SetSize(30, 8)
	out := ng.View()
	if !strings.Contains(out, "too small") {
		t.Error("expected 'too small' message for tiny terminal")
	}
}

func TestNodeGraphRenderDebug(t *testing.T) {
	ng := NewNodeGraph()
	ng.SetSize(120, 40)
	out := ng.View()
	// Strip ANSI codes for easier debugging
	clean := stripANSI(out)
	lines := strings.Split(clean, "\n")
	t.Logf("Total lines: %d", len(lines))
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			t.Logf("line %d: %q", i, trimmed[:min(len(trimmed),80)])
		}
	}
	for _, label := range []string{"Orchestra", "Frontend", "Backend", "Database", "QA"} {
		if !strings.Contains(clean, label) {
			t.Errorf("expected label %q not found", label)
		}
	}
}

func min(a, b int) int {
	if a < b { return a }
	return b
}

func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		if r == '\x1b' {
			inEscape = true
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
