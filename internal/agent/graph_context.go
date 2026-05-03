package agent

import (
	"database/sql"
	"strings"

	"github.com/gede-cahya/Smara-CLI/internal/graphify"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
)

// codebaseKeywords are trigger words that suggest the user is asking about the codebase.
var codebaseKeywords = []string{
	"how does", "how is", "what is", "where is", "explain", "describe",
	"function", "method", "struct", "interface", "package", "module",
	"auth", "router", "handler", "controller", "service", "middleware",
	"database", "model", "repo", "repository", "cache", "config",
	"flow", "call", "invoke", "import", "depend", "reference",
	"codebase", "project", "source", "file", "implementation",
	"why", "what connects", "how does", "berfungsi", "cara kerja",
}

// IsCodebaseQuery returns true if the prompt appears to ask about the codebase.
func IsCodebaseQuery(prompt string) bool {
	lower := strings.ToLower(prompt)
	for _, kw := range codebaseKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// BuildGraphContext attempts to load a graph for the current workspace and
// returns compact graph context relevant to the query.
func BuildGraphContext(db *sql.DB, prompt string, workspaceID int64) (string, error) {
	if db == nil {
		return "", nil
	}
	gs, err := graphify.NewGraphStore(db)
	if err != nil {
		return "", err
	}

	graphs, err := gs.ListGraphs()
	if err != nil || len(graphs) == 0 {
		return "", nil
	}

	// Use the most recently updated graph
	var graphID string
	if len(graphs) > 0 {
		if id, ok := graphs[0]["graph_id"].(string); ok {
			graphID = id
		}
	}
	if graphID == "" {
		return "", nil
	}

	g, err := gs.LoadGraph(graphID)
	if err != nil {
		return "", nil
	}

	// Query the graph with the user prompt
	result := g.Query(prompt, 2)
	if len(result.Nodes) == 0 {
		// Try a broader search with just the first noun phrase
		keywords := extractKeywords(prompt)
		for _, kw := range keywords {
			result = g.Query(kw, 1)
			if len(result.Nodes) > 0 {
				break
			}
		}
	}

	if len(result.Nodes) == 0 {
		return "", nil
	}

	sub := graphify.NewGraph(g.ID+"_agent", g.RootPath)
	for _, n := range result.Nodes {
		sub.AddNode(n)
	}
	for _, e := range result.Edges {
		sub.AddEdge(e)
	}

	ctx := graphify.ToPromptContext(prompt, sub)
	// Rough token estimate: ~4 chars per token
	if len(ctx) > 8000 {
		ctx = graphify.ToCompactText(sub, 8000)
	}
	return ctx, nil
}

func extractKeywords(prompt string) []string {
	lower := strings.ToLower(prompt)
	// Remove common stop words and punctuation
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "must": true, "shall": true, "can": true,
		"need": true, "dare": true, "ought": true, "used": true, "to": true,
		"of": true, "in": true, "for": true, "on": true, "with": true,
		"at": true, "by": true, "from": true, "as": true, "into": true,
		"through": true, "during": true, "before": true, "after": true,
		"above": true, "below": true, "between": true, "under": true,
		"and": true, "but": true, "or": true, "yet": true, "so": true,
		"if": true, "because": true, "although": true, "though": true,
		"while": true, "where": true, "when": true, "that": true, "which": true,
		"who": true, "whom": true, "whose": true, "what": true, "how": true,
		"this": true, "these": true, "those": true, "i": true, "you": true,
		"he": true, "she": true, "it": true, "we": true, "they": true,
		"me": true, "him": true, "her": true, "us": true, "them": true,
		"my": true, "your": true, "his": true,
		"its": true, "their": true, "mine": true, "yours": true, "hers": true,
		"ours": true, "theirs": true, "bagaimana": true, "apa": true,
		"siapa": true, "mengapa": true, "kapan": true, "di mana": true,
		"yang": true, "dan": true, "atau": true, "tetapi": true, "dari": true,
		"untuk": true, "pada": true, "dalam": true, "dengan": true, "ini": true,
		"itu": true, "tersebut": true, "saya": true, "kamu": true, "dia": true,
		"kita": true, "mereka": true,
	}
	words := strings.FieldsFunc(lower, func(r rune) bool {
		return r == ' ' || r == '?' || r == '.' || r == ',' || r == '!' || r == ':' || r == ';' || r == '(' || r == ')' || r == '"' || r == '\''
	})
	var result []string
	for _, w := range words {
		if !stopWords[w] && len(w) > 2 {
			result = append(result, w)
		}
	}
	return result
}

// injectGraphContext adds graph context to a system prompt if applicable.
func injectGraphContext(db *sql.DB, prompt string, workspaceID int64, messages *[]llm.Message) {
	if !IsCodebaseQuery(prompt) {
		return
	}
	ctx, err := BuildGraphContext(db, prompt, workspaceID)
	if err != nil || ctx == "" {
		return
	}
	*messages = append(*messages, llm.Message{
		Role:    llm.RoleSystem,
		Content: ctx,
	})
}
