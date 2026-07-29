package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/agent/workflow"
	"github.com/gede-cahya/Smara-CLI/internal/browser"
	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/imageflow"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/gede-cahya/Smara-CLI/internal/metrics"
	"github.com/gede-cahya/Smara-CLI/internal/orchestration"
	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

// --- Status ---

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	mode := s.Supervisor.GetMode()
	modeInfo := agent.GetModeInfo(mode)
	providerEndpoint := providerHealthEndpoint(s.Cfg)
	providerOnline, providerError := providerReachable(providerEndpoint)
	provider, model := s.currentProviderModel()

	// Build 9Router-specific info
	var router9Info map[string]interface{}
	if s.Cfg != nil && s.Cfg.Provider == "custom" {
		router9Info = map[string]interface{}{
			"base_url":       s.Cfg.CustomBaseURL,
			"provider_name":  s.Cfg.CustomProviderName,
			"model":          model,
			"native_tool":    llm.ModelSupportsNativeToolCall(model),
			"stream_disabled": s.Cfg.CustomDisableStream,
		}
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status":            "running",
		"mode":              string(mode),
		"mode_label":        modeInfo.Label,
		"mode_desc":         modeInfo.Description,
		"mode_emoji":        modeInfo.Emoji,
		"provider":          provider,
		"model":             model,
		"provider_online":   providerOnline,
		"provider_endpoint": providerEndpoint,
		"provider_error":    providerError,
		"workspace":         s.Cfg.ActiveWorkspace,
		"version":           "1.0.0",
		"web_sessions":      s.WebSessions != nil,
		"router9":           router9Info,
	})
}

func providerHealthEndpoint(cfg *config.SmaraConfig) string {
	if cfg == nil {
		return ""
	}
	switch cfg.Provider {
	case "ollama":
		return cfg.OllamaHost
	case "custom":
		return cfg.CustomBaseURL
	case "openai":
		if cfg.OpenAIBaseURL != "" {
			return cfg.OpenAIBaseURL
		}
		return "https://api.openai.com"
	case "openrouter":
		return "https://openrouter.ai"
	case "anthropic":
		return "https://api.anthropic.com"
	default:
		return ""
	}
}

func providerReachable(endpoint string) (bool, string) {
	if endpoint == "" {
		return false, "endpoint provider belum dikonfigurasi"
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Hostname() == "" {
		return false, "endpoint provider tidak valid"
	}
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(parsed.Hostname(), port), 450*time.Millisecond)
	if err != nil {
		return false, err.Error()
	}
	_ = conn.Close()
	return true, ""
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

func allowsWorkflowModeAction(requestMode string, fallback agent.Mode) bool {
	if agent.ValidMode(requestMode) {
		return agent.Mode(requestMode) == agent.ModeWorkflow
	}
	return fallback == agent.ModeWorkflow
}

func effectiveRequestMode(requestMode string, fallback agent.Mode) agent.Mode {
	if agent.ValidMode(requestMode) {
		return agent.Mode(requestMode)
	}
	return fallback
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
	if allowsWorkflowModeAction(req.Mode, s.Supervisor.GetMode()) {
		if response, handled, err := s.tryRunCustomWorkflowPrompt(req.Message); handled {
			if err != nil {
				errorResponse(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonResponse(w, http.StatusOK, chatResponse{Response: llm.SanitizeForUser(response)})
			return
		}
		if response, handled, err := s.tryCreateCustomWorkflowPrompt(req.Message); handled {
			if err != nil {
				errorResponse(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonResponse(w, http.StatusOK, chatResponse{Response: llm.SanitizeForUser(response)})
			return
		}
	}
	requestMode := effectiveRequestMode(req.Mode, s.Supervisor.GetMode())
	if requestMode == agent.ModeParallel && orchestration.IsAgentSwarmWorkflowPrompt(req.Message) {
		log.Printf("[web] Routing chat prompt to Agent Swarm Workflow")
		runID := fmt.Sprintf("web-planning-%d", time.Now().UnixNano())
		s.OrchestrationStore.StartPlanning(runID, "Agent Swarm Workflow", req.Message)
		result, err := s.runWorkflowWithLiveStatus(ctx, req.Message)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("agent swarm workflow gagal: %v", err))
			return
		}
		jsonResponse(w, http.StatusOK, chatResponse{Response: llm.SanitizeForUser(formatAgentSwarmCompletion(result, 0))})
		return
	}
	if orchestration.ShouldAutoParallelOrchestrate(req.Message, requestMode) {
		log.Printf("[web] Auto-routing complex chat prompt to parallel orchestration")
		result, err := s.runWorkflowWithLiveStatus(ctx, req.Message)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("auto parallel orchestration gagal: %v", err))
			return
		}
		jsonResponse(w, http.StatusOK, chatResponse{Response: llm.SanitizeForUser(formatAutoParallelCompletion(result, 0))})
		return
	}
	result, err := s.Supervisor.ProcessPrompt(ctx, req.Message)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, chatResponse{Response: s.rewriteGeneratedImageLinks(llm.SanitizeForUser(result.Response))})
}

func (s *Server) handleGeneratedImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		errorResponse(w, http.StatusBadRequest, "path required")
		return
	}
	allowed, err := s.generatedImagePathAllowed(path)
	if err != nil {
		errorResponse(w, http.StatusForbidden, err.Error())
		return
	}
	info, err := os.Stat(allowed)
	if err != nil || info.IsDir() {
		errorResponse(w, http.StatusNotFound, "image not found")
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filepath.Base(allowed)))
	http.ServeFile(w, r, allowed)
}

func (s *Server) handleLocalImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		errorResponse(w, http.StatusBadRequest, "path required")
		return
	}
	allowed, err := localImagePathAllowed(path)
	if err != nil {
		errorResponse(w, http.StatusForbidden, err.Error())
		return
	}
	info, err := os.Stat(allowed)
	if err != nil || info.IsDir() {
		errorResponse(w, http.StatusNotFound, "image not found")
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filepath.Base(allowed)))
	http.ServeFile(w, r, allowed)
}

func localImagePathAllowed(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	ext := strings.ToLower(filepath.Ext(absPath))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".webp" {
		return "", fmt.Errorf("hanya file gambar yang diizinkan")
	}
	dirs := []string{"/tmp", os.TempDir()}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".smara", "clip-images"), filepath.Join(home, ".smara", "images"))
	}
	for _, dir := range dirs {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(absDir, absPath)
		if err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
			return absPath, nil
		}
	}
	return "", fmt.Errorf("path di luar direktori gambar lokal yang diizinkan")
}

func (s *Server) rewriteGeneratedImageLinks(text string) string {
	return rewriteGeneratedImageMarkdown(text, func(path string) (string, error) {
		if _, err := s.generatedImagePathAllowed(path); err != nil {
			return "", err
		}
		return "/api/generated-image?path=" + url.QueryEscape(path), nil
	})
}

func rewriteGeneratedImageMarkdown(text string, rewrite func(string) (string, error)) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		start := strings.Index(text[i:], "![")
		if start < 0 {
			b.WriteString(text[i:])
			break
		}
		start += i
		b.WriteString(text[i:start])

		labelEnd := strings.Index(text[start:], "](")
		if labelEnd < 0 {
			b.WriteString(text[start:])
			break
		}
		labelEnd += start
		pathStart := labelEnd + 2
		pathEndRel := strings.Index(text[pathStart:], ")")
		if pathEndRel < 0 {
			b.WriteString(text[start:])
			break
		}
		pathEnd := pathStart + pathEndRel
		path := text[pathStart:pathEnd]

		newPath, err := rewrite(path)
		if err != nil {
			b.WriteString(text[start : pathEnd+1])
		} else {
			b.WriteString(text[start:pathStart])
			b.WriteString(newPath)
			b.WriteString(")")
		}
		i = pathEnd + 1
	}
	return b.String()
}

func (s *Server) generatedImagePathAllowed(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	outputDir := ""
	if s.Cfg != nil {
		outputDir = s.Cfg.ImageOutputDir
	}
	if outputDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		outputDir = filepath.Join(home, ".smara", "images")
	}
	absDir, err := filepath.Abs(outputDir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path di luar direktori gambar")
	}
	return absPath, nil
}

func (s *Server) handleBrowserArtifact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		errorResponse(w, http.StatusBadRequest, "path required")
		return
	}
	allowed, err := browserArtifactPathAllowed(path)
	if err != nil {
		errorResponse(w, http.StatusForbidden, err.Error())
		return
	}
	info, err := os.Stat(allowed)
	if err != nil || info.IsDir() {
		errorResponse(w, http.StatusNotFound, "artifact not found")
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filepath.Base(allowed)))
	http.ServeFile(w, r, allowed)
}

func browserArtifactPathAllowed(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	ext := strings.ToLower(filepath.Ext(absPath))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".webp" && ext != ".md" {
		return "", fmt.Errorf("hanya file gambar/report browser yang diizinkan")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(home, ".smara", "artifacts", "browser-runs")
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path di luar direktori artifact browser")
	}
	return absPath, nil
}

func (s *Server) runBrowserTest(ctx context.Context, prompt string) (string, error) {
	task, err := browser.Plan(prompt)
	if err != nil {
		return "", err
	}
	res, runErr := browser.Run(ctx, task, browser.Options{Timeout: 45 * time.Second})
	return s.formatBrowserTestResponse(task, res, runErr), nil
}

func (s *Server) formatBrowserTestResponse(task browser.Task, res browser.Result, runErr error) string {
	var sb strings.Builder
	sb.WriteString("## Web Test Result\n\n")
	sb.WriteString(fmt.Sprintf("Status: %s\n", res.Status))
	sb.WriteString("Browser: Chromium via go-rod\n")
	sb.WriteString(fmt.Sprintf("URL: %s\n", task.URL))
	if runErr != nil {
		sb.WriteString(fmt.Sprintf("Detail: %s\n", runErr.Error()))
	}
	if res.ScreenshotPath != "" {
		imgURL := "/api/browser-artifact?path=" + url.QueryEscape(res.ScreenshotPath)
		sb.WriteString(fmt.Sprintf("Output: %s\n", res.ScreenshotPath))
		sb.WriteString(fmt.Sprintf("Screenshot: %s\n\n", imgURL))
		sb.WriteString(fmt.Sprintf("![Browser Screenshot](%s)\n\n", imgURL))
	}
	if res.ReportPath != "" {
		sb.WriteString(fmt.Sprintf("Report: %s\n\n", res.ReportPath))
	}
	sb.WriteString("### Steps\n")
	steps := res.Steps
	if len(steps) == 0 {
		for _, st := range task.Steps {
			steps = append(steps, browser.StepResult{Step: st, Status: "planned"})
		}
	}
	for i, sr := range steps {
		line := fmt.Sprintf("%d. %s `%s` — %s", i+1, sr.Step.Action, sr.Step.Target, sr.Status)
		if sr.Error != "" {
			line += ": " + sr.Error
		}
		sb.WriteString(line + "\n")
	}
	sb.WriteString("\nContoh flow Test: `Buka <URL> klik <teks> tunggu <teks> ambil screenshot`.\n")
	return sb.String()
}

// --- WebSocket ---

