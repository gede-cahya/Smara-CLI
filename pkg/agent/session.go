// Package agent provides the multi-agent system for Smara.
package agent

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/gede-cahya/Smara-CLI/pkg/llm"
	"github.com/gede-cahya/Smara-CLI/pkg/mcp"
	"github.com/gede-cahya/Smara-CLI/pkg/session"
)

// SessionRegistry manages active sessions in memory.
type SessionRegistry struct {
	mu       sync.RWMutex
	sessions map[string]*session.Session
	current  *session.Session
}

// NewSessionRegistry creates a new session registry.
func NewSessionRegistry() *SessionRegistry {
	return &SessionRegistry{
		sessions: make(map[string]*session.Session),
	}
}

// generateSessionID creates a unique session ID.
func generateSessionID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Create creates a new session and sets it as current.
func (r *SessionRegistry) Create(cfg SessionConfig) (*session.Session, error) {
	id := generateSessionID()
	now := time.Now()

	s := &session.Session{
		ID:         id,
		Name:       cfg.Name,
		State:      session.StateActive,
		Mode:       cfg.Mode,
		MCPServers: cfg.MCPServers,
		History:    make([]llm.Message, 0),
		Tasks:      make([]session.Task, 0),
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	r.mu.Lock()
	r.sessions[id] = s
	r.current = s
	r.mu.Unlock()

	return s, nil
}

// Get retrieves a session by ID.
func (r *SessionRegistry) Get(id string) (*session.Session, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sessions[id]
	return s, ok
}

// Switch changes the current session.
func (r *SessionRegistry) Switch(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if s, ok := r.sessions[id]; ok {
		r.current = s
		return nil
	}
	return fmt.Errorf("session tidak ditemukan: %s", id)
}

// Current returns the current active session.
func (r *SessionRegistry) Current() *session.Session {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.current
}

// List returns all sessions.
func (r *SessionRegistry) List() []*session.Session {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*session.Session, 0, len(r.sessions))
	for _, s := range r.sessions {
		list = append(list, s)
	}
	return list
}

// EndCurrent marks the current session as ended.
func (r *SessionRegistry) EndCurrent() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.current != nil {
		r.current.State = session.StateEnded
		r.current.UpdatedAt = time.Now()
	}
	return nil
}

// End marks a session as ended by ID.
func (r *SessionRegistry) End(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if s, ok := r.sessions[id]; ok {
		s.State = session.StateEnded
		s.UpdatedAt = time.Now()
		return nil
	}
	return fmt.Errorf("session tidak ditemukan: %s", id)
}

// Register adds a session to the registry (for loading from persistence).
func (r *SessionRegistry) Register(s *session.Session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[s.ID] = s
	if r.current == nil {
		r.current = s
	}
}

// IsCurrent checks if a session ID is the current session.
func (r *SessionRegistry) IsCurrent(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.current != nil && r.current.ID == id
}

// UpdateHistory appends to a session's history.
func (r *SessionRegistry) UpdateHistory(sessionID string, userMsg, assistantMsg string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if s, ok := r.sessions[sessionID]; ok {
		s.History = append(s.History,
			llm.Message{Role: llm.RoleUser, Content: userMsg},
			llm.Message{Role: llm.RoleAssistant, Content: assistantMsg},
		)
		s.UpdatedAt = time.Now()
	}
}

// AddTask adds a task to a session.
func (r *SessionRegistry) AddTask(sessionID string, task session.Task) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if s, ok := r.sessions[sessionID]; ok {
		s.Tasks = append(s.Tasks, task)
		s.UpdatedAt = time.Now()
	}
}

// AppendHistory appends messages to a session's history and returns the session ID for auto-save.
func (r *SessionRegistry) AppendHistory(sessionID string, userMsg, assistantMsg string) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if s, ok := r.sessions[sessionID]; ok {
		s.History = append(s.History,
			llm.Message{Role: llm.RoleUser, Content: userMsg},
			llm.Message{Role: llm.RoleAssistant, Content: assistantMsg},
		)
		s.UpdatedAt = time.Now()
		return s.ID
	}
	return ""
}

// SetMode changes the mode of a session.
func (r *SessionRegistry) SetMode(sessionID string, mode string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if s, ok := r.sessions[sessionID]; ok {
		s.Mode = mode
		s.UpdatedAt = time.Now()
	}
}

// SessionManager manages sessions with persistence and MCP integration.
type SessionManager struct {
	registry    *SessionRegistry
	store       *session.SQLiteStore
	mcpClients  map[string]*mcp.Client
	mu          sync.RWMutex
	saveOnTurn  bool // auto-save after each conversation turn
}

