package agent

import (
	"fmt"
	"strings"
)

func buildPlanningTemplate(kind, goal, context string) (string, error) {
	kind = strings.TrimSpace(kind)
	goal = strings.TrimSpace(goal)
	context = strings.TrimSpace(context)
	if goal == "" {
		return "", fmt.Errorf("argumen 'goal' wajib diisi")
	}

	header := planningTemplateHeader(kind, goal, context)
	switch kind {
	case "clarify-requirements":
		return header + `
## Yang sudah jelas
-

## Pertanyaan klarifikasi
1.
2.
3.

## Keputusan yang dibutuhkan
-

## Acceptance criteria awal
-
`, nil
	case "implementation-plan":
		return header + `
## Context
- Problem yang diselesaikan:
- Outcome yang diharapkan:

## Recommended approach
1.
2.
3.

## Files/tools likely needed
-

## Verification
-

## Risks / rollback
-
`, nil
	case "risk-review":
		return header + `
## Risk checklist
- Security:
- Data loss / migration:
- Backward compatibility:
- Performance:
- UX / workflow regression:
- Operational risk:

## Mitigations
-

## Stop/go recommendation
-
`, nil
	case "test-plan":
		return header + `
## Golden path
-

## Edge cases
-

## Regression checks
-

## Automated tests
-

## Manual verification
-
`, nil
	case "agile-minsky":
		return header + `
## User story
As a ..., I want ..., so that ...

## Acceptance criteria
- Given ..., when ..., then ...
-

## Sprint slices
1. Thin vertical slice:
2. Hardening slice:
3. Polish/observability slice:

## Minsky frames
- Agents involved:
- Goals and subgoals:
- Known constraints:
- Unknowns to reduce first:
- Failure modes:
- Feedback loop:

## Execution backlog
- [ ]
- [ ]
- [ ]
`, nil
	default:
		return "", fmt.Errorf("jenis planning_template '%s' tidak dikenal", kind)
	}
}

func planningTemplateHeader(kind, goal, context string) string {
	var sb strings.Builder
	sb.WriteString("# Planning Template: ")
	sb.WriteString(kind)
	sb.WriteString("\n\n")
	sb.WriteString("## Goal\n")
	sb.WriteString(goal)
	sb.WriteString("\n")
	if context != "" {
		sb.WriteString("\n## Context\n")
		sb.WriteString(context)
		sb.WriteString("\n")
	}
	return sb.String()
}
