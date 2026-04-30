package llm

import (
	"regexp"
	"strings"
)

var (
	dsmlInvokeRe = regexp.MustCompile(`<\|\s*DSML\s*\|\s*invoke\s+name="([^"]+)"\s*>`)
	dsmlParamRe  = regexp.MustCompile(`<\|\s*DSML\s*\|\s*parameter\s+name="([^"]+)"(?:\s+string="true")?\s*>(.*?)</\|\s*DSML\s*\|\s*parameter\s*>`)
	dsmlOpenTagRe = regexp.MustCompile(`<\|\s*DSML\s*\|[^>]*>`)
)

// ExtractToolCallsFromContent parses DSML-style tool calls from raw LLM content.
// Returns extracted ToolCalls and cleaned content.
func ExtractToolCallsFromContent(content string) ([]ToolCall, string) {
	if !strings.Contains(content, "<| DSML |") {
		return nil, content
	}

	cleaned := content
	var toolCalls []ToolCall

	// Find all invoke blocks with submatch indices
	invokeMatches := dsmlInvokeRe.FindAllStringSubmatchIndex(content, -1)
	for i, inv := range invokeMatches {
		funcName := content[inv[2]:inv[3]] // submatch group 1

		// Determine the end of this invoke block
		end := len(content)
		if i+1 < len(invokeMatches) {
			end = invokeMatches[i+1][0]
		}

		block := content[inv[0]:end]
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