// NewSessionManager creates a new session manager with persistence.
func NewSessionManager(store *session.SQLiteStore) *SessionManager {
	return &SessionManager{
		registry:   NewSessionRegistry(),
		store:      store,
		mcpClients: make(map[string]*mcp.Client),
		saveOnTurn: true, // enabled by default
	}
}

// SetSaveOnTurn enables or disables auto-save after each turn.
func (m *SessionManager) SetSaveOnTurn(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saveOnTurn = enabled
}

// saveIfNeeded persists a session if saveOnTurn is enabled.
func (m *SessionManager) saveIfNeeded(sessionID string) {
	if !m.saveOnTurn || m.store == nil {
		return
	}
	m.SaveSession(sessionID) // ignore error — best effort
}

// LoadSessions loads all sessions from persistent store.
func (m *SessionManager) LoadSessions() error {
	if m.store == nil {
		return nil
	}

	sessions, err := m.store.ListSessions()
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range sessions {
		m.registry.Register(&sessions[i])
	}

	return nil
}

// SaveSession persists a session to the store.
func (m *SessionManager) SaveSession(sessionID string) error {
	m.mu.RLock()
	session, ok := m.registry.Get(sessionID)
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("session tidak ditemukan: %s", sessionID)
	}

	if m.store == nil {
		return nil
	}

	return m.store.UpdateSession(session)
}

// ConnectMCP connects an MCP server to a session.
func (m *SessionManager) ConnectMCP(sessionID string, serverName string, client *mcp.Client) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.registry.Get(sessionID)
	if !ok {
		return fmt.Errorf("session tidak ditemukan: %s", sessionID)
	}

	for _, s := range session.MCPServers {
		if s == serverName {
			return nil
		}
	}

	session.MCPServers = append(session.MCPServers, serverName)
	m.mcpClients[serverName] = client

	if m.store != nil {
		m.store.UpdateSession(session)
	}

	return nil
}

// DisconnectMCP disconnects an MCP server from a session.
func (m *SessionManager) DisconnectMCP(sessionID string, serverName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.registry.Get(sessionID)
	if !ok {
		return fmt.Errorf("session tidak ditemukan: %s", sessionID)
	}

	newServers := make([]string, 0)
	for _, s := range session.MCPServers {
		if s != serverName {
			newServers = append(newServers, s)
		}
	}
	session.MCPServers = newServers

	if client, ok := m.mcpClients[serverName]; ok {
		client.Close()
		delete(m.mcpClients, serverName)
	}

	if m.store != nil {
		m.store.UpdateSession(session)
	}

	return nil
}

// GetMCPClients returns all MCP clients for a session.
func (m *SessionManager) GetMCPClients(sessionID string) map[string]*mcp.Client {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.registry.Get(sessionID)
	if !ok {
		return nil
	}

	clients := make(map[string]*mcp.Client)
	for _, serverName := range session.MCPServers {
		if client, ok := m.mcpClients[serverName]; ok {
			clients[serverName] = client
		}
	}

	return clients
}

// ResumeSession resumes an agentic session.
func (m *SessionManager) ResumeSession(sessionID string) (*session.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.registry.Get(sessionID)
	if !ok {
		return nil, fmt.Errorf("session tidak ditemukan: %s", sessionID)
	}

	if !sess.IsAgentic {
		return nil, fmt.Errorf("session bukan agentic mode")
	}

	sess.State = session.StateActive
	sess.UpdatedAt = time.Now()

	if m.store != nil {
		m.store.UpdateSession(sess)
	}

	return sess, nil
}

// AddMemoryReference adds a memory reference to the session.
func (m *SessionManager) AddMemoryReference(sessionID string, memoryID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.registry.Get(sessionID)
	if !ok {
		return fmt.Errorf("session tidak ditemukan: %s", sessionID)
	}

	session.MemoryIDs = append(session.MemoryIDs, memoryID)
	session.UpdatedAt = time.Now()

	if m.store != nil {
		m.store.UpdateSession(session)
	}

	return nil
}

// UpdateContext updates the session context/summary.
func (m *SessionManager) UpdateContext(sessionID string, context string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.registry.Get(sessionID)
	if !ok {
		return fmt.Errorf("session tidak ditemukan: %s", sessionID)
	}

	session.Context = context
	session.UpdatedAt = time.Now()

	if m.store != nil {
		m.store.UpdateSession(session)
	}

	return nil
}

// Registry returns the underlying session registry.
func (m *SessionManager) Registry() *SessionRegistry {
	return m.registry
}