type wsMessage struct {
	Type          string                 `json:"type"`
	Payload       string                 `json:"payload"`
	SessionID     string                 `json:"session_id,omitempty"`
	RunID         string                 `json:"run_id,omitempty"`
	Mode          string                 `json:"mode,omitempty"`
	Phase         string                 `json:"phase,omitempty"`
	Description   string                 `json:"description,omitempty"`
	Tool          string                 `json:"tool,omitempty"`
	Server        string                 `json:"server,omitempty"`
	Output        string                 `json:"output,omitempty"`
	Args          map[string]interface{} `json:"args,omitempty"`
	Role          string                 `json:"role,omitempty"`
	Stats         *wsStats               `json:"stats,omitempty"`
	RequestPrompt string                 `json:"request_prompt,omitempty"`
	Provider      string                 `json:"provider,omitempty"`
	Model         string                 `json:"model,omitempty"`
}

type wsStats struct {
	PromptCount      int     `json:"prompt_count"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	AvgTokens        int     `json:"avg_tokens"`
	Duration         string  `json:"duration"`
	DurationMs       int64   `json:"duration_ms,omitempty"`
	Cost             float64 `json:"cost"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd,omitempty"`
}

func (s *Server) currentProviderModel() (string, string) {
	provider := "unknown"
	model := ""
	if s.Supervisor != nil {
		provider = s.Supervisor.GetProviderName()
		model = s.Supervisor.GetModel()
	}
	if model == "" && s.Cfg != nil {
		model = s.Cfg.Model
	}
	return provider, model
}

func promptResultWSStats(result *agent.PromptResult, provider, model string) *wsStats {
	if result == nil {
		return nil
	}
	duration := ""
	if result.Duration > 0 {
		duration = result.Duration.Round(time.Millisecond).String()
	}
	cost := metrics.EstimateCost(provider, model, int64(result.InputTokens), int64(result.OutputTokens))
	return &wsStats{
		InputTokens:      result.InputTokens,
		OutputTokens:     result.OutputTokens,
		TotalTokens:      result.TotalTokens,
		Duration:         duration,
		DurationMs:       result.Duration.Milliseconds(),
		Cost:             cost,
		EstimatedCostUSD: cost,
	}
}

func (s *Server) chatWSMessage(sessionID, payload, requestPrompt string, result *agent.PromptResult) wsMessage {
	provider, model := s.currentProviderModel()
	// Prefer the per-session model so the label matches what that specific
	// session's supervisor actually used. The global providerCfg and main
	// supervisor can lag behind after a model switch, causing a stale/wrong
	// model label (e.g. showing "kimi" when the session actually used "glm-5.2").
	if s.WebSessions != nil && sessionID != "" {
		if p, m, ok := s.WebSessions.SessionModelInfo(sessionID); ok {
			provider = p
			model = m
		}
	}
	// Fallback to global providerCfg only if per-session lookup failed.
	if s.WebSessions != nil && (provider == "" || model == "" || provider == "unknown") {
		cfg := s.WebSessions.ProviderConfig()
		if cfg.Name != "" && provider == "unknown" {
			provider = cfg.Name
		}
		if cfg.Model != "" && model == "" {
			model = cfg.Model
		}
	}
	// FINAL sanitization boundary — no DSML tags should ever reach the WS client
	payload = llm.SanitizeForUser(payload)
	return wsMessage{
		Type:          "chat",
		SessionID:     sessionID,
		Payload:       payload,
		RequestPrompt: requestPrompt,
		Provider:      provider,
		Model:         model,
		Stats:         promptResultWSStats(result, provider, model),
	}
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

	_ = conn.WriteJSON(wsMessage{Type: "connected", Payload: sessionID})

	for {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}

		switch msg.Type {
		case "chat":
			if s.WebSessions != nil && msg.SessionID != "" {
				go s.handleWSWebSessionChat(conn, msg)
			} else {
				go s.handleWSChat(session, msg)
			}
		case "session":
			if msg.Payload != "" {
				session.ID = msg.Payload
			}
		case "mode_change":
			if agent.ValidMode(msg.Mode) {
				s.Supervisor.SetMode(agent.Mode(msg.Mode))
				mi := agent.GetModeInfo(agent.Mode(msg.Mode))
				_ = conn.WriteJSON(wsMessage{Type: "mode", Mode: msg.Mode, Payload: mi.Label, Description: mi.Description})
			}
		case "cancel":
			if s.WebSessions != nil && msg.SessionID != "" {
				_ = s.WebSessions.Cancel(msg.SessionID)
				_ = conn.WriteJSON(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "cancelled"})
			}
		case "ping":
			_ = conn.WriteJSON(wsMessage{Type: "pong"})
		}
	}
}

