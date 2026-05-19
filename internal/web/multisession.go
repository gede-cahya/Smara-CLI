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
}

func NewWebSessionManager(provider llm.Provider, providerCfg llm.ProviderConfig, memStore memory.MemoryStore, workspace string, workspaceID int64, maxIter int, storePath string) *WebSessionManager {
	if storePath == "" {
		home, _ := os.UserHomeDir()
		storePath = filepath.Join(home, ".smara", "web-sessions.json")
	}
	m := &WebSessionManager{sessions: map[string]*WebAgentSession{}, storePath: storePath, provider: provider, providerCfg: providerCfg, memStore: memStore, workspace: workspace, workspaceID: workspaceID, maxIter: maxIter}
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

func (m *WebSessionManager) Save() error {
	m.mu.RLock()
	list := make([]*WebAgentSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s)
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
	s.Name = name
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
	if ok && s.cancel != nil {
		s.cancel()
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
	defer s.mu.Unlock()
	s.cancel = nil
	if err != nil {
		s.Status = WebSessionError
		s.Error = err.Error()
		s.UpdatedAt = time.Now()
		_ = m.Save()
		return nil, err
	}
	s.Status = WebSessionCompleted
	s.History = append(s.History, WebChatMessage{Role: "assistant", Content: res.Response, Timestamp: time.Now()})
	s.UpdatedAt = time.Now()
	_ = m.Save()
	return res, nil
}
