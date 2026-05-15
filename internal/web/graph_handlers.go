package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gede-cahya/Smara-CLI/internal/graphify"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
)

// --- Graph API Handlers ---

func (s *Server) handleGraphInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Path == "" {
		req.Path = "."
	}

	memStore, ok := s.MemStore.(*memory.SQLiteStore)
	if !ok {
		errorResponse(w, http.StatusServiceUnavailable, "memory store unavailable")
		return
	}
	gs, err := graphify.NewGraphStore(memStore.DB())
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("graph store: %v", err))
		return
	}

	g, err := graphify.ParseGoCodebase(req.Path, req.Name)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("parse: %v", err))
		return
	}
	g.AssignCommunities()
	if err := gs.SaveGraph(g); err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("save: %v", err))
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"graph_id":   req.Name,
		"node_count": g.NodeCount(),
		"edge_count": g.EdgeCount(),
		"root_path":  req.Path,
	})
}

func (s *Server) handleGraphList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	memStore, ok := s.MemStore.(*memory.SQLiteStore)
	if !ok {
		errorResponse(w, http.StatusServiceUnavailable, "memory store unavailable")
		return
	}
	gs, err := graphify.NewGraphStore(memStore.DB())
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("graph store: %v", err))
		return
	}
	graphs, err := gs.ListGraphs()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"graphs": graphs})
}

func (s *Server) handleGraphGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	graphID := r.PathValue("id")
	if graphID == "" {
		graphID = r.URL.Query().Get("id")
	}
	if graphID == "" {
		errorResponse(w, http.StatusBadRequest, "graph id required")
		return
	}

	memStore, ok := s.MemStore.(*memory.SQLiteStore)
	if !ok {
		errorResponse(w, http.StatusServiceUnavailable, "memory store unavailable")
		return
	}
	gs, err := graphify.NewGraphStore(memStore.DB())
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("graph store: %v", err))
		return
	}
	g, err := gs.LoadGraph(graphID)
	if err != nil {
		errorResponse(w, http.StatusNotFound, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"graph_id":   g.ID,
		"root_path":  g.RootPath,
		"node_count": g.NodeCount(),
		"edge_count": g.EdgeCount(),
		"languages":  g.Languages(),
	})
}

func (s *Server) handleGraphNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	graphID := r.PathValue("id")
	if graphID == "" {
		graphID = r.URL.Query().Get("id")
	}
	if graphID == "" {
		errorResponse(w, http.StatusBadRequest, "graph id required")
		return
	}

	memStore, ok := s.MemStore.(*memory.SQLiteStore)
	if !ok {
		errorResponse(w, http.StatusServiceUnavailable, "memory store unavailable")
		return
	}
	gs, err := graphify.NewGraphStore(memStore.DB())
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("graph store: %v", err))
		return
	}
	g, err := gs.LoadGraph(graphID)
	if err != nil {
		errorResponse(w, http.StatusNotFound, err.Error())
		return
	}

	nodeType := r.URL.Query().Get("type")
	lang := r.URL.Query().Get("language")
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
		limit = v
	}

	var nodes []*graphify.Node
	for _, n := range g.Nodes {
		if nodeType != "" && n.Type != nodeType {
			continue
		}
		if lang != "" && n.Language != lang {
			continue
		}
		nodes = append(nodes, n)
		if len(nodes) >= limit {
			break
		}
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"nodes": nodes,
		"total": g.NodeCount(),
	})
}

func (s *Server) handleGraphQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	graphID := r.PathValue("id")
	if graphID == "" {
		graphID = r.URL.Query().Get("id")
	}
	if graphID == "" {
		errorResponse(w, http.StatusBadRequest, "graph id required")
		return
	}
	q := r.URL.Query().Get("q")
	if q == "" {
		errorResponse(w, http.StatusBadRequest, "q parameter required")
		return
	}
	depth := 2
	if d, err := strconv.Atoi(r.URL.Query().Get("depth")); err == nil && d > 0 {
		depth = d
	}

	memStore, ok := s.MemStore.(*memory.SQLiteStore)
	if !ok {
		errorResponse(w, http.StatusServiceUnavailable, "memory store unavailable")
		return
	}
	gs, err := graphify.NewGraphStore(memStore.DB())
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("graph store: %v", err))
		return
	}
	g, err := gs.LoadGraph(graphID)
	if err != nil {
		errorResponse(w, http.StatusNotFound, err.Error())
		return
	}
	result := g.Query(q, depth)
	jsonResponse(w, http.StatusOK, result)
}