func (s *Server) handleWSChat(session *ChatSession, msg wsMessage) {
	_ = session.WriteJSON(wsMessage{Type: "thinking", Payload: "true"})

	s.Supervisor.SetCallback(agent.AgenticCallback{
		OnPhaseChange: func(phase, description string) {
			_ = session.WriteJSON(wsMessage{Type: "phase", Phase: phase, Description: description})
		},
		OnToolCall: func(server, tool string, args map[string]interface{}) {
			_ = session.WriteJSON(wsMessage{Type: "tool_call", Server: server, Tool: tool, Args: args})
		},
		OnToolResult: func(output string) {
			preview := formatToolResultPreview(s.rewriteGeneratedImageLinks(output))
			_ = session.WriteJSON(wsMessage{Type: "tool_result", Output: preview})
		},
		OnStream: func(chunk string, isThinking bool) {
			_ = session.WriteJSON(wsMessage{Type: "stream", Payload: s.rewriteGeneratedImageLinks(chunk), Args: map[string]interface{}{"is_thinking": isThinking}})
		},
		OnLog: func(role, content string) {
			if role == "tool_progress" {
				var ev wsToolProgressEvent
				if err := json.Unmarshal([]byte(content), &ev); err == nil && ev.Event != "" {
					level := "info"
					if strings.Contains(strings.ToLower(ev.Event), "error") || strings.Contains(strings.ToLower(ev.Event), "timeout") {
						level = "warn"
					}
					_ = session.WriteJSON(wsMessage{
						Type:    "process_log",
						Payload: ev.Message,
						Role:    "process",
						Args: map[string]interface{}{
							"event":   ev.Event,
							"level":   level,
							"message": ev.Message,
							"tool":    ev.Tool,
							"details": ev.Details,
						},
					})
					return
				}
			}
			_ = session.WriteJSON(wsMessage{Type: "log", Payload: content, Role: role})
		},
	})

	timeoutSec := 1800
	if s.Cfg != nil && s.Cfg.AgentRequestTimeoutSec > 0 {
		timeoutSec = s.Cfg.AgentRequestTimeoutSec
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	prompt := injectAttachmentSteer(msg.Payload)
	activeMode := msg.Mode
	if activeMode == "" {
		activeMode = string(s.Supervisor.GetMode())
	}
	if agent.Mode(activeMode) == agent.ModeWorkflow {
		if response, handled, err := s.tryRunCustomWorkflowPrompt(msg.Payload); handled {
			_ = session.WriteJSON(wsMessage{Type: "thinking", Payload: "false"})
			if err != nil {
				_ = session.WriteJSON(wsMessage{Type: "error", Payload: err.Error()})
				return
			}
			_ = session.WriteJSON(wsMessage{Type: "chat", Payload: response, RequestPrompt: msg.Payload})
			s.recordSimpleUsageEvent("workflow", msg.Payload, response)
			return
		}
		if response, handled, err := s.tryCreateCustomWorkflowPrompt(msg.Payload); handled {
			_ = session.WriteJSON(wsMessage{Type: "thinking", Payload: "false"})
			if err != nil {
				_ = session.WriteJSON(wsMessage{Type: "error", Payload: err.Error()})
				return
			}
			_ = session.WriteJSON(wsMessage{Type: "chat", Payload: response, RequestPrompt: msg.Payload})
			s.recordSimpleUsageEvent("workflow", msg.Payload, response)
			return
		}
	}
	if orchestration.ShouldAutoParallelOrchestrate(msg.Payload, agent.Mode(activeMode)) {
		log.Printf("[web] Auto-routing complex websocket prompt to parallel orchestration")
		result, err := s.runWorkflowWithLiveStatus(ctx, msg.Payload)
		_ = session.WriteJSON(wsMessage{Type: "thinking", Payload: "false"})
		if err != nil {
			_ = session.WriteJSON(wsMessage{Type: "error", Payload: fmt.Sprintf("auto parallel orchestration gagal: %v", err)})
			return
		}
		summary := strings.TrimSpace(result.FinalSummary)
		if summary == "" {
			summary = "Workflow selesai tanpa ringkasan tambahan."
		}
		_ = session.WriteJSON(wsMessage{Type: "chat", Payload: fmt.Sprintf("✅ Auto parallel orchestration selesai\n\n%s\nProject: %s", summary, result.ProjectPath), RequestPrompt: msg.Payload})
		s.recordSimpleUsageEvent("parallel", msg.Payload, summary)
		return
	}

	if activeMode == string(agent.ModeTest) && browser.IsBrowserPrompt(msg.Payload) {
		_ = session.WriteJSON(wsMessage{Type: "tool_call", Server: "browser", Tool: "browser_run", Args: map[string]interface{}{"prompt": msg.Payload}})
		output, err := s.runBrowserTest(ctx, msg.Payload)
		_ = session.WriteJSON(wsMessage{Type: "thinking", Payload: "false"})
		if err != nil {
			_ = session.WriteJSON(wsMessage{Type: "error", Payload: err.Error()})
			return
		}
		_ = session.WriteJSON(wsMessage{Type: "tool_result", Output: output})
		_ = session.WriteJSON(wsMessage{Type: "chat", Payload: output, RequestPrompt: msg.Payload})
		return
	}

	result, err := s.Supervisor.ProcessPrompt(ctx, prompt)
	s.Supervisor.SetCallback(agent.AgenticCallback{})
	_ = session.WriteJSON(wsMessage{Type: "thinking", Payload: "false"})
	if err != nil {
		_ = session.WriteJSON(wsMessage{Type: "error", Payload: err.Error()})
		return
	}
	_ = session.WriteJSON(s.chatWSMessage("", s.rewriteGeneratedImageLinks(result.Response), msg.Payload, result))

	stats := s.Supervisor.GetStats()
	var durStr string
	if stats.LastDuration > 0 {
		durStr = stats.LastDuration.Round(time.Millisecond).String()
	}
	_ = session.WriteJSON(wsMessage{Type: "stats", Stats: &wsStats{
		PromptCount:      stats.PromptCount,
		InputTokens:      stats.InputTokens,
		OutputTokens:     stats.OutputTokens,
		TotalTokens:      stats.TotalTokens,
		AvgTokens:        stats.AvgTokensPerReq,
		Duration:         durStr,
		DurationMs:       stats.LastDuration.Milliseconds(),
		Cost:             stats.TotalCost,
		EstimatedCostUSD: stats.TotalCost,
	}})

	if s.Cfg != nil {
		path := metrics.DefaultAnalyticsPath(s.Cfg.DBPath)
		provider, model := s.currentProviderModel()
		log.Printf("[analytics] recording: provider=%s model=%s in=%d out=%d total=%d", provider, model, result.InputTokens, result.OutputTokens, result.TotalTokens)
		if err := metrics.AppendUsageEvent(path, metrics.UsageEvent{
			Timestamp:    time.Now(),
			Provider:     provider,
			Model:        model,
			PromptCount:  1,
			RequestCount: 1,
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
			TotalTokens:  result.TotalTokens,
			CostUSD:      metrics.EstimateCost(provider, model, int64(result.InputTokens), int64(result.OutputTokens)),
			DurationMs:   result.Duration.Milliseconds(),
			Workspace:    s.Cfg.ActiveWorkspace,
		}); err != nil {
			log.Printf("[analytics] FAILED to record: %v", err)
		} else {
			log.Printf("[analytics] recorded OK to %s", path)
		}
	} else {
		log.Printf("[analytics] SKIPPED: s.Cfg is nil")
	}
}

// recordSimpleUsageEvent records a basic analytics event for non-ProcessPrompt
// paths (workflow, parallel orchestration, etc.) using character-based token estimation.
func (s *Server) recordSimpleUsageEvent(eventType, prompt, response string) {
	if s.Cfg == nil {
		return
	}
	path := metrics.DefaultAnalyticsPath(s.Cfg.DBPath)
	provider, model := s.currentProviderModel()
	inputTokens := len(prompt) / 4
	outputTokens := len(response) / 4
	log.Printf("[analytics] recording %s: provider=%s model=%s in=%d out=%d", eventType, provider, model, inputTokens, outputTokens)
	if err := metrics.AppendUsageEvent(path, metrics.UsageEvent{
		Timestamp:    time.Now(),
		Provider:     provider,
		Model:        model,
		PromptCount:  1,
		RequestCount: 1,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  inputTokens + outputTokens,
		CostUSD:      metrics.EstimateCost(provider, model, int64(inputTokens), int64(outputTokens)),
		DurationMs:   0,
		Workspace:    s.Cfg.ActiveWorkspace,
	}); err != nil {
		log.Printf("[analytics] FAILED to record %s: %v", eventType, err)
	}
}

func (s *Server) tryRunCustomWorkflowPrompt(prompt string) (string, bool, error) {
	return s.tryRunCustomWorkflowPromptWithProgress(prompt, nil)
}

func (s *Server) tryRunCustomWorkflowPromptWithProgress(prompt string, onProgress func(event, message, role, taskID string, details map[string]interface{})) (string, bool, error) {
	candidates, parallelRequested, ok := extractCustomWorkflowRunRequests(prompt)
	if !ok {
		if inferred, parallel, matched := inferCustomWorkflowRunRequests(prompt); matched {
			candidates = inferred
			parallelRequested = parallel
			ok = true
		}
	}
	if !ok {
		return "", false, nil
	}
	resolved := make([]customWorkflowMatch, 0, len(candidates))
	missing := []string{}
	for _, candidate := range candidates {
		cw, matched, err := findCustomWorkflowByNameOrAgent(candidate)
		if err != nil {
			return "", true, err
		}
		if cw == nil {
			missing = append(missing, candidate)
			continue
		}
		resolved = append(resolved, customWorkflowMatch{Candidate: candidate, Name: matched, Workflow: cw})
	}
	if len(resolved) == 0 {
		return "", true, fmt.Errorf("workflow/agent tidak ditemukan: %s. Tidak membuat blueprint baru; pilih workflow existing yang tersimpan%s", strings.Join(missing, ", "), existingCustomWorkflowHint())
	}
	if len(missing) > 0 {
		return "", true, fmt.Errorf("workflow/agent tidak ditemukan: %s. Tidak membuat blueprint baru; pilih workflow existing yang tersimpan%s", strings.Join(missing, ", "), existingCustomWorkflowHint())
	}

	responses, err := s.runResolvedCustomWorkflowMatchesWithProgress(resolved, parallelRequested, onProgress)
	if err != nil {
		return "", true, err
	}
	if len(responses) == 1 {
		return responses[0], true, nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d custom workflow existing selesai dijalankan.\n", len(responses)))
	for i, response := range responses {
		sb.WriteString(fmt.Sprintf("\n## Workflow %d\n%s\n", i+1, response))
	}
	return strings.TrimSpace(sb.String()), true, nil
}

type customWorkflowMatch struct {
	Candidate string
	Name      string
	Workflow  *workflow.CustomWorkflow
}

func (s *Server) runResolvedCustomWorkflowMatches(items []customWorkflowMatch, parallelRequested bool) ([]string, error) {
	return s.runResolvedCustomWorkflowMatchesWithProgress(items, parallelRequested, nil)
}

func (s *Server) runResolvedCustomWorkflowMatchesWithProgress(items []customWorkflowMatch, parallelRequested bool, onProgress func(event, message, role, taskID string, details map[string]interface{})) ([]string, error) {
	responses := make([]string, 0, len(items))
	for _, item := range items {
		response, err := s.runResolvedCustomWorkflowWithProgress(item, false, onProgress)
		if err != nil {
			if parallelRequested {
				return nil, fmt.Errorf("workflow dijalankan serial; gagal menjalankan '%s': %w", item.Name, err)
			}
			return nil, err
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func (s *Server) runResolvedCustomWorkflow(item customWorkflowMatch, parallelRequested bool) (string, error) {
	return s.runResolvedCustomWorkflowWithProgress(item, parallelRequested, nil)
}

func (s *Server) runResolvedCustomWorkflowWithProgress(item customWorkflowMatch, parallelRequested bool, onProgress func(event, message, role, taskID string, details map[string]interface{})) (string, error) {
	parallelRequested = false
	useTaskStore := false
	runID := fmt.Sprintf("custom-%d", time.Now().UnixNano())
	if useTaskStore && s.OrchestrationStore != nil {
		s.OrchestrationStore.Start(runID, customWorkflowExecutionPlan(item.Workflow, item.Name, parallelRequested))
	}
	started := map[string]time.Time{}
	result, err := workflow.RunCustomWorkflowWithProgressMode(s.Supervisor, s.Supervisor.GetProvider(), item.Workflow, &workflow.CustomWorkflowProgress{
		OnBlueprintReady: func(_ workflow.Blueprint, waves [][]string) {
			if useTaskStore && s.OrchestrationStore != nil {
				s.OrchestrationStore.Start(runID, customWorkflowExecutionPlan(item.Workflow, item.Name, parallelRequested))
			}
			if onProgress != nil {
				onProgress("blueprint_ready", fmt.Sprintf("Workflow %s siap: %d step serial", item.Name, len(waves)), "", "", map[string]interface{}{"steps": waves})
			}
		},
		OnWaveStart: func(wave int, roles []string) {
			if onProgress != nil {
				onProgress("step_start", fmt.Sprintf("Step %d mulai: %s", wave+1, strings.Join(roles, ", ")), "", "", map[string]interface{}{"step": wave + 1, "roles": roles})
			}
		},
		OnWaveComplete: func(wave int, results map[string][]agent.TaskResult) {
			if onProgress != nil {
				onProgress("step_complete", fmt.Sprintf("Step %d selesai", wave+1), "", "", map[string]interface{}{"step": wave + 1})
			}
		},
		OnRoleStart: func(role string) {
			started[role] = time.Now()
			if useTaskStore && s.OrchestrationStore != nil {
				s.OrchestrationStore.UpdateSubtaskStatus(role, workflow.StatusRunning, "", "", 0)
			}
			if onProgress != nil {
				onProgress("role_start", fmt.Sprintf("Role %s mulai", role), role, "", nil)
			}
		},
		OnTaskStart: func(role string, task agent.Task) {
			if onProgress == nil {
				return
			}
			details := map[string]interface{}{
				"task_id":     task.ID,
				"task_type":   task.Type,
				"description": truncateWorkflowDetail(strings.TrimSpace(task.Description), 2000),
			}
			if task.MCPServer != "" || task.ToolName != "" {
				details["mcp_server"] = task.MCPServer
				details["tool_name"] = task.ToolName
				if task.ToolArgs != nil {
					details["tool_args"] = task.ToolArgs
				}
			}
			onProgress("task_start", fmt.Sprintf("%s mulai: %s", role, task.ID), role, task.ID, details)
		},
		OnTaskStream: func(role, taskID, chunk string, isThinking bool) {
			if onProgress == nil || chunk == "" {
				return
			}
			onProgress("task_stream", chunk, role, taskID, map[string]interface{}{
				"is_thinking": isThinking,
				"chunk_chars": len(chunk),
			})
		},
		OnTaskComplete: func(role, taskID string, taskResult agent.TaskResult) {
			duration := time.Duration(0)
			if start := started[role]; !start.IsZero() {
				duration = time.Since(start)
			}
			details := map[string]interface{}{"status": taskResult.Status, "duration_ms": duration.Milliseconds(), "error": taskResult.Error}
			if output := strings.TrimSpace(taskResult.Output); output != "" {
				details["output"] = truncateWorkflowDetail(output, 12000)
			}
			if task, ok := findCustomWorkflowTask(item.Workflow, role, taskID); ok && (task.MCPServer != "" || task.ToolName != "") {
				details["mcp_server"] = task.MCPServer
				details["tool_name"] = task.ToolName
				if task.ToolArgs != nil {
					details["tool_args"] = task.ToolArgs
				}
			}
			if useTaskStore && s.OrchestrationStore != nil {
				s.OrchestrationStore.UpdateSubtaskStatus(role, taskResultStatus(taskResult), strings.TrimSpace(taskResult.Output), taskResult.Error, duration)
			}
			if onProgress != nil {
				onProgress("task_complete", fmt.Sprintf("%s selesai: %s", role, taskID), role, taskID, details)
			}
		},
	}, parallelRequested)
	if err != nil {
		if useTaskStore && s.OrchestrationStore != nil {
			s.OrchestrationStore.Complete(workflow.StatusFailed, "", err.Error())
		}
		return "", fmt.Errorf("gagal menjalankan custom workflow '%s': %w", item.Name, err)
	}
	response := formatCustomWorkflowRunResponse(item.Name, result, parallelRequested)
	if useTaskStore && s.OrchestrationStore != nil {
		s.OrchestrationStore.MarkAll(workflow.StatusSuccess, "Custom workflow selesai. "+result.FinalSummary)
		s.OrchestrationStore.Complete(workflow.StatusSuccess, response, "")
	}
	return response, nil
}

func findCustomWorkflowTask(cw *workflow.CustomWorkflow, role, taskID string) (workflow.Task, bool) {
	if cw == nil {
		return workflow.Task{}, false
	}
	for _, agent := range cw.Agents {
		if agent.Role != role {
			continue
		}
		for _, task := range agent.Tasks {
			if task.ID == taskID || strings.TrimPrefix(taskID, role+"-") == task.ID {
				return task, true
			}
		}
	}
	return workflow.Task{}, false
}

func truncateWorkflowDetail(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "\n... [truncated]"
}

func customWorkflowExecutionPlan(cw *workflow.CustomWorkflow, matched string, parallelRequested bool) workflow.ExecutionPlan {
	planID := "custom-workflow-" + sanitizeRunID(matched)
	plan := workflow.ExecutionPlan{
		ID:       planID,
		Task:     workflow.OrchestrationTask{ID: planID + "-task", Title: "Custom workflow: " + matched, Description: cw.Description, Kind: workflow.TaskKindReadOnly, RiskLevel: workflow.RiskLow},
		Metadata: map[string]interface{}{"custom_workflow": cw.Name},
	}
	bp := cw.ToBlueprint()
	runner := workflow.NewRunner(bp, nil, nil)
	runner.Serial = true
	waves, _ := runner.BuildWaves()
	for _, a := range cw.Agents {
		id := a.Role
		description := a.Description
		if len(a.Tasks) > 0 {
			description = fmt.Sprintf("%s\n\n%d task(s): %s", a.Description, len(a.Tasks), firstNonEmpty(a.Tasks[0].Description, a.Tasks[0].ID))
		}
		plan.Subtasks = append(plan.Subtasks, workflow.Subtask{ID: id, Title: a.Role, Description: description, Kind: workflow.TaskKindReadOnly, DependsOn: append([]string(nil), a.DependsOn...), CanParallel: false, RiskLevel: workflow.RiskLow, Status: workflow.StatusPending})
	}
	for i, wave := range waves {
		plan.Batches = append(plan.Batches, workflow.ExecutionBatch{ID: fmt.Sprintf("step-%d", i+1), Name: fmt.Sprintf("Step %d", i+1), Mode: workflow.BatchModeSerial, SubtaskIDs: append([]string(nil), wave...), MaxConcurrency: 1})
	}
	return plan
}

func sanitizeRunID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if b.Len() > 0 {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "run"
	}
	return out
}
func extractCustomWorkflowRunName(prompt string) (string, bool) {
	names, _, ok := extractCustomWorkflowRunRequests(prompt)
	if !ok || len(names) == 0 {
		return "", false
	}
	return names[0], true
}

func extractCustomWorkflowRunRequest(prompt string) (string, bool, bool) {
	names, parallel, ok := extractCustomWorkflowRunRequests(prompt)
	if !ok || len(names) == 0 {
		return "", false, false
	}
	return names[0], parallel, true
}

func extractCustomWorkflowRunRequests(prompt string) ([]string, bool, bool) {
	text := strings.TrimSpace(prompt)
	if text == "" {
		return nil, false, false
	}
	text = strings.Trim(text, " \t\n\r`'\"")
	lower := strings.ToLower(text)
	prefixes := []string{
		"jalankan custom workflow ", "jalankan workflow ", "run custom workflow ", "run workflow ",
		"execute custom workflow ", "execute workflow ", "mulai custom workflow ", "mulai workflow ",
		"start custom workflow ", "start workflow ", "jalankan ", "halankan ", "jlnkan ", "run ", "execute ", "mulai ", "start ",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			raw := strings.TrimSpace(text[len(prefix):])
			name, parallelRequested := stripCustomWorkflowParallelSuffix(raw)
			names := splitCustomWorkflowRunCandidates(name)
			if len(names) > 0 {
				return names, parallelRequested, true
			}
		}
	}
	return nil, false, false
}

func inferCustomWorkflowRunRequests(prompt string) ([]string, bool, bool) {
	text := strings.TrimSpace(prompt)
	if text == "" {
		return nil, false, false
	}
	if isCustomWorkflowNonRunPrompt(text) {
		return nil, false, false
	}
	parallelRequested := strings.Contains(strings.ToLower(text), "parallel") || strings.Contains(strings.ToLower(text), "paralel")
	names, err := workflow.ListCustomWorkflows()
	if err != nil || len(names) == 0 {
		return nil, parallelRequested, false
	}
	seen := map[string]bool{}
	var candidates []string
	for _, name := range names {
		cw, err := workflow.LoadCustomWorkflow(name)
		if err != nil || cw == nil {
			continue
		}
		if customWorkflowMentioned(text, cw, name) {
			key := normalizeWorkflowLookupKey(cw.Name)
			if key != "" && !seen[key] {
				seen[key] = true
				candidates = append(candidates, cw.Name)
			}
		}
	}
	return candidates, parallelRequested, len(candidates) > 0
}

func isCustomWorkflowNonRunPrompt(prompt string) bool {
	p := strings.ToLower(strings.TrimSpace(prompt))
	if p == "" {
		return false
	}
	nonRunTerms := []string{
		"tidak nyambung", "nggak nyambung", "ngga nyambung", "gak nyambung", "ga nyambung",
		"tolong di perbaiki", "tolong diperbaiki", "di perbaiki", "diperbaiki", "perbaiki", "fix", "repair",
		"node-builder", "node builder", "builder", "edit", "ubah", "update", "cek", "check", "inspect", "debug",
		"kenapa", "mengapa", "gimana", "bagaimana", "apakah", "?",
	}
	for _, term := range nonRunTerms {
		if strings.Contains(p, term) {
			return true
		}
	}
	return false
}

func customWorkflowMentioned(prompt string, cw *workflow.CustomWorkflow, fileName string) bool {
	promptKey := normalizeWorkflowLookupKey(prompt)
	keys := []string{normalizeWorkflowLookupKey(cw.Name), normalizeWorkflowLookupKey(fileName)}
	for _, a := range cw.Agents {
		keys = append(keys, normalizeWorkflowLookupKey(a.Role))
	}
	for _, key := range keys {
		if key == "" {
			continue
		}
		for _, alias := range workflowLookupAliases(key) {
			if alias != "" && strings.Contains(promptKey, alias) {
				return true
			}
		}
	}
	return false
}

func splitCustomWorkflowRunCandidates(text string) []string {
	clean := strings.Trim(text, " \t\n\r`'\".,!")
	if clean == "" {
		return nil
	}
	replacer := strings.NewReplacer(",", "\n", "&", "\n", " + ", "\n", " dan ", "\n", " and ", "\n")
	parts := strings.Split(replacer.Replace(" "+clean+" "), "\n")
	seen := map[string]bool{}
	var out []string
	for _, part := range parts {
		part = strings.Trim(part, " \t\n\r`'\".,!")
		if part == "" {
			continue
		}
		key := normalizeWorkflowLookupKey(part)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, part)
	}
	return out
}

func stripCustomWorkflowParallelSuffix(name string) (string, bool) {
	clean := strings.Trim(name, " \t\n\r`'\".,!")
	lower := strings.ToLower(clean)
	suffixes := []string{" secara parallel", " secara paralel", " in parallel", " parallel", " paralel"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(lower, suffix) {
			trimmed := strings.TrimSpace(clean[:len(clean)-len(suffix)])
			trimmed = strings.Trim(trimmed, " \t\n\r`'\".,!")
			return trimmed, trimmed != ""
		}
	}
	return clean, false
}

func isCustomWorkflowQuestion(prompt string) bool {
	p := strings.ToLower(strings.TrimSpace(prompt))
	if !strings.Contains(p, "custom workflow") && !strings.Contains(p, "workflow") {
		return false
	}
	questionTerms := []string{"?", "apakah", "apa ", "gimana", "bagaimana", "ada fitur", "bisa gak", "bisa nggak", "bisa ngga", "bisakah", "kalau", "untuk custom workflow"}
	for _, term := range questionTerms {
		if strings.Contains(p, term) {
			return true
		}
	}
	return strings.Contains(p, "saya mau buat") && (strings.Contains(p, "nanti") || strings.Contains(p, "yang bisa"))
}

func (s *Server) tryCreateCustomWorkflowPrompt(prompt string) (string, bool, error) {
	if isCustomWorkflowQuestion(prompt) {
		return "", false, nil
	}
	name, ok := extractCustomWorkflowCreateName(prompt)
	if !ok {
		return "", false, nil
	}
	uniqueName, err := uniqueCustomWorkflowName(name)
	if err != nil {
		return "", true, err
	}
	cw := buildPromptCustomWorkflow(uniqueName, prompt)
	if err := workflow.SaveCustomWorkflow(cw); err != nil {
		return "", true, fmt.Errorf("gagal menyimpan custom workflow '%s': %w", uniqueName, err)
	}
	return formatCustomWorkflowCreateResponse(cw), true, nil
}

func extractCustomWorkflowCreateName(prompt string) (string, bool) {
	text := strings.TrimSpace(prompt)
	if text == "" {
		return "", false
	}
	lower := strings.ToLower(text)
	markers := []string{
		"buatkan custom workflow ",
		"buat custom workflow ",
		"bikin custom workflow ",
		"create custom workflow ",
		"generate custom workflow ",
		"scaffold custom workflow ",
	}
	for _, marker := range markers {
		if idx := strings.Index(lower, marker); idx >= 0 {
			name := deriveCustomWorkflowName(text[idx+len(marker):])
			if name != "" {
				return name, true
			}
		}
	}
	return "", false
}

func deriveCustomWorkflowName(text string) string {
	lower := strings.ToLower(text)
	words := strings.FieldsFunc(lower, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	stopAfter := map[string]bool{"nanti": true, "lalu": true, "kemudian": true, "yang": true, "dengan": true, "agar": true, "supaya": true, "dan": true}
	skip := map[string]bool{"untuk": true, "sebuah": true, "satu": true, "custom": true, "workflow": true, "agent": true}
	parts := make([]string, 0, 4)
	for _, word := range words {
		if word == "" || skip[word] {
			continue
		}
		if len(parts) > 0 && stopAfter[word] {
			break
		}
		parts = append(parts, word)
		if len(parts) == 4 {
			break
		}
	}
	return strings.Join(parts, "-")
}

func uniqueCustomWorkflowName(base string) (string, error) {
	names, err := workflow.ListCustomWorkflows()
	if err != nil {
		return "", err
	}
	used := make(map[string]bool, len(names))
	for _, name := range names {
		used[strings.ToLower(name)] = true
	}
	candidate := base
	for i := 2; used[strings.ToLower(candidate)]; i++ {
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
	return candidate, nil
}

func buildPromptCustomWorkflow(name, prompt string) *workflow.CustomWorkflow {
	if isLogoImageWorkflowPrompt(prompt) {
		return buildLogoImageCustomWorkflow(name, prompt)
	}
	return buildTypedPromptCustomWorkflow(name, prompt)
}

type promptWorkflowFeatures struct {
	tool   bool
	skill  bool
	memory bool
	loop   bool
}

func detectPromptWorkflowFeatures(prompt string) promptWorkflowFeatures {
	p := strings.ToLower(prompt)
	containsAny := func(words ...string) bool {
		for _, word := range words {
			if strings.Contains(p, word) {
				return true
			}
		}
		return false
	}
	return promptWorkflowFeatures{
		tool:   containsAny("tool", "tools", "command", "terminal", "shell", "jalankan", "run ", "execute", "eksekusi", "build", "test", "deploy", "release", "publish", "upload", "github", "git ", "npm", "go test", "docker"),
		skill:  containsAny("skill", "skills", "kemampuan", "spesialisasi", "agent spesialis"),
		memory: containsAny("memory", "memori", "ingat", "context", "konteks", "riwayat"),
		loop:   containsAny("loop", "ulang", "mengulang", "pengulangan", "retry", "interval", "for each", "foreach", "sampai berhasil", "until success"),
	}
}

func buildTypedPromptCustomWorkflow(name, prompt string) *workflow.CustomWorkflow {
	features := detectPromptWorkflowFeatures(prompt)
	agents := []workflow.CustomAgent{
		{
			Role:        "master",
			Description: "Agent node orkestrator. Menangkap tujuan, scope, guardrail, dan routing node typed dari prompt utama.",
			Skills:      []string{"orchestrator", "agent"},
			Tasks: []workflow.Task{{
				ID:          "intake",
				Type:        "llm",
				Description: "Baca prompt utama berikut, tetapkan tujuan workflow, acceptance criteria, batasan aman, dan node yang harus dijalankan: " + prompt,
			}},
		},
	}
	lastRole := "master"
	if features.memory {
		agents = append(agents, workflow.CustomAgent{
			Role:        "memory-context",
			Description: "Memory node untuk mengambil konteks workspace yang relevan sebelum agent/tool berjalan.",
			Skills:      []string{"memory"},
			Tasks: []workflow.Task{{
				ID:          "load-context",
				Type:        "llm",
				Description: "Gunakan hasil memory hydration sebagai konteks workflow. Ringkas memori yang relevan dan guardrail yang harus dipakai.",
			}},
			DependsOn:  []string{lastRole},
			InputsFrom: map[string][]string{lastRole: {"intake"}},
			Memory:     &workflow.MemoryNodeConfig{Action: "search", Query: prompt, Limit: 5},
		})
		lastRole = "memory-context"
	}
	if features.skill {
		agents = append(agents, workflow.CustomAgent{
			Role:        "skill-router",
			Description: "Skill node untuk memilih skill yang relevan dan menerjemahkannya menjadi instruksi kerja konkret.",
			Skills:      []string{"skill", "skill-audit", "planning"},
			Tasks: []workflow.Task{{
				ID:          "map-skills",
				Type:        "llm",
				Description: "Petakan skill yang diperlukan untuk prompt ini, skill yang sudah cocok, gap skill, dan instruksi penggunaan skill per node.",
			}},
			DependsOn:  []string{lastRole},
			InputsFrom: map[string][]string{lastRole: {"intake", "load-context"}},
		})
		lastRole = "skill-router"
	}
	agents = append(agents, workflow.CustomAgent{
		Role:        "workflow-agent",
		Description: "Agent node utama yang melakukan reasoning dan menyusun rencana eksekusi berdasarkan master, memory, dan skill.",
		Skills:      []string{"agent", "planning", "execution-plan"},
		Tasks: []workflow.Task{{
			ID:          "plan",
			Type:        "llm",
			Description: "Susun rencana kerja konkret untuk menjalankan tujuan workflow. Jika ada tool node, hasilkan parameter dan urutan command yang aman.",
		}},
		DependsOn:  []string{lastRole},
		InputsFrom: map[string][]string{lastRole: {"intake", "load-context", "map-skills"}},
	})
	lastRole = "workflow-agent"
	if features.loop {
		agents = append(agents, workflow.CustomAgent{
			Role:        "loop-controller",
			Description: "Loop node dengan guard eksplisit. Dipakai untuk pengulangan terbatas, retry, interval, atau evaluasi sampai kondisi terpenuhi.",
			Skills:      []string{"loop", "control-flow"},
			Tasks: []workflow.Task{{
				ID:          "guard",
				Type:        "llm",
				Description: "Tentukan kondisi berhenti, guard keamanan, dan cara mengevaluasi hasil tiap iterasi sebelum node berikutnya berjalan.",
			}},
			DependsOn:  []string{lastRole},
			InputsFrom: map[string][]string{lastRole: {"plan"}},
			Loop:       &workflow.LoopNodeConfig{Mode: "count", MaxIterations: 3, DelayMs: 1000, TimeoutMs: 0, OnError: "stop"},
		})
		lastRole = "loop-controller"
	}
	if features.tool {
		agents = append(agents, workflow.CustomAgent{
			Role:        "tool-runner",
			Description: "Tool node executable. Jalankan aksi eksternal melalui builtin run_command agar Smara Web menampilkan live tool_call/tool_result.",
			Skills:      []string{"tool", "terminal", "run_command"},
			Tasks: []workflow.Task{{
				ID:          "run",
				Type:        "mcp",
				Description: "Jalankan command awal yang aman untuk workflow ini. Edit tool_args.command di Node Builder untuk command produksi spesifik.",
				MCPServer:   "builtin",
				ToolName:    "run_command",
				ToolArgs: map[string]interface{}{
					"command":     generatedWorkflowToolCommand(name, prompt),
					"timeout_sec": 1800,
				},
			}},
			DependsOn:  []string{lastRole},
			InputsFrom: map[string][]string{lastRole: {"plan", "guard"}},
		})
		lastRole = "tool-runner"
	}
	agents = append(agents, workflow.CustomAgent{
		Role:        "final-agent",
		Description: "Agent node finalizer. Mengompilasi output agent/tool/memory/loop menjadi laporan akhir yang actionable.",
		Skills:      []string{"agent", "reporting", "qa"},
		Tasks: []workflow.Task{{
			ID:          "final",
			Type:        "llm",
			Description: "Buat laporan final Markdown: apa yang dijalankan, output penting, risiko, status acceptance criteria, dan next action.",
		}},
		DependsOn:  []string{lastRole},
		InputsFrom: map[string][]string{lastRole: {"run", "plan", "guard"}},
	})
	return &workflow.CustomWorkflow{
		Name:        name,
		Description: prompt,
		Agents:      agents,
	}
}

func generatedWorkflowToolCommand(name, prompt string) string {
	return strings.Join([]string{
		"set -e",
		"echo " + shellSingleQuote("== Smara generated workflow tool node =="),
		"echo " + shellSingleQuote("workflow="+name),
		"echo " + shellSingleQuote("prompt="+prompt),
		"echo " + shellSingleQuote("Edit tool_args.command di Custom Workflow Node Builder untuk command produksi spesifik."),
	}, "\n")
}

func shellSingleQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func isLogoImageWorkflowPrompt(prompt string) bool {
	p := strings.ToLower(prompt)
	return strings.Contains(p, "logo") || strings.Contains(p, "generate image") || strings.Contains(p, "mengenerate image") || strings.Contains(p, "gambar") || strings.Contains(p, "desain visual")
}

func buildLogoImageCustomWorkflow(name, prompt string) *workflow.CustomWorkflow {
	imagePrompt := "Professional logo design based on this brief: " + prompt + ". Create a clean, memorable, high-quality logo on a simple background, suitable for brand identity presentation."
	return &workflow.CustomWorkflow{
		Name:        name,
		Description: prompt,
		Agents: []workflow.CustomAgent{
			{Role: "master", Description: "Orkestrator workflow desain logo. Tangkap brief, tujuan brand, constraint visual, output image, review, dan revised prompt.", Skills: []string{"orchestrator", "logo-design"}, Tasks: []workflow.Task{{ID: "brief", Description: "Baca prompt utama berikut dan susun brief logo: audience, brand personality, visual direction, warna, style, constraint, dan acceptance criteria: " + prompt}}},
			{Role: "logo-brief-analyst", Description: "Brand strategist yang memperjelas brief logo menjadi arahan visual yang siap digenerate.", Skills: []string{"brand", "logo", "creative-brief"}, Tasks: []workflow.Task{{ID: "creative-brief", Description: "Ubah brief master menjadi creative brief logo yang konkret: konsep utama, simbol/metafora, warna, typography direction, mood, dan negative constraints."}}, DependsOn: []string{"master"}, InputsFrom: map[string][]string{"master": {"brief"}}},
			{Role: "logo-designer", Description: "Designer agent yang membuat prompt image logo profesional dari creative brief.", Skills: []string{"logo-design", "prompt-engineering", "visual-design"}, Tasks: []workflow.Task{{ID: "image-prompt", Description: "Buat final image prompt untuk generate_image. Prompt harus spesifik, profesional, tidak generik, dan cocok untuk logo/brand identity."}}, DependsOn: []string{"logo-brief-analyst"}, InputsFrom: map[string][]string{"logo-brief-analyst": {"creative-brief"}}},
			{Role: "image-generator", Description: "Tool node yang menjalankan builtin generate_image untuk menghasilkan logo dari prompt desain.", Skills: []string{"tool", "image-generation"}, Tasks: []workflow.Task{{ID: "generate", Description: "Generate logo image menggunakan builtin generate_image berdasarkan brief dan prompt desain.", Type: "mcp", MCPServer: "builtin", ToolName: "generate_image", ToolArgs: map[string]interface{}{"prompt": imagePrompt, "size": "1024x1024", "quality": "high"}}}, DependsOn: []string{"logo-designer"}, InputsFrom: map[string][]string{"logo-designer": {"image-prompt"}}},
			{Role: "logo-reviewer", Description: "Reviewer desain logo yang menjelaskan keistimewaan, kekurangan, dan kualitas hasil image.", Skills: []string{"design-review", "brand", "critique"}, Tasks: []workflow.Task{{ID: "review", Description: "Review hasil logo: jelaskan keistimewaan, brand fit, readability, scalability, memorability, risiko visual, dan rekomendasi revisi."}}, DependsOn: []string{"image-generator"}, InputsFrom: map[string][]string{"image-generator": {"generate"}}},
			{Role: "revised-prompt-writer", Description: "Prompt engineer yang membuat revised prompt berdasarkan review agar user bisa generate versi lebih baik.", Skills: []string{"prompt-engineering", "revision"}, Tasks: []workflow.Task{{ID: "revised-prompt", Description: "Buat revised image prompt yang lebih kuat berdasarkan hasil dan review. Sertakan alasan perubahan dan varian prompt alternatif bila perlu."}}, DependsOn: []string{"logo-reviewer"}, InputsFrom: map[string][]string{"logo-reviewer": {"review"}}},
			{Role: "report-writer", Description: "Finalizer yang mengompilasi link/image output, review, keistimewaan logo, dan revised prompt.", Skills: []string{"reporting"}, Tasks: []workflow.Task{{ID: "final", Description: "Buat laporan final Markdown: image output, brief, konsep logo, keistimewaan, kekurangan, rekomendasi revisi, dan revised prompt siap pakai."}}, DependsOn: []string{"revised-prompt-writer"}, InputsFrom: map[string][]string{"revised-prompt-writer": {"revised-prompt"}}},
		},
	}
}

func formatCustomWorkflowCreateResponse(cw *workflow.CustomWorkflow) string {
	roles := make([]string, 0, len(cw.Agents))
	for _, a := range cw.Agents {
		roles = append(roles, a.Role)
	}
	return fmt.Sprintf("Custom workflow '%s' berhasil dibuat dan disimpan.\n\nAgents: %s\n\nBuka Custom Workflow Node Builder untuk edit node, atau jalankan dari chat dengan: `jalankan custom workflow %s`", cw.Name, strings.Join(roles, ", "), cw.Name)
}

func findCustomWorkflowByNameOrAgent(candidate string) (*workflow.CustomWorkflow, string, error) {
	workflows, err := workflow.LoadAllCustomWorkflows()
	if err != nil {
		return nil, "", err
	}
	cw, matched := matchCustomWorkflowWithName(candidate, workflows)
	return cw, matched, nil
}

func matchCustomWorkflowWithName(candidate string, workflows []*workflow.CustomWorkflow) (*workflow.CustomWorkflow, string) {
	wanted := strings.ToLower(strings.TrimSpace(candidate))
	wantedKey := normalizeWorkflowLookupKey(candidate)
	wantedAliases := workflowLookupAliases(wantedKey)
	var fallback *workflow.CustomWorkflow
	fallbackName := ""
	for _, cw := range workflows {
		if cw == nil {
			continue
		}
		workflowKeys := []string{normalizeWorkflowLookupKey(cw.Name)}
		if strings.EqualFold(cw.Name, candidate) || containsAnyLookupKey(workflowKeys, wantedAliases) {
			return cw, cw.Name
		}
		for _, a := range cw.Agents {
			roleKey := normalizeWorkflowLookupKey(a.Role)
			agentKeys := append([]string{roleKey}, agentLookupAliases(cw.Name, roleKey)...)
			if strings.EqualFold(a.Role, candidate) || containsAnyLookupKey(agentKeys, wantedAliases) {
				return cw, cw.Name
			}
			if fallback == nil && anyWorkflowKeyMatches(agentKeys, wantedAliases) {
				fallback = cw
				fallbackName = cw.Name
			}
		}
		if fallback == nil && wanted != "" {
			for _, key := range workflowKeys {
				if strings.Contains(strings.ToLower(cw.Name), wanted) || anyWorkflowKeyMatches([]string{key}, wantedAliases) {
					fallback = cw
					fallbackName = cw.Name
					break
				}
			}
		}
	}
	return fallback, fallbackName
}

func existingCustomWorkflowHint() string {
	names, err := workflow.ListCustomWorkflows()
	if err != nil || len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	if len(names) > 8 {
		names = names[:8]
	}
	return ". Workflow tersedia: " + strings.Join(names, ", ")
}

func containsLookupKey(keys []string, wanted string) bool {
	return containsAnyLookupKey(keys, workflowLookupAliases(wanted))
}

func containsAnyLookupKey(keys, wantedAliases []string) bool {
	for _, key := range keys {
		for _, wanted := range wantedAliases {
			if wanted != "" && (key == wanted || workflowKeyLooksLike(key, wanted)) {
				return true
			}
		}
	}
	return false
}

func anyWorkflowKeyMatches(keys, wantedAliases []string) bool {
	for _, key := range keys {
		for _, wanted := range wantedAliases {
			if workflowKeyMatches(key, wanted) {
				return true
			}
		}
	}
	return false
}

func workflowLookupAliases(key string) []string {
	key = normalizeWorkflowLookupKey(key)
	if key == "" {
		return nil
	}
	aliases := []string{key}
	for _, suffix := range []string{"-agent", "-workflow"} {
		if strings.HasSuffix(key, suffix) {
			aliases = append(aliases, strings.TrimSuffix(key, suffix))
		}
	}
	parts := strings.Split(key, "-")
	for i := 1; i < len(parts)-1; i++ {
		aliases = append(aliases, strings.Join(parts[i:], "-"))
	}
	seen := map[string]bool{}
	out := aliases[:0]
	for _, alias := range aliases {
		alias = strings.Trim(alias, "-")
		if alias == "" || seen[alias] {
			continue
		}
		seen[alias] = true
		out = append(out, alias)
	}
	return out
}

func agentLookupAliases(workflowName, roleKey string) []string {
	workflowKey := normalizeWorkflowLookupKey(workflowName)
	roleKey = normalizeWorkflowLookupKey(roleKey)
	if workflowKey == "" || roleKey == "" {
		return nil
	}
	aliases := []string{}
	for _, suffix := range []string{"-workflow", "-agent"} {
		if strings.HasSuffix(workflowKey, suffix) {
			base := strings.TrimSuffix(workflowKey, suffix)
			aliases = append(aliases, base+"-"+roleKey)
		}
	}
	return aliases
}

func workflowKeyMatches(key, wanted string) bool {
	if key == "" || wanted == "" {
		return false
	}
	return strings.Contains(key, wanted) || strings.Contains(wanted, key) || workflowKeyLooksLike(key, wanted)
}

func workflowKeyLooksLike(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return levenshteinDistance(a, b) <= maxWorkflowLookupDistance(a, b)
}

func maxWorkflowLookupDistance(a, b string) int {
	longest := len(a)
	if len(b) > longest {
		longest = len(b)
	}
	if longest >= 18 {
		return 3
	}
	if longest >= 10 {
		return 2
	}
	return 1
}

func levenshteinDistance(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			cur[j] = minInt(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

func minInt(values ...int) int {
	best := values[0]
	for _, v := range values[1:] {
		if v < best {
			best = v
		}
	}
	return best
}

func normalizeWorkflowLookupKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if valid {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func formatCustomWorkflowRunResponse(name string, result *workflow.CustomWorkflowResult, parallelRequested bool) string {
	if result == nil {
		return fmt.Sprintf("Custom workflow '%s' selesai dijalankan.", name)
	}
	var sb strings.Builder
	if parallelRequested {
		sb.WriteString(fmt.Sprintf("Custom workflow '%s' selesai dijalankan secara serial. Permintaan parallel di workflow diabaikan.\n", name))
	} else {
		sb.WriteString(fmt.Sprintf("Custom workflow '%s' selesai dijalankan.\n", name))
	}
	if result.FinalSummary != "" {
		sb.WriteString("\n")
		sb.WriteString(result.FinalSummary)
		sb.WriteString("\n")
	}
	if parallelRequested {
		sb.WriteString("\nWorkflow execution:\n")
		sb.WriteString("- Parallel task dinonaktifkan untuk mode workflow; agent dijalankan satu per satu.\n")
		if waves := formatWorkflowWaves(result.Waves); waves != "" {
			sb.WriteString(waves)
		}
	}
	if result.ProjectPath != "" {
		sb.WriteString(fmt.Sprintf("\nProject: %s\n", result.ProjectPath))
	}
	if len(result.AgentOutputs) > 0 {
		sb.WriteString("\nAgent outputs:\n")
		roles := make([]string, 0, len(result.AgentOutputs))
		for role := range result.AgentOutputs {
			roles = append(roles, role)
		}
		sort.Strings(roles)
		for _, role := range roles {
			sb.WriteString(fmt.Sprintf("- %s: %d task result(s)\n", role, len(result.AgentOutputs[role])))
		}
	}
	if result.QAResult.Status != "" {
		sb.WriteString(fmt.Sprintf("\nQA: %s", result.QAResult.Status))
		if len(result.QAResult.Issues) > 0 {
			sb.WriteString(fmt.Sprintf(" (%d issue(s))", len(result.QAResult.Issues)))
		}
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

func formatWorkflowWaves(waves [][]string) string {
	if len(waves) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, wave := range waves {
		if len(wave) == 0 {
			continue
		}
		roles := append([]string(nil), wave...)
		sort.Strings(roles)
		mode := "serial"
		if len(roles) > 1 {
			mode = "parallel"
		}
		sb.WriteString(fmt.Sprintf("- Wave %d (%s): %s\n", i+1, mode, strings.Join(roles, ", ")))
	}
	return sb.String()
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
	switch r.Method {
	case http.MethodGet:
		limitStr := r.URL.Query().Get("limit")
		limit := 50
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
		}
		tags := r.URL.Query()["tags"]
		source := r.URL.Query().Get("source")
		wsID := s.resolveMemoryWorkspaceID(r.URL.Query().Get("workspace"))

		filters := memory.MemoryFilters{
			Limit:   limit,
			SortBy:  "created_at",
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
		jsonResponse(w, http.StatusOK, map[string]interface{}{"memories": mems, "total": total})
	case http.MethodPost:
		var req struct {
			Content   string   `json:"content"`
			Tags      []string `json:"tags"`
			Source    string   `json:"source"`
			Workspace string   `json:"workspace"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errorResponse(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		req.Content = strings.TrimSpace(req.Content)
		if req.Content == "" {
			errorResponse(w, http.StatusBadRequest, "content wajib diisi")
			return
		}
		source := strings.TrimSpace(req.Source)
		if source == "" {
			source = "web"
		}
		mem, err := s.MemStore.Save(req.Content, strings.Join(req.Tags, ","), source, s.resolveMemoryWorkspaceID(req.Workspace), nil)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, mem)
	case http.MethodDelete:
		id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil || id <= 0 {
			errorResponse(w, http.StatusBadRequest, "id wajib numeric")
			return
		}
		if err := s.MemStore.Delete(id); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		errorResponse(w, http.StatusMethodNotAllowed, "GET/POST/DELETE only")
	}
}

func (s *Server) resolveMemoryWorkspaceID(name string) int64 {
	wsID := s.resolveWorkspaceID()
	if name == "" {
		return wsID
	}
	w, err := s.MemStore.GetWorkspaceByName(name)
	if err == nil && w != nil {
		return w.ID
	}
	return wsID
}

func (s *Server) handleMemorySearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req struct {
		Query     string `json:"query"`
		Limit     int    `json:"limit"`
		Workspace string `json:"workspace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Limit == 0 {
		req.Limit = 10
	}

	wsID := s.resolveMemoryWorkspaceID(req.Workspace)
	mems, err := s.MemStore.SearchFullText(req.Query, wsID, memory.MemoryFilters{
		Limit:         req.Limit,
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

func (s *Server) handleRoadmapFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	rawPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if rawPath == "" {
		errorResponse(w, http.StatusBadRequest, "path required")
		return
	}
	resolved, root, err := s.resolveWorkspaceFilePath(rawPath)
	if err != nil {
		errorResponse(w, http.StatusForbidden, err.Error())
		return
	}
	ext := strings.ToLower(filepath.Ext(resolved))
	if ext != ".md" && ext != ".markdown" {
		errorResponse(w, http.StatusBadRequest, "roadmap file must be markdown")
		return
	}
	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		errorResponse(w, http.StatusNotFound, "roadmap not found")
		return
	}
	if info.Size() > 2*1024*1024 {
		errorResponse(w, http.StatusBadRequest, "roadmap file too large")
		return
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	rel, _ := filepath.Rel(root, resolved)
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"path":          resolved,
		"relative_path": rel,
		"name":          filepath.Base(resolved),
		"content":       string(content),
		"size":          info.Size(),
		"updated_at":    info.ModTime(),
		"workspace":     s.Cfg.ActiveWorkspace,
	})
}

func (s *Server) resolveWorkspaceFilePath(rawPath string) (string, string, error) {
	root, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	if s.MemStore != nil && s.Cfg != nil && strings.TrimSpace(s.Cfg.ActiveWorkspace) != "" {
		if ws, err := s.MemStore.GetWorkspaceByName(s.Cfg.ActiveWorkspace); err == nil && ws != nil && strings.TrimSpace(ws.Path) != "" {
			root = ws.Path
		}
	}
	if !filepath.IsAbs(root) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", "", err
		}
		root = filepath.Join(cwd, root)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	candidate := rawPath
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(absRoot, candidate)
	}
	absPath, err := filepath.Abs(candidate)
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("path outside active workspace")
	}
	return absPath, absRoot, nil
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

		// When model-related config changes, apply immediately to the running supervisor
		// AND to the web session manager so active/new web sessions pick up the new model.
		if s.Supervisor != nil {
			switch req.Key {
			case "model", "custom_model":
				provider := s.Cfg.Provider
				if provider == "" {
					provider = "custom"
				}
				log.Printf("[config] model changed to %s (provider=%s), applying immediately", valStr, provider)
				if err := s.Supervisor.SetModel(provider, valStr); err != nil {
					log.Printf("[config] failed to apply model change: %v", err)
				}
				if s.WebSessions != nil {
					s.WebSessions.UpdateProviderConfig(llm.ProviderConfig{
						Name:            provider,
						Model:           valStr,
						Host:            s.Cfg.CustomBaseURL,
						APIKey:          s.Cfg.CustomAPIKey,
						ReasoningEffort: s.Cfg.ReasoningEffort,
					})
				}
			case "provider":
				model := s.Cfg.Model
				if valStr == "custom" {
					model = s.Cfg.CustomModel
				}
				log.Printf("[config] provider changed to %s (model=%s), applying immediately", valStr, model)
				if err := s.Supervisor.SetModel(valStr, model); err != nil {
					log.Printf("[config] failed to apply provider change: %v", err)
				}
				if s.WebSessions != nil {
					s.WebSessions.UpdateProviderConfig(llm.ProviderConfig{
						Name:            valStr,
						Model:           model,
						Host:            s.Cfg.CustomBaseURL,
						APIKey:          s.Cfg.CustomAPIKey,
						ReasoningEffort: s.Cfg.ReasoningEffort,
					})
				}
			}
		}
		jsonResponse(w, http.StatusOK, map[string]interface{}{"ok": true})
	default:
		errorResponse(w, http.StatusMethodNotAllowed, "only GET or POST")
	}
}

// --- Static Provider Models ---

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	jsonResponse(w, http.StatusOK, llm.AvailableProviders())
}

// --- Live 9Router Models ---

func (s *Server) handle9RouterModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}

	cfg := config.Get()
	if cfg == nil {
		errorResponse(w, http.StatusInternalServerError, "config not loaded")
		return
	}

	baseURL := cfg.CustomBaseURL
	apiKey := cfg.CustomAPIKey
	if baseURL == "" || apiKey == "" {
		// Return empty list if custom provider not configured
		jsonResponse(w, http.StatusOK, map[string]interface{}{"models": []string{}})
		return
	}

	// Call 9Router /v1/models endpoint
	modelsURL := strings.TrimRight(baseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, modelsURL, nil)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		errorResponse(w, http.StatusBadGateway, "gagal menghubungi 9router: "+err.Error())
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Forward the response as-is
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
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
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 {
		days = 30
	}
	path := metrics.DefaultAnalyticsPath(s.Cfg.DBPath)
	log.Printf("[metrics] reading from %s (days=%d)", path, days)
	summary, err := metrics.ReadAnalyticsSummary(path, s.Cfg.DBPath, days)
	if err != nil {
		log.Printf("[metrics] error: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("[metrics] OK: prompts=%d tokens=%d models=%d daily=%d", summary.TotalPrompts, summary.TotalTokens, len(summary.Models), len(summary.Daily))
	jsonResponse(w, http.StatusOK, summary)
}

// --- Mode ---

func (s *Server) handleMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"mode":      string(s.Supervisor.GetMode()),
			"mode_info": agent.GetModeInfo(s.Supervisor.GetMode()),
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
	if _, handled, _ := s.tryRunCustomWorkflowPrompt(req.Prompt); handled {
		errorResponse(w, http.StatusBadRequest, "prompt ini adalah perintah menjalankan custom workflow existing; gunakan /api/custom-workflow/run atau chat, bukan generate blueprint")
		return
	}
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
	if response, handled, err := s.tryRunCustomWorkflowPrompt(req.Prompt); handled {
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]interface{}{"status": "custom_workflow_executed", "response": response})
		return
	}
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

type skillWebItem struct {
	Name          string           `json:"name"`
	Description   string           `json:"description"`
	Version       int              `json:"version"`
	Tags          []string         `json:"tags"`
	Params        []skill.ParamDef `json:"params,omitempty"`
	ParentID      string           `json:"parent_id,omitempty"`
	CategoryPath  []string         `json:"category_path,omitempty"`
	Dependencies  []string         `json:"dependencies,omitempty"`
	Lineage       []lineageWebEntry `json:"lineage,omitempty"`
	RunCount      int              `json:"run_count,omitempty"`
	SuccessRate   float64          `json:"success_rate,omitempty"`
	AvgDurationMS int64            `json:"avg_duration_ms,omitempty"`
	LastRun       *time.Time       `json:"last_run,omitempty"`
	NeedsAttention bool            `json:"needs_attention,omitempty"`
}

type lineageWebEntry struct {
	Version     int      `json:"version"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	StepCount   int      `json:"step_count"`
	RefinedAt   string   `json:"refined_at,omitempty"`
	RefinedFrom string   `json:"refined_from,omitempty"`
}

func (s *Server) skillItemForWeb(sk *skill.Skill) skillWebItem {
	item := skillWebItem{
		Name:         sk.Name,
		Description:  sk.Description,
		Version:      sk.Version,
		Tags:         sk.Tags,
		Params:       sk.Params,
		ParentID:     sk.ParentID,
		CategoryPath: sk.CategoryPath,
		Dependencies: sk.Dependencies,
	}
	for _, l := range sk.Lineage {
		item.Lineage = append(item.Lineage, lineageWebEntry{Version: l.Version, Description: l.Description, Tags: l.Tags, StepCount: l.StepCount, RefinedAt: l.RefinedAt.Format("2006-01-02 15:04"), RefinedFrom: l.RefinedFrom})
	}
	if s.SkillTracker != nil {
		if total, success, avgMs, lastRun, err := s.SkillTracker.GetStats(sk.Name); err == nil {
			item.RunCount = total
			item.AvgDurationMS = avgMs
			item.LastRun = lastRun
			if total > 0 {
				item.SuccessRate = float64(success) / float64(total) * 100
			}
			item.NeedsAttention = total >= 3 && item.SuccessRate < 70
		}
	}
	return item
}

func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		names, err := skill.List()
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		var items []skillWebItem
		for _, n := range names {
			sk, err := skill.Load(n)
			if err != nil {
				continue
			}
			items = append(items, s.skillItemForWeb(sk))
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

func (s *Server) handleSkillRecommend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		jsonResponse(w, http.StatusOK, map[string]interface{}{"recommendations": []interface{}{}})
		return
	}
	limit := 5
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		limit = v
	}
	names, err := skill.List()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	skills := make([]*skill.Skill, 0, len(names))
	byName := make(map[string]*skill.Skill, len(names))
	for _, name := range names {
		sk, err := skill.Load(name)
		if err != nil || sk == nil {
			continue
		}
		skills = append(skills, sk)
		byName[sk.Name] = sk
	}
	recs := skill.RecommendSkills(query, skills, skill.RecommendationOptions{Limit: limit, LowConfidence: 25, StatsProvider: s.SkillTracker})
	items := make([]map[string]interface{}, 0, len(recs))
	for _, rec := range recs {
		item := map[string]interface{}{
			"skill_name":    rec.SkillName,
			"score":         rec.Score,
			"confidence":    rec.Confidence,
			"reasons":       rec.Reasons,
			"clarify":       rec.Clarify,
			"success_rate":  rec.SuccessRate,
			"recently_used": rec.RecentlyUsed,
		}
		if sk := byName[rec.SkillName]; sk != nil {
			webItem := s.skillItemForWeb(sk)
			item["skill"] = webItem
		}
		items = append(items, item)
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"query": query, "recommendations": items})
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
		"skill_name":      name,
		"total_runs":      total,
		"success_rate":    rate,
		"avg_duration_ms": avgMs,
		"last_run":        lastRun,
	})
}

func (s *Server) handleSkillTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	name := r.URL.Query().Get("name")
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
		Name     string `json:"name"`
		Notes    string `json:"notes"`
		Proposal string `json:"proposal"`
		Apply    bool   `json:"apply"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		errorResponse(w, http.StatusBadRequest, "name required")
		return
	}

	if req.Apply {
		proposal := cleanSkillJSONOutput(req.Proposal)
		if proposal == "" {
			errorResponse(w, http.StatusBadRequest, "proposal required")
			return
		}
		refined, err := skill.ApplyNamedRefinement(name, []byte(proposal), nil, "manual")
		if err != nil {
			errorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		if s.SkillTracker != nil {
			_ = s.SkillTracker.RecordImprovement(skill.SkillImprovement{
				SkillName:    refined.Name,
				Version:      refined.Version,
				TriggeredAt:  time.Now(),
				Trigger:      "manual-refine",
				Applied:      true,
				ProposedJSON: proposal,
			})
		}
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"status":  "applied",
			"applied": true,
			"skill":   refined,
		})
		return
	}

	provider := s.Supervisor.GetProvider()
	if provider == nil {
		errorResponse(w, http.StatusServiceUnavailable, "provider not available")
		return
	}
	var prompt string
	var err error
	if s.SkillTracker != nil {
		prompt, _, err = skill.BuildRefinementPromptFull(name, s.SkillTracker, nil)
	} else {
		prompt, err = skill.BuildRefinementPrompt(name, nil)
	}
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if notes := strings.TrimSpace(req.Notes); notes != "" {
		prompt += "\n\nCatatan manual dari user:\n" + notes
	}
	resp, err := provider.Chat([]llm.Message{
		{Role: "system", Content: "You are a Smara skill refiner. Output only valid JSON matching the Skill schema. Do not wrap it in markdown."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		errorResponse(w, http.StatusBadGateway, "skill refine failed: "+err.Error())
		return
	}
	proposal := cleanSkillJSONOutput(resp.Content)
	if _, err := skill.FromJSON([]byte(proposal)); err != nil {
		errorResponse(w, http.StatusBadGateway, "provider returned invalid skill JSON: "+err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status":   "proposed",
		"applied":  false,
		"name":     name,
		"prompt":   prompt,
		"proposal": proposal,
	})
}

func cleanSkillJSONOutput(raw string) string {
	text := strings.TrimSpace(raw)
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		if len(lines) > 1 {
			if strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
				lines = lines[1:]
			}
			if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
				lines = lines[:len(lines)-1]
			}
			text = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	if start := strings.Index(text, "{"); start >= 0 {
		if end := strings.LastIndex(text, "}"); end > start {
			return strings.TrimSpace(text[start : end+1])
		}
	}
	return text
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

func (s *Server) handleSkillsBundled(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	items, err := skill.ListBundledSkills()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
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
	sk, err := skill.InstallBundledSkill(req.Name, "", true)
	if err != nil {
		errorResponse(w, http.StatusNotFound, err.Error())
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
	if s.Supervisor.GetMode() != agent.ModeWorkflow {
		errorResponse(w, http.StatusBadRequest, "custom workflow hanya dapat dijalankan saat mode workflow aktif")
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
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		errorResponse(w, http.StatusBadRequest, "nama target workflow wajib diisi")
		return
	}
	cw, err := workflow.CustomWorkflowFromJSON([]byte(req.JSON))
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid workflow JSON: "+err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	cw.Name = req.Name
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "replace"
	}
	merged := false
	if mode == "merge" {
		existing, err := workflow.LoadCustomWorkflow(req.Name)
		if err != nil || existing == nil {
			errorResponse(w, http.StatusBadRequest, "workflow target untuk merge tidak ditemukan: "+req.Name)
			return
		}
		cw = workflow.MergeCustomWorkflow(existing, cw)
		cw.Name = req.Name
		merged = true
	}
	if err := workflow.SaveCustomWorkflow(cw); err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	status := "imported"
	if merged {
		status = "merged"
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"status": status, "name": req.Name, "agents": len(cw.Agents), "merged": merged})
}

// --- Image Flow ---

func (s *Server) handleImageFlowList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	items, err := imageflow.List()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"workflows": items})
}

