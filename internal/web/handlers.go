package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/gede-cahya/Smara-CLI/internal/skill"
	"github.com/gede-cahya/Smara-CLI/internal/agent/workflow"
)

// --- Status ---

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	mode := s.Supervisor.GetMode()
	modeInfo := agent.GetModeInfo(mode)
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status":      "running",
		"mode":        string(mode),
		"mode_label":  modeInfo.Label,
		"mode_desc":   modeInfo.Description,
		"mode_emoji":  modeInfo.Emoji,
		"provider":    s.Supervisor.GetProvider().Name(),
		"workspace":   s.Cfg.ActiveWorkspace,
		"version":     "1.0.0",
	})
}

// --- Chat (non-streaming fallback) ---

type chatRequest struct {
	Message   string `json:"message"`
	Workspace string `json:"workspace"`
	Mode      string `json:"mode"`
}

type chatResponse struct {
	Response string `json:"response"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	ctx := r.Context()
	result, err := s.Supervisor.ProcessPrompt(ctx, req.Message)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, chatResponse{Response: result.Response})
}

// --- WebSocket ---

type wsMessage struct {
	Type        string `json:"type"`
	Payload     string `json:"payload"`
	Mode        string `json:"mode,omitempty"`
	Phase       string `json:"phase,omitempty"`
	Description string `json:"description,omitempty"`
	Tool        string `json:"tool,omitempty"`
	Server      string `json:"server,omitempty"`
	Output      string `json:"output,omitempty"`
	Args        map[string]interface{} `json:"args,omitempty"`
	Role        string `json:"role,omitempty"`
	Stats       *wsStats `json:"stats,omitempty"`
}

type wsStats struct {
	PromptCount int     `json:"prompt_count"`
	TotalTokens int     `json:"total_tokens"`
	AvgTokens   int     `json:"avg_tokens"`
	Duration    string  `json:"duration"`
	Cost        float64 `json:"cost"`
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	sessionID := fmt.Sprintf("ws-%d-%d", time.Now().Unix(), rand.Intn(10000))
	session := &ChatSession{
		ID:        sessionID,
		Conn:      conn,
		History:   []llm.Message{},
		Workspace: s.Cfg.ActiveWorkspace,
		Mode:      s.Supervisor.GetMode(),
	}

	s.mu.Lock()
	s.sessions[sessionID] = session
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.sessions, sessionID)
		s.mu.Unlock()
	}()

	// Send welcome / connection ack
	_ = conn.WriteJSON(wsMessage{Type: "connected", Payload: sessionID})

	for {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			if _, ok := err.(interface{ Close() bool }); !ok {
				return
			}
			return
		}

		switch msg.Type {
		case "chat":
			go s.handleWSChat(session, msg)
		case "mode_change":
			if agent.ValidMode(msg.Mode) {
				s.Supervisor.SetMode(agent.Mode(msg.Mode))
				mi := agent.GetModeInfo(agent.Mode(msg.Mode))
				_ = conn.WriteJSON(wsMessage{Type: "mode", Mode: msg.Mode, Payload: mi.Label, Description: mi.Description})
			}
		case "ping":
			_ = conn.WriteJSON(wsMessage{Type: "pong"})
		}
	}
}

func (s *Server) handleWSChat(session *ChatSession, msg wsMessage) {
	// Send thinking indicator
	_ = session.WriteJSON(wsMessage{Type: "thinking", Payload: "true"})

	// Wire real-time callbacks for this request
	s.Supervisor.SetCallback(agent.AgenticCallback{
		OnPhaseChange: func(phase, description string) {
			_ = session.WriteJSON(wsMessage{Type: "phase", Phase: phase, Description: description})
		},
		OnToolCall: func(server, tool string, args map[string]interface{}) {
			_ = session.WriteJSON(wsMessage{Type: "tool_call", Server: server, Tool: tool, Args: args})
		},
		OnToolResult: func(output string) {
			_ = session.WriteJSON(wsMessage{Type: "tool_result", Output: output})
		},
		OnLog: func(role, content string) {
			_ = session.WriteJSON(wsMessage{Type: "log", Payload: content, Role: role})
		},
	})

	// Wall-clock cap for the whole agentic turn. Configurable via
	// agent_request_timeout_sec; default in config is 1800s (30 min) so
	// long roadmap chains aren't killed mid-task. Falls back to 30 min if
	// the config field is missing or zero.
	timeoutSec := 1800
	if s.Cfg != nil && s.Cfg.AgentRequestTimeoutSec > 0 {
		timeoutSec = s.Cfg.AgentRequestTimeoutSec
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	prompt := injectAttachmentSteer(msg.Payload)
	result, err := s.Supervisor.ProcessPrompt(ctx, prompt)

	// Clear callbacks
	s.Supervisor.SetCallback(agent.AgenticCallback{})

	_ = session.WriteJSON(wsMessage{Type: "thinking", Payload: "false"})

	if err != nil {
		_ = session.WriteJSON(wsMessage{Type: "error", Payload: err.Error()})
		return
	}

	_ = session.WriteJSON(wsMessage{Type: "chat", Payload: result.Response})

	// Send stats
	stats := s.Supervisor.GetStats()
	var durStr string
	if stats.LastDuration > 0 {
		durStr = stats.LastDuration.Round(time.Millisecond).String()
	}
	_ = session.WriteJSON(wsMessage{Type: "stats", Stats: &wsStats{
		PromptCount: stats.PromptCount,
		TotalTokens: stats.TotalTokens,
		AvgTokens:   stats.AvgTokensPerReq,
		Duration:    durStr,
		Cost:        stats.TotalCost,
	}})
}

// --- Memories ---

func (s *Server) resolveWorkspaceID() int64 {
	wsID := s.Cfg.ActiveWorkspaceID
	if wsID == 0 && s.Cfg.ActiveWorkspace != "" {
		w, err := s.MemStore.GetWorkspaceByName(s.Cfg.ActiveWorkspace)
		if err == nil && w != nil {
			wsID = w.ID
			s.Cfg.ActiveWorkspaceID = w.ID
		}
	}
	return wsID
}

func (s *Server) handleMemories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
		limit = v
	}
	tags := r.URL.Query()["tags"]
	source := r.URL.Query().Get("source")

	// Allow overriding workspace via query param
	wsID := s.resolveWorkspaceID()
	if wsName := r.URL.Query().Get("workspace"); wsName != "" {
		w, err := s.MemStore.GetWorkspaceByName(wsName)
		if err == nil && w != nil {
			wsID = w.ID
		}
	}

	filters := memory.MemoryFilters{
		Limit: limit,
		SortBy: "created_at",
		SortDir: "DESC",
		SearchFilters: memory.SearchFilters{
			Tags:    tags,
			Sources: []string{},
		},
	}
	if source != "" {
		filters.Sources = []string{source}
	}

	mems, total, err := s.MemStore.ListMemoriesWithFilters(wsID, filters)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"memories": mems,
		"total":    total,
	})
}

func (s *Server) handleMemorySearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Limit == 0 {
		req.Limit = 10
	}

	wsID := s.resolveWorkspaceID()
	mems, err := s.MemStore.SearchFullText(req.Query, wsID, memory.MemoryFilters{
		Limit: req.Limit,
		SearchFilters: memory.SearchFilters{MinScore: 0.1},
	})
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"results": mems})
}

// --- Workspaces ---

func (s *Server) handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	wss, err := s.MemStore.ListWorkspaces()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"workspaces": wss,
		"active":     s.Cfg.ActiveWorkspace,
	})
}

func (s *Server) handleWorkspaceSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := config.Set("active_workspace", req.Name); err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Cfg.ActiveWorkspace = req.Name
	jsonResponse(w, http.StatusOK, map[string]string{"status": "switched", "workspace": req.Name})
}

func (s *Server) handleWorkspaceCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Path == "" {
		req.Path = "."
	}
	ws, err := s.MemStore.CreateWorkspace(req.Name, req.Path)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, ws)
}

func (s *Server) handleCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	wsID := s.resolveWorkspaceID()
	if wsName := r.URL.Query().Get("workspace"); wsName != "" {
		w, err := s.MemStore.GetWorkspaceByName(wsName)
		if err == nil && w != nil {
			wsID = w.ID
		}
	}
	cats, err := s.MemStore.ListCategories(wsID, true)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"categories": cats})
}

// --- Config ---

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings := config.AllSettings()
		jsonResponse(w, http.StatusOK, settings)
	case http.MethodPost:
		var req struct {
			Key   string      `json:"key"`
			Value interface{} `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errorResponse(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		valStr := fmt.Sprintf("%v", req.Value)
		if err := config.Set(req.Key, valStr); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"status": "updated"})
	default:
		errorResponse(w, http.StatusMethodNotAllowed, "only GET/POST")
	}
}

