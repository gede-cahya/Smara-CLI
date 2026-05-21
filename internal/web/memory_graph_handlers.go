package web

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gede-cahya/Smara-CLI/internal/memory"
)

// asSQLiteStore casts the memory store to its concrete type so we can call
// graph-specific methods. Returns nil if the store doesn't support graphs.
func (s *Server) asSQLiteStore() *memory.SQLiteStore {
	if v, ok := s.MemStore.(*memory.SQLiteStore); ok {
		return v
	}
	return nil
}

// GET /api/memories/graph?limit=N&edge_limit=N&mode=overview|neighborhood|search
// Returns nodes + edges for the memory graph.
func (s *Server) handleMemoryGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	store := s.asSQLiteStore()
	if store == nil {
		errorResponse(w, http.StatusNotImplemented, "memory store tidak mendukung graph")
		return
	}
	limit := 300
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v >= 0 {
		limit = v
	}
	edgeLimit := 1000
	if v, err := strconv.Atoi(r.URL.Query().Get("edge_limit")); err == nil && v >= 0 {
		edgeLimit = v
	}
	minWeight := 0.0
	if v, err := strconv.ParseFloat(r.URL.Query().Get("min_weight"), 64); err == nil && v >= 0 {
		minWeight = v
	}
	focusID := int64(0)
	if v, err := strconv.ParseInt(r.URL.Query().Get("focus_id"), 10, 64); err == nil && v > 0 {
		focusID = v
	}
	depth := 1
	if v, err := strconv.Atoi(r.URL.Query().Get("depth")); err == nil && v > 0 {
		depth = v
	}
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "overview"
	}
	includeAuto := r.URL.Query().Get("auto") != "0"
	includeManual := r.URL.Query().Get("manual") != "0"
	searchQuery := r.URL.Query().Get("q")
	if searchQuery != "" && mode == "overview" {
		mode = "search"
	}
	wsID := s.resolveWorkspaceID()
	if name := r.URL.Query().Get("workspace"); name != "" {
		ws, err := s.MemStore.GetWorkspaceByName(name)
		if err == nil && ws != nil {
			wsID = ws.ID
		}
	}
	data, err := store.BuildGraphWithOptions(wsID, memory.GraphBuildOptions{
		Mode: mode, NodeLimit: limit, EdgeLimit: edgeLimit, MinWeight: minWeight,
		FocusID: focusID, Depth: depth, SearchQuery: searchQuery,
		IncludeAutoLinks: includeAuto, IncludeManualLinks: includeManual,
	})
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, data)
}

// /api/memories/links
//
//	GET ?memory_id=N        → list all links for a memory
//	POST {source,target,relation,weight,note}
//	DELETE ?id=N            → remove link by id
func (s *Server) handleMemoryLinks(w http.ResponseWriter, r *http.Request) {
	store := s.asSQLiteStore()
	if store == nil {
		errorResponse(w, http.StatusNotImplemented, "memory store tidak mendukung graph")
		return
	}
	switch r.Method {
	case http.MethodGet:
		mid, err := strconv.ParseInt(r.URL.Query().Get("memory_id"), 10, 64)
		if err != nil {
			errorResponse(w, http.StatusBadRequest, "memory_id wajib & numeric")
			return
		}
		links, err := store.ListLinksFor(mid)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]interface{}{"links": links})

	case http.MethodPost:
		var req struct {
			SourceID      int64   `json:"source_id"`
			TargetID      int64   `json:"target_id"`
			Relation      string  `json:"relation"`
			Weight        float64 `json:"weight"`
			Note          string  `json:"note"`
			Bidirectional bool    `json:"bidirectional"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errorResponse(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		var link *memory.MemoryLink
		var err error
		if req.Bidirectional {
			link, err = store.AddBidirectionalLink(req.SourceID, req.TargetID, req.Relation, req.Weight, req.Note)
		} else {
			link, err = store.AddLink(req.SourceID, req.TargetID, req.Relation, req.Weight, req.Note)
		}
		if err != nil {
			errorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, link)

	case http.MethodDelete:
		id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil {
			errorResponse(w, http.StatusBadRequest, "id wajib & numeric")
			return
		}
		if err := store.RemoveLink(id); err != nil {
			errorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		errorResponse(w, http.StatusMethodNotAllowed, "GET/POST/DELETE only")
	}
}

// POST /api/memories/autolink
// Body: {threshold:0.78, top_k:5, replace:true}
func (s *Server) handleMemoryAutolink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	store := s.asSQLiteStore()
	if store == nil {
		errorResponse(w, http.StatusNotImplemented, "memory store tidak mendukung graph")
		return
	}
	var req struct {
		Threshold float64 `json:"threshold"`
		TopK      int     `json:"top_k"`
		Replace   bool    `json:"replace"`
		WikiLinks bool    `json:"wikilinks"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Threshold == 0 {
		req.Threshold = 0.78
	}
	if req.TopK == 0 {
		req.TopK = 5
	}
	report, err := store.AutoLinkSmart(memory.AutoLinkOptions{
		WorkspaceID: s.resolveWorkspaceID(),
		Threshold:   req.Threshold,
		MaxPerNode:  req.TopK,
		Replace:     req.Replace,
	})
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	wikiCreated := 0
	if req.WikiLinks {
		wikiCreated, err = store.AutoLinkWikiLinks(s.resolveWorkspaceID())
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"mode":                 report.Mode,
		"created":              report.Created,
		"wikilinks_created":    wikiCreated,
		"memories_scanned":     report.MemoriesScanned,
		"with_embedding":       report.WithEmbedding,
		"embedding_ratio":      report.EmbeddingRatio,
		"threshold":            report.Threshold,
		"top_k":                report.TopK,
		"fell_back_to_lexical": report.FellBackToLexical,
	})
}