func (s *Server) handleImageFlowGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	name := r.URL.Query().Get("name")
	if strings.TrimSpace(name) == "" {
		errorResponse(w, http.StatusBadRequest, "name required")
		return
	}
	wf, err := imageflow.Load(name)
	if err != nil {
		errorResponse(w, http.StatusNotFound, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, wf)
}

func (s *Server) handleImageFlowSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var wf imageflow.Workflow
	if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := imageflow.Save(&wf); err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "saved", "name": wf.Name})
}

func (s *Server) handleImageFlowDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST/DELETE")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		req.Name = r.URL.Query().Get("name")
	}
	if strings.TrimSpace(req.Name) == "" {
		errorResponse(w, http.StatusBadRequest, "name required")
		return
	}
	if err := imageflow.Delete(req.Name); err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted", "name": req.Name})
}

func (s *Server) handleImageFlowRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	var req struct {
		Workflow imageflow.Workflow `json:"workflow"`
		Priority int                `json:"priority,omitempty"`
	}
	var wf imageflow.Workflow
	priority := 0
	if err := json.Unmarshal(raw, &req); err == nil && strings.TrimSpace(req.Workflow.Name) != "" {
		wf = req.Workflow
		priority = req.Priority
	} else if err := json.Unmarshal(raw, &wf); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	job, err := imageflow.StartJobWithOptions(&wf, imageflow.JobOptions{Priority: priority})
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, job)
}

