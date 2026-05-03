package graphify

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ParseGoCodebase walks a directory and parses all Go files into a Graph.
func ParseGoCodebase(rootPath string, graphID string) (*Graph, error) {
	g := NewGraph(graphID, rootPath)
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil // skip tests
		}
		return parseGoFile(path, rootPath, g)
	})
	if err != nil {
		return nil, err
	}

	// Second pass: resolve call graph
	resolveCallGraph(g)

	g.ComputeGodScores()
	return g, nil
}

func parseGoFile(filePath, rootPath string, g *Graph) error {
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil // skip unreadable files
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil // skip unparseable files
	}

	relPath, _ := filepath.Rel(rootPath, filePath)
	if relPath == "" {
		relPath = filePath
	}

	// Package node
	pkgID := NodeID(relPath, f.Name.Name, "package")
	g.AddNode(&Node{
		ID:         pkgID,
		Label:      f.Name.Name,
		Type:       "package",
		SourceFile: relPath,
		Language:   "go",
		Content:    fmt.Sprintf("package %s", f.Name.Name),
	})

	// Track imports
	imports := make(map[string]string) // alias -> import path
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		alias := path
		if imp.Name != nil {
			alias = imp.Name.Name
		} else {
			// derive alias from last path component
			parts := strings.Split(path, "/")
			alias = parts[len(parts)-1]
		}
		imports[alias] = path

		impID := NodeID(relPath, path, "import")
		g.AddNode(&Node{
			ID:         impID,
			Label:      path,
			Type:       "import",
			SourceFile: relPath,
			Language:   "go",
			Content:    fmt.Sprintf("import %q", path),
		})
		g.AddEdge(&Edge{
			ID:         EdgeID(pkgID, impID, "imports"),
			Source:     pkgID,
			Target:     impID,
			Relation:   "imports",
			Confidence: "EXTRACTED",
			SourceFile: relPath,
		})
	}

	// Track types and functions for call graph resolution
	typeMap := make(map[string]string)
	funcMap := make(map[string]string)

	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			parseFuncDecl(x, fset, relPath, g, pkgID, &typeMap, &funcMap)
		case *ast.GenDecl:
			parseGenDecl(x, fset, relPath, g, pkgID, &typeMap)
		}
		return true
	})

	// Store type and func maps as metadata on the graph for call graph resolution
	if g.Metadata == nil {
		g.Metadata = make(map[string]interface{})
	}
	if g.Metadata[filePath+"_types"] == nil {
		g.Metadata[filePath+"_types"] = typeMap
		g.Metadata[filePath+"_funcs"] = funcMap
		g.Metadata[filePath+"_imports"] = imports
	}

	// Extract rationale comments
	extractRationaleComments(f, fset, relPath, g)

	return nil
}

func parseFuncDecl(fn *ast.FuncDecl, fset *token.FileSet, relPath string, g *Graph, pkgID string, typeMap, funcMap *map[string]string) {
	name := fn.Name.Name
	if name == "" {
		return
	}

	pos := fset.Position(fn.Pos())
	sig := funcSignature(fn)
	doc := ""
	if fn.Doc != nil {
		doc = fn.Doc.Text()
	}

	nodeType := "function"
	nodeID := NodeID(relPath, name, nodeType)
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		nodeType = "method"
		recvType := typeExprToString(fn.Recv.List[0].Type)
		nodeID = NodeID(relPath, recvType+"."+name, nodeType)
		(*funcMap)[name] = nodeID // simplistic, doesn't handle receiver type
	} else {
		(*funcMap)[name] = nodeID
	}

	g.AddNode(&Node{
		ID:         nodeID,
		Label:      name,
		Type:       nodeType,
		SourceFile: relPath,
		SourceLine: pos.Line,
		Language:   "go",
		Content:    sig,
		Metadata: map[string]interface{}{
			"docstring": doc,
			"signature": sig,
		},
	})

	g.AddEdge(&Edge{
		ID:         EdgeID(pkgID, nodeID, "contains"),
		Source:     pkgID,
		Target:     nodeID,
		Relation:   "contains",
		Confidence: "EXTRACTED",
		SourceFile: relPath,
	})

	// Find calls within the function body
	if fn.Body != nil {
		findCalls(fn.Body, nodeID, relPath, g, *funcMap, *typeMap)
	}
}

