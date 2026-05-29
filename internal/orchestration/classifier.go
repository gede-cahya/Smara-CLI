package orchestration

import (
	"strings"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
)

// ShouldAutoParallelOrchestrate classifies whether a user prompt is substantial
// multi-step work that should be routed through the parallel workflow planner.
func ShouldAutoParallelOrchestrate(prompt string, mode agent.Mode) bool {
	text := strings.TrimSpace(prompt)
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	if hasParallelOptOut(lower) {
		return false
	}
	if IsAgentSwarmWorkflowPrompt(lower) {
		return true
	}
	if isQuestionOrFollowUp(lower) {
		return false
	}
	if strings.Contains(lower, "custom workflow") || strings.Contains(lower, "workflow custom") || strings.Contains(lower, "workflow") || strings.Contains(lower, "-agent") {
		// Custom workflows have their own explicit runner/router. Do not send
		// custom-workflow requests or feature wishes into the generic auto planner,
		// otherwise a saved custom workflow may be ignored when the user asks for
		// parallel orchestration.
		return false
	}
	words := strings.Fields(lower)
	if isSimpleChat(lower, len(words)) {
		return false
	}
	if hasParallelSignal(lower) {
		return hasExplicitWorkIntent(lower)
	}
	if mode == agent.ModeWorkflow && hasExplicitWorkIntent(lower) && looksComplexEnoughForWorkflowMode(lower, len(words)) {
		return true
	}
	strongPhrases := []string{
		"buatkan aplikasi", "buat aplikasi", "implementasikan", "tambahkan fitur", "perbaiki bug", "debug", "deploy", "release", "audit", "refactor", "migrasi", "setup", "install", "build dan test", "test dan build", "cek dan perbaiki", "analisis repo", "update docs", "sinkronkan docs",
	}
	score := 0
	for _, p := range strongPhrases {
		if strings.Contains(lower, p) {
			score += 2
		}
	}
	verbs := []string{"buat", "bikin", "tambah", "ubah", "perbaiki", "fix", "debug", "test", "build", "deploy", "release", "audit", "refactor", "update", "install", "setup", "cek", "verifikasi"}
	for _, v := range verbs {
		if strings.Contains(lower, v) {
			score++
		}
	}
	connectors := []string{" dan ", " lalu ", " kemudian ", ",", ";", " serta ", " sekaligus "}
	for _, c := range connectors {
		if strings.Contains(lower, c) {
			score++
		}
	}
	return score >= 4 && (len(words) >= 14 || strings.Contains(lower, "repo") || strings.Contains(lower, "project") || strings.Contains(lower, "vps") || strings.Contains(lower, "server"))
}

func hasParallelOptOut(lower string) bool {
	optOuts := []string{"jangan parallel", "jangan paralel", "tanpa orchestration", "tanpa orkestrasi", "jawab singkat"}
	for _, optOut := range optOuts {
		if strings.Contains(lower, optOut) {
			return true
		}
	}
	return false
}

func isQuestionOrFollowUp(lower string) bool {
	terms := []string{"?", "tadi saya", "kok", "kenapa", "mengapa", "gimana", "bagaimana", "apakah", "belum ada", "sudah jalan", "idle"}
	for _, term := range terms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func isSimpleChat(lower string, wordCount int) bool {
	if wordCount >= 10 {
		return false
	}
	greetings := map[string]bool{"halo": true, "hallo": true, "hello": true, "hi": true, "hai": true, "pagi": true, "siang": true, "sore": true, "malam": true, "thanks": true, "terima kasih": true}
	return greetings[strings.TrimSpace(lower)] || !hasExplicitWorkIntent(lower)
}

func hasParallelSignal(lower string) bool {
	terms := []string{"parallel orchestration", "parallel orchestrasion", "paralel orchestration", "orkestrasi paralel", "secara parallel", "secara paralel", " in parallel"}
	for _, term := range terms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func looksComplexEnoughForWorkflowMode(lower string, wordCount int) bool {
	if wordCount >= 10 {
		return true
	}
	markers := []string{"repo", "project", "vps", "server", "build", "test", "release", "deploy", "audit", "refactor", "workflow"}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func hasExplicitWorkIntent(lower string) bool {
	actions := []string{
		"jalankan", "run", "execute", "mulai", "start", "kerjakan", "implementasikan", "buat", "buatkan", "bikin",
		"tambah", "tambahkan", "ubah", "perbaiki", "fix", "debug", "test", "build", "deploy", "release", "audit", "refactor",
		"update", "install", "setup", "cek", "verifikasi", "sinkronkan", "generate",
	}
	for _, action := range actions {
		if strings.Contains(lower, action) {
			return true
		}
	}
	return false
}

// IsAgentSwarmWorkflowPrompt detects explicit requests to run Smara's Agent Swarm
// Workflow: automatic task decomposition, agent spawning, parallel wave execution,
// result merge, and QA. It intentionally requires an action verb so questions like
// "apakah agent swarm bisa...?" stay in normal chat.
func IsAgentSwarmWorkflowPrompt(prompt string) bool {
	lower := strings.ToLower(strings.TrimSpace(prompt))
	if lower == "" {
		return false
	}
	hasSwarm := strings.Contains(lower, "agent swarm") || strings.Contains(lower, "swarm workflow") || strings.Contains(lower, "multi agent") || strings.Contains(lower, "multi-agent") || strings.Contains(lower, "agentic agent")
	if !hasSwarm {
		return false
	}
	actions := []string{"jalankan", "run", "execute", "mulai", "start", "spawn", "pecah", "breakdown", "kerjakan", "implementasikan"}
	for _, action := range actions {
		if strings.Contains(lower, action) {
			return true
		}
	}
	return false
}
