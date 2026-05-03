package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/graphify"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/gede-cahya/Smara-CLI/internal/ui"
)

var (
	graphifyName   string
	graphifyPath   string
	graphifyFormat string
	graphifyOut    string
	graphifyDepth  int
	graphifyBudget int
	graphifyDFS    bool
)

var graphifyCmd = &cobra.Command{
	Use:   "graphify",
	Short: "Knowledge graph from codebase",
	Long: `Generate and query knowledge graphs from source code.

Subcommands:
  init      Parse codebase and store graph
  query     Search graph with natural language
  path      Find shortest path between two nodes
  explain   Show node neighborhood
  export    Export graph to JSON/SVG/GraphML/Neo4j
  list      List stored graphs
  delete    Remove a stored graph`,
}

var graphifyInitCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Parse codebase into knowledge graph",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runGraphifyInit,
}

var graphifyQueryCmd = &cobra.Command{
	Use:   "query [text]",
	Short: "Query graph for nodes and edges",
	Args:  cobra.ExactArgs(1),
	RunE:  runGraphifyQuery,
}

var graphifyPathCmd = &cobra.Command{
	Use:   "path [from] [to]",
	Short: "Shortest path between two nodes",
	Args:  cobra.ExactArgs(2),
	RunE:  runGraphifyPath,
}

var graphifyExplainCmd = &cobra.Command{
	Use:   "explain [node]",
	Short: "Explain a node and its neighbors",
	Args:  cobra.ExactArgs(1),
	RunE:  runGraphifyExplain,
}

var graphifyExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export graph to various formats",
	RunE:  runGraphifyExport,
}

var graphifyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List stored graphs",
	RunE:  runGraphifyList,
}

var graphifyDeleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete a stored graph",
	Args:  cobra.ExactArgs(1),
	RunE:  runGraphifyDelete,
}

func init() {
	graphifyInitCmd.Flags().StringVar(&graphifyName, "name", "", "graph name (default: directory name)")
	graphifyInitCmd.Flags().StringVar(&graphifyPath, "path", ".", "codebase path")

	graphifyQueryCmd.Flags().StringVar(&graphifyName, "name", "", "graph name")
	graphifyQueryCmd.Flags().IntVar(&graphifyDepth, "depth", 2, "neighborhood depth")
	graphifyQueryCmd.Flags().IntVar(&graphifyBudget, "budget", 0, "max tokens for compact output")

	graphifyPathCmd.Flags().StringVar(&graphifyName, "name", "", "graph name")

	graphifyExplainCmd.Flags().StringVar(&graphifyName, "name", "", "graph name")
	graphifyExplainCmd.Flags().IntVar(&graphifyDepth, "depth", 2, "neighborhood depth")

	graphifyExportCmd.Flags().StringVar(&graphifyName, "name", "", "graph name")
	graphifyExportCmd.Flags().StringVar(&graphifyFormat, "format", "json", "export format: json, svg, graphml, neo4j")
	graphifyExportCmd.Flags().StringVar(&graphifyOut, "out", "", "output file")

	graphifyDeleteCmd.Flags().StringVar(&graphifyName, "name", "", "graph name")

	graphifyCmd.AddCommand(graphifyInitCmd)
	graphifyCmd.AddCommand(graphifyQueryCmd)
	graphifyCmd.AddCommand(graphifyPathCmd)
	graphifyCmd.AddCommand(graphifyExplainCmd)
	graphifyCmd.AddCommand(graphifyExportCmd)
	graphifyCmd.AddCommand(graphifyListCmd)
	graphifyCmd.AddCommand(graphifyDeleteCmd)

	rootCmd.AddCommand(graphifyCmd)
}

func openGraphStore() (*graphify.GraphStore, *memory.SQLiteStore, error) {
	cfg := config.Get()
	memStore, err := memory.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open db: %w", err)
	}
	gs, err := graphify.NewGraphStore(memStore.DB())
	if err != nil {
		memStore.Close()
		return nil, nil, err
	}
	return gs, memStore, nil
}