func (s *Server) handleImageFlowStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	id := r.URL.Query().Get("id")
	if strings.TrimSpace(id) == "" {
		errorResponse(w, http.StatusBadRequest, "id required")
		return
	}
	job, ok := imageflow.GetJob(id)
	if !ok {
		errorResponse(w, http.StatusNotFound, "job not found")
		return
	}
	jsonResponse(w, http.StatusOK, job)
}

func (s *Server) handleImageFlowCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if strings.TrimSpace(req.ID) == "" {
		req.ID = r.URL.Query().Get("id")
	}
	if strings.TrimSpace(req.ID) == "" {
		errorResponse(w, http.StatusBadRequest, "id required")
		return
	}
	job, ok := imageflow.CancelJob(req.ID)
	if !ok {
		errorResponse(w, http.StatusNotFound, "job not found")
		return
	}
	jsonResponse(w, http.StatusOK, job)
}

func (s *Server) handleImageFlowRetry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if strings.TrimSpace(req.ID) == "" {
		req.ID = r.URL.Query().Get("id")
	}
	if strings.TrimSpace(req.ID) == "" {
		errorResponse(w, http.StatusBadRequest, "id required")
		return
	}
	job, ok, err := imageflow.RetryJob(req.ID)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	if !ok {
		errorResponse(w, http.StatusNotFound, "job not found")
		return
	}
	jsonResponse(w, http.StatusOK, job)
}

