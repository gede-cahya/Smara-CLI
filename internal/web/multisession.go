package web

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/mcp"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
)

type WebSessionStatus string

const (
	WebSessionIdle      WebSessionStatus = "idle"
	WebSessionRunning   WebSessionStatus = "running"
	WebSessionCancelled WebSessionStatus = "cancelled"
	WebSessionError     WebSessionStatus = "error"
	WebSessionCompleted WebSessionStatus = "completed"
	WebSessionArchived  WebSessionStatus = "archived"
)

type WebChatMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type webAgentSessionSnapshot struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Mode      string           `json:"mode"`
	Workspace string           `json:"workspace"`
	Status    WebSessionStatus `json:"status"`
	Archived  bool             `json:"archived"`
	History   []WebChatMessage `json:"history"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
	Error     string           `json:"error,omitempty"`
}

type WebAgentSessionDTO struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Mode         string           `json:"mode"`
	Workspace    string           `json:"workspace"`
	Status       WebSessionStatus `json:"status"`
	Archived     bool             `json:"archived"`
	History      []WebChatMessage `json:"history"`
	TotalHistory int              `json:"total_history"`
	HistoryLimit int              `json:"history_limit,omitempty"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
	Error        string           `json:"error,omitempty"`
}

type WebAgentSession struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Mode      string           `json:"mode"`
	Workspace string           `json:"workspace"`
	Status    WebSessionStatus `json:"status"`
	Archived  bool             `json:"archived"`
	History   []WebChatMessage `json:"history"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
	Error     string           `json:"error,omitempty"`

	supervisor *agent.Supervisor  `json:"-"`
	cancel     context.CancelFunc `json:"-"`
	mu         sync.Mutex         `json:"-"`
}

type WebSessionManager struct {
	mu          sync.RWMutex
	sessions    map[string]*WebAgentSession
	storePath   string
	provider    llm.Provider
	providerCfg llm.ProviderConfig
	memStore    memory.MemoryStore
	workspace   string
	workspaceID int64
	maxIter     int
	mcpClients  map[string]*mcp.Client
	mcpInfo     map[string]agent.MCPServerInfo
}

func NewWebSessionManager(provider llm.Provider, providerCfg llm.ProviderConfig, memStore memory.MemoryStore, workspace string, workspaceID int64, maxIter int, storePath string) *WebSessionManager {
	if storePath == "" {
		home, _ := os.UserHomeDir()
		storePath = filepath.Join(home, ".smara", "web-sessions.json")
	}
	m := &WebSessionManager{
		sessions:    map[string]*WebAgentSession{},
		storePath:   storePath,
		provider:    provider,
		providerCfg: providerCfg,
		memStore:    memStore,
		workspace:   workspace,
		workspaceID: workspaceID,
		maxIter:     maxIter,
		mcpClients:  map[string]*mcp.Client{},
		mcpInfo:     map[string]agent.MCPServerInfo{},
	}
	_ = m.Load()
	return m
}

func (m *WebSessionManager) Load() error {
	b, err := os.ReadFile(m.storePath)
	if err != nil {
		return nil
	}
	var list []*WebAgentSession
	if err := json.Unmarshal(b, &list); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range list {
		if s.ID == "" {
			continue
		}
		if s.Status == WebSessionRunning {
			s.Status = WebSessionIdle
		}
		s.sessionsResetRuntime()
		m.sessions[s.ID] = s
	}
	return nil
}

func (s *WebAgentSession) sessionsResetRuntime() { s.supervisor = nil; s.cancel = nil }

func (m *WebSessionManager) SetMCPConnections(clients map[string]*mcp.Client, info map[string]agent.MCPServerInfo) {
	clientCopy := make(map[string]*mcp.Client, len(clients))
	for name, client := range clients {
		clientCopy[name] = client
	}
	infoCopy := make(map[string]agent.MCPServerInfo, len(info))
	for name, serverInfo := range info {
		serverInfo.Tools = append([]mcp.Tool(nil), serverInfo.Tools...)
		infoCopy[name] = serverInfo
	}

	m.mu.Lock()
	m.mcpClients = clientCopy
	m.mcpInfo = infoCopy
	var supervisors []*agent.Supervisor
	for _, session := range m.sessions {
		session.mu.Lock()
		if session.supervisor != nil {
			supervisors = append(supervisors, session.supervisor)
		}
		session.mu.Unlock()
	}
	m.mu.Unlock()

	for _, sup := range supervisors {
		applyMCPConnectionsToSupervisor(sup, clientCopy, infoCopy)
	}
}

func (m *WebSessionManager) mcpSnapshot() (map[string]*mcp.Client, map[string]agent.MCPServerInfo) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	clients := make(map[string]*mcp.Client, len(m.mcpClients))
	for name, client := range m.mcpClients {
		clients[name] = client
	}
	info := make(map[string]agent.MCPServerInfo, len(m.mcpInfo))
	for name, serverInfo := range m.mcpInfo {
		serverInfo.Tools = append([]mcp.Tool(nil), serverInfo.Tools...)
		info[name] = serverInfo
	}
	return clients, info
}

func applyMCPConnectionsToSupervisor(sup *agent.Supervisor, clients map[string]*mcp.Client, info map[string]agent.MCPServerInfo) {
	for name, client := range clients {
		if existing, ok := sup.GetMCPClient(name); !ok || existing != client {
			sup.RegisterMCPClient(name, client)
		}
	}
	for name, serverInfo := range info {
		if serverInfo.Connected {
			sup.UpdateMCPInfo(name, serverInfo.Tools)
		} else if serverInfo.Error != "" {
			sup.UpdateMCPError(name, serverInfo.Error)
		}
	}
}

func snapshotSession(s *WebAgentSession) webAgentSessionSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return webAgentSessionSnapshot{
		ID:        s.ID,
		Name:      s.Name,
		Mode:      s.Mode,
		Workspace: s.Workspace,
		Status:    s.Status,
		Archived:  s.Archived,
		History:   append([]WebChatMessage(nil), s.History...),
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
		Error:     s.Error,
	}
}

func (m *WebSessionManager) Save() error {
	m.mu.RLock()
	list := make([]webAgentSessionSnapshot, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, snapshotSession(s))
	}
	m.mu.RUnlock()
	sort.Slice(list, func(i, j int) bool { return list[i].UpdatedAt.After(list[j].UpdatedAt) })
	if err := os.MkdirAll(filepath.Dir(m.storePath), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.storePath, b, 0o600)
}

func (m *WebSessionManager) ensureSupervisor(s *WebAgentSession) *agent.Supervisor {
	clients, info := m.mcpSnapshot()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.supervisor != nil {
		return s.supervisor
	}
	sup := agent.NewSupervisorWithConfig(m.provider, m.providerCfg, m.memStore)
	if agent.ValidMode(s.Mode) {
		sup.SetMode(agent.Mode(s.Mode))
	}
	if m.maxIter > 0 {
		sup.SetMaxIterations(m.maxIter)
	}
	applyMCPConnectionsToSupervisor(sup, clients, info)
	created, _ := sup.CreateSession(agent.SessionConfig{Name: s.Name, WorkspaceID: m.workspaceID, Mode: s.Mode, IsAgentic: true})
	if created != nil {
		for _, hm := range s.History {
			role := llm.RoleAssistant
			if hm.Role == "user" {
				role = llm.RoleUser
			}
			created.History = append(created.History, llm.Message{Role: role, Content: hm.Content})
		}
		_ = sup.SwitchSession(created.ID)
	}
	s.supervisor = sup
	return sup
}

func (m *WebSessionManager) List(includeArchived bool) []*WebAgentSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*WebAgentSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		if !includeArchived && s.Archived {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

func compactHistory(history []WebChatMessage, limit int) []WebChatMessage {
	if limit <= 0 || len(history) <= limit {
		return append([]WebChatMessage(nil), history...)
	}
	return append([]WebChatMessage(nil), history[len(history)-limit:]...)
}

func sessionDTO(s *WebAgentSession, historyLimit int) *WebAgentSessionDTO {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &WebAgentSessionDTO{
		ID:           s.ID,
		Name:         s.Name,
		Mode:         s.Mode,
		Workspace:    s.Workspace,
		Status:       s.Status,
		Archived:     s.Archived,
		History:      compactHistory(s.History, historyLimit),
		TotalHistory: len(s.History),
		HistoryLimit: historyLimit,
		CreatedAt:    s.CreatedAt,
		UpdatedAt:    s.UpdatedAt,
		Error:        s.Error,
	}
}

func (m *WebSessionManager) ListCompact(includeArchived bool, historyLimit int) []*WebAgentSessionDTO {
	m.mu.RLock()
	list := make([]*WebAgentSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		if !includeArchived && s.Archived {
			continue
		}
		list = append(list, s)
	}
	m.mu.RUnlock()
	sort.Slice(list, func(i, j int) bool { return list[i].UpdatedAt.After(list[j].UpdatedAt) })
	out := make([]*WebAgentSessionDTO, 0, len(list))
	for _, s := range list {
		out = append(out, sessionDTO(s, historyLimit))
	}
	return out
}

func (m *WebSessionManager) GetCompact(id string, historyLimit int) (*WebAgentSessionDTO, bool) {
	s, ok := m.Get(id)
	if !ok {
		return nil, false
	}
	return sessionDTO(s, historyLimit), true
}

func (m *WebSessionManager) Get(id string) (*WebAgentSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *WebSessionManager) Create(name, mode string) *WebAgentSession {
	if strings.TrimSpace(name) == "" {
		name = "Session " + time.Now().Format("15:04")
	}
	if mode == "" {
		mode = "ask"
	}
	now := time.Now()
	s := &WebAgentSession{ID: fmt.Sprintf("web-%d", now.UnixNano()), Name: name, Mode: mode, Workspace: m.workspace, Status: WebSessionIdle, History: []WebChatMessage{{Role: "assistant", Content: "Halo! Saya Smara. Sesi backend siap berjalan paralel.", Timestamp: now}}, CreatedAt: now, UpdatedAt: now}
	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()
	_ = m.Save()
	return s
}

func (m *WebSessionManager) Rename(id, name string) error {
	s, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("session tidak ditemukan")
	}
	s.mu.Lock()
	s.Name = strings.TrimSpace(name)
	s.UpdatedAt = time.Now()
	s.mu.Unlock()
	return m.Save()
}

func (m *WebSessionManager) Archive(id string, archived bool) error {
	s, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("session tidak ditemukan")
	}
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.Archived = archived
	if archived {
		s.Status = WebSessionArchived
	} else {
		s.Status = WebSessionIdle
	}
	s.UpdatedAt = time.Now()
	s.mu.Unlock()
	return m.Save()
}

func (m *WebSessionManager) Delete(id string) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if ok {
		s.mu.Lock()
		if s.cancel != nil {
			s.cancel()
			s.cancel = nil
		}
		s.mu.Unlock()
	}
	delete(m.sessions, id)
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("session tidak ditemukan")
	}
	return m.Save()
}

func (m *WebSessionManager) Cancel(id string) error {
	s, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("session tidak ditemukan")
	}
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.Status = WebSessionCancelled
		s.cancel = nil
	}
	s.UpdatedAt = time.Now()
	s.mu.Unlock()
	return m.Save()
}

func (m *WebSessionManager) RecordDirectResult(id, prompt, mode, response string, status WebSessionStatus, errText string) error {
	s, ok := m.Get(id)
	if !ok {
		s = m.Create("Session "+time.Now().Format("15:04"), mode)
	}
	now := time.Now()
	s.mu.Lock()
	if mode != "" {
		s.Mode = mode
	}
	s.Status = status
	s.Error = errText
	if strings.TrimSpace(prompt) != "" {
		s.History = append(s.History, WebChatMessage{Role: "user", Content: prompt, Timestamp: now})
	}
	if strings.TrimSpace(response) != "" {
		s.History = append(s.History, WebChatMessage{Role: "assistant", Content: response, Timestamp: now})
	}
	s.UpdatedAt = now
	s.mu.Unlock()
	return m.Save()
}

func (m *WebSessionManager) Run(ctx context.Context, id, prompt, mode string, cb agent.AgenticCallback) (*agent.PromptResult, error) {
	s, ok := m.Get(id)
	if !ok {
		s = m.Create("Session "+time.Now().Format("15:04"), mode)
		id = s.ID
	}
	if s.Archived {
		return nil, fmt.Errorf("session diarsipkan")
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	if s.Status == WebSessionRunning {
		s.mu.Unlock()
		cancel()
		return nil, fmt.Errorf("session sedang berjalan")
	}
	s.Status = WebSessionRunning
	s.Error = ""
	s.cancel = cancel
	if mode != "" {
		s.Mode = mode
	}
	s.History = append(s.History, WebChatMessage{Role: "user", Content: prompt, Timestamp: time.Now()})
	s.UpdatedAt = time.Now()
	s.mu.Unlock()
	_ = m.Save()
	sup := m.ensureSupervisor(s)
	if agent.ValidMode(mode) {
		sup.SetMode(agent.Mode(mode))
	}
	sup.SetCallback(cb)
	res, err := sup.ProcessPrompt(runCtx, prompt)
	sup.SetCallback(agent.AgenticCallback{})
	s.mu.Lock()
	s.cancel = nil
	if err != nil {
		if runCtx.Err() != nil || ctx.Err() != nil {
			s.Status = WebSessionCancelled
			s.Error = ""
		} else {
			s.Status = WebSessionError
			s.Error = err.Error()
		}
		s.UpdatedAt = time.Now()
		s.mu.Unlock()
		_ = m.Save()
		return nil, err
	}
	s.Status = WebSessionCompleted
	s.History = append(s.History, WebChatMessage{Role: "assistant", Content: res.Response, Timestamp: time.Now()})
	s.UpdatedAt = time.Now()
	s.mu.Unlock()
	_ = m.Save()
	return res, nil
}
