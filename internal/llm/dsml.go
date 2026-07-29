package llm

import (
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

var dsmlIDSeq atomic.Int64

// pipe-like chars that models may use instead of ASCII |
var pipeLike = []rune{'｜', '┃', '│', '║', '┆', '┇', '┊', '┋'}

var pipeLikeFast = func() map[rune]struct{} {
	m := make(map[rune]struct{}, len(pipeLike))
	for _, r := range pipeLike {
		m[r] = struct{}{}
	}
	return m
}()

// normalizePipes replaces all pipe-like Unicode chars with ASCII |.
func normalizePipes(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { _, ok := pipeLikeFast[r]; return ok }) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if _, ok := pipeLikeFast[r]; ok {
			b.WriteRune('|')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var (
	// Matches any messy variation of DSML tags (e.g. "< | | DSML | | tag>" or "<| DSML |tag>")
	// Also handles "<｜｜DSML｜｜tool_calls>" with fullwidth pipes normalized.
	dsmlTagNormalizeRe = regexp.MustCompile(`(?s)</?\s*\|(?:\s*\|)*\s*DSML\s*\|(?:\s*\|)*\s*([^>]*?)>`)

	// Regexes for normalized format <|DSML|...>
	dsmlInvokeRe  = regexp.MustCompile(`<\|DSML\|invoke\s+name="([^"]+)"\s*>`)
	dsmlParamRe   = regexp.MustCompile(`<\|DSML\|parameter\s+name="([^"]+)"(?:\s+string="true")?\s*>(.*?)</\|DSML\|parameter\s*>`)
	dsmlOpenTagRe = regexp.MustCompile(`</?\|DSML\|[^>]*>`)
	// Aggressive residual cleaner for partial/truncated DSML leftovers at end of answer
	// e.g. "<|DSML|invoke name="skill_run">" without closing tag.
	dsmlResidualRe = regexp.MustCompile(`(?s)</?\|DSML\|.*`)
	// Plain leak: literal "skill_run auto-xxx" or "skill_run" told without DSML wrapper but
	// from the supervisor fallback summary that accidentally includes tool names
	// — we DON'T strip plain tool names in prose, only DSML tags. However some
	// models hallucinate DSML-like backtick code blocks:
	//   <｜｜DSML｜｜invoke name="skill_run"> <｜｜DSML｜｜parameter ...>auto-cek-status-vps</...>
	// which normal re already handles. The residual re catches leftovers.
)

// normalizeDSMLTags converts any variation of DSML tags to standard <|DSML|...> format.
func normalizeDSMLTags(content string) string {
	// First replace any Unicode pipe-like chars with ASCII pipe
	content = normalizePipes(content)
	// Fast path: no DSML string at all
	if !strings.Contains(content, "DSML") {
		return content
	}
	return dsmlTagNormalizeRe.ReplaceAllStringFunc(content, func(match string) string {
		submatches := dsmlTagNormalizeRe.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		tagContent := strings.TrimSpace(submatches[1])
		if strings.HasPrefix(match, "</") {
			return "</|DSML|" + tagContent + ">"
		}
		return "<|DSML|" + tagContent + ">"
	})
}

// dsmlPrefixRe matches the start of a DSML tag after '<'.
var dsmlPrefixRe = regexp.MustCompile(`(?i)^<\s*[\|｜┃│║┆┇┊┋\s]*D`)

// DSMLStreamFilter buffers streaming chunks and emits only text outside DSML tags.
// It holds back trailing text that might be the start of an incomplete DSML tag.
type DSMLStreamFilter struct {
	buf strings.Builder
}

// Write appends a chunk and returns text safe to display (DSML tags removed).
// Incomplete DSML prefixes at the end of the chunk are retained internally.
func (f *DSMLStreamFilter) Write(chunk string) string {
	f.buf.WriteString(chunk)
	raw := f.buf.String()

	lastLT := strings.LastIndex(raw, "<")
	if lastLT == -1 {
		f.buf.Reset()
		_, cleaned := ExtractToolCallsFromContent(raw)
		return cleaned
	}

	tail := raw[lastLT:]
	if !dsmlPrefixRe.MatchString(tail) {
		f.buf.Reset()
		_, cleaned := ExtractToolCallsFromContent(raw)
		return cleaned
	}

	prefix := raw[:lastLT]
	f.buf.Reset()
	f.buf.WriteString(tail)
	_, cleaned := ExtractToolCallsFromContent(prefix)
	return cleaned
}

// Close flushes any remaining buffered text at the end of the stream.
func (f *DSMLStreamFilter) Close() string {
	raw := f.buf.String()
	f.buf.Reset()
	_, cleaned := ExtractToolCallsFromContent(raw)
	return cleaned
}

// ExtractToolCallsFromContent parses DSML-style tool calls from raw LLM content.
// Returns extracted ToolCalls and cleaned content.
// ThinkStreamFilter splits a streamed token sequence into visible content and
// inline <think>...</think> reasoning. Some providers (notably the custom /
// OpenAI-compatible ones) emit reasoning inline in the content delta instead of
// a separate `reasoning` field. Without splitting, "<think>" markup leaks into
// the live answer stream. This filter buffers partial tags across chunks so a
// "<thi" split across two deltas is never shown.
type ThinkStreamFilter struct {
	buf     strings.Builder // holds a possibly-incomplete tag prefix at the tail
	inThink bool            // currently inside a <think> block
}

// thinkOpen/thinkClose are the literal tags we route on.
const (
	thinkOpen  = "<think>"
	thinkClose = "</think>"
)

// Write consumes a streamed chunk and returns the text that is safe to display
// (content) and the reasoning text (thinking) extracted from <think> blocks.
// Trailing bytes that could be the start of a tag are retained internally and
// emitted on a later Write or on Close.
func (f *ThinkStreamFilter) Write(chunk string) (content string, thinking string) {
	f.buf.WriteString(chunk)
	work := f.buf.String()
	f.buf.Reset()

	var out, think strings.Builder
	for {
		if f.inThink {
			idx := strings.Index(work, thinkClose)
			if idx == -1 {
				// Still inside think. Emit all but a possible partial close tag.
				keep := partialTagSuffix(work, thinkClose)
				think.WriteString(work[:len(work)-keep])
				f.buf.WriteString(work[len(work)-keep:])
				break
			}
			think.WriteString(work[:idx])
			work = work[idx+len(thinkClose):]
			f.inThink = false
			continue
		}

		idx := strings.Index(work, thinkOpen)
		if idx == -1 {
			// No open tag. Emit all but a possible partial open tag.
			keep := partialTagSuffix(work, thinkOpen)
			out.WriteString(work[:len(work)-keep])
			f.buf.WriteString(work[len(work)-keep:])
			break
		}
		out.WriteString(work[:idx])
		work = work[idx+len(thinkOpen):]
		f.inThink = true
	}

	return out.String(), think.String()
}

// Close flushes any buffered tail at end of stream. Any leftover is treated as
// content if we're outside a think block, otherwise as thinking.
func (f *ThinkStreamFilter) Close() (content string, thinking string) {
	rem := f.buf.String()
	f.buf.Reset()
	if rem == "" {
		return "", ""
	}
	if f.inThink {
		return "", rem
	}
	return rem, ""
}

// partialTagSuffix returns the length of the trailing portion of s that is a
// proper prefix of tag (e.g. "<thi" for "<think>"). It lets the filter hold
// back bytes that might complete into a tag on the next chunk. Returns 0 when
// no suffix of s is a non-empty prefix of tag.
func partialTagSuffix(s, tag string) int {
	max := len(tag) - 1
	if max > len(s) {
		max = len(s)
	}
	for n := max; n > 0; n-- {
		if strings.HasPrefix(tag, s[len(s)-n:]) {
			return n
		}
	}
	return 0
}

// StripThinkTags removes <think>...</think> reasoning blocks from content and
// returns the cleaned prose plus the extracted reasoning. Providers other than
// Ollama (e.g. the OpenAI-compatible "custom" provider) do not strip these
// inline reasoning tags, so the supervisor calls this centrally to avoid empty
// `<think></think>` shards leaking into final answers and thought summaries.
func StripThinkTags(content string) (cleaned string, thinking string) {
	return stripThinkingTags(content)
}

// dsmlInvokeEndRe finds </|DSML|invoke> or any closing variation
var dsmlInvokeEndRe = regexp.MustCompile(`(?i)</\|DSML\|invoke[^>]*>`)

func ExtractToolCallsFromContent(content string) ([]ToolCall, string) {
	// Normalize messy tag formats from models like deepseek-v4-flash
	normalized := normalizeDSMLTags(content)

	if !strings.Contains(normalized, "<|DSML|") {
		if !strings.Contains(content, "DSML") {
			return nil, content
		}
		// DSML substring but no normalized tag (partial/truncated)
		stripped := dsmlOpenTagRe.ReplaceAllString(normalized, "")
		stripped = dsmlResidualRe.ReplaceAllString(stripped, "")
		stripped = strings.TrimSpace(stripped)
		return nil, stripped
	}

	var toolCalls []ToolCall

	// Use index-based removal to avoid Replace string collisions when blocks repeat
	type removal struct{ s, e int }
	var removals []removal

	invokeMatches := dsmlInvokeRe.FindAllStringSubmatchIndex(normalized, -1)
	for _, inv := range invokeMatches {
		funcName := normalized[inv[2]:inv[3]]

		// End of this invoke = next invoke start, or </|DSML|invoke> after this one, or end
		start := inv[0]
		// look for explicit close tag </|DSML|invoke> after start
		end := len(normalized)
		if loc := dsmlInvokeEndRe.FindStringIndex(normalized[start:]); loc != nil {
			end = start + loc[1]
		}
		// but don't span past next invoke opening if close tag missing
		// (find next invoke start after current start)
		for _, next := range invokeMatches {
			if next[0] > start && next[0] < end {
				end = next[0]
				break
			}
		}

		block := normalized[start:end]
		args := make(map[string]interface{})
		for _, pm := range dsmlParamRe.FindAllStringSubmatch(block, -1) {
			if len(pm) >= 3 {
				args[pm[1]] = pm[2]
			}
		}

		seq := dsmlIDSeq.Add(1)
		toolCalls = append(toolCalls, ToolCall{
			ID:       fmt.Sprintf("dsml_%s_%d_%d", funcName, time.Now().UnixNano(), seq),
			Function: funcName,
			Args:     args,
		})
		removals = append(removals, removal{start, end})
	}

	// Build cleaned by skipping removed ranges (reverse order to keep indices stable via builder)
	if len(removals) == 0 {
		// No invoke matched but we have DSML tags — strip all
		cleaned := dsmlOpenTagRe.ReplaceAllString(normalized, "")
		cleaned = dsmlResidualRe.ReplaceAllString(cleaned, "")
		return toolCalls, strings.TrimSpace(cleaned)
	}

	var b strings.Builder
	prev := 0
	for _, r := range removals {
		if r.s > prev {
			b.WriteString(normalized[prev:r.s])
		}
		prev = r.e
	}
	b.WriteString(normalized[prev:])
	cleaned := b.String()
	cleaned = dsmlOpenTagRe.ReplaceAllString(cleaned, "")
	cleaned = dsmlResidualRe.ReplaceAllString(cleaned, "")
	cleaned = strings.TrimSpace(cleaned)

	return toolCalls, cleaned
}

// SanitizeForUser is the final defense-in-depth sanitizer that must be called
// before any LLM output is shown to user on ANY surface (web, telegram, discord, cli).
// It guarantees zero DSML tag leaks, handles partial/truncated tags, and also
// removes the specific leak reported: "<|DSML|invoke name="skill_run"> ... </|DSML|invoke>"
// appearing as raw text in final answers.
func SanitizeForUser(text string) string {
	if text == "" {
		return text
	}
	_, cleaned := ExtractToolCallsFromContent(text)
	// Double-clean aggressive: some models emit nested/recursive DSML
	if strings.Contains(cleaned, "<|DSML|") || strings.Contains(cleaned, "DSML") && strings.Contains(cleaned, "<") {
		cleaned = dsmlResidualRe.ReplaceAllString(cleaned, "")
		cleaned = dsmlOpenTagRe.ReplaceAllString(cleaned, "")
	}
	return strings.TrimSpace(cleaned)
}