func (s *Server) handleImageFlowEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	id := r.URL.Query().Get("id")
	if strings.TrimSpace(id) == "" {
		errorResponse(w, http.StatusBadRequest, "id required")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		errorResponse(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	ctx := r.Context()
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	last := ""
	for {
		job, exists := imageflow.GetJob(id)
		if !exists {
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", `{"error":"job not found"}`)
			flusher.Flush()
			return
		}
		data, _ := json.Marshal(job)
		payload := string(data)
		if payload != last {
			fmt.Fprintf(w, "event: job\ndata: %s\n\n", payload)
			flusher.Flush()
			last = payload
		}
		if job.Status == "success" || job.Status == "failed" || job.Status == "canceled" {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) handleImageFlowAssets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	assets, err := imageflow.ListAssets()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	showArchived := r.URL.Query().Get("archived") == "1" || strings.EqualFold(r.URL.Query().Get("archived"), "true")
	filtered := make([]imageflow.Asset, 0, len(assets))
	for _, asset := range assets {
		if asset.Archived && !showArchived {
			continue
		}
		if mode != "" && strings.ToLower(asset.Mode) != mode {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(strings.Join([]string{asset.ID, asset.Workflow, asset.JobID, asset.Path, asset.Model, asset.Mode, asset.Provider, asset.Prompt}, " "))
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		asset, ok := s.imageFlowAssetForWeb(asset)
		if !ok {
			continue
		}
		filtered = append(filtered, asset)
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"assets": filtered})
}

func (s *Server) imageFlowAssetForWeb(asset imageflow.Asset) (imageflow.Asset, bool) {
	path := strings.TrimSpace(asset.Path)
	if path == "" {
		return asset, false
	}
	allowed, err := s.generatedImagePathAllowed(path)
	if err != nil {
		return asset, false
	}
	info, err := os.Stat(allowed)
	if err != nil || info.IsDir() {
		return asset, false
	}
	asset.Path = allowed
	asset.ImageURL = "/api/generated-image?path=" + url.QueryEscape(allowed)
	return asset, true
}

func (s *Server) handleImageFlowAssetImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req struct {
		Path     string `json:"path"`
		Workflow string `json:"workflow"`
		Mode     string `json:"mode"`
		Prompt   string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.Path = strings.TrimSpace(req.Path)
	if req.Path == "" {
		errorResponse(w, http.StatusBadRequest, "path required")
		return
	}
	asset := imageflow.Asset{
		ID:        fmt.Sprintf("asset-import-%d", time.Now().UnixNano()),
		Workflow:  strings.TrimSpace(req.Workflow),
		Path:      req.Path,
		Mode:      strings.TrimSpace(req.Mode),
		Prompt:    strings.TrimSpace(req.Prompt),
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	if asset.Workflow == "" {
		asset.Workflow = "Imported Asset"
	}
	if asset.Mode == "" {
		asset.Mode = "imported"
	}
	if err := imageflow.SaveAsset(asset); err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"status": "imported", "asset": asset})
}

func (s *Server) handleImageFlowAssetDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST/DELETE")
		return
	}
	var req struct {
		ID         string `json:"id"`
		DeleteFile bool   `json:"delete_file"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if strings.TrimSpace(req.ID) == "" {
		req.ID = r.URL.Query().Get("id")
	}
	if strings.TrimSpace(req.ID) == "" {
		errorResponse(w, http.StatusBadRequest, "id required")
		return
	}
	asset, ok, err := imageflow.DeleteAsset(req.ID, req.DeleteFile)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		errorResponse(w, http.StatusNotFound, "asset not found")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"status": "deleted", "asset": asset})
}

func (s *Server) handleImageFlowAssetArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req struct {
		ID       string `json:"id"`
		Archived bool   `json:"archived"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.ID) == "" {
		errorResponse(w, http.StatusBadRequest, "id required")
		return
	}
	asset, ok, err := imageflow.ArchiveAsset(req.ID, req.Archived)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		errorResponse(w, http.StatusNotFound, "asset not found")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"status": "updated", "asset": asset})
}

