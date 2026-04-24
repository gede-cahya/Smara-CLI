package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type exploreItem struct {
	path     string
	name     string
	fileType string
	language string
	depth    int
}

func (i exploreItem) Title() string { return i.name }
func (i exploreItem) Description() string {
	if i.fileType == "dir" {
		return "📁 directory"
	}
	return getLanguageIcon(i.language) + " " + i.language
}
func (i exploreItem) FilterValue() string { return i.name }

type exploreTUIModel struct {
	list          list.Model
	viewport      viewport.Model
	allResults    []ExploreResult
	visibleItems  []exploreItem
	selectedIndex int
	preview       string
	width         int
	height        int
	previewWidth  int
	listWidth     int
}

func newExploreTUIModel(results []ExploreResult) *exploreTUIModel {
	m := &exploreTUIModel{
		allResults: results,
	}
	m.refreshVisibleItems()
	return m
}

func (m *exploreTUIModel) refreshVisibleItems() {
	m.visibleItems = nil
	for _, r := range m.allResults {
		if r.Depth == 0 {
			m.visibleItems = append(m.visibleItems, itemFromResult(r))
			continue
		}
		parentPath := filepath.Dir(r.Path)
		parentExpanded := false
		for _, pr := range m.allResults {
			if pr.Path == parentPath && pr.Type == "dir" {
				parentExpanded = true
				break
			}
		}
		if parentExpanded {
			m.visibleItems = append(m.visibleItems, itemFromResult(r))
		}
	}
}

func itemFromResult(r ExploreResult) exploreItem {
	return exploreItem{
		path:     r.Path,
		name:     r.Name,
		fileType: r.Type,
		language: r.Language,
		depth:    r.Depth,
	}
}

func (m *exploreTUIModel) toggleExpand(path string) {
	for i, r := range m.allResults {
		if r.Path == path && r.Type == "dir" {
			m.allResults[i].Expanded = !m.allResults[i].Expanded
			break
		}
	}
	m.refreshVisibleItems()
	m.list.SetItems(itemsFromExplores(m.visibleItems))
}

func itemsFromExplores(items []exploreItem) []list.Item {
	result := make([]list.Item, len(items))
	for i, item := range items {
		result[i] = item
	}
	return result
}

var (
	dirStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#66D9EF")).Bold(true)
	fileStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#E2E2E2"))
	previewStyle  = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("#3C3C3C")).PaddingLeft(1)
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#767676"))
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FAFAFA")).Background(lipgloss.Color("#7D56F4")).PaddingLeft(2).PaddingRight(2)
	chevronOpen   = "▼"
	chevronClosed = "▶"
)

func isExpanded(path string, results []ExploreResult) bool {
	for _, r := range results {
		if r.Path == path && r.Type == "dir" {
			return r.Expanded
		}
	}
	return false
}

func (m *exploreTUIModel) Init() tea.Cmd {
	return nil
}

func (m *exploreTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.previewWidth = msg.Width * 40 / 100
		if m.previewWidth < 30 {
			m.previewWidth = 30
		}
		m.listWidth = msg.Width - m.previewWidth - 4
		m.viewport.Width = m.previewWidth
		m.viewport.Height = msg.Height - 4
		m.list.SetWidth(m.listWidth)
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	}

	newList, cmd := m.list.Update(msg)
	m.list = newList
	cmds = append(cmds, cmd)

	newVP, vpCmd := m.viewport.Update(msg)
	m.viewport = newVP
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

func (m *exploreTUIModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		return m, tea.Quit
	case tea.KeyCtrlC:
		return m, tea.Quit
	}

	switch msg.String() {
	case "q", "Q":
		return m, tea.Quit
	case "j", "down":
		m.list.CursorDown()
		m.updatePreview()
		return m, nil
	case "k", "up":
		m.list.CursorUp()
		m.updatePreview()
		return m, nil
	case "l", "right", " ":
		m.toggleExpandAtCursor()
		return m, nil
	case "h", "left":
		m.toggleExpandAtCursor()
		return m, nil
	case "g":
		m.list.Select(0)
		m.updatePreview()
		return m, nil
	case "G":
		m.list.Select(len(m.visibleItems) - 1)
		m.updatePreview()
		return m, nil
	case "enter":
		m.toggleExpandAtCursor()
		return m, nil
	}

	return m, nil
}

func (m *exploreTUIModel) toggleExpandAtCursor() {
	idx := m.list.Index()
	if idx >= 0 && idx < len(m.visibleItems) {
		item := m.visibleItems[idx]
		if item.fileType == "dir" {
			m.toggleExpand(item.path)
		}
	}
}

func (m *exploreTUIModel) updatePreview() {
	idx := m.list.Index()
	if idx >= 0 && idx < len(m.visibleItems) {
		item := m.visibleItems[idx]
		if item.fileType == "file" {
			m.preview, _ = PreviewFile(item.path, 50)
			m.viewport.SetContent(m.preview)
		} else {
			m.viewport.SetContent("")
		}
	}
}

func (m *exploreTUIModel) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	var sb strings.Builder

	sb.WriteString(headerStyle.Render(" Explore Codebase ") + "\n")

	listView := m.list.View()
	previewView := previewStyle.Render(m.viewport.View())

	sb.WriteString(fmt.Sprintf("%s│%s\n", listView, previewView))

	help := helpStyle.Render(" j/k↑↓ navigate • l/h→← expand/collapse • Enter toggle • q quit")
	sb.WriteString(help + "\n")

	return sb.String()
}

func RunExploreInteractive(path string, depth int) error {
	results, err := ExploreCodebase(path, depth)
	if err != nil {
		return err
	}

	model := newExploreTUIModel(results)

	items := itemsFromExplores(model.visibleItems)
	model.list = list.New(items, list.NewDefaultDelegate(), model.listWidth, model.height-4)
	model.list.SetShowStatusBar(false)
	model.list.SetFilteringEnabled(true)

	model.viewport = viewport.New(model.previewWidth, model.height-4)

	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		return err
	}

	return nil
}
