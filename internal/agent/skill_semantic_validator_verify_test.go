package agent

import (
	"strings"
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

// TestPlanningTemplateEnumGuard proves the refiner-side validator rejects the
// exact bad 'kind' values that caused the 9router skill to fail 10/10 runs,
// and accepts the valid one we settled on.
func TestPlanningTemplateEnumGuard(t *testing.T) {
	bad := []string{"9router-entrypoint", "general", "coding"}
	for _, k := range bad {
		err := validateStepsAgainstBuiltins([]skill.Step{
			{Tool: "planning_template", Args: map[string]interface{}{"kind": k, "goal": "x"}},
		})
		if err == nil {
			t.Fatalf("expected rejection for kind=%q, got nil", k)
		}
		if !strings.Contains(err.Error(), "tidak valid") {
			t.Fatalf("kind=%q: unexpected error: %v", k, err)
		}
	}

	if err := validateStepsAgainstBuiltins([]skill.Step{
		{Tool: "planning_template", Args: map[string]interface{}{"kind": "implementation-plan", "goal": "x"}},
	}); err != nil {
		t.Fatalf("valid kind rejected: %v", err)
	}

	// Missing required 'goal' must also be caught.
	if err := validateStepsAgainstBuiltins([]skill.Step{
		{Tool: "planning_template", Args: map[string]interface{}{"kind": "implementation-plan"}},
	}); err == nil {
		t.Fatal("expected rejection for missing required 'goal'")
	}
}