// --- MCP Status ---

func (s *Server) handleMCPStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	info := s.Supervisor.GetMCPInfo()
	toolCount := 0
	servers := make([]map[string]interface{}, 0, len(info))
	for name, i := range info {
		servers = append(servers, map[string]interface{}{
			"name":      name,
			"connected": i.Connected,
			"tools":     len(i.Tools),
			"error":     i.Error,
		})
		toolCount += len(i.Tools)
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"servers":    servers,
		"tool_count": toolCount,
	})
}

// --- Metrics ---

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	// Read metrics from file if collector available
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"note":   "metrics from metrics.json",
	})
}

// --- Mode ---

func (s *Server) handleMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"mode":       string(s.Supervisor.GetMode()),
			"mode_info":  agent.GetModeInfo(s.Supervisor.GetMode()),
		})
		return
	}
	var req struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if !agent.ValidMode(req.Mode) {
		errorResponse(w, http.StatusBadRequest, "invalid mode")
		return
	}
	s.Supervisor.SetMode(agent.Mode(req.Mode))
	mi := agent.GetModeInfo(agent.Mode(req.Mode))
	jsonResponse(w, http.StatusOK, mi)
}

// --- Blueprint ---

func (s *Server) handleBlueprintGenerate(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[web] panic in handleBlueprintGenerate: %v", rec)
			errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("internal server error: %v", rec))
		}
	}()
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[web] handleBlueprintGenerate: invalid JSON: %v", err)
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	log.Printf("[web] handleBlueprintGenerate: prompt=%q provider=%q", req.Prompt, s.Supervisor.GetProvider().Name())
	bp, err := workflow.GenerateBlueprintWithProvider(s.Supervisor.GetProvider(), s.Supervisor.GetMCPInfo(), req.Prompt)
	if err != nil {
		log.Printf("[web] handleBlueprintGenerate: failed: %v", err)
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("blueprint generate failed: %v", err))
		return
	}
	log.Printf("[web] handleBlueprintGenerate: success, project=%q", bp.ProjectName)
	jsonResponse(w, http.StatusOK, bp)
}

