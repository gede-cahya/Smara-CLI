package ui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// EditTodo represents a single file edit operation tracked in the sidebar
type EditTodo struct {
	ID          string
	FilePath    string
	Status      string // pending, done, failed
	OldContent  string
	NewContent  string
	Description string
	CreatedAt   time.Time
}

// TodoList manages a collection of edit todos for the sidebar
type TodoList struct {
	todos    []EditTodo
	selected int
	width    int
	height   int
}

// Add appends a new todo to the list and selects it
func (tl *TodoList) Add(todo EditTodo) {
	tl.todos = append(tl.todos, todo)
	tl.selected = len(tl.todos) - 1
}

// SetStatus updates the status of the most recent todo matching the path
func (tl *TodoList) SetStatus(filePath, status string) {
	for i := len(tl.todos) - 1; i >= 0; i-- {
		if tl.todos[i].FilePath == filePath {
			tl.todos[i].Status = status
			return
		}
	}
}

// Selected returns the currently selected todo
func (tl *TodoList) Selected() *EditTodo {
	if tl.selected < 0 || tl.selected >= len(tl.todos) {
		return nil
	}
	return &tl.todos[tl.selected]
}

// NavigateUp moves selection up
func (tl *TodoList) NavigateUp() {
	if tl.selected > 0 {
		tl.selected--
	}
}

// NavigateDown moves selection down
func (tl *TodoList) NavigateDown() {
	if tl.selected < len(tl.todos)-1 {
		tl.selected++
	}
}

// Render returns the full sidebar content string
func (tl *TodoList) Render() string {
	if len(tl.todos) == 0 {
		return dimStyle.Render("  Belum ada edit.")
	}

	var sb strings.Builder
	sb.WriteString(titleStyle.Render(" Todos ") + "\n\n")

	for i, todo := range tl.todos {
		statusIcon := "⏳"
		if todo.Status == "done" {
			statusIcon = "✅"
		} else if todo.Status == "failed" {
			statusIcon = "❌"
		}

		fileName := todo.FilePath
		if len(fileName) > tl.width-8 && tl.width > 8 {
			fileName = "..." + fileName[len(fileName)-tl.width+11:]
		}

		line := fmt.Sprintf("  %s %s", statusIcon, fileName)
		if i == tl.selected {
			line = selectedStyle.Render(line)
		} else {
			line = todoItemStyle.Render(line)
		}
		sb.WriteString(line + "\n")
	}

	// Render diff for selected todo
	if tl.selected >= 0 && tl.selected < len(tl.todos) {
		todo := tl.todos[tl.selected]
		sb.WriteString("\n")
		sb.WriteString(diffHeaderStyle.Render(fmt.Sprintf("  %s ", todo.FilePath)) + "\n")
		sb.WriteString(renderDiff(todo.OldContent, todo.NewContent, tl.width))
	}

	return sb.String()
}

// Syntax & Diff styles
var (
	diffAddedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A6E22E")) // green

	diffRemovedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F92672")) // red

	diffContextStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#75715E")) // dim gray

	diffHeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#66D9EF")). // cyan
			Bold(true)

	diffLineNumStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#5F5F5F")) // dark gray

	keywordStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F92672")) // magenta

	stringStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A6E22E")) // green

	commentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#75715E")).
			Italic(true) // gray italic

	numberStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#AE81FF")) // purple

	typeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#66D9EF")) // cyan

	selectedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#7D56F4")).
			Foreground(lipgloss.Color("#FAFAFA"))

	todoItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E2E2E2"))

	sidebarBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("#3C3C3C"))
)

var (
	goKeywords = regexp.MustCompile(`\b(func|package|import|return|var|const|type|struct|interface|if|else|for|range|switch|case|default|defer|go|select|break|continue|map|chan|nil|true|false)\b`)
	goTypes    = regexp.MustCompile(`\b(int|string|bool|float32|float64|error|byte|rune|uint|uint8|uint16|uint32|uint64|int8|int16|int32|int64|uintptr|any|comparable)\b`)
	goNumbers  = regexp.MustCompile(`\b\d+(\.\d+)?\b`)
	goStrings  = regexp.MustCompile(`"([^"\\]|\\.)*"|` + "`" + `([^` + "`" + `\\]|\\.)*` + "`")
	goComments = regexp.MustCompile(`//.*$|/\*[\s\S]*?\*/`)
)