func runGraphifyInit(cmd *cobra.Command, args []string) error {
	path := graphifyPath
	if len(args) > 0 {
		path = args[0]
	}
	path, _ = filepath.Abs(path)

	name := graphifyName
	if name == "" {
		name = filepath.Base(path)
	}

	ui.PrintInfo("Parsing Go codebase: %s", path)
	start := time.Now()
	g, err := graphify.ParseGoCodebase(path, name)
	if err != nil {
		return fmt.Errorf("parse failed: %w", err)
	}
	g.AssignCommunities()
	elapsed := time.Since(start)
	ui.PrintSuccess("Parsed in %s: %d nodes, %d edges", elapsed.Round(time.Millisecond), g.NodeCount(), g.EdgeCount())

	ui.PrintInfo("Storing graph '%s'...", name)
	gs, _, err := openGraphStore()
	if err != nil {
		return err
	}
	if err := gs.SaveGraph(g); err != nil {
		return fmt.Errorf("save failed: %w", err)
	}
	ui.PrintSuccess("Graph '%s' stored successfully", name)
	return nil
}

func runGraphifyQuery(cmd *cobra.Command, args []string) error {
	if graphifyName == "" {
		return fmt.Errorf("--name required")
	}
	gs, _, err := openGraphStore()
	if err != nil {
		return err
	}
	g, err := gs.LoadGraph(graphifyName)
	if err != nil {
		return err
	}
	result := g.Query(args[0], graphifyDepth)
	if graphifyBudget > 0 {
		sub := graphify.NewGraph(g.ID+"_query", g.RootPath)
		for _, n := range result.Nodes {
			sub.AddNode(n)
		}
		for _, e := range result.Edges {
			sub.AddEdge(e)
		}
		fmt.Println(graphify.ToCompactText(sub, graphifyBudget))
		return nil
	}
	fmt.Printf("Nodes (%d):\n", len(result.Nodes))
	for _, n := range result.Nodes {
		fmt.Printf("  - %s (%s) %s:%d\n", n.Label, n.Type, n.SourceFile, n.SourceLine)
	}
	fmt.Printf("\nEdges (%d):\n", len(result.Edges))
	for _, e := range result.Edges {
		fmt.Printf("  - %s --[%s]--> %s\n", e.Source, e.Relation, e.Target)
	}
	return nil
}

func runGraphifyPath(cmd *cobra.Command, args []string) error {
	if graphifyName == "" {
		return fmt.Errorf("--name required")
	}
	gs, _, err := openGraphStore()
	if err != nil {
		return err
	}
	g, err := gs.LoadGraph(graphifyName)
	if err != nil {
		return err
	}
	result := g.FindPath(args[0], args[1])
	if result == nil {
		fmt.Println("No path found")
		return nil
	}
	fmt.Printf("Path: %s\n", strings.Join(result.Path, " -> "))
	for _, e := range result.Edges {
		fmt.Printf("  %s --[%s]--> %s\n", e.Source, e.Relation, e.Target)
	}
	return nil
}

func runGraphifyExplain(cmd *cobra.Command, args []string) error {
	if graphifyName == "" {
		return fmt.Errorf("--name required")
	}
	gs, _, err := openGraphStore()
	if err != nil {
		return err
	}
	g, err := gs.LoadGraph(graphifyName)
	if err != nil {
		return err
	}
	result := g.ExplainNode(args[0], graphifyDepth)
	fmt.Println(graphify.ToCompactText(&graphify.Graph{
		ID:       g.ID,
		RootPath: g.RootPath,
		Nodes:    make(map[string]*graphify.Node),
		Edges:    result.Edges,
	}, 0))
	for _, n := range result.Nodes {
		fmt.Printf("\n%s (%s)\n", n.Label, n.Type)
		if n.Content != "" {
			fmt.Printf("  Content: %s\n", n.Content)
		}
		if n.SourceFile != "" {
			fmt.Printf("  Source: %s:%d\n", n.SourceFile, n.SourceLine)
		}
	}
	return nil
}