func (s *Server) handleBlueprintExecute(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[web] panic in handleBlueprintExecute: %v", rec)
			errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("internal server error: %v", rec))
		}
	}()
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req struct {
		Prompt     string `json:"prompt"`
		ProjectDir string `json:"project_dir,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[web] handleBlueprintExecute: invalid JSON: %v", err)
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	var projectDir string
	if req.ProjectDir != "" {
		projectDir = req.ProjectDir
	} else {
		projectDir = filepath.Join(os.TempDir(), fmt.Sprintf("smara-workflow-%d", time.Now().Unix()))
		_ = os.MkdirAll(projectDir, 0755)
	}
	log.Printf("[web] handleBlueprintExecute: prompt=%q projectDir=%q provider=%q", req.Prompt, projectDir, s.Supervisor.GetProvider().Name())
	result, err := workflow.RunWorkflowWithDir(s.Supervisor, s.Supervisor.GetProvider(), req.Prompt, projectDir)
	if err != nil {
		log.Printf("[web] handleBlueprintExecute: failed: %v", err)
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("workflow execution failed: %v", err))
		return
	}
	log.Printf("[web] handleBlueprintExecute: success, projectDir=%q", projectDir)
	jsonResponse(w, http.StatusOK, result)
}

// --- Skills ---

func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		names, err := skill.List()
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		type lineageEntry struct {
			Version     int      `json:"version"`
			Description string   `json:"description,omitempty"`
			Tags        []string `json:"tags,omitempty"`
			StepCount   int      `json:"step_count"`
			RefinedAt   string   `json:"refined_at,omitempty"`
			RefinedFrom string   `json:"refined_from,omitempty"`
		}
		type skillItem struct {
			Name         string         `json:"name"`
			Description  string         `json:"description"`
			Version      int            `json:"version"`
			Tags         []string       `json:"tags"`
			ParentID     string         `json:"parent_id,omitempty"`
			CategoryPath []string       `json:"category_path,omitempty"`
			Dependencies []string       `json:"dependencies,omitempty"`
			Lineage      []lineageEntry `json:"lineage,omitempty"`
		}
		var items []skillItem
		for _, n := range names {
			sk, err := skill.Load(n)
			if err != nil {
				continue
			}
			var lin []lineageEntry
			for _, l := range sk.Lineage {
				lin = append(lin, lineageEntry{
					Version:     l.Version,
					Description: l.Description,
					Tags:        l.Tags,
					StepCount:   l.StepCount,
					RefinedAt:   l.RefinedAt.Format("2006-01-02 15:04"),
					RefinedFrom: l.RefinedFrom,
				})
			}
			items = append(items, skillItem{
				Name:         sk.Name,
				Description:  sk.Description,
				Version:      sk.Version,
				Tags:         sk.Tags,
				ParentID:     sk.ParentID,
				CategoryPath: sk.CategoryPath,
				Dependencies: sk.Dependencies,
				Lineage:      lin,
			})
		}
		jsonResponse(w, http.StatusOK, map[string]interface{}{"skills": items})
	case http.MethodDelete:
		name := r.URL.Query().Get("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "name required")
			return
		}
		if err := skill.Delete(name, nil); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		errorResponse(w, http.StatusMethodNotAllowed, "only GET/DELETE")
	}
}

func (s *Server) handleSkillRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req struct {
		Name string                 `json:"name"`
		Args map[string]interface{} `json:"args,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	sk, err := skill.Load(req.Name)
	if err != nil {
		errorResponse(w, http.StatusNotFound, err.Error())
		return
	}
	// Apply runtime args if provided
	if len(req.Args) > 0 {
		sk = sk.WithArgs(req.Args)
	}
	start := time.Now()
	result, err := sk.Run(s.Supervisor.SkillExecutor())
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.SkillTracker != nil {
		_ = s.SkillTracker.LogRun(sk.Name, fmt.Sprintf("web-%d", time.Now().UnixNano()), "manual", s.Cfg.ActiveWorkspace, string(s.Supervisor.GetMode()), result, start)
	}
	jsonResponse(w, http.StatusOK, result)
}

