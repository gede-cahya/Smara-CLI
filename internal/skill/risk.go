package skill

import (
	"fmt"
	"sort"
	"strings"
)

// RiskMetadata describes the declared security/operational risk of running a skill.
type RiskMetadata struct {
	Level            string   `json:"level"` // low, medium, high, critical
	Categories       []string `json:"categories,omitempty"`
	Reasons          []string `json:"reasons,omitempty"`
	RequiresApproval bool     `json:"requires_approval,omitempty"`
}

// RiskAssessment is the computed run-time risk review for a skill.
type RiskAssessment struct {
	SkillName         string   `json:"skill_name"`
	Level             string   `json:"level"`
	Categories        []string `json:"categories,omitempty"`
	Reasons           []string `json:"reasons,omitempty"`
	RequiresApproval  bool     `json:"requires_approval"`
	StepCount         int      `json:"step_count"`
	SimulationSummary []string `json:"simulation_summary,omitempty"`
}

var riskRank = map[string]int{"low": 0, "medium": 1, "high": 2, "critical": 3}

// AssessRisk returns declared risk metadata augmented by heuristic classification.
func AssessRisk(sk *Skill) RiskAssessment {
	if sk == nil {
		return RiskAssessment{Level: "low"}
	}
	level := "low"
	cats := map[string]bool{}
	reasons := map[string]bool{}
	if sk.Risk != nil {
		if normalized := normalizeRiskLevel(sk.Risk.Level); normalized != "" {
			level = normalized
		}
		for _, c := range sk.Risk.Categories {
			if strings.TrimSpace(c) != "" {
				cats[strings.TrimSpace(c)] = true
			}
		}
		for _, r := range sk.Risk.Reasons {
			if strings.TrimSpace(r) != "" {
				reasons[strings.TrimSpace(r)] = true
			}
		}
	}
	for i, st := range sk.Steps {
		stLevel, stCats, stReasons := classifyStepRisk(i+1, st)
		if riskRank[stLevel] > riskRank[level] {
			level = stLevel
		}
		for _, c := range stCats {
			cats[c] = true
		}
		for _, r := range stReasons {
			reasons[r] = true
		}
	}
	requires := riskRank[level] >= riskRank["high"]
	if sk.Risk != nil && sk.Risk.RequiresApproval {
		requires = true
	}
	if sk.AutoSkill != nil && sk.AutoSkill.Enabled && !sk.AutoSkill.ApprovalRequired {
		requires = false
	}
	return RiskAssessment{
		SkillName:         sk.Name,
		Level:             level,
		Categories:        sortedKeys(cats),
		Reasons:           sortedKeys(reasons),
		RequiresApproval:  requires,
		StepCount:         len(sk.Steps),
		SimulationSummary: BuildDryRunSummary(sk),
	}
}

// BuildDryRunSummary summarizes what would be executed without running tools.
func BuildDryRunSummary(sk *Skill) []string {
	if sk == nil {
		return nil
	}
	out := make([]string, 0, len(sk.Steps))
	for i, st := range sk.Steps {
		out = append(out, fmt.Sprintf("step %d: %s args=%s", i+1, st.Tool, summarizeArgs(st.Args)))
	}
	return out
}

func ApprovalRequired(sk *Skill) bool { return AssessRisk(sk).RequiresApproval }

func normalizeRiskLevel(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "low", "medium", "high", "critical":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return ""
	}
}

func classifyStepRisk(idx int, st Step) (string, []string, []string) {
	tool := strings.ToLower(st.Tool)
	blob := tool + " " + strings.ToLower(fmt.Sprintf("%v", st.Args))
	level := "low"
	var cats, reasons []string
	add := func(l, c, r string) {
		if riskRank[l] > riskRank[level] {
			level = l
		}
		cats = append(cats, c)
		reasons = append(reasons, fmt.Sprintf("step %d: %s", idx, r))
	}
	if containsAny(blob, "rm -rf", "mkfs", "dd if=", "drop database", "truncate table", "delete from", "shutdown", "reboot") {
		add("critical", "destructive", "potentially destructive command/query")
	}
	if containsAny(blob, "delete", "remove", "destroy", "wipe", "purge") {
		add("high", "mutating", "delete/remove/destroy operation")
	}
	if containsAny(blob, "write_file", "edit_file", "run_command", "ssh_exec", "ssh_manage", "rsync", "scp", "docker compose up", "systemctl", "pm2", "deploy") {
		add("high", "system-write", "tool or command can modify local/remote system state")
	}
	if containsAny(blob, "http://", "https://", "curl ", "wget ", "git clone", "npm install", "pip install", "apt install", "ssh") {
		add("medium", "network", "network or remote dependency access")
	}
	return level, cats, reasons
}

func summarizeArgs(args map[string]interface{}) string {
	if len(args) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := fmt.Sprintf("%v", args[k])
		if len(v) > 80 {
			v = v[:80] + "..."
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
