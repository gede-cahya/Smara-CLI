package llm

import (
	"regexp"
	"strings"
)

var pipeLike = []rune{'｜', '┃', '│', '║', '┆', '┇', '┊', '┋'}

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
	dsmlTagNormalizeRe = regexp.MustCompile(`(?s)</?\s*\|(?:\s*\|)*\s*DSML\s*\|(?:\s*\|)*\s*([^>]*?)>`)
	dsmlInvokeRe       = regexp.MustCompile(`<\|DSML\|invoke\s+name="([^"]+)"\s*>`)
	dsmlParamRe        = regexp.MustCompile(`<\|DSML\|parameter\s+name="([^"]+)"(?:\s+string="true")?\s*>(.*?)</\|DSML\|parameter\s*>`)
	dsmlOpenTagRe      = regexp.MustCompile(`<\|DSML\|[^>]*>`)
)

func normalizeDSMLTags(content string) string {
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

var dsmlPrefixRe = regexp.MustCompile(`(?i)^<\s*[\|｜┃│║┆┇┊┋\s]*D`)

type DSMLStreamFilter struct {
	buf strings.Builder
}

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

func (f *DSMLStreamFilter) Close() string {
	raw := f.buf.String()
	f.buf.Reset()
	_, cleaned := ExtractToolCallsFromContent(raw)
	return cleaned
}

func ExtractToolCallsFromContent(content string) ([]ToolCall, string) {
	normalized := normalizeDSMLTags(content)
	if !strings.Contains(normalized, "<|DSML|") {
		return nil, content
	}

	cleaned := normalized
	var toolCalls []ToolCall
	invokeMatches := dsmlInvokeRe.FindAllStringSubmatchIndex(normalized, -1)
	for i, inv := range invokeMatches {
		funcName := normalized[inv[2]:inv[3]]
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
		cleaned = strings.Replace(cleaned, block, "", 1)
	}

	cleaned = dsmlOpenTagRe.ReplaceAllString(cleaned, "")
	cleaned = strings.TrimSpace(cleaned)
	return toolCalls, cleaned
}