func parseGenDecl(decl *ast.GenDecl, fset *token.FileSet, relPath string, g *Graph, pkgID string, typeMap *map[string]string) {
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			name := s.Name.Name
			pos := fset.Position(s.Pos())
			var typeType string
			var content string
			switch s.Type.(type) {
			case *ast.StructType:
				typeType = "struct"
				content = fmt.Sprintf("type %s struct", name)
			case *ast.InterfaceType:
				typeType = "interface"
				content = fmt.Sprintf("type %s interface", name)
			default:
				typeType = "type"
				content = fmt.Sprintf("type %s ...", name)
			}
			nodeID := NodeID(relPath, name, typeType)
			(*typeMap)[name] = nodeID
			g.AddNode(&Node{
				ID:         nodeID,
				Label:      name,
				Type:       typeType,
				SourceFile: relPath,
				SourceLine: pos.Line,
				Language:   "go",
				Content:    content,
			})
			g.AddEdge(&Edge{
				ID:         EdgeID(pkgID, nodeID, "contains"),
				Source:     pkgID,
				Target:     nodeID,
				Relation:   "contains",
				Confidence: "EXTRACTED",
				SourceFile: relPath,
			})
		case *ast.ValueSpec:
			for _, n := range s.Names {
				if n.Name == "" {
					continue
				}
				pos := fset.Position(n.Pos())
				nodeID := NodeID(relPath, n.Name, "variable")
				g.AddNode(&Node{
					ID:         nodeID,
					Label:      n.Name,
					Type:       "variable",
					SourceFile: relPath,
					SourceLine: pos.Line,
					Language:   "go",
				})
				g.AddEdge(&Edge{
					ID:         EdgeID(pkgID, nodeID, "contains"),
					Source:     pkgID,
					Target:     nodeID,
					Relation:   "contains",
					Confidence: "EXTRACTED",
					SourceFile: relPath,
				})
			}
		}
	}
}

func funcSignature(fn *ast.FuncDecl) string {
	var b strings.Builder
	b.WriteString("func ")
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		b.WriteString("(")
		for i, f := range fn.Recv.List {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(typeExprToString(f.Type))
		}
		b.WriteString(") ")
	}
	b.WriteString(fn.Name.Name)
	b.WriteString("(")
	if fn.Type.Params != nil {
		for i, f := range fn.Type.Params.List {
			if i > 0 {
				b.WriteString(", ")
			}
			names := []string{}
			for _, n := range f.Names {
				names = append(names, n.Name)
			}
			if len(names) > 0 {
				b.WriteString(strings.Join(names, ", "))
				b.WriteString(" ")
			}
			b.WriteString(typeExprToString(f.Type))
		}
	}
	b.WriteString(")")
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		b.WriteString(" ")
		if len(fn.Type.Results.List) > 1 || len(fn.Type.Results.List[0].Names) > 0 {
			b.WriteString("(")
		}
		for i, f := range fn.Type.Results.List {
			if i > 0 {
				b.WriteString(", ")
			}
			for j, n := range f.Names {
				if j > 0 {
					b.WriteString(", ")
				}
				b.WriteString(n.Name)
			}
			if len(f.Names) > 0 {
				b.WriteString(" ")
			}
			b.WriteString(typeExprToString(f.Type))
		}
		if len(fn.Type.Results.List) > 1 || len(fn.Type.Results.List[0].Names) > 0 {
			b.WriteString(")")
		}
	}
	return b.String()
}

func typeExprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeExprToString(t.X)
	case *ast.ArrayType:
		if t.Len != nil {
			return fmt.Sprintf("[%s]%s", typeExprToString(t.Len), typeExprToString(t.Elt))
		}
		return "[]" + typeExprToString(t.Elt)
	case *ast.SelectorExpr:
		return typeExprToString(t.X) + "." + t.Sel.Name
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", typeExprToString(t.Key), typeExprToString(t.Value))
	case *ast.ChanType:
		return "chan " + typeExprToString(t.Value)
	case *ast.FuncType:
		return "func(...)"
	case *ast.StructType:
		return "struct{}"
	default:
		return fmt.Sprintf("%T", expr)
	}
}

