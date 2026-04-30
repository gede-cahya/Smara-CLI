package workflow

import (
	"strings"
)

// MapRoleToMCP maps a role to relevant MCP server names based on keyword matching.
func MapRoleToMCP(role string, availableServers []string) []string {
	roleLower := strings.ToLower(role)
	var matches []string

	for _, server := range availableServers {
		serverLower := strings.ToLower(server)
		if matchesRole(roleLower, serverLower) {
			matches = append(matches, server)
		}
	}

	return matches
}

// matchesRole checks if a server name matches a role via keywords.
func matchesRole(role, server string) bool {
	switch role {
	case "frontend", "designer":
		return containsAny(server, []string{"stitch", "figma", "screen", "ui"})
	case "backend":
		return containsAny(server, []string{"file", "edit", "terminal", "code", "run"})
	case "database":
		return containsAny(server, []string{"sql", "db", "migrate", "database"})
	case "devops":
		return containsAny(server, []string{"deploy", "docker", "ssh", "vercel"})
	case "qa":
		return containsAny(server, []string{"file", "view", "read", "terminal"})
	default:
		// For custom roles, accept any server
		return true
	}
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
		switch roleLower {
		case "frontend", "designer":
			if containsAny(toolLower, []string{"stitch", "figma", "screen", "generate", "design"}) {
				matches = append(matches, tool)
			}
		case "backend":
			if containsAny(toolLower, []string{"write", "edit", "read", "view", "terminal", "run", "command"}) {
				matches = append(matches, tool)
			}
		case "database":
			if containsAny(toolLower, []string{"sql", "db", "terminal", "run", "command"}) {
				matches = append(matches, tool)
			}
		case "devops":
			if containsAny(toolLower, []string{"deploy", "docker", "ssh", "terminal", "run", "command"}) {
				matches = append(matches, tool)
			}
		case "qa":
			if containsAny(toolLower, []string{"view", "read", "check", "test"}) {
				matches = append(matches, tool)
			}
		default:
			matches = append(matches, tool)
		}
	}

	return matches
}