func (s *Server) handleSkillImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req struct {
		Name   string `json:"name"`
		Format string `json:"format"` // json or md
		Data   string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	var sk *skill.Skill
	var err error
	switch req.Format {
	case "md", "markdown":
		sk, err = skill.ParseMarkdownSkill([]byte(req.Data))
	default:
		sk, err = skill.FromJSON([]byte(req.Data))
	}
	if err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	sk.Name = req.Name
	if req.Format == "md" || req.Format == "markdown" {
		err = skill.SaveAsMarkdown(sk, nil)
	} else {
		err = skill.Save(sk, nil)
	}
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "imported", "name": sk.Name})
}

// --- Skill Tree & Analytics ---

func (s *Server) handleSkillTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	tm, err := skill.BuildTree()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if r.URL.Query().Get("format") == "graph" {
		nodes, edges := tm.ToGraphJSON()
		jsonResponse(w, http.StatusOK, map[string]interface{}{"nodes": nodes, "edges": edges})
		return
	}
	jsonResponse(w, http.StatusOK, tm.AllNodes())
}

func (s *Server) handleSkillTreeSubtree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	name := r.URL.Path[len("/api/skills/tree/"):]
	if name == "" {
		errorResponse(w, http.StatusBadRequest, "name required")
		return
	}
	tm, err := skill.BuildTree()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	subtree, err := tm.GetSubtree(name)
	if err != nil {
		errorResponse(w, http.StatusNotFound, err.Error())
		return
	}
	deps, _ := tm.GetDependencies(name)
	suggest := tm.SuggestNextSkills(name)
	issues := tm.ValidateTree()
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"subtree":      subtree,
		"dependencies": deps,
		"suggestions":  suggest,
		"issues":       issues,
	})
}

func (s *Server) handleSkillStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		errorResponse(w, http.StatusBadRequest, "name required")
		return
	}
	if s.SkillTracker == nil {
		errorResponse(w, http.StatusServiceUnavailable, "tracker not available")
		return
	}
	total, success, avgMs, lastRun, err := s.SkillTracker.GetStats(name)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	rate := 0.0
	if total > 0 {
		rate = float64(success) / float64(total) * 100
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"skill_name":   name,
		"total_runs":   total,
		"success_rate": rate,
		"avg_duration_ms": avgMs,
		"last_run":     lastRun,
	})
}

func (s *Server) handleSkillTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		errorResponse(w, http.StatusBadRequest, "name required")
		return
	}
	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
		limit = v
	}
	if s.SkillTracker == nil {
		errorResponse(w, http.StatusServiceUnavailable, "tracker not available")
		return
	}
	items, err := s.SkillTracker.GetTimeline(name, limit)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"timeline": items})
}