func (s *Server) handleImageFlowAssetCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req struct {
		MaxAgeHours int  `json:"max_age_hours"`
		DeleteFiles bool `json:"delete_files"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.MaxAgeHours <= 0 {
		req.MaxAgeHours = 24 * 30
	}
	removed, err := imageflow.CleanupAssets(time.Duration(req.MaxAgeHours)*time.Hour, req.DeleteFiles)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"status": "cleaned", "removed": removed})
}

func (s *Server) handleImageFlowModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"current": imageflow.ModelStatus(), "available": imageflow.AvailableModels()})
}

func (s *Server) handleImageFlowJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"jobs": imageflow.ListJobs(), "stats": imageflow.JobQueueStats()})
}

func (s *Server) handleImageFlowMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	jsonResponse(w, http.StatusOK, imageflow.UsageMetrics())
}

func (s *Server) handleImageFlowAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	events, err := imageflow.ReadAuditEvents(limit)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"events": events})
}

func (s *Server) handleImageFlowTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"templates": imageflow.BuiltinTemplates()})
}

func (s *Server) handleImageFlowAgentCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req struct {
		Name        string `json:"name"`
		Instruction string `json:"instruction"`
		Prompt      string `json:"prompt"`
		Save        bool   `json:"save"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	wf := imageflow.NewWorkflowFromPrompt(req.Name, req.Instruction, req.Prompt)
	if req.Save {
		if err := imageflow.Save(&wf); err != nil {
			errorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"workflow": wf, "issues": imageflow.LintWorkflow(&wf)})
}