func findCalls(body ast.Node, callerID, relPath string, g *Graph, funcMap, typeMap map[string]string) {
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			funcName := callExprToString(x.Fun)
			if funcName == "" {
				return true
			}
			// Try to resolve to a known function
			targetID := ""
			if id, ok := funcMap[funcName]; ok {
				targetID = id
			} else if strings.Contains(funcName, ".") {
				parts := strings.SplitN(funcName, ".", 2)
				if id, ok := funcMap[parts[1]]; ok {
					targetID = id
				}
			}
			if targetID != "" && targetID != callerID {
				g.AddEdge(&Edge{
					ID:         EdgeID(callerID, targetID, "calls"),
					Source:     callerID,
					Target:     targetID,
					Relation:   "calls",
					Confidence: "EXTRACTED",
					SourceFile: relPath,
				})
			}
		}
		return true
	})
}

func callExprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return callExprToString(t.X) + "." + t.Sel.Name
	default:
		return ""
	}
}

func resolveCallGraph(g *Graph) {
	// Build a global function map for cross-file resolution
	globalFuncMap := make(map[string]string)
	globalTypeMap := make(map[string]string)

	for id, n := range g.Nodes {
		if n.Type == "function" || n.Type == "method" {
			globalFuncMap[n.Label] = id
			// Also add qualified form if possible
			parts := strings.Split(id, ":")
			if len(parts) >= 3 {
				shortID := parts[len(parts)-1]
				globalFuncMap[shortID] = id
			}
		}
		if n.Type == "struct" || n.Type == "interface" || n.Type == "type" {
			globalTypeMap[n.Label] = id
		}
	}

	// Update edges that couldn't be resolved
	for _, e := range g.Edges {
		if e.Relation == "calls" {
			// Check if target is a known function but edge was created before resolution
			// (call graph is already built in parseGoFile, so this is mostly a no-op)
		}
	}
}

var rationalePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)//\s*WHY:\s*(.+)$`),
	regexp.MustCompile(`(?i)//\s*NOTE:\s*(.+)$`),
	regexp.MustCompile(`(?i)//\s*HACK:\s*(.+)$`),
	regexp.MustCompile(`(?i)//\s*IMPORTANT:\s*(.+)$`),
	regexp.MustCompile(`(?i)/\*\s*WHY:\s*(.+?)\*/`),
	regexp.MustCompile(`(?i)/\*\s*NOTE:\s*(.+?)\*/`),
	regexp.MustCompile(`(?i)/\*\s*HACK:\s*(.+?)\*/`),
	regexp.MustCompile(`(?i)/\*\s*IMPORTANT:\s*(.+?)\*/`),
}

func extractRationaleComments(f *ast.File, fset *token.FileSet, relPath string, g *Graph) {
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			for _, re := range rationalePatterns {
				matches := re.FindStringSubmatch(c.Text)
				if len(matches) > 1 {
					pos := fset.Position(c.Pos())
					text := strings.TrimSpace(matches[1])
					nodeID := NodeID(relPath, fmt.Sprintf("rationale_%d", pos.Line), "rationale")
					g.AddNode(&Node{
						ID:         nodeID,
						Label:      text,
						Type:       "rationale",
						SourceFile: relPath,
						SourceLine: pos.Line,
						Language:   "go",
						Content:    text,
					})

					// Try to attach to nearest function/type above this line
					nearest := findNearestNode(g, relPath, pos.Line)
					if nearest != "" {
						g.AddEdge(&Edge{
							ID:             EdgeID(nearest, nodeID, "rationale_for"),
							Source:         nodeID,
							Target:         nearest,
							Relation:       "rationale_for",
							Confidence:     "EXTRACTED",
							SourceFile:     relPath,
							InferredReason: text,
						})
					}
				}
			}
		}
	}
}

func findNearestNode(g *Graph, relPath string, line int) string {
	var nearest *Node
	nearestDist := 1000000
	for _, n := range g.Nodes {
		if n.SourceFile == relPath && n.SourceLine > 0 && n.SourceLine <= line {
			dist := line - n.SourceLine
			if dist < nearestDist {
				nearestDist = dist
				nearest = n
			}
		}
	}
	if nearest != nil {
		return nearest.ID
	}
	return ""
}
