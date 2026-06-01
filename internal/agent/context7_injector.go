package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

// context7PerLibraryTimeout is the max time allowed for resolving + fetching
// docs for a single library via Context7 MCP. Prevents slow/dead MCP servers
// from blocking the entire prompt processing pipeline.
const context7PerLibraryTimeout = 5 * time.Second

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

		// Resolve and fetch with timeout to prevent blocking on slow MCP servers
		docs, uri := ci.resolveWithTimeout(entry, prompt, executor)
		if docs == "" {
			continue
		}

		ci.docsCache[entry.Name] = docs
		results = append(results, Context7DocResult{
			Library: entry.Name,
			URI:     uri,
			Docs:    docs,
		})
		if uri != "" {
			injectedParts = append(injectedParts, fmt.Sprintf("[Context7: %s (%s)]\n%s", entry.Name, uri, docs))
		} else {
			injectedParts = append(injectedParts, fmt.Sprintf("[Context7: %s]\n%s", entry.Name, docs))
		}
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

// resolveWithTimeout wraps the Context7 resolve+fetch calls with a timeout.
func (ci *Context7Injector) resolveWithTimeout(entry Context7RegistryEntry, prompt string, executor skill.StepExecutor) (docs, uri string) {
	type result struct {
		docs string
		uri  string
	}
	ch := make(chan result, 1)
	go func() {
		var r result
		// Step 1: Resolve library ID
		resolveResult, err := executor("resolve", map[string]interface{}{
			"libraryName": entry.Context7Library,
			"query":       prompt,
		})
		if err != nil {
			ch <- r
			return
		}
		r.uri = strings.TrimSpace(resolveResult)
		if r.uri == "" {
			ch <- r
			return
		}
		// Step 2: Fetch docs
		docsResult, err := executor("get-library-documentation", map[string]interface{}{
			"uri":   r.uri,
			"query": prompt,
		})
		if err != nil {
			ch <- r
			return
		}
		r.docs = docsResult
		ch <- r
	}()

	select {
	case r := <-ch:
		return r.docs, r.uri
	case <-time.After(context7PerLibraryTimeout):
		return "", ""
	}
}

// Context7DocResult holds the outcome of a single library docs fetch.
type Context7DocResult struct {
	Library string
	URI     string
	Docs    string
}
