package agent

import (
	"strings"
	"unicode"

	"github.com/gede-cahya/Smara-CLI/internal/config"
)

// ShouldAutoOrchestrate is a lightweight, side-effect-free heuristic used before
// the normal supervisor loop to decide whether a prompt is a good candidate for
// safe parallel task orchestration.
func ShouldAutoOrchestrate(prompt string, cfg config.ParallelOrchestrationConfig) bool {
	if !cfg.Enabled {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(prompt))
	// Parallel orchestration must be opt-in. Do not auto-enable for normal
	// workflow requests unless the user explicitly asks for parallel/paralel mode.
	if !containsAny(text, []string{"parallel", "paralel", "secara paralel", "pakai paralel", "mode paralel", "parallel orchestration"}) {
		return false
	}
	if len(text) < minPromptLength(cfg.AutoThreshold) || wordCount(text) < minPromptWords(cfg.AutoThreshold) {
		return false
	}
	if containsAny(text, []string{"secara berurutan", "step by step", "satu per satu", "jangan paralel", "no parallel", "sequential"}) {
		return false
	}
	if looksLikeDirectCommand(text) {
		return false
	}
	if containsAny(text, []string{"hapus ", "rm -rf", "drop database", "format disk", "delete production", "destroy"}) {
		return false
	}

	return true
}

func minPromptLength(th string) int {
	if strings.EqualFold(th, "aggressive") {
		return 18
	}
	return 28
}
func minPromptWords(th string) int {
	if strings.EqualFold(th, "aggressive") {
		return 3
	}
	return 5
}
func countSignals(s string, needles []string) int {
	n := 0
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			n++
		}
	}
	return n
}
func containsAny(s string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
func looksLikeDirectCommand(s string) bool {
	fields := strings.Fields(s)
	if len(fields) == 0 || len(fields) > 5 {
		return false
	}
	cmd := fields[0]
	if strings.HasPrefix(cmd, "./") {
		return true
	}
	known := map[string]bool{"go": true, "npm": true, "pnpm": true, "yarn": true, "make": true, "git": true, "docker": true, "kubectl": true, "ssh": true, "curl": true}
	if !known[cmd] {
		return false
	}
	for _, r := range s {
		if unicode.IsPunct(r) && r != '-' && r != '.' && r != '/' {
			return false
		}
	}
	return true
}

func wordCount(s string) int {
	return len(strings.Fields(s))
}
