package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ExploreResult represents a single file or directory in the explore tree
type ExploreResult struct {
	Path     string
	Name     string
	Type     string // "dir" or "file"
	Size     int64
	Language string
	Depth    int
	Expanded bool
}

// PathNode represents a node in the tree with path-based lookup
type PathNode struct {
	Result   ExploreResult
	Parent   *PathNode
	Children []*PathNode
}

// ExploreTree wraps results with tree structure for interactive navigation
type ExploreTree struct {
	Root    *PathNode
	Results []ExploreResult
	NodeMap map[string]*PathNode
}

// ExploreCodebase walks a directory tree up to a given depth and returns results
func ExploreCodebase(root string, depth int) ([]ExploreResult, error) {
	if root == "" {
		root = "."
	}
	if depth <= 0 {
		depth = 2
	}

	var results []ExploreResult

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		if rel == "." {
			return nil
		}

		currentDepth := strings.Count(rel, string(os.PathSeparator))
		if currentDepth >= depth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip unwanted directories
		name := d.Name()
		if d.IsDir() {
			skipDirs := map[string]bool{
				".git": true, "node_modules": true, "vendor": true, "dist": true,
				"build": true, "__pycache__": true, ".next": true, ".kilo": true,
			}
			if skipDirs[name] {
				return filepath.SkipDir
			}
		}

		info, _ := d.Info()
		size := int64(0)
		if info != nil {
			size = info.Size()
		}

		lang := detectLanguage(name)

		results = append(results, ExploreResult{
			Path: path,
			Name: name,
			Type: func() string {
				if d.IsDir() {
					return "dir"
				}
				return "file"
			}(),
			Size:     size,
			Language: lang,
			Depth:    currentDepth,
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sort: directories first, then alphabetically
	sort.Slice(results, func(i, j int) bool {
		if results[i].Type != results[j].Type {
			return results[i].Type == "dir"
		}
		return results[i].Path < results[j].Path
	})

	return results, nil
}

// BuildExploreTree builds a tree structure from flat results
func BuildExploreTree(results []ExploreResult) *ExploreTree {
	tree := &ExploreTree{
		Results: results,
		NodeMap: make(map[string]*PathNode),
	}

	if len(results) == 0 {
		return tree
	}

	for _, r := range results {
		node := &PathNode{Result: r}
		tree.NodeMap[r.Path] = node
	}

	for _, r := range results {
		node := tree.NodeMap[r.Path]
		parentPath := filepath.Dir(r.Path)
		if parentPath != "." && parentPath != "" {
			if parent, ok := tree.NodeMap[parentPath]; ok {
				node.Parent = parent
				parent.Children = append(parent.Children, node)
			}
		} else {
			tree.Root = node
		}
	}

	return tree
}

// UpdateExpandState toggles expansion state for a path
func UpdateExpandState(tree *ExploreTree, path string, expanded bool) {
	if node, ok := tree.NodeMap[path]; ok {
		node.Result.Expanded = expanded
		tree.NodeMap[path] = node
	}
}

// GetVisibleResults returns filtered results based on expansion state
func GetVisibleResults(tree *ExploreTree) []ExploreResult {
	var visible []ExploreResult
	for _, r := range tree.Results {
		if r.Depth == 0 {
			visible = append(visible, r)
			continue
		}
		parentPath := filepath.Dir(r.Path)
		if parent, ok := tree.NodeMap[parentPath]; ok && parent.Result.Expanded {
			visible = append(visible, r)
		} else if parentPath == "." || parentPath == "" {
			visible = append(visible, r)
		}
	}
	return visible
}

// RenderExplore returns a tree-like string for the sidebar or chat
func RenderExplore(results []ExploreResult) string {
	if len(results) == 0 {
		return dimStyle.Render("  (direktori kosong)")
	}

	var sb strings.Builder
	sb.WriteString(titleStyle.Render(" Explore ") + "\n\n")

	for _, r := range results {
		indent := strings.Repeat("  ", r.Depth)
		icon := getLanguageIcon(r.Language)
		if r.Type == "dir" {
			icon = "📁"
		}

		nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E2E2E2"))
		if r.Type == "dir" {
			nameStyle = nameStyle.Bold(true).Foreground(lipgloss.Color("#66D9EF"))
		}

		line := fmt.Sprintf("%s%s %s", indent, icon, nameStyle.Render(r.Name))
		sb.WriteString(line + "\n")
	}

	return sb.String()
}

func detectLanguage(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".go":
		return "go"
	case ".js", ".jsx", ".mjs":
		return "js"
	case ".ts", ".tsx":
		return "ts"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".c", ".h":
		return "c"
	case ".cpp", ".hpp", ".cc":
		return "cpp"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".html", ".htm":
		return "html"
	case ".css", ".scss", ".sass", ".less":
		return "css"
	case ".json":
		return "json"
	case ".md":
		return "markdown"
	case ".yaml", ".yml":
		return "yaml"
	case ".sh", ".bash", ".zsh":
		return "shell"
	case ".sql":
		return "sql"
	case ".dockerfile":
		return "docker"
	default:
		if strings.HasPrefix(name, "Dockerfile") {
			return "docker"
		}
		if strings.HasPrefix(name, "Makefile") {
			return "make"
		}
		return "unknown"
	}
}

// PreviewFile reads a file and returns preview content with syntax highlighting indicator
func PreviewFile(path string, maxLines int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("gagal membaca file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	if maxLines <= 0 {
		maxLines = 50
	}

	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}

	var sb strings.Builder
	lang := detectLanguage(path)
	icon := getLanguageIcon(lang)

	sb.WriteString(titleStyle.Render(fmt.Sprintf(" %s Preview: %s ", icon, filepath.Base(path))) + "\n\n")

	for i, line := range lines {
		sb.WriteString(fmt.Sprintf("%4d │ %s\n", i+1, line))
	}

	if len(strings.Split(string(data), "\n")) > maxLines {
		sb.WriteString(dimStyle.Render(fmt.Sprintf("\n  ... (%d more lines)", len(strings.Split(string(data), "\n"))-maxLines)))
	}

	return sb.String(), nil
}

func getLanguageIcon(lang string) string {
	switch lang {
	case "go":
		return "🐹"
	case "js":
		return "📜"
	case "ts":
		return "📘"
	case "python":
		return "🐍"
	case "rust":
		return "🦀"
	case "java":
		return "☕"
	case "c", "cpp":
		return "🔧"
	case "ruby":
		return "💎"
	case "php":
		return "🐘"
	case "html":
		return "🌐"
	case "css":
		return "🎨"
	case "json":
		return "📋"
	case "markdown":
		return "📝"
	case "yaml":
		return "🗂️"
	case "shell":
		return "🐚"
	case "sql":
		return "🗄️"
	case "docker":
		return "🐳"
	case "make":
		return "🔨"
	default:
		return "📄"
	}
}
