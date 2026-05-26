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
	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

// --- Status ---

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	mode := s.Supervisor.GetMode()
	modeInfo := agent.GetModeInfo(mode)
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status":       "running",
		"mode":         string(mode),
		"mode_label":   modeInfo.Label,
		"mode_desc":    modeInfo.Description,
		"mode_emoji":   modeInfo.Emoji,
		"provider":     s.Supervisor.GetProvider().Name(),
		"workspace":    s.Cfg.ActiveWorkspace,
		"version":      "1.0.0",
		"web_sessions": s.WebSessions != nil,
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
	if response, handled, err := s.tryRunCustomWorkflowPrompt(req.Message); handled {
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, chatResponse{Response: response})
		return
	}
	if response, handled, err := s.tryCreateCustomWorkflowPrompt(req.Message); handled {
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, chatResponse{Response: response})
		return
	}
	result, err := s.Supervisor.ProcessPrompt(ctx, req.Message)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, chatResponse{Response: s.rewriteGeneratedImageLinks(result.Response)})
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
	if response, handled, err := s.tryRunCustomWorkflowPrompt(msg.Payload); handled {
		_ = session.WriteJSON(wsMessage{Type: "thinking", Payload: "false"})
		if err != nil {
			_ = session.WriteJSON(wsMessage{Type: "error", Payload: err.Error()})
			return
		}
		_ = session.WriteJSON(wsMessage{Type: "chat", Payload: response, RequestPrompt: msg.Payload})
		return
	}
	if response, handled, err := s.tryCreateCustomWorkflowPrompt(msg.Payload); handled {
		_ = session.WriteJSON(wsMessage{Type: "thinking", Payload: "false"})
		if err != nil {
			_ = session.WriteJSON(wsMessage{Type: "error", Payload: err.Error()})
			return
		}
		_ = session.WriteJSON(wsMessage{Type: "chat", Payload: response, RequestPrompt: msg.Payload})
		return
	}

	activeMode := msg.Mode
	if activeMode == "" {
		activeMode = string(s.Supervisor.GetMode())
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
		_ = metrics.AppendUsageEvent(path, metrics.UsageEvent{
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
		})
	}
}