func (s *Server) handleSkillAnalytics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	if s.SkillTracker == nil {
		errorResponse(w, http.StatusServiceUnavailable, "tracker not available")
		return
	}
	analytics, err := s.SkillTracker.GlobalAnalytics()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, analytics)
}

func (s *Server) handleSkillRefine(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	prompt, err := skill.BuildRefinementPrompt(req.Name, nil)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"prompt": prompt})
}

func (s *Server) handleSkillDependencies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req struct {
		Name         string   `json:"name"`
		Dependencies []string `json:"dependencies"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	sk, err := skill.Load(req.Name)
	if err != nil {
		errorResponse(w, http.StatusNotFound, err.Error())
		return
	}
	sk.Dependencies = req.Dependencies
	if err := skill.Save(sk, nil); err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"name": req.Name, "dependencies": req.Dependencies})
}

// --- Bundled Skills ---

func findBundledSkillsDir() string {
	candidates := []string{"skills"}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "skills"))
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		repoRoot := filepath.Join(filepath.Dir(file), "..", "..")
		candidates = append(candidates, filepath.Join(repoRoot, "skills"))
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	return ""
}

func (s *Server) handleSkillsBundled(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	dir := findBundledSkillsDir()
	if dir == "" {
		jsonResponse(w, http.StatusOK, map[string]interface{}{"skills": []interface{}{}})
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	var items []map[string]interface{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var sk skill.Skill
		if err := json.Unmarshal(data, &sk); err != nil {
			continue
		}
		items = append(items, map[string]interface{}{
			"name":        sk.Name,
			"description": sk.Description,
			"version":     sk.Version,
			"tags":        sk.Tags,
		})
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"skills": items})
}

func (s *Server) handleSkillsInstallBundled(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	dir := findBundledSkillsDir()
	if dir == "" {
		errorResponse(w, http.StatusNotFound, "bundled skills directory not found")
		return
	}
	path := filepath.Join(dir, req.Name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		errorResponse(w, http.StatusNotFound, fmt.Sprintf("bundled skill '%s' not found", req.Name))
		return
	}
	sk, err := skill.FromJSON(data)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid bundled skill JSON: "+err.Error())
		return
	}
	if err := skill.Save(sk, nil); err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "installed", "name": sk.Name})
}

// --- Custom Workflow ---

func (s *Server) handleCustomWorkflowList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	items, err := workflow.ListCustomWorkflows()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	var result []map[string]interface{}
	for _, name := range items {
		cw, err := workflow.LoadCustomWorkflow(name)
		if err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"name":        cw.Name,
			"description": cw.Description,
			"agents":      len(cw.Agents),
		})
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"workflows": result})
}

func (s *Server) handleCustomWorkflowGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		errorResponse(w, http.StatusBadRequest, "name required")
		return
	}
	cw, err := workflow.LoadCustomWorkflow(name)
	if err != nil {
		errorResponse(w, http.StatusNotFound, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, cw)
}

func (s *Server) handleCustomWorkflowSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var cw workflow.CustomWorkflow
	if err := json.NewDecoder(r.Body).Decode(&cw); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := workflow.SaveCustomWorkflow(&cw); err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "saved", "name": cw.Name})
}

func (s *Server) handleCustomWorkflowDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST/DELETE")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		name := r.URL.Query().Get("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "name required")
			return
		}
		req.Name = name
	}
	if err := workflow.DeleteCustomWorkflow(req.Name); err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted", "name": req.Name})
}

func (s *Server) handleCustomWorkflowRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req struct {
		Name       string `json:"name"`
		ProjectDir string `json:"project_dir,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	cw, err := workflow.LoadCustomWorkflow(req.Name)
	if err != nil {
		errorResponse(w, http.StatusNotFound, err.Error())
		return
	}
	if req.ProjectDir != "" {
		cw.ProjectDir = req.ProjectDir
	}
	result, err := workflow.RunCustomWorkflow(s.Supervisor, s.Supervisor.GetProvider(), cw)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, result)
}

