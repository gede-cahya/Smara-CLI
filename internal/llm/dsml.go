package llm

import (
	"regexp"
	"strings"
)

// pipe-like chars that models may use instead of ASCII |
var pipeLike = []rune{'｜', '┃', '│', '║', '┆', '┇', '┊', '┋'}

// normalizePipes replaces all pipe-like Unicode chars with ASCII |.
func normalizePipes(s string) string {
	var b strings.Builder
	for _, r := range s {
		isPipe := false
		for _, p := range pipeLike {
			if r == p {
				isPipe = true
				break
			}
		}
		if isPipe {
			b.WriteRune('|')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var (
	// Matches any messy variation of DSML tags (e.g. "< | | DSML | | tag>" or "<| DSML |tag>")
	dsmlTagNormalizeRe = regexp.MustCompile(`(?s)</?\s*\|(?:\s*\|)*\s*DSML\s*\|(?:\s*\|)*\s*([^>]*?)>`)

	// Regexes for normalized format <|DSML|...>
	dsmlInvokeRe  = regexp.MustCompile(`<\|DSML\|invoke\s+name="([^"]+)"\s*>`)
	dsmlParamRe   = regexp.MustCompile(`<\|DSML\|parameter\s+name="([^"]+)"(?:\s+string="true")?\s*>(.*?)</\|DSML\|parameter\s*>`)
	dsmlOpenTagRe = regexp.MustCompile(`<\|DSML\|[^>]*>`)
)

// normalizeDSMLTags converts any variation of DSML tags to standard <|DSML|...> format.
func normalizeDSMLTags(content string) string {
	// First replace any Unicode pipe-like chars with ASCII pipe
	content = normalizePipes(content)

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

func ExtractToolCallsFromContent(content string) ([]ToolCall, string) {
	// Normalize messy tag formats from models like deepseek-v4-flash
	normalized := normalizeDSMLTags(content)

	if !strings.Contains(normalized, "<|DSML|") {
		return nil, content
	}

	cleaned := normalized
	var toolCalls []ToolCall

	// Find all invoke blocks with submatch indices
	invokeMatches := dsmlInvokeRe.FindAllStringSubmatchIndex(normalized, -1)
	for i, inv := range invokeMatches {
		funcName := normalized[inv[2]:inv[3]] // submatch group 1

		// Determine the end of this invoke block
		end := len(normalized)
		if i+1 < len(invokeMatches) {
			end = invokeMatches[i+1][0]
		}

		block := normalized[inv[0]:end]
		args := make(map[string]interface{})

		paramMatches := dsmlParamRe.FindAllStringSubmatch(block, -1)
		for _, pm := range paramMatches {
			if len(pm) >= 3 {
				args[pm[1]] = pm[2]
			}
		}

		toolCalls = append(toolCalls, ToolCall{
			ID:       "dsml_" + funcName + "_" + string(rune('a'+i)),
			Function: funcName,
			Args:     args,
		})

		// Remove this DSML block from cleaned content
		cleaned = strings.Replace(cleaned, block, "", 1)
	}

	// Strip any remaining dangling DSML tags
	cleaned = dsmlOpenTagRe.ReplaceAllString(cleaned, "")
	cleaned = strings.TrimSpace(cleaned)

	return toolCalls, cleaned
}
