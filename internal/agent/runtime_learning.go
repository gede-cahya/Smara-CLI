package agent

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

// maxAutoRefineAttempts caps how many times a single skill may be auto-refined
// while still below the success threshold. Past this, repeated failures share
// the same root cause and the LLM is guessing rather than fixing, so the skill
// is frozen for manual review instead of churning out more broken versions.
const maxAutoRefineAttempts = 2

var autoRefineRuntime = struct {
	sync.Mutex
	inFlight map[string]bool
}{inFlight: make(map[string]bool)}

func (s *Supervisor) captureSelfImprovement(userPrompt string) {
	lesson, ok := detectSelfImprovementCorrection(userPrompt)
	if !ok || BuiltinDB == nil {
		return
	}
	_, err := SaveSelfImprovementMemory(BuiltinDB, SelfImprovementMemory{
		Type:       "user-correction",
		Scope:      "agent",
		Summary:    lesson,
		Lesson:     lesson,
		AppliesTo:  []string{"chat", "workflow", "skill"},
		Confidence: 0.95,
		AutoApply:  true,
	})
	if err != nil {
		log.Printf("[self-improvement] failed to save correction: %v", err)
	}
}

func (s *Supervisor) logSkillRunAndMaybeRefine(name string, result *skill.RunResult, start time.Time) {
	if BuiltinDB == nil || result == nil {
		return
	}
	tracker, err := skill.NewExecutionTracker(BuiltinDB)
	if err != nil {
		log.Printf("[auto-refine] tracker init failed: %v", err)
		return
	}
	versionID := ""
	if sk, err := skill.Load(name); err == nil {
		versionID = fmt.Sprintf("v%d", sk.Version)
	}
	if err := tracker.LogRunWithMetadata(
		name,
		versionID,
		true,
		fmt.Sprintf("agent-%d", start.UnixNano()),
		"agent",
		"",
		string(s.mode),
		result,
		start,
	); err != nil {
		log.Printf("[auto-refine] failed to log skill run: %v", err)
		return
	}
	if !config.Get().AutoSkillRefine {
		return
	}
	go s.maybeAutoRefineSkill(name)
}

func (s *Supervisor) maybeAutoRefineSkill(name string) {
	if BuiltinDB == nil || s.provider == nil {
		return
	}
	autoRefineRuntime.Lock()
	if autoRefineRuntime.inFlight[name] {
		autoRefineRuntime.Unlock()
		return
	}
	autoRefineRuntime.inFlight[name] = true
	autoRefineRuntime.Unlock()
	defer func() {
		autoRefineRuntime.Lock()
		delete(autoRefineRuntime.inFlight, name)
		autoRefineRuntime.Unlock()
	}()

	tracker, err := skill.NewExecutionTracker(BuiltinDB)
	if err != nil {
		return
	}
	cfg := skill.DefaultRefinerConfig()
	cfg.AutoApply = config.Get().AutoSkillRefineApply
	should, rate, err := skill.ShouldRefine(name, tracker, cfg)
	if err != nil || !should {
		return
	}
	improvements, _ := tracker.GetImprovements(name, maxAutoRefineAttempts)
	if len(improvements) > 0 && time.Since(improvements[0].TriggeredAt) < 24*time.Hour {
		return
	}
	// Stop-gate: if we've already auto-refined this skill maxAutoRefineAttempts
	// times and the success rate is still below threshold, stop regenerating
	// proposals. Repeated failures with the same root cause mean the LLM is
	// guessing, not fixing; freeze and surface it for human review instead.
	autoCount := 0
	for _, imp := range improvements {
		if imp.Trigger == "auto-refine" {
			autoCount++
		}
	}
	if autoCount >= maxAutoRefineAttempts {
		log.Printf("[auto-refine] skill %s frozen after %d attempts (rate %.0f%%); needs manual review", name, autoCount, rate*100)
		return
	}
	proposal, current, err := skill.RefineSkill(name, tracker, BuiltinDB, s.provider)
	if err != nil {
		log.Printf("[auto-refine] failed to generate proposal for %s: %v", name, err)
		return
	}
	refined, err := skill.AutoApplyRefinement(proposal, current, tracker, cfg)
	if err != nil {
		log.Printf("[auto-refine] failed for %s: %v", name, err)
		return
	}
	action := "proposal stored"
	if cfg.AutoApply {
		action = fmt.Sprintf("applied v%d", refined.Version)
	}
	_, _ = SaveSelfImprovementMemory(BuiltinDB, SelfImprovementMemory{
		Type:       "skill-auto-refine",
		Scope:      "skill:" + name,
		Summary:    fmt.Sprintf("Skill %s auto-refine %s after success rate %.0f%%.", name, action, rate*100),
		Lesson:     fmt.Sprintf("Prioritaskan skill %s versi terbaru dan pantau hasil eksekusinya.", name),
		AppliesTo:  []string{"skill", name},
		Confidence: 0.9,
		AutoApply:  true,
	})
	log.Printf("[auto-refine] skill %s: %s", name, action)
}
