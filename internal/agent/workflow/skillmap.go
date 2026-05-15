package workflow

import (
	"strings"
)

// KeywordMap groups keywords by domain for flexible matching.
var roleKeywords = map[string][]string{
	// Software
	"frontend": {"stitch", "figma", "screen", "ui", "react", "component"},
	"backend":  {"file", "edit", "terminal", "code", "run", "command", "api", "server"},
	"database": {"sql", "db", "migrate", "database", "query", "schema"},
	"devops":   {"deploy", "docker", "ssh", "vercel", "container", "ci", "pipeline"},
	"qa":       {"file", "view", "read", "terminal", "test", "check", "audit"},
	// Marketing
	"content":         {"write", "edit", "view", "content", "copy", "blog"},
	"copywriter":      {"write", "edit", "view", "content", "copy", "text"},
	"seo":             {"search", "keyword", "rank", "analytics"},
	"visual_designer": {"stitch", "figma", "screen", "design", "image", "graphic"},
	// Legal
	"legal":      {"write", "edit", "view", "read", "document", "clause", "contract"},
	"contract":   {"write", "edit", "view", "document", "clause", "template"},
	"compliance": {"check", "audit", "verify", "read", "document"},
	// Data
	"data":          {"file", "read", "write", "csv", "table", "query", "pandas"},
	"visualization": {"chart", "graph", "dashboard", "plot", "screen"},
	// Design
	"brand":       {"stitch", "figma", "design", "brand", "identity"},
	"illustrator": {"stitch", "figma", "image", "draw", "design"},
	"typography":  {"font", "text", "style", "design"},
}

// MapRoleToMCP maps a role to relevant MCP server names based on keyword matching.
func MapRoleToMCP(role string, availableServers []string) []string {
	roleLower := strings.ToLower(role)
	var matches []string

	for _, server := range availableServers {
		serverLower := strings.ToLower(server)
		if matchesRoleByKeywords(roleLower, serverLower) {
			matches = append(matches, server)
		}
	}

	return matches
}

// matchesRoleByKeywords checks if a server name matches a role via keywords.
func matchesRoleByKeywords(role, server string) bool {
	// Direct keyword matching from roleKeywords map
	for roleKey, keywords := range roleKeywords {
		if strings.Contains(role, roleKey) && containsAny(server, keywords) {
			return true
		}
	}

	// Software fallback
	if containsAny(role, []string{"frontend", "designer"}) {
		return containsAny(server, []string{"stitch", "figma", "screen", "ui"})
	}
	if containsAny(role, []string{"backend"}) {
		return containsAny(server, []string{"file", "edit", "terminal", "code", "run"})
	}
	if containsAny(role, []string{"database"}) {
		return containsAny(server, []string{"sql", "db", "migrate", "database"})
	}
	if containsAny(role, []string{"devops"}) {
		return containsAny(server, []string{"deploy", "docker", "ssh", "vercel"})
	}
	if containsAny(role, []string{"qa"}) {
		return containsAny(server, []string{"file", "view", "read", "terminal"})
	}

	// For unknown/custom roles, accept servers with general-purpose keywords
	return containsAny(server, []string{"file", "edit", "write", "view", "terminal", "stitch", "figma"})
}

// containsAny returns true if s contains any of the substrings.
func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// FilterToolsByRole filters a list of tool names to those relevant for a role.
func FilterToolsByRole(role string, tools []string) []string {
	roleLower := strings.ToLower(role)
	var matches []string

	for _, tool := range tools {
		toolLower := strings.ToLower(tool)

		// Software roles
		if containsAny(roleLower, []string{"frontend", "designer"}) {
			if containsAny(toolLower, []string{"stitch", "figma", "screen", "generate", "design"}) {
				matches = append(matches, tool)
			}
			continue
		}
		if containsAny(roleLower, []string{"backend"}) {
			if containsAny(toolLower, []string{"write", "edit", "read", "view", "terminal", "run", "command"}) {
				matches = append(matches, tool)
			}
			continue
		}
		if containsAny(roleLower, []string{"database"}) {
			if containsAny(toolLower, []string{"sql", "db", "terminal", "run", "command"}) {
				matches = append(matches, tool)
			}
			continue
		}
		if containsAny(roleLower, []string{"devops"}) {
			if containsAny(toolLower, []string{"deploy", "docker", "ssh", "terminal", "run", "command"}) {
				matches = append(matches, tool)
			}
			continue
		}
		if containsAny(roleLower, []string{"qa"}) {
			if containsAny(toolLower, []string{"view", "read", "check", "test"}) {
				matches = append(matches, tool)
			}
			continue
		}

		// Marketing roles
		if containsAny(roleLower, []string{"content", "copywriter", "strategist", "campaign"}) {
			if containsAny(toolLower, []string{"write", "edit", "view", "read", "file", "content"}) {
				matches = append(matches, tool)
			}
			continue
		}
		if containsAny(roleLower, []string{"seo"}) {
			if containsAny(toolLower, []string{"search", "read", "view", "analyze"}) {
				matches = append(matches, tool)
			}
			continue
		}

		// Legal roles
		if containsAny(roleLower, []string{"legal", "contract", "compliance"}) {
			if containsAny(toolLower, []string{"write", "edit", "view", "read", "file", "document"}) {
				matches = append(matches, tool)
			}
			continue
		}

		// Data roles
		if containsAny(roleLower, []string{"data", "visualization"}) {
			if containsAny(toolLower, []string{"write", "edit", "read", "view", "file", "chart", "graph", "terminal"}) {
				matches = append(matches, tool)
			}
			continue
		}

		// Design roles
		if containsAny(roleLower, []string{"brand", "illustrator", "typography", "visual_designer"}) {
			if containsAny(toolLower, []string{"stitch", "figma", "design", "generate", "image", "draw"}) {
				matches = append(matches, tool)
			}
			continue
		}

		// Default: include all tools for unknown roles
		matches = append(matches, tool)
	}

	return matches
}