func (s *Server) handleGraphNeighbors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	graphID := r.PathValue("id")
	if graphID == "" {
		graphID = r.URL.Query().Get("id")
	}
	nodeID := r.PathValue("nodeId")
	if nodeID == "" {
		nodeID = r.URL.Query().Get("node_id")
	}
	if graphID == "" || nodeID == "" {
		errorResponse(w, http.StatusBadRequest, "graph id and node id required")
		return
	}
	depth := 2
	if d, err := strconv.Atoi(r.URL.Query().Get("depth")); err == nil && d > 0 {
		depth = d
	}

	memStore, ok := s.MemStore.(*memory.SQLiteStore)
	if !ok {
		errorResponse(w, http.StatusServiceUnavailable, "memory store unavailable")
		return
	}
	gs, err := graphify.NewGraphStore(memStore.DB())
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("graph store: %v", err))
		return
	}
	g, err := gs.LoadGraph(graphID)
	if err != nil {
		errorResponse(w, http.StatusNotFound, err.Error())
		return
	}
	nbrIDs := g.GetNeighborsByDepth(nodeID, depth)
	var nodes []*graphify.Node
	for _, id := range nbrIDs {
		if n, ok := g.Nodes[id]; ok {
			nodes = append(nodes, n)
		}
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"node_id":   nodeID,
		"depth":     depth,
		"neighbors": nodes,
		"count":     len(nodes),
	})
}

func (s *Server) handleGraphPath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	graphID := r.PathValue("id")
	if graphID == "" {
		graphID = r.URL.Query().Get("id")
	}
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if graphID == "" || from == "" || to == "" {
		errorResponse(w, http.StatusBadRequest, "id, from, and to required")
		return
	}

	memStore, ok := s.MemStore.(*memory.SQLiteStore)
	if !ok {
		errorResponse(w, http.StatusServiceUnavailable, "memory store unavailable")
		return
	}
	gs, err := graphify.NewGraphStore(memStore.DB())
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("graph store: %v", err))
		return
	}
	g, err := gs.LoadGraph(graphID)
	if err != nil {
		errorResponse(w, http.StatusNotFound, err.Error())
		return
	}
	result := g.FindPath(from, to)
	if result == nil {
		jsonResponse(w, http.StatusOK, map[string]interface{}{"path": nil})
		return
	}
	jsonResponse(w, http.StatusOK, result)
}

func (s *Server) handleGraphData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	graphID := r.URL.Query().Get("id")
	if graphID == "" {
		errorResponse(w, http.StatusBadRequest, "graph id required")
		return
	}

	memStore, ok := s.MemStore.(*memory.SQLiteStore)
	if !ok {
		errorResponse(w, http.StatusServiceUnavailable, "memory store unavailable")
		return
	}
	gs, err := graphify.NewGraphStore(memStore.DB())
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("graph store: %v", err))
		return
	}
	g, err := gs.LoadGraph(graphID)
	if err != nil {
		errorResponse(w, http.StatusNotFound, err.Error())
		return
	}

	const maxNodes = 500
	var nodes []*graphify.Node
	for _, n := range g.Nodes {
		nodes = append(nodes, n)
		if len(nodes) >= maxNodes {
			break
		}
	}

	nodeIDs := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		nodeIDs[n.ID] = true
	}

	var edges []*graphify.Edge
	for _, e := range g.Edges {
		if nodeIDs[e.Source] && nodeIDs[e.Target] {
			edges = append(edges, e)
		}
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"graph_id":   g.ID,
		"root_path":  g.RootPath,
		"node_count": g.NodeCount(),
		"edge_count": g.EdgeCount(),
		"truncated":  g.NodeCount() > maxNodes,
		"nodes":      nodes,
		"edges":      edges,
	})
}

func (s *Server) handleGraphExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	graphID := r.PathValue("id")
	if graphID == "" {
		graphID = r.URL.Query().Get("id")
	}
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	memStore, ok := s.MemStore.(*memory.SQLiteStore)
	if !ok {
		errorResponse(w, http.StatusServiceUnavailable, "memory store unavailable")
		return
	}
	gs, err := graphify.NewGraphStore(memStore.DB())
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("graph store: %v", err))
		return
	}
	g, err := gs.LoadGraph(graphID)
	if err != nil {
		errorResponse(w, http.StatusNotFound, err.Error())
		return
	}

	switch format {
	case "json":
		data, err := g.ToJSON()
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.json\"", graphID))
		w.Write(data)
	default:
		errorResponse(w, http.StatusBadRequest, fmt.Sprintf("unsupported format: %s", format))
	}
}
