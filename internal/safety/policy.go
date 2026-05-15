package safety

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/config"
)

type RiskLevel string

type Decision string

const (
	RiskLow         RiskLevel = "low"
	RiskMedium      RiskLevel = "medium"
	RiskHigh        RiskLevel = "high"
	RiskDestructive RiskLevel = "destructive"

	DecisionAllow Decision = "allow"
	DecisionAsk   Decision = "ask"
	DecisionDeny  Decision = "deny"
)

type Policy struct {
	Version   int          `json:"version"`
	Rules     []PolicyRule `json:"rules"`
	UpdatedAt time.Time    `json:"updated_at"`
}

type PolicyRule struct {
	Tool      string     `json:"tool"`
	Action    ActionType `json:"action,omitempty"`
	Target    string     `json:"target,omitempty"`
	Risk      RiskLevel  `json:"risk"`
	Decision  Decision   `json:"decision"`
	Reason    string     `json:"reason,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type PolicyRequest struct {
	Tool   string
	Action ActionType
	Target string
}

type PolicyResult struct {
	Decision Decision   `json:"decision"`
	Risk     RiskLevel  `json:"risk"`
	Reason   string     `json:"reason,omitempty"`
	Rule     PolicyRule `json:"rule,omitempty"`
}

func DefaultPolicy() *Policy {
	return &Policy{Version: 1, Rules: []PolicyRule{}, UpdatedAt: time.Now()}
}

func LoadPolicy() (*Policy, error) {
	path := PolicyPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultPolicy(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("gagal baca policy: %w", err)
	}
	var p Policy
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("gagal parse policy: %w", err)
	}
	if p.Version == 0 {
		p.Version = 1
	}
	return &p, nil
}

func SavePolicy(p *Policy) error {
	if err := os.MkdirAll(filepath.Dir(PolicyPath()), 0755); err != nil {
		return err
	}
	p.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("gagal marshal policy: %w", err)
	}
	return os.WriteFile(PolicyPath(), data, 0644)
}

func PolicyPath() string {
	cfg := config.Get()
	return filepath.Join(filepath.Dir(cfg.DBPath), "policy.json")
}

func (p *Policy) UpsertRule(rule PolicyRule) {
	now := time.Now()
	rule.Tool = strings.TrimSpace(rule.Tool)
	rule.Target = strings.TrimSpace(rule.Target)
	if rule.Risk == "" {
		rule.Risk = RiskMedium
	}
	if rule.Decision == "" {
		rule.Decision = DecisionAsk
	}
	for i := range p.Rules {
		if p.Rules[i].Tool == rule.Tool && p.Rules[i].Action == rule.Action && p.Rules[i].Target == rule.Target {
			rule.CreatedAt = p.Rules[i].CreatedAt
			rule.UpdatedAt = now
			p.Rules[i] = rule
			return
		}
	}
	rule.CreatedAt = now
	rule.UpdatedAt = now
	p.Rules = append(p.Rules, rule)
}

func (p *Policy) Evaluate(req PolicyRequest) PolicyResult {
	if req.Action == "" {
		req.Action = actionForTool(req.Tool)
	}
	bestScore := -1
	var best *PolicyRule
	for i := range p.Rules {
		rule := &p.Rules[i]
		if rule.Tool != "*" && rule.Tool != req.Tool {
			continue
		}
		if rule.Action != "" && rule.Action != req.Action {
			continue
		}
		if rule.Target != "" && !strings.Contains(req.Target, rule.Target) {
			continue
		}
		score := 0
		if rule.Tool == req.Tool {
			score += 4
		}
		if rule.Action == req.Action {
			score += 2
		}
		if rule.Target != "" {
			score++
		}
		if score > bestScore {
			bestScore = score
			best = rule
		}
	}
	if best != nil {
		return PolicyResult{Decision: best.Decision, Risk: best.Risk, Reason: best.Reason, Rule: *best}
	}
	return PolicyResult{Decision: DecisionAllow, Risk: riskForAction(req.Action)}
}

func actionForTool(tool string) ActionType {
	if action, ok := toolActions[tool]; ok {
		return action
	}
	lower := strings.ToLower(tool)
	switch {
	case strings.Contains(lower, "delete"), strings.Contains(lower, "remove"):
		return ActionDelete
	case strings.Contains(lower, "write"), strings.Contains(lower, "edit"), strings.Contains(lower, "save"):
		return ActionWrite
	case strings.Contains(lower, "execute"), strings.Contains(lower, "run"), strings.Contains(lower, "deploy"):
		return ActionExecute
	default:
		return ActionRead
	}
}

func riskForAction(action ActionType) RiskLevel {
	switch action {
	case ActionRead:
		return RiskLow
	case ActionWrite:
		return RiskMedium
	case ActionExecute:
		return RiskHigh
	case ActionDelete:
		return RiskDestructive
	default:
		return RiskMedium
	}
}
