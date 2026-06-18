package skill

import (
	"database/sql"
	"fmt"
	"reflect"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/llm"
)

// RefinerConfig controls auto-refinement thresholds.
type RefinerConfig struct {
	SuccessThreshold float64 // e.g. 0.70
	MinRuns          int     // e.g. 3
	AutoApply        bool    // default false (manual approval)
}

// DefaultRefinerConfig returns sensible defaults.
func DefaultRefinerConfig() RefinerConfig {
	return RefinerConfig{SuccessThreshold: 0.70, MinRuns: 3, AutoApply: false}
}

// ShouldRefine returns true if a skill needs refinement based on recent runs.
func ShouldRefine(name string, tracker *ExecutionTracker, cfg RefinerConfig) (bool, float64, error) {
	total, success, _, _, err := tracker.GetStats(name)
	if err != nil {
		return false, 0, err
	}
	if total < cfg.MinRuns {
		return false, 0, nil
	}
	rate := float64(success) / float64(total)
	return rate < cfg.SuccessThreshold, rate, nil
}

// BuildRefinementPromptFull creates a system prompt for LLM skill refinement
// using execution history and feedback.
func BuildRefinementPromptFull(name string, tracker *ExecutionTracker, db *sql.DB) (string, *Skill, error) {
	sk, err := Load(name)
	if err != nil {
		return "", nil, err
	}
	history, err := tracker.GetTimeline(name, 10)
	if err != nil {
		history = nil
	}
	feedback, _ := GetFeedback(db, name, 5)

	jsonBytes, _ := sk.ToJSON()
	prompt := fmt.Sprintf("Kamu adalah Skill Refiner untuk Smara.\n\nSkill '%s' (v%d):\n%s\n\n", sk.Name, sk.Version, string(jsonBytes))

	if len(history) > 0 {
		prompt += "Execution history (last 10):\n"
		for _, h := range history {
			status := "SUCCESS"
			if !h.Success {
				status = "FAIL"
			}
			prompt += fmt.Sprintf("- %s: %s (%d ms)\n", h.StartedAt.Format("2006-01-02 15:04"), status, h.DurationMs)
			if h.ErrorMessage != "" {
				prompt += fmt.Sprintf("  Error: %s\n", h.ErrorMessage)
			}
		}
	}
	if len(feedback) > 0 {
		prompt += "\nUser feedback:\n"
		for _, f := range feedback {
			prompt += fmt.Sprintf("- %s: notes=%q\n", f.CreatedAt.Format("2006-01-02"), f.Notes)
		}
	}
	prompt += "\nBuat versi baru skill JSON yang lebih baik. Output HANYA JSON skill, tanpa penjelasan."
	return prompt, sk, nil
}

// RefineSkill sends a refinement prompt to the LLM and returns the proposed JSON.
func RefineSkill(name string, tracker *ExecutionTracker, db *sql.DB, provider llm.Provider) (string, *Skill, error) {
	prompt, sk, err := BuildRefinementPromptFull(name, tracker, db)
	if err != nil {
		return "", nil, err
	}

	msgs := []llm.Message{
		{Role: "system", Content: "You are a skill refiner. Output only valid JSON matching the Skill schema."},
		{Role: "user", Content: prompt},
	}
	resp, err := provider.Chat(msgs)
	if err != nil {
		return "", nil, fmt.Errorf("refinement LLM call failed: %w", err)
	}
	return resp.Content, sk, nil
}

// StepSemanticValidator, when set, validates that step arguments are coherent
// (e.g. enum-constrained args point at known values). The agent package wires
// this up at init so the refiner can reject proposals that would fail at run
// time. It stays nil-safe for callers that don't need semantic checks.
var StepSemanticValidator func(steps []Step) error

// stepsEqual reports whether two step slices are semantically identical so the
// refiner can skip proposals that don't actually change behaviour.
func stepsEqual(a, b []Step) bool {
	return reflect.DeepEqual(a, b)
}

// AutoApplyRefinement checks predicted improvement and applies if over threshold or manual.
func AutoApplyRefinement(proposedJSON string, sk *Skill, tracker *ExecutionTracker, cfg RefinerConfig) (*Skill, error) {
	newSkill, err := FromJSON([]byte(proposedJSON))
	if err != nil {
		return nil, fmt.Errorf("invalid proposed skill JSON: %w", err)
	}
	newSkill.Name = sk.Name
	newSkill.Version = sk.Version + 1
	if err := newSkill.Validate(); err != nil {
		return nil, fmt.Errorf("invalid refined skill: %w", err)
	}
	if StepSemanticValidator != nil {
		if err := StepSemanticValidator(newSkill.Steps); err != nil {
			return nil, fmt.Errorf("refined skill rejected: %w", err)
		}
	}
	if stepsEqual(sk.Steps, newSkill.Steps) {
		return nil, fmt.Errorf("refined skill identical to v%d, skipping", sk.Version)
	}
	// Preserve ancestry so the Hierarchy view can render the refine chain.
	AttachLineage(newSkill, sk, "auto")
	if !cfg.AutoApply {
		if tracker != nil {
			_ = tracker.RecordImprovement(SkillImprovement{
				SkillName:     sk.Name,
				Version:       newSkill.Version,
				TriggeredAt:   time.Now(),
				Trigger:       "auto-refine",
				ChangeSummary: "Refinement proposal generated and waiting for review.",
				Applied:       false,
				ProposedJSON:  proposedJSON,
			})
		}
		return newSkill, nil
	}
	if err := Save(newSkill, nil); err != nil {
		return nil, fmt.Errorf("failed to save refined skill: %w", err)
	}
	if tracker != nil {
		_ = tracker.RecordImprovement(SkillImprovement{
			SkillName:     sk.Name,
			Version:       newSkill.Version,
			TriggeredAt:   time.Now(),
			Trigger:       "auto-refine",
			ChangeSummary: "Automatically refined after repeated execution failures.",
			Applied:       true,
			ProposedJSON:  proposedJSON,
		})
	}
	return newSkill, nil
}