// HighlightCode applies simple regex-based syntax highlighting
func HighlightCode(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = highlightLine(line)
	}
	return strings.Join(lines, "\n")
}

func highlightLine(line string) string {
	if strings.HasPrefix(strings.TrimSpace(line), "//") {
		return commentStyle.Render(line)
	}

	type placeholder struct {
		text  string
		style lipgloss.Style
		kind  string
	}
	var placeholders []placeholder

	// Protect comments with placeholders
	line = goComments.ReplaceAllStringFunc(line, func(s string) string {
		idx := len(placeholders)
		placeholders = append(placeholders, placeholder{text: s, style: commentStyle, kind: "C"})
		return fmt.Sprintf("\x00C%d\x00", idx)
	})

	// Protect strings with placeholders
	line = goStrings.ReplaceAllStringFunc(line, func(s string) string {
		idx := len(placeholders)
		placeholders = append(placeholders, placeholder{text: s, style: stringStyle, kind: "S"})
		return fmt.Sprintf("\x00S%d\x00", idx)
	})

	// Color keywords, types, numbers
	line = goKeywords.ReplaceAllStringFunc(line, func(s string) string {
		return keywordStyle.Render(s)
	})
	line = goTypes.ReplaceAllStringFunc(line, func(s string) string {
		return typeStyle.Render(s)
	})
	line = goNumbers.ReplaceAllStringFunc(line, func(s string) string {
		return numberStyle.Render(s)
	})

	// Restore placeholders
	for i, ph := range placeholders {
		marker := fmt.Sprintf("\x00%s%d\x00", ph.kind, i)
		line = strings.Replace(line, marker, ph.style.Render(ph.text), 1)
	}

	return line
}

// renderDiff produces a simple unified-diff-like view showing changed lines only
func renderDiff(old, new string, width int) string {
	oldLines := strings.Split(old, "\n")
	newLines := strings.Split(new, "\n")

	newSet := make(map[string]bool)
	for _, l := range newLines {
		newSet[l] = true
	}
	oldSet := make(map[string]bool)
	for _, l := range oldLines {
		oldSet[l] = true
	}

	var sb strings.Builder
	maxLines := 80
	lineCount := 0

	// Show removed lines
	for _, l := range oldLines {
		if lineCount >= maxLines {
			sb.WriteString(diffContextStyle.Render("  ... (truncated)\n"))
			break
		}
		if !newSet[l] {
			prefix := diffRemovedStyle.Render("-")
			content := diffRemovedStyle.Render(truncateLine(l, width-4))
			sb.WriteString(fmt.Sprintf("%s %s\n", prefix, content))
			lineCount++
		}
	}

	// Show added lines
	for _, l := range newLines {
		if lineCount >= maxLines {
			sb.WriteString(diffContextStyle.Render("  ... (truncated)\n"))
			break
		}
		if !oldSet[l] {
			prefix := diffAddedStyle.Render("+")
			content := diffAddedStyle.Render(truncateLine(l, width-4))
			sb.WriteString(fmt.Sprintf("%s %s\n", prefix, content))
			lineCount++
		}
	}

	if lineCount == 0 {
		sb.WriteString(diffContextStyle.Render("  (tidak ada perubahan)\n"))
	}

	return sb.String()
}

func truncateLine(line string, maxLen int) string {
	if maxLen <= 0 || len(line) <= maxLen {
		return line
	}
	return line[:maxLen] + "…"
}

// EditTodoMsg is sent from the agent to the UI when a file is edited
type EditTodoMsg struct {
	Todo EditTodo
}

// ToggleSidebarMsg toggles the sidebar visibility
type ToggleSidebarMsg struct{}