func (s *Server) handleImageFlowAgentExplain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var wf imageflow.Workflow
	if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	jsonResponse(w, http.StatusOK, imageflow.ExplainWorkflow(&wf))
}

func (s *Server) handleImageFlowAgentLint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var wf imageflow.Workflow
	if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"issues": imageflow.LintWorkflow(&wf)})
}

func (s *Server) handleImageFlowAgentFix(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var wf imageflow.Workflow
	if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	fixed, actions := imageflow.FixWorkflow(&wf)
	if fixed == nil {
		errorResponse(w, http.StatusBadRequest, strings.Join(actions, ", "))
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"workflow": fixed, "actions": actions, "issues": imageflow.LintWorkflow(fixed)})
}

func (s *Server) handleImageFlowAgentOptimize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var wf imageflow.Workflow
	if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"suggestions": imageflow.OptimizeWorkflow(&wf)})
}

func (s *Server) handleImageFlowTemplateRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	var req struct {
		TemplateID string                 `json:"template_id"`
		Params     map[string]interface{} `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	tmpl, ok := imageflow.TemplateByID(req.TemplateID)
	if !ok {
		errorResponse(w, http.StatusNotFound, "template not found")
		return
	}
	wf := imageflow.ApplyParameters(&tmpl.Workflow, req.Params)
	job, err := imageflow.StartJob(&wf)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, job)
}

type fsEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
}

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
	jsonResponse(w, http.StatusOK, map[string]interface{}{"path": cwd})
}

func (s *Server) handleFSList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "only GET")
		return
	}
	p := strings.TrimSpace(r.URL.Query().Get("path"))
	if p == "" {
		var err error
		p, err = os.Getwd()
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
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
	jsonResponse(w, http.StatusOK, map[string]interface{}{"path": p, "entries": result})
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
//
//	{
//	  "mode": "overwrite"|"skip"|"rename",   // default overwrite
//	  "dry_run": false,                      // default false
//	  "envelope": { ... TreeExport ... }     // OR
//	  "envelope_json": "<stringified>"       // OR raw body containing
//	                                         // a naked TreeExport
//	}
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
