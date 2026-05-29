package skill

import (
	"strings"
	"testing"
)

func TestAssessRiskLowByDefault(t *testing.T) {
	sk := &Skill{Name: "safe-skill", Description: "Safe skill", Steps: []Step{{Tool: "read_file", Args: map[string]interface{}{"path": "README.md"}}}}
	got := AssessRisk(sk)
	if got.Level != "low" {
		t.Fatalf("expected low risk, got %s", got.Level)
	}
	if got.RequiresApproval {
		t.Fatalf("low risk should not require approval")
	}
	if len(got.SimulationSummary) != 1 || !strings.Contains(got.SimulationSummary[0], "read_file") {
		t.Fatalf("unexpected dry-run summary: %#v", got.SimulationSummary)
	}
}

func TestAssessRiskHighForSystemWrite(t *testing.T) {
	sk := &Skill{Name: "write-skill", Steps: []Step{{Tool: "run_command", Args: map[string]interface{}{"command": "systemctl restart app"}}}}
	got := AssessRisk(sk)
	if got.Level != "high" {
		t.Fatalf("expected high risk, got %s reasons=%v", got.Level, got.Reasons)
	}
	if !got.RequiresApproval {
		t.Fatalf("high risk should require approval")
	}
	if !containsString(got.Categories, "system-write") {
		t.Fatalf("expected system-write category, got %v", got.Categories)
	}
}

func TestAssessRiskCriticalForDestructiveCommand(t *testing.T) {
	sk := &Skill{Name: "danger-skill", Steps: []Step{{Tool: "run_command", Args: map[string]interface{}{"command": "rm -rf /tmp/demo"}}}}
	got := AssessRisk(sk)
	if got.Level != "critical" {
		t.Fatalf("expected critical risk, got %s reasons=%v", got.Level, got.Reasons)
	}
	if !got.RequiresApproval {
		t.Fatalf("critical risk should require approval")
	}
	if !containsString(got.Categories, "destructive") {
		t.Fatalf("expected destructive category, got %v", got.Categories)
	}
}

func TestAssessRiskDeclaredMetadataCanRequireApproval(t *testing.T) {
	sk := &Skill{
		Name: "declared-risk",
		Risk: &RiskMetadata{Level: "medium", Categories: []string{"remote"}, Reasons: []string{"user declared"}, RequiresApproval: true},
		Steps: []Step{{Tool: "read_file", Args: map[string]interface{}{"path": "README.md"}}},
	}
	got := AssessRisk(sk)
	if got.Level != "medium" {
		t.Fatalf("expected declared medium risk, got %s", got.Level)
	}
	if !got.RequiresApproval {
		t.Fatalf("declared requires_approval should be honored")
	}
	if !containsString(got.Categories, "remote") || !containsString(got.Reasons, "user declared") {
		t.Fatalf("declared metadata missing: cats=%v reasons=%v", got.Categories, got.Reasons)
	}
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
