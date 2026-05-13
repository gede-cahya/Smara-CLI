package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// IterationBudget controls how many agentic iterations a prompt may use.
//
// Behavior — adaptive in three ways:
//  1. Per-mode base budget (ASK is small, WORKFLOW is large).
//  2. Stuck-loop detection: if the same tool call (function + args) is
//     repeated 3 times in a row OR appears 5 times in the trailing window,
//     the budget is forced to terminate early to avoid wasting tokens.
//  3. Progress extension: when the model is still calling new tools at
//     the end of the base budget, the budget extends in chunks toward a
//     hard ceiling. "Progress" = a tool fingerprint we have not seen
//     recently.
type IterationBudget struct {
	mode      Mode
	base      int  // initial budget
	hardCap   int  // absolute upper bound, never exceeded
	current   int  // dynamic budget that grows on progress
	override  bool // user explicitly set max via SetMaxIterations
	overrideN int

	// Recent tool-call fingerprints (rolling window).
	recent   []string
	repeatN  int      // how many tool calls back to keep
	lastFp   string   // most recent fingerprint
	repeats  int      // consecutive repeats of lastFp
}

// NewIterationBudget creates a budget with sensible defaults for the mode.
// userMax is the value from config / SetMaxIterations; pass 0 to use mode defaults.
func NewIterationBudget(mode Mode, userMax int) *IterationBudget {
	b := &IterationBudget{
		mode:    mode,
		repeatN: 8, // last 8 calls considered for repetition
	}

	// User override wins, but still gets stuck-loop & progress-extend logic
	// applied within that ceiling.
	if userMax > 0 {
		b.base = userMax
		b.hardCap = userMax
		b.current = userMax
		b.override = true
		b.overrideN = userMax
		return b
	}

	switch mode {
	case ModeAsk:
		// Q&A rarely chains tools; keep it tight.
		b.base, b.hardCap = 5, 15
	case ModeRush:
		// Direct execution: a few tool calls to act, then final.
		b.base, b.hardCap = 15, 40
	case ModePlan:
		// Plan first, execute tool calls only after approval.
		b.base, b.hardCap = 12, 30
	case ModeTest:
		b.base, b.hardCap = 10, 25
	case ModeWorkflow:
		// Multi-step: SSH chains, file edits, verifications.
		b.base, b.hardCap = 30, 80
	default:
		b.base, b.hardCap = 20, 50
	}
	b.current = b.base
	return b
}

// Limit returns the current iteration cap.
func (b *IterationBudget) Limit() int {
	return b.current
}

// HardCap returns the absolute upper bound.
func (b *IterationBudget) HardCap() int {
	return b.hardCap
}

// ShouldContinue reports whether iteration `i` (0-indexed) is allowed.
// It also extends the budget when the model is still making progress.
func (b *IterationBudget) ShouldContinue(i int) bool {
	if i < b.current {
		return true
	}
	// Past base budget: grant extension only if recent activity looks productive.
	if b.current >= b.hardCap {
		return false
	}
	if b.makingProgress() {
		// Grow by 25% of base, never exceeding hard cap.
		grow := b.base / 4
		if grow < 3 {
			grow = 3
		}
		b.current += grow
		if b.current > b.hardCap {
			b.current = b.hardCap
		}
		return i < b.current
	}
	return false
}

// RecordToolCalls records a batch of tool calls executed in one iteration.
// Returns true if a stuck-loop was detected and the loop should terminate.
func (b *IterationBudget) RecordToolCalls(toolName string, args map[string]interface{}) bool {
	fp := fingerprint(toolName, args)

	if fp == b.lastFp {
		b.repeats++
	} else {
		b.repeats = 1
		b.lastFp = fp
	}

	b.recent = append(b.recent, fp)
	if len(b.recent) > b.repeatN {
		b.recent = b.recent[len(b.recent)-b.repeatN:]
	}

	// 3 in a row = certain stuck loop.
	if b.repeats >= 3 {
		return true
	}
	// 4+ occurrences in the last `repeatN` calls = oscillating loop
	// (A/B/A/B/A/B/A/B is 4 of each in a window of 8 — clearly stuck).
	count := 0
	for _, x := range b.recent {
		if x == fp {
			count++
		}
	}
	return count >= 4
}

// Stuck returns true if the most recent activity matched a known stuck pattern.
func (b *IterationBudget) Stuck() bool {
	return b.repeats >= 3
}

// makingProgress reports whether recent calls are diverse enough to suggest
// the model is actually doing new work (not spinning on the same tool).
func (b *IterationBudget) makingProgress() bool {
	if len(b.recent) == 0 {
		return false
	}
	// Count distinct fingerprints in the trailing window.
	seen := make(map[string]struct{}, len(b.recent))
	for _, fp := range b.recent {
		seen[fp] = struct{}{}
	}
	// At least 50% of window must be unique to count as "making progress".
	return len(seen)*2 >= len(b.recent)
}

// fingerprint produces a stable hash for a tool call.
// Args are sorted by key so map iteration order doesn't change the hash.
func fingerprint(toolName string, args map[string]interface{}) string {
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(toolName)
	sb.WriteString("|")
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(stringValue(args[k]))
		sb.WriteString(";")
	}

	h := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(h[:8]) // 16 chars; collision risk negligible for this scope
}

func stringValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case bool, int, int32, int64, float32, float64:
		return strings.TrimSpace(strings.NewReplacer().Replace(formatScalar(x)))
	default:
		return formatScalar(x)
	}
}

func formatScalar(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	default:
		// Use fmt-like format without importing fmt to keep this hot path tiny.
		// For non-string types, JSON-ish coercion is good enough.
		return jsonScalar(x)
	}
}

func jsonScalar(v interface{}) string {
	// Minimal: numbers and bools roundtrip via Sprint; complex types
	// fall through to a placeholder. This is only used for fingerprinting,
	// not user-visible serialization.
	switch x := v.(type) {
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return itoa(int64(x))
	case int32:
		return itoa(int64(x))
	case int64:
		return itoa(x)
	case float64:
		return ftoa(x)
	case float32:
		return ftoa(float64(x))
	default:
		return "<obj>"
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func ftoa(f float64) string {
	// Quick & dirty; collisions don't matter for fingerprints.
	if f == float64(int64(f)) {
		return itoa(int64(f))
	}
	// Lossy 3-decimal representation.
	scaled := int64(f * 1000)
	intPart := scaled / 1000
	frac := scaled % 1000
	if frac < 0 {
		frac = -frac
	}
	return itoa(intPart) + "." + pad3(int64(frac))
}

func pad3(n int64) string {
	s := itoa(n)
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}