func (s *Server) handleCustomWorkflowImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req struct {
		Name string `json:"name"`
		JSON string `json:"json"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	cw, err := workflow.CustomWorkflowFromJSON([]byte(req.JSON))
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid workflow JSON: "+err.Error())
		return
	}
	cw.Name = req.Name
	if err := workflow.SaveCustomWorkflow(cw); err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "imported", "name": req.Name})
}

// --- Filesystem Browser ---

func (s *Server) handleFSCwd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"path": cwd})
}

type fsEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
}

func (s *Server) handleFSList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	p := r.URL.Query().Get("path")
	if p == "" {
		cwd, _ := os.Getwd()
		p = cwd
	}
	p, err := filepath.Abs(p)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid path")
		return
	}
	info, err := os.Stat(p)
	if err != nil || !info.IsDir() {
		errorResponse(w, http.StatusBadRequest, "not a directory")
		return
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	result := []fsEntry{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		result = append(result, fsEntry{Name: e.Name(), IsDir: e.IsDir()})
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"path":    p,
		"entries": result,
	})
}


// handleSkillExport returns the entire skill tree as a downloadable JSON
// envelope. GET /api/skills/export?source=machine-name
//
// The response is served with attachment headers so browsers prompt the
// user with a save dialog.
func (s *Server) handleSkillExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}

	source := r.URL.Query().Get("source")

	var db = underlyingDB(s)
	envelope, err := skill.ExportAll(db, source)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	// If client explicitly asks for raw JSON in body (for the app itself),
	// skip attachment headers. Default behavior is download.
	if r.URL.Query().Get("inline") != "1" {
		filename := fmt.Sprintf("smara-skills-%s.json", time.Now().Format("2006-01-02"))
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	}
	w.Header().Set("Content-Type", "application/json")
	if err := skill.WriteExport(w, envelope); err != nil {
		log.Printf("[skill-export] write error: %v", err)
	}
}

// handleSkillTreeImport accepts a JSON envelope in the request body and
// merges it into ~/.smara/skills/. POST /api/skills/import-tree
//
// Body format:
//   {
//     "mode": "overwrite"|"skip"|"rename",   // default overwrite
//     "dry_run": false,                      // default false
//     "envelope": { ... TreeExport ... }     // OR
//     "envelope_json": "<stringified>"       // OR raw body containing
//                                            // a naked TreeExport
//   }
func (s *Server) handleSkillTreeImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}

	var req struct {
		Mode         string            `json:"mode"`
		DryRun       bool              `json:"dry_run"`
		Envelope     *skill.TreeExport `json:"envelope,omitempty"`
		EnvelopeJSON string            `json:"envelope_json,omitempty"`
	}
	// Read the whole body first so we can fall back to parsing it as a raw
	// envelope when the client sends the envelope directly.
	raw, err := readAllLimited(r.Body, 64*1024*1024) // 64 MB cap
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "gagal baca body: "+err.Error())
		return
	}

	var env *skill.TreeExport
	if err := json.Unmarshal(raw, &req); err == nil && (req.Envelope != nil || req.EnvelopeJSON != "") {
		if req.Envelope != nil {
			env = req.Envelope
		} else if req.EnvelopeJSON != "" {
			e, err := skill.ReadExport(strings.NewReader(req.EnvelopeJSON))
			if err != nil {
				errorResponse(w, http.StatusBadRequest, "envelope_json invalid: "+err.Error())
				return
			}
			env = e
		}
	} else {
		// Fall back: entire body is a TreeExport envelope.
		e, err := skill.ReadExport(strings.NewReader(string(raw)))
		if err != nil {
			errorResponse(w, http.StatusBadRequest, "body bukan TreeExport valid: "+err.Error())
			return
		}
		env = e
	}

	mode, err := skill.ValidateImportModeString(req.Mode)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	db := underlyingDB(s)
	result, err := skill.ImportAll(db, env, mode, req.DryRun)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, result)
}

// underlyingDB returns the shared *sql.DB handle from the skill tracker
// so export/import can read/write the auto_skill_patterns table. Returns
// nil if no tracker is configured; skill JSON still works in that case.
func underlyingDB(s *Server) *sql.DB {
	if s == nil || s.SkillTracker == nil {
		return nil
	}
	return s.SkillTracker.DB()
}

// readAllLimited reads up to max bytes and returns an error if exceeded.
func readAllLimited(r io.Reader, max int64) ([]byte, error) {
	limited := &io.LimitedReader{R: r, N: max + 1}
	buf, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(buf)) > max {
		return nil, fmt.Errorf("request body terlalu besar (>%d bytes)", max)
	}
	return buf, nil
}