func (s *Server) tryRunCustomWorkflowPrompt(prompt string) (string, bool, error) {
	candidate, ok := extractCustomWorkflowRunName(prompt)
	if !ok {
		return "", false, nil
	}
	cw, matched, err := findCustomWorkflowByNameOrAgent(candidate)
	if err != nil || cw == nil {
		return "", false, nil
	}
	result, err := workflow.RunCustomWorkflow(s.Supervisor, s.Supervisor.GetProvider(), cw)
	if err != nil {
		return "", true, fmt.Errorf("gagal menjalankan custom workflow '%s': %w", matched, err)
	}
	return formatCustomWorkflowRunResponse(matched, result), true, nil
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

func extractCustomWorkflowRunName(prompt string) (string, bool) {
	text := strings.TrimSpace(prompt)
	if text == "" {
		return "", false
	}
	text = strings.Trim(text, " \t\n\r`'\"")
	lower := strings.ToLower(text)
	prefixes := []string{
		"jalankan custom workflow ",
		"jalankan workflow ",
		"run custom workflow ",
		"run workflow ",
		"execute custom workflow ",
		"execute workflow ",
		"mulai custom workflow ",
		"mulai workflow ",
		"start custom workflow ",
		"start workflow ",
		"jalankan ",
		"run ",
		"execute ",
		"mulai ",
		"start ",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			name := strings.TrimSpace(text[len(prefix):])
			name = strings.Trim(name, " \t\n\r`'\".,!")
			if name != "" {
				return name, true
			}
		}
	}
	return "", false
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
	return &workflow.CustomWorkflow{
		Name:        name,
		Description: prompt,
		Agents: []workflow.CustomAgent{
			{Role: "master", Description: "Orkestrator workflow dari prompt utama. Pecah tujuan menjadi scope, batasan, agent route, dan acceptance criteria.", Skills: []string{"orchestrator"}, Tasks: []workflow.Task{{ID: "intake", Description: "Baca prompt utama berikut, tetapkan tujuan workflow, asumsi, batasan aman, dan urutan kerja: " + prompt}}},
			{Role: "discover-logic", Description: "Agent discovery yang memetakan logic, alur keputusan, input/output, dan dependency penting.", Skills: []string{"analysis", "workflow"}, Tasks: []workflow.Task{{ID: "discover", Description: "Temukan logic inti, aktor, data/input, output, dependency, dan area yang perlu diverifikasi dari prompt master."}}, DependsOn: []string{"master"}, InputsFrom: map[string][]string{"master": {"intake"}}},
			{Role: "range-summarizer", Description: "Agent yang merangkum range/scope dan batasan agar workflow tidak melebar ke hal generik.", Skills: []string{"planning", "scope"}, Tasks: []workflow.Task{{ID: "range", Description: "Ringkas range kerja, prioritas, out-of-scope, dan guardrail eksekusi."}}, DependsOn: []string{"discover-logic"}, InputsFrom: map[string][]string{"discover-logic": {"discover"}}},
			{Role: "skill-fit-auditor", Description: "Agent yang menilai skill/tool yang cocok, yang terlalu generik, dan gap kemampuan.", Skills: []string{"skill-audit", "evaluation"}, Tasks: []workflow.Task{{ID: "fit", Description: "Petakan skill yang dibutuhkan, skill yang fit, skill yang terlalu generik, dan rekomendasi perbaikan instruction."}}, DependsOn: []string{"discover-logic"}, InputsFrom: map[string][]string{"discover-logic": {"discover"}}},
			{Role: "weakness-auditor", Description: "Agent kritik yang mencari kelemahan workflow, ambiguity, edge case, dan risiko salah routing.", Skills: []string{"risk", "review"}, Tasks: []workflow.Task{{ID: "weakness", Description: "Jelaskan kelemahan, failure mode, ambiguity, risiko over-generic, dan mitigasi praktis."}}, DependsOn: []string{"range-summarizer", "skill-fit-auditor"}, InputsFrom: map[string][]string{"range-summarizer": {"range"}, "skill-fit-auditor": {"fit"}}},
			{Role: "report-writer", Description: "Agent finalizer yang mengompilasi hasil menjadi laporan actionable.", Skills: []string{"reporting"}, Tasks: []workflow.Task{{ID: "final", Description: "Buat laporan final Markdown: ringkasan logic, range/scope, skill fit, kelemahan, rekomendasi, dan next actions."}}, DependsOn: []string{"weakness-auditor"}, InputsFrom: map[string][]string{"weakness-auditor": {"weakness"}}},
		},
	}
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
	names, err := workflow.ListCustomWorkflows()
	if err != nil {
		return nil, "", err
	}
	wanted := strings.ToLower(strings.TrimSpace(candidate))
	var fallback *workflow.CustomWorkflow
	fallbackName := ""
	for _, name := range names {
		cw, err := workflow.LoadCustomWorkflow(name)
		if err != nil {
			continue
		}
		if strings.EqualFold(cw.Name, candidate) || strings.EqualFold(name, candidate) {
			return cw, cw.Name, nil
		}
		for _, a := range cw.Agents {
			if strings.EqualFold(a.Role, candidate) {
				return cw, cw.Name, nil
			}
			if fallback == nil && strings.Contains(strings.ToLower(a.Role), wanted) {
				fallback = cw
				fallbackName = cw.Name
			}
		}
		if fallback == nil && strings.Contains(strings.ToLower(cw.Name), wanted) {
			fallback = cw
			fallbackName = cw.Name
		}
	}
	return fallback, fallbackName, nil
}

func formatCustomWorkflowRunResponse(name string, result *workflow.CustomWorkflowResult) string {
	if result == nil {
		return fmt.Sprintf("Custom workflow '%s' selesai dijalankan.", name)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Custom workflow '%s' selesai dijalankan.\n", name))
	if result.FinalSummary != "" {
		sb.WriteString("\n")
		sb.WriteString(result.FinalSummary)
		sb.WriteString("\n")
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
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 {
		days = 30
	}
	path := metrics.DefaultAnalyticsPath(s.Cfg.DBPath)
	summary, err := metrics.ReadAnalyticsSummary(path, s.Cfg.DBPath, days)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
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
		filtered = append(filtered, asset)
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"assets": filtered})
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
