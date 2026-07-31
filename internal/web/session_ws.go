package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/agent/workflow"
	"github.com/gede-cahya/Smara-CLI/internal/browser"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/orchestration"
	"github.com/gorilla/websocket"
)

type wsToolProgressEvent struct {
	Tool    string                 `json:"tool"`
	Event   string                 `json:"event"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

type pendingStreamChunk struct {
	chunk      string
	isThinking bool
}

func (s *Server) handleWSWebSessionChat(conn *websocket.Conn, msg wsMessage) {
	if s.WebSessions == nil {
		_ = conn.WriteJSON(wsMessage{Type: "error", SessionID: msg.SessionID, Payload: "web session manager belum aktif"})
		_ = conn.WriteJSON(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "error"})
		return
	}
	runID := msg.RunID
	if runID == "" {
		runID = fmt.Sprintf("web-%d", time.Now().UnixNano())
	}
	var writeMu sync.Mutex
	write := func(v interface{}) {
		writeMu.Lock()
		defer writeMu.Unlock()
		if message, ok := v.(wsMessage); ok {
			message.RunID = runID
			v = message
		}
		_ = conn.WriteJSON(v)
	}
	write(wsMessage{Type: "thinking", SessionID: msg.SessionID, Payload: "true"})
	write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "running"})

	activeMode := msg.Mode
	if activeMode == "" {
		activeMode = "ask"
	}

	runStarted := time.Now()
	var eventMu sync.Mutex
	lastEventAt := runStarted
	lastEventName := "run_start"
	touch := func(name string) {
		eventMu.Lock()
		lastEventAt = time.Now()
		lastEventName = name
		eventMu.Unlock()
	}
	emitLog := func(level, event, message, tool string, details map[string]interface{}) {
		now := time.Now()
		ev := processLogEvent{
			Timestamp: now,
			SessionID: msg.SessionID,
			RunID:     runID,
			Level:     level,
			Event:     event,
			Message:   message,
			Mode:      activeMode,
			Tool:      tool,
			ElapsedMs: now.Sub(runStarted).Milliseconds(),
			Details:   redactProcessDetails(details),
		}
		path := s.appendProcessLog(ev)
		if event != "heartbeat" {
			touch(event)
		}
		role := "process"
		if level == "warn" {
			role = "warning"
		} else if level == "error" {
			role = "error"
		}
		write(wsMessage{
			Type:      "process_log",
			SessionID: msg.SessionID,
			Payload:   formatProcessLogLine(ev, path),
			Role:      role,
			Args: map[string]interface{}{
				"event":      event,
				"level":      level,
				"message":    message,
				"phase":      ev.Details["phase"],
				"tool":       tool,
				"log_path":   path,
				"elapsed_ms": ev.ElapsedMs,
				"run_id":     ev.RunID,
				"details":    ev.Details,
			},
		})
	}
	cfg := s.WebSessions.ProviderConfig()
	emitLog("info", "run_start", "Request diterima dan proses dimulai.", "", map[string]interface{}{
		"prompt_chars":          len(msg.Payload),
		"timeout_sec":           timeoutSecFromServer(s),
		"provider":              cfg.Name,
		"model":                 cfg.Model,
		"reasoning_effort":      cfg.ReasoningEffort,
		"custom_disable_stream": s.Cfg != nil && s.Cfg.CustomDisableStream,
	})
	recordDirectResult := func(response string, status WebSessionStatus, errText string) {
		if s.WebSessions == nil {
			return
		}
		if err := s.WebSessions.RecordDirectResult(msg.SessionID, msg.Payload, activeMode, response, status, errText); err != nil {
			emitLog("warn", "session_save_failed", "Gagal menyimpan response ke riwayat sesi web.", "", map[string]interface{}{"error": err.Error()})
		}
	}

	heartbeatDone := make(chan struct{})
	defer close(heartbeatDone)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				eventMu.Lock()
				silence := time.Since(lastEventAt)
				name := lastEventName
				eventMu.Unlock()
				if silence >= 15*time.Second {
					message := fmt.Sprintf("Proses masih berjalan; belum ada event baru selama %s. Event terakhir: %s. Jika belum ada tool, Smara sedang menunggu respons provider/model.", silence.Round(time.Second), name)
					if name == "custom_workflow" {
						message = fmt.Sprintf("Workflow masih berjalan; belum ada tool/final response baru selama %s. Detail step internal disembunyikan dari chat.", silence.Round(time.Second))
					}
					emitLog("warn", "heartbeat", message, "", map[string]interface{}{
						"silence_ms": silence.Milliseconds(),
						"last_event": name,
					})
				}
			case <-heartbeatDone:
				return
			}
		}
	}()

	timeoutSec := 1800
	if s.Cfg != nil && s.Cfg.AgentRequestTimeoutSec > 0 {
		timeoutSec = s.Cfg.AgentRequestTimeoutSec
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	prompt := injectAttachmentSteer(msg.Payload)

	streamCh := make(chan pendingStreamChunk, 256)
	streamFlushCh := make(chan chan struct{})
	streamDone := make(chan struct{})
	defer close(streamDone)
	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		var b strings.Builder
		var isThinking bool
		hasBuffered := false
		flush := func() {
			if !hasBuffered || b.Len() == 0 {
				return
			}
			payload := b.String()
			thinking := isThinking
			b.Reset()
			hasBuffered = false
			// Hardening: stream chunks must never leak DSML tags
			payload = llm.SanitizeStreamChunk(payload)
			write(wsMessage{Type: "stream", SessionID: msg.SessionID, Payload: s.rewriteGeneratedImageLinks(payload), Args: map[string]interface{}{"is_thinking": thinking}})
		}
		appendChunk := func(ev pendingStreamChunk) {
			if ev.chunk == "" {
				return
			}
			if hasBuffered && ev.isThinking != isThinking {
				flush()
			}
			isThinking = ev.isThinking
			hasBuffered = true
			b.WriteString(ev.chunk)
			if b.Len() >= 2048 {
				flush()
			}
		}
		drain := func() {
			for {
				select {
				case ev := <-streamCh:
					appendChunk(ev)
				default:
					flush()
					return
				}
			}
		}
		for {
			select {
			case ev := <-streamCh:
				appendChunk(ev)
			case <-ticker.C:
				flush()
			case ack := <-streamFlushCh:
				drain()
				close(ack)
			case <-streamDone:
				drain()
				return
			}
		}
	}()
	flushStreams := func() {
		ack := make(chan struct{})
		select {
		case streamFlushCh <- ack:
			<-ack
		case <-streamDone:
		}
	}

	var toolMu sync.Mutex
	toolStarted := map[string]time.Time{}
	var toolStack []string
	var phaseMu sync.Mutex
	lastPhase := ""
	lastPhaseDescription := ""
	lastPhaseAt := time.Time{}
	emitPhase := func(phase, description string) {
		phaseMu.Lock()
		now := time.Now()
		if phase == lastPhase && description == lastPhaseDescription && now.Sub(lastPhaseAt) < 5*time.Second {
			phaseMu.Unlock()
			return
		}
		lastPhase = phase
		lastPhaseDescription = description
		lastPhaseAt = now
		phaseMu.Unlock()
		emitLog("info", "phase", description, "", map[string]interface{}{"phase": phase})
		write(wsMessage{Type: "phase", SessionID: msg.SessionID, Phase: phase, Description: description})
	}
	cb := agent.AgenticCallback{
		OnPhaseChange: func(phase, description string) {
			emitPhase(phase, description)
		},
		OnToolCall: func(server, tool string, args map[string]interface{}) {
			toolMu.Lock()
			toolStack = append(toolStack, tool)
			toolStarted[tool] = time.Now()
			toolMu.Unlock()
			emitLog("info", "tool_start", "Tool mulai dijalankan.", tool, map[string]interface{}{"server": server, "args": args})
			write(wsMessage{Type: "tool_call", SessionID: msg.SessionID, Server: server, Tool: tool, Args: args})
		},
		OnToolResult: func(output string) {
			toolMu.Lock()
			tool := ""
			if len(toolStack) > 0 {
				tool = toolStack[len(toolStack)-1]
				toolStack = toolStack[:len(toolStack)-1]
			}
			startedAt := toolStarted[tool]
			delete(toolStarted, tool)
			toolMu.Unlock()
			level := "info"
			if isProcessErrorText(output) {
				level = "error"
			}
			details := map[string]interface{}{"output_chars": len(output), "preview": output}
			if !startedAt.IsZero() {
				details["duration_ms"] = time.Since(startedAt).Milliseconds()
			}
			emitLog(level, "tool_done", "Tool selesai.", tool, details)
			preview := formatToolResultPreview(s.rewriteGeneratedImageLinks(output))
			write(wsMessage{Type: "tool_result", SessionID: msg.SessionID, Output: preview})
		},
		OnStream: func(chunk string, isThinking bool) {
			touch("stream")
			// Hardening: filter DSML in stream at source without stripping spaces
			if !isThinking {
				chunk = llm.SanitizeStreamChunk(chunk)
			}
			select {
			case streamCh <- pendingStreamChunk{chunk: chunk, isThinking: isThinking}:
			default:
				write(wsMessage{Type: "stream", SessionID: msg.SessionID, Payload: s.rewriteGeneratedImageLinks(chunk), Args: map[string]interface{}{"is_thinking": isThinking}})
			}
		},
		OnIteration: func(current, max int) {
			emitLog("info", "iteration", fmt.Sprintf("Iterasi agent %d/%d.", current, max), "", nil)
		},
		OnLog: func(role, content string) {
			if role == "tool_progress" {
				var ev wsToolProgressEvent
				if err := json.Unmarshal([]byte(content), &ev); err == nil && ev.Event != "" {
					level := "info"
					if strings.Contains(strings.ToLower(ev.Event), "error") || strings.Contains(strings.ToLower(ev.Event), "timeout") {
						level = "warn"
					}
					emitLog(level, ev.Event, ev.Message, ev.Tool, ev.Details)
					return
				}
			}
			touch("log")
			write(wsMessage{Type: "log", SessionID: msg.SessionID, Payload: content, Role: role})
		},
	}

	if agent.Mode(activeMode) == agent.ModeWorkflow {
		if response, handled, err := s.tryRunCustomWorkflowPromptWithProgress(msg.Payload, func(event, message, role, taskID string, details map[string]interface{}) {
			if event == "task_stream" {
				touch("task_stream")
				chunk := strings.TrimRight(message, "\r\n")
				if chunk != "" {
					write(wsMessage{
						Type:      "log",
						SessionID: msg.SessionID,
						Payload:   chunk,
						Role:      "Terminal",
						Args: map[string]interface{}{
							"event":         "task_stream",
							"stream_append": true,
						},
					})
				}
				return
			}
			touch(event)
			if details == nil {
				details = map[string]interface{}{}
			}
			level := "info"
			if event == "task_complete" {
				if errText, _ := details["error"].(string); strings.TrimSpace(errText) != "" {
					level = "error"
				}
			}
			emitLog(level, event, message, "custom_workflow", details)
			switch event {
			case "blueprint_ready":
				write(wsMessage{Type: "phase", SessionID: msg.SessionID, Phase: "Workflow", Description: message})
			case "step_start", "step_complete":
				write(wsMessage{Type: "phase", SessionID: msg.SessionID, Phase: "Workflow Step", Description: message})
			case "role_start":
				write(wsMessage{Type: "phase", SessionID: msg.SessionID, Phase: role, Description: message})
			case "task_start":
				toolName, _ := details["tool_name"].(string)
				server, _ := details["mcp_server"].(string)
				if toolName == "" {
					write(wsMessage{Type: "phase", SessionID: msg.SessionID, Phase: "Workflow Task", Description: message})
					return
				}
				if server == "" {
					server = "smara"
				}
				args := map[string]interface{}{
					"role":        role,
					"task_id":     taskID,
					"type":        details["task_type"],
					"description": details["description"],
				}
				if rawArgs, ok := details["tool_args"].(map[string]interface{}); ok {
					args["args"] = rawArgs
				}
				write(wsMessage{Type: "phase", SessionID: msg.SessionID, Phase: "Workflow Task", Description: message})
				write(wsMessage{Type: "tool_call", SessionID: msg.SessionID, Server: server, Tool: toolName, Args: args})
			case "task_complete":
				output, _ := details["output"].(string)
				if strings.TrimSpace(output) == "" {
					output, _ = details["error"].(string)
				}
				if strings.TrimSpace(output) == "" {
					output = message
				}
				write(wsMessage{Type: "tool_result", SessionID: msg.SessionID, Output: s.rewriteGeneratedImageLinks(output)})
			}
		}); handled {
			if err != nil {
				write(wsMessage{Type: "thinking", SessionID: msg.SessionID, Payload: "false"})
				emitLog("error", "run_error", err.Error(), "", nil)
				recordDirectResult("", WebSessionError, err.Error())
				write(wsMessage{Type: "error", SessionID: msg.SessionID, Payload: err.Error()})
				write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "error"})
				return
			}
			write(wsMessage{Type: "thinking", SessionID: msg.SessionID, Payload: "false"})
			emitLog("info", "run_complete", "Custom workflow selesai.", "", map[string]interface{}{"duration_ms": time.Since(runStarted).Milliseconds()})
			recordDirectResult(response, WebSessionCompleted, "")
			write(wsMessage{Type: "chat", SessionID: msg.SessionID, Payload: response})
			write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "completed"})
			return
		}
	}
	if agent.Mode(activeMode) == agent.ModeWorkflow {
		if response, handled, err := s.tryCreateCustomWorkflowPrompt(msg.Payload); handled {
			if err != nil {
				write(wsMessage{Type: "thinking", SessionID: msg.SessionID, Payload: "false"})
				emitLog("error", "run_error", err.Error(), "", nil)
				recordDirectResult("", WebSessionError, err.Error())
				write(wsMessage{Type: "error", SessionID: msg.SessionID, Payload: err.Error()})
				write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "error"})
				return
			}
			write(wsMessage{Type: "thinking", SessionID: msg.SessionID, Payload: "false"})
			emitLog("info", "run_complete", "Custom workflow berhasil dibuat.", "", map[string]interface{}{"duration_ms": time.Since(runStarted).Milliseconds()})
			recordDirectResult(response, WebSessionCompleted, "")
			write(wsMessage{Type: "chat", SessionID: msg.SessionID, Payload: response, RequestPrompt: msg.Payload})
			write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "completed"})
			return
		}
	}
	if activeMode == string(agent.ModeTest) && browser.IsBrowserPrompt(msg.Payload) {
		emitLog("info", "browser_test_start", "Browser test mulai dijalankan.", "browser_run", nil)
		write(wsMessage{Type: "tool_call", SessionID: msg.SessionID, Server: "browser", Tool: "browser_run", Args: map[string]interface{}{"prompt": msg.Payload}})
		output, err := s.runBrowserTest(ctx, msg.Payload)
		write(wsMessage{Type: "thinking", SessionID: msg.SessionID, Payload: "false"})
		if err != nil {
			emitLog("error", "run_error", err.Error(), "browser_run", nil)
			recordDirectResult("", WebSessionError, err.Error())
			write(wsMessage{Type: "error", SessionID: msg.SessionID, Payload: err.Error()})
			write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "error"})
			return
		}
		emitLog("info", "browser_test_done", "Browser test selesai.", "browser_run", map[string]interface{}{"output_chars": len(output)})
		write(wsMessage{Type: "tool_result", SessionID: msg.SessionID, Output: output})
		recordDirectResult(output, WebSessionCompleted, "")
		write(wsMessage{Type: "chat", SessionID: msg.SessionID, Payload: output})
		write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "completed"})
		return
	}
	if agent.Mode(activeMode) == agent.ModeParallel && orchestration.IsAgentSwarmWorkflowPrompt(msg.Payload) {
		emitLog("info", "agent_swarm_start", "Agent Swarm Workflow mulai dijalankan.", "agent_swarm_workflow", nil)
		write(wsMessage{Type: "chat", SessionID: msg.SessionID, Payload: "⏳ Agent Swarm Workflow sedang berjalan. Smara memecah tugas, spawn agent yang dibutuhkan, menjalankan wave paralel, merge hasil, lalu QA.", RequestPrompt: msg.Payload})
		write(wsMessage{Type: "tool_call", SessionID: msg.SessionID, Server: "smara", Tool: "agent_swarm_workflow", Args: map[string]interface{}{"status": "running", "mode": activeMode}})
		result, err := s.runWorkflowWithLiveStatusAndProgress(ctx, msg.Payload, func(step, status string) {
			message := fmt.Sprintf("%s: %s", step, status)
			emitLog("info", "orchestration_progress", message, "parallel_orchestration", map[string]interface{}{"step": step, "status": status})
			write(wsMessage{Type: "phase", SessionID: msg.SessionID, Phase: step, Description: status})
		})
		write(wsMessage{Type: "thinking", SessionID: msg.SessionID, Payload: "false"})
		if err != nil {
			emitLog("error", "run_error", err.Error(), "agent_swarm_workflow", nil)
			recordDirectResult("", WebSessionError, err.Error())
			write(wsMessage{Type: "error", SessionID: msg.SessionID, Payload: fmt.Sprintf("Agent Swarm Workflow gagal: %v", err)})
			write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "error"})
			return
		}
		payload := formatAgentSwarmCompletion(result, time.Since(runStarted))
		emitLog("info", "run_complete", "Agent Swarm Workflow selesai.", "agent_swarm_workflow", map[string]interface{}{"duration_ms": time.Since(runStarted).Milliseconds()})
		recordDirectResult(payload, WebSessionCompleted, "")
		write(wsMessage{Type: "chat", SessionID: msg.SessionID, Payload: payload, RequestPrompt: msg.Payload})
		write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "completed"})
		return
	}
	if orchestration.ShouldAutoParallelOrchestrate(msg.Payload, agent.Mode(activeMode)) {
		emitLog("info", "orchestration_start", "Auto parallel orchestration mulai dijalankan.", "parallel_orchestration", nil)
		write(wsMessage{Type: "chat", SessionID: msg.SessionID, Payload: "⏳ Auto parallel orchestration sedang berjalan. Smara sedang membuat blueprint, membagi pekerjaan ke beberapa agent, lalu menjalankan wave paralel. Progress detail muncul di timeline/run status; saya akan kirim ringkasan lengkap setelah selesai.", RequestPrompt: msg.Payload})
		write(wsMessage{Type: "tool_call", SessionID: msg.SessionID, Server: "smara", Tool: "parallel_orchestration", Args: map[string]interface{}{"status": "running", "mode": activeMode}})
		result, err := s.runWorkflowWithLiveStatusAndProgress(ctx, msg.Payload, func(step, status string) {
			message := fmt.Sprintf("%s: %s", step, status)
			emitLog("info", "orchestration_progress", message, "parallel_orchestration", map[string]interface{}{"step": step, "status": status})
			write(wsMessage{Type: "phase", SessionID: msg.SessionID, Phase: step, Description: status})
		})
		write(wsMessage{Type: "thinking", SessionID: msg.SessionID, Payload: "false"})
		if err != nil {
			emitLog("error", "run_error", err.Error(), "parallel_orchestration", nil)
			recordDirectResult("", WebSessionError, err.Error())
			write(wsMessage{Type: "error", SessionID: msg.SessionID, Payload: fmt.Sprintf("auto parallel orchestration gagal: %v", err)})
			write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "error"})
			return
		}
		payload := formatAutoParallelCompletion(result, time.Since(runStarted))
		emitLog("info", "run_complete", "Auto parallel orchestration selesai.", "parallel_orchestration", map[string]interface{}{"duration_ms": time.Since(runStarted).Milliseconds()})
		recordDirectResult(payload, WebSessionCompleted, "")
		write(wsMessage{Type: "chat", SessionID: msg.SessionID, Payload: payload, RequestPrompt: msg.Payload})
		write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "completed"})
		return
	}
	result, err := s.WebSessions.Run(ctx, msg.SessionID, prompt, activeMode, cb)
	flushStreams()
	write(wsMessage{Type: "thinking", SessionID: msg.SessionID, Payload: "false"})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			emitLog("warn", "run_cancelled", "Proses dibatalkan.", "", nil)
			write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "cancelled"})
			return
		}
		if errors.Is(err, context.DeadlineExceeded) {
			emitLog("error", "run_timeout", "Proses melewati batas waktu request.", "", nil)
			write(wsMessage{Type: "error", SessionID: msg.SessionID, Payload: "Request timeout: proses melewati batas waktu."})
			write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "error"})
			return
		}
		emitLog("error", "run_error", err.Error(), "", nil)
		write(wsMessage{Type: "error", SessionID: msg.SessionID, Payload: err.Error()})
		write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "error"})
		return
	}
	emitLog("info", "run_complete", "Proses selesai.", "", map[string]interface{}{
		"duration_ms":   result.Duration.Milliseconds(),
		"tools":         result.ToolsExecuted,
		"input_tokens":  result.InputTokens,
		"output_tokens": result.OutputTokens,
	})
	// Hardening: sanitize final response before WS emit
	finalPayload := llm.SanitizeForUser(result.Response)
	write(s.chatWSMessage(msg.SessionID, s.rewriteGeneratedImageLinks(finalPayload), msg.Payload, result))
	write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "completed"})
}

func formatAutoParallelCompletion(result *workflow.WorkflowResult, duration time.Duration) string {
	return formatWorkflowCompletion("✅ Auto parallel orchestration selesai", result, duration)
}

func formatAgentSwarmCompletion(result *workflow.WorkflowResult, duration time.Duration) string {
	return formatWorkflowCompletion("✅ Agent Swarm Workflow selesai", result, duration)
}

func formatWorkflowCompletion(title string, result *workflow.WorkflowResult, duration time.Duration) string {
	if result == nil {
		return title + "\n\nWorkflow selesai, tetapi ringkasan hasil tidak tersedia."
	}
	summary := strings.TrimSpace(result.FinalSummary)
	if summary == "" {
		summary = "Workflow selesai tanpa ringkasan tambahan."
	}
	agentCount := len(result.AgentOutputs)
	taskCount := 0
	completedTasks := 0
	failedTasks := 0
	for _, outputs := range result.AgentOutputs {
		taskCount += len(outputs)
		for _, output := range outputs {
			switch output.Status {
			case agent.TaskCompleted:
				completedTasks++
			case agent.TaskFailed:
				failedTasks++
			}
		}
	}
	qaStatus := strings.TrimSpace(result.QAResult.Status)
	if qaStatus == "" {
		qaStatus = "UNKNOWN"
	}
	var b strings.Builder
	b.WriteString(title + "\n\n")
	b.WriteString("**Status akhir**\n")
	b.WriteString(fmt.Sprintf("- Ringkasan: %s\n", summary))
	if duration > 0 {
		b.WriteString(fmt.Sprintf("- Durasi: %s\n", duration.Round(time.Second)))
	}
	b.WriteString(fmt.Sprintf("- Agent: %d\n", agentCount))
	b.WriteString(fmt.Sprintf("- Task: %d selesai, %d gagal, %d total\n", completedTasks, failedTasks, taskCount))
	b.WriteString(fmt.Sprintf("- Parallel execution: %t, max concurrency: %d\n", result.ParallelExecution, result.MaxConcurrency))
	for i, wave := range result.ExecutionWaves {
		mode := "serial"
		if len(wave) > 1 {
			mode = "parallel"
		}
		b.WriteString(fmt.Sprintf("  - Wave %d (%s): %s\n", i+1, mode, strings.Join(wave, ", ")))
	}
	b.WriteString(fmt.Sprintf("- QA: %s", qaStatus))
	if len(result.QAResult.Issues) > 0 {
		b.WriteString(fmt.Sprintf(" (%d issue)\n", len(result.QAResult.Issues)))
		for i, issue := range result.QAResult.Issues {
			if i >= 5 {
				b.WriteString(fmt.Sprintf("  - ...dan %d issue lain\n", len(result.QAResult.Issues)-i))
				break
			}
			b.WriteString(fmt.Sprintf("  - %s\n", strings.TrimSpace(issue)))
		}
	} else {
		b.WriteString(" (0 issue)\n")
	}
	if strings.TrimSpace(result.ProjectPath) != "" {
		b.WriteString(fmt.Sprintf("- Project: `%s`\n", result.ProjectPath))
	}
	b.WriteString("\nProgress detail selama run tersedia di timeline/run status.")
	return b.String()
}

func timeoutSecFromServer(s *Server) int {
	if s != nil && s.Cfg != nil && s.Cfg.AgentRequestTimeoutSec > 0 {
		return s.Cfg.AgentRequestTimeoutSec
	}
	return 1800
}

func isProcessErrorText(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	return strings.HasPrefix(lower, "error:") ||
		strings.HasPrefix(lower, "failed:") ||
		strings.HasPrefix(lower, "gagal:") ||
		strings.Contains(lower, "\nerror:") ||
		strings.Contains(lower, "\nfailed:") ||
		strings.Contains(lower, "\ngagal:")
}
