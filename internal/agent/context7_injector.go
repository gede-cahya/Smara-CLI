package agent

import (
	"fmt"
	"strings"

	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

// Context7Injector handles auto-detection and injection of Context7 docs into prompts.
type Context7Injector struct {
	// resolvedLibraries tracks which libraries have been resolved this session
	// to avoid redundant fetches.
	resolvedLibraries map[string]bool

	// docsCache stores fetched docs snippets keyed by library name.
	docsCache map[string]string
}

// NewContext7Injector creates a fresh injector for a new session.
func NewContext7Injector() *Context7Injector {
	return &Context7Injector{
		resolvedLibraries: make(map[string]bool),
		docsCache:         make(map[string]string),
	}
}

// DetectAndInject scans the user prompt for library keywords, resolves them via
// Context7, and returns injected context + any docs that were fetched.
// The executor is used to run the context7-resolve and context7-docs skills.
func (ci *Context7Injector) DetectAndInject(prompt string, executor skill.StepExecutor) (string, []Context7DocResult, error) {
	entries, err := DetectLibrariesFromPrompt(prompt)
	if err != nil {
		return prompt, nil, fmt.Errorf("context7 detection failed: %w", err)
	}

	if len(entries) == 0 {
		return prompt, nil, nil
	}

	var results []Context7DocResult
	var injectedParts []string

	for _, entry := range entries {
		if ci.resolvedLibraries[entry.Name] {
			// Use cached docs if available
			if cached, ok := ci.docsCache[entry.Name]; ok && cached != "" {
				results = append(results, Context7DocResult{
					Library: entry.Name,
					Docs:    cached,
				})
				injectedParts = append(injectedParts, fmt.Sprintf("[Context7: %s]\n%s", entry.Name, cached))
			}
			continue
		}

		// Mark as resolved (even if fetch fails, to avoid retry loops)
		ci.resolvedLibraries[entry.Name] = true

		if entry.Context7Library == "" {
			continue
		}

		// Step 1: Resolve library ID via context7-resolve skill
		resolveResult, err := executor("resolve", map[string]interface{}{
			"libraryName": entry.Context7Library,
			"query":       prompt,
		})
		if err != nil {
			// Resolve may fail if MCP server not available; skip gracefully
			continue
		}

		// The resolve result should contain the Context7-compatible URI
		uri := strings.TrimSpace(resolveResult)
		if uri == "" {
			continue
		}

		// Step 2: Fetch docs via get-library-documentation
		docsResult, err := executor("get-library-documentation", map[string]interface{}{
			"uri":   uri,
			"query": prompt,
		})
		if err != nil {
			continue
		}

		ci.docsCache[entry.Name] = docsResult
		results = append(results, Context7DocResult{
			Library: entry.Name,
			URI:     uri,
			Docs:    docsResult,
		})
		injectedParts = append(injectedParts, fmt.Sprintf("[Context7: %s (%s)]\n%s", entry.Name, uri, docsResult))
	}

	if len(injectedParts) == 0 {
		return prompt, results, nil
	}

	// Prepend context to the prompt
	enriched := fmt.Sprintf("%s\n\n---\nUser request:\n%s",
		strings.Join(injectedParts, "\n\n"),
		prompt,
	)

	return enriched, results, nil
}

// Context7DocResult holds the outcome of a single library docs fetch.
type Context7DocResult struct {
	Library string
	URI     string
	Docs    string
}
