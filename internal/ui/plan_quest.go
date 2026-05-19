package ui

import (
	"strings"
)

const (
	planQuestStartMarker = "[[SMARA_PLAN_QUEST]]"
	planQuestEndMarker   = "[[/SMARA_PLAN_QUEST]]"
)

// PlanQuest represents a structured clarification question emitted by Plan mode.
type PlanQuest struct {
	Title       string
	Options     []string
	AllowCustom bool
}

// ParsePlanQuest extracts a [[SMARA_PLAN_QUEST]] block from content and returns
// the content with the block removed. If no complete block is present, quest is nil.
func ParsePlanQuest(content string) (cleanContent string, quest *PlanQuest) {
	start := strings.Index(content, planQuestStartMarker)
	if start < 0 {
		return content, nil
	}

	afterStart := start + len(planQuestStartMarker)
	endRel := strings.Index(content[afterStart:], planQuestEndMarker)
	if endRel < 0 {
		return content, nil
	}
	end := afterStart + endRel
	block := content[afterStart:end]

	q := &PlanQuest{AllowCustom: true}
	inOptions := false
	for _, rawLine := range strings.Split(block, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "title:"):
			q.Title = strings.TrimSpace(line[len("title:"):])
			inOptions = false
		case strings.HasPrefix(lower, "options:"):
			inOptions = true
		case strings.HasPrefix(lower, "allow_custom:"):
			value := strings.TrimSpace(line[len("allow_custom:"):])
			q.AllowCustom = value == "" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes") || strings.EqualFold(value, "ya")
			inOptions = false
		case inOptions && strings.HasPrefix(line, "-"):
			option := strings.TrimSpace(strings.TrimPrefix(line, "-"))
			if option != "" {
				q.Options = append(q.Options, option)
			}
		}
	}

	if q.Title == "" && len(q.Options) == 0 {
		return content, nil
	}
	if q.Title == "" {
		q.Title = "Pilih salah satu opsi"
	}

	clean := strings.TrimSpace(content[:start] + content[end+len(planQuestEndMarker):])
	return clean, q
}
