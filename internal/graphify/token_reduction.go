package graphify

import (
	"fmt"
	"strings"
)

// ToCompactText serializes a graph into compact text representation for LLM context.
func ToCompactText(g *Graph, budget int) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Graph: %s (%d nodes, %d edges)\n", g.ID, g.NodeCount(), g.EdgeCount()))
	b.WriteString(fmt.Sprintf("Root: %s\n\n", g.RootPath))

	// Include all nodes first
	b.WriteString("Nodes:\n")
	for _, n := range g.Nodes {
		line := fmt.Sprintf("- %s (%s", n.Label, n.Type)
		if n.SourceFile != "" {
			line += fmt.Sprintf(", %s:%d", n.SourceFile, n.SourceLine)
		}
		line += ")"
		if n.Content != "" {
			content := n.Content
			if len(content) > 120 {
				content = content[:120] + "..."
			}
			line += fmt.Sprintf(" -- %q", content)
		}
		line += "\n"
		b.WriteString(line)
	}

	b.WriteString("\nEdges:\n")
	for _, e := range g.Edges {
		line := fmt.Sprintf("- %s --[%s]--> %s", e.Source, e.Relation, e.Target)
		if e.Confidence != "" {
			line += fmt.Sprintf(" (%s", e.Confidence)
			if e.ConfidenceScore > 0 {
				line += fmt.Sprintf(", %.2f", e.ConfidenceScore)
			}
			line += ")"
		}
		if e.SourceFile != "" {
			line += fmt.Sprintf(" [%s]", e.SourceFile)
		}
		line += "\n"
		b.WriteString(line)
	}

	result := b.String()
	if budget > 0 && len(result) > budget {
		result = result[:budget]
		idx := strings.LastIndex(result, "\n")
		if idx > 0 {
			result = result[:idx] + "\n... (truncated)\n"
		}
	}
	return result
}

// ToPromptContext formats a subgraph as system prompt context.
func ToPromptContext(query string, g *Graph) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Graph context for query: %q\n\n", query))
	b.WriteString(ToCompactText(g, 0))
	b.WriteString("\nPrefer this graph structure over guessing. Cite source files when possible.\n")
	return b.String()
}
