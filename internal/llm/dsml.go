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
	dsmlTagNormalizeRe = regexp.MustCompile(`(?s)</?\s*\|(?:\s*\|)?\s*DSML\s*\|(?:\s*\|)?\s*([^>]*?)>`)

	// Regexes for normalized format <|DSML|...>
	dsmlInvokeRe = regexp.MustCompile(`<\|DSML\|invoke\s+name="([^"]+)"\s*>`)
	dsmlParamRe  = regexp.MustCompile(`<\|DSML\|parameter\s+name="([^"]+)"(?:\s+string="true")?\s*>(.*?)</\|DSML\|parameter\s*>`)
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

// ExtractToolCallsFromContent parses DSML-style tool calls from raw LLM content.
// Returns extracted ToolCalls and cleaned content.
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