func runGraphifyExport(cmd *cobra.Command, args []string) error {
	if graphifyName == "" {
		return fmt.Errorf("--name required")
	}
	gs, _, err := openGraphStore()
	if err != nil {
		return err
	}
	g, err := gs.LoadGraph(graphifyName)
	if err != nil {
		return err
	}

	outPath := graphifyOut
	if outPath == "" {
		outPath = fmt.Sprintf("%s.%s", graphifyName, graphifyFormat)
	}

	switch graphifyFormat {
	case "json":
		data, err := g.ToJSON()
		if err != nil {
			return err
		}
		if err := os.WriteFile(outPath, data, 0644); err != nil {
			return err
		}
	case "svg":
		return exportSVG(g, outPath)
	case "graphml":
		return exportGraphML(g, outPath)
	case "neo4j":
		return exportNeo4j(g, outPath)
	default:
		return fmt.Errorf("unknown format: %s", graphifyFormat)
	}
	ui.PrintSuccess("Exported to %s", outPath)
	return nil
}

func runGraphifyList(cmd *cobra.Command, args []string) error {
	gs, _, err := openGraphStore()
	if err != nil {
		return err
	}
	graphs, err := gs.ListGraphs()
	if err != nil {
		return err
	}
	if len(graphs) == 0 {
		fmt.Println("No graphs stored")
		return nil
	}
	for _, g := range graphs {
		fmt.Printf("%s: %s (%d nodes, %d edges) updated %s\n",
			g["graph_id"], g["root_path"], g["node_count"], g["edge_count"], g["updated_at"])
	}
	return nil
}

func runGraphifyDelete(cmd *cobra.Command, args []string) error {
	gs, _, err := openGraphStore()
	if err != nil {
		return err
	}
	if err := gs.DeleteGraph(args[0]); err != nil {
		return err
	}
	ui.PrintSuccess("Graph '%s' deleted", args[0])
	return nil
}

func exportSVG(g *graphify.Graph, outPath string) error {
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="800" height="600">`)
	b.WriteString(`<rect width="800" height="600" fill="#f8f9fa"/>`)
	b.WriteString(`<text x="10" y="20" font-size="14" fill="#333">Graph: ` + g.ID + `</text>`)
	b.WriteString(`</svg>`)
	return os.WriteFile(outPath, []byte(b.String()), 0644)
}

func exportGraphML(g *graphify.Graph, outPath string) error {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<graphml xmlns="http://graphml.graphdrawing.org/xmlns">`)
	b.WriteString(`<graph id="` + g.ID + `" edgedefault="directed">`)
	for _, n := range g.Nodes {
		b.WriteString(fmt.Sprintf(`<node id="%s"><data key="label">%s</data></node>`, n.ID, n.Label))
	}
	for _, e := range g.Edges {
		b.WriteString(fmt.Sprintf(`<edge source="%s" target="%s"><data key="relation">%s</data></edge>`, e.Source, e.Target, e.Relation))
	}
	b.WriteString(`</graph></graphml>`)
	return os.WriteFile(outPath, []byte(b.String()), 0644)
}

func exportNeo4j(g *graphify.Graph, outPath string) error {
	var b strings.Builder
	for _, n := range g.Nodes {
		b.WriteString(fmt.Sprintf("CREATE (:%s {id: '%s', label: '%s', type: '%s'});\n",
			strings.Title(n.Type), n.ID, n.Label, n.Type))
	}
	for _, e := range g.Edges {
		b.WriteString(fmt.Sprintf("MATCH (a {id: '%s'}), (b {id: '%s'}) CREATE (a)-[:%s]->(b);\n",
			e.Source, e.Target, strings.Title(e.Relation)))
	}
	return os.WriteFile(outPath, []byte(b.String()), 0644)
}
