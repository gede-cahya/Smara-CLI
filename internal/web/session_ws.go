package web

import (
	"context"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/browser"
	"github.com/gorilla/websocket"
)

func (s *Server) handleWSWebSessionChat(conn *websocket.Conn, msg wsMessage) {
	if s.WebSessions == nil {
		_ = conn.WriteJSON(wsMessage{Type: "error", SessionID: msg.SessionID, Payload: "web session manager belum aktif"})
		_ = conn.WriteJSON(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "error"})
		return
	}
	write := func(v interface{}) { _ = conn.WriteJSON(v) }
	write(wsMessage{Type: "thinking", SessionID: msg.SessionID, Payload: "true"})
	write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "running"})

	timeoutSec := 1800
	if s.Cfg != nil && s.Cfg.AgentRequestTimeoutSec > 0 {
		timeoutSec = s.Cfg.AgentRequestTimeoutSec
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	prompt := injectAttachmentSteer(msg.Payload)
	cb := agent.AgenticCallback{
		OnPhaseChange: func(phase, description string) {
			write(wsMessage{Type: "phase", SessionID: msg.SessionID, Phase: phase, Description: description})
		},
		OnToolCall: func(server, tool string, args map[string]interface{}) {
			write(wsMessage{Type: "tool_call", SessionID: msg.SessionID, Server: server, Tool: tool, Args: args})
		},
		OnToolResult: func(output string) {
			preview := formatToolResultPreview(s.rewriteGeneratedImageLinks(output))
			write(wsMessage{Type: "tool_result", SessionID: msg.SessionID, Output: preview})
		},
		OnLog: func(role, content string) {
			write(wsMessage{Type: "log", SessionID: msg.SessionID, Payload: content, Role: role})
		},
	}

	if response, handled, err := s.tryRunCustomWorkflowPrompt(msg.Payload); handled {
		write(wsMessage{Type: "thinking", SessionID: msg.SessionID, Payload: "false"})
		if err != nil {
			write(wsMessage{Type: "error", SessionID: msg.SessionID, Payload: err.Error()})
			write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "error"})
			return
		}
		write(wsMessage{Type: "chat", SessionID: msg.SessionID, Payload: response})
		write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "completed"})
		return
	}
	activeMode := msg.Mode
	if activeMode == "" {
		activeMode = "ask"
	}
	if activeMode == string(agent.ModeTest) && browser.IsBrowserPrompt(msg.Payload) {
		write(wsMessage{Type: "tool_call", SessionID: msg.SessionID, Server: "browser", Tool: "browser_run", Args: map[string]interface{}{"prompt": msg.Payload}})
		output, err := s.runBrowserTest(ctx, msg.Payload)
		write(wsMessage{Type: "thinking", SessionID: msg.SessionID, Payload: "false"})
		if err != nil {
			write(wsMessage{Type: "error", SessionID: msg.SessionID, Payload: err.Error()})
			write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "error"})
			return
		}
		write(wsMessage{Type: "tool_result", SessionID: msg.SessionID, Output: output})
		write(wsMessage{Type: "chat", SessionID: msg.SessionID, Payload: output})
		write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "completed"})
		return
	}
	result, err := s.WebSessions.Run(ctx, msg.SessionID, prompt, activeMode, cb)
	write(wsMessage{Type: "thinking", SessionID: msg.SessionID, Payload: "false"})
	if err != nil {
		write(wsMessage{Type: "error", SessionID: msg.SessionID, Payload: err.Error()})
		write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "error"})
		return
	}
	write(s.chatWSMessage(msg.SessionID, s.rewriteGeneratedImageLinks(result.Response), msg.Payload, result))
	write(wsMessage{Type: "session_status", SessionID: msg.SessionID, Payload: "completed"})
}
