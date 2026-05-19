package web

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/mcp"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/gede-cahya/Smara-CLI/internal/metrics"
	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Server runs the Smara web interface.
type Server struct {
	Addr          string
	Supervisor    *agent.Supervisor
	MemStore      memory.MemoryStore
	Metrics       *metrics.MetricsCollector
	Cfg           *config.SmaraConfig
	WebSessions   *WebSessionManager
	AuthToken     string
	RemoteDesktop *RemoteDesktopManager
	mcpClients    map[string]*mcp.Client
	SkillTracker  *skill.ExecutionTracker
	mu            sync.RWMutex
	sessions      map[string]*ChatSession
}

// ChatSession tracks a single WebSocket conversation.
type ChatSession struct {
	ID        string
	Conn      *websocket.Conn
	History   []llm.Message
	Workspace string
	Mode      agent.Mode
	mu        sync.Mutex
}

func (cs *ChatSession) WriteJSON(v interface{}) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.Conn.WriteJSON(v)
}

// NewServer creates a new web server.
func NewServer(addr string, supervisor *agent.Supervisor, memStore memory.MemoryStore, mc *metrics.MetricsCollector, cfg *config.SmaraConfig) *Server {
	srv := &Server{
		Addr:       addr,
		Supervisor: supervisor,
		MemStore:   memStore,
		Metrics:    mc,
		Cfg:        cfg,
		mcpClients: make(map[string]*mcp.Client),
		sessions:   make(map[string]*ChatSession),
	}
	if memStore != nil {
		if m, ok := memStore.(*memory.SQLiteStore); ok {
			if t, err := skill.NewExecutionTracker(m.DB()); err == nil {
				srv.SkillTracker = t
			}
		}
	}
	return srv
}

// Start launches the HTTP server and blocks until shutdown.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/fs/cwd", s.handleFSCwd)
	mux.HandleFunc("/api/fs/list", s.handleFSList)
	mux.HandleFunc("/api/memories", s.handleMemories)
	mux.HandleFunc("/api/memories/search", s.handleMemorySearch)
	mux.HandleFunc("/api/memories/graph", s.handleMemoryGraph)
	mux.HandleFunc("/api/memories/links", s.handleMemoryLinks)
	mux.HandleFunc("/api/memories/autolink", s.handleMemoryAutolink)
	mux.HandleFunc("/api/workspaces", s.handleWorkspaces)
	mux.HandleFunc("/api/workspaces/switch", s.handleWorkspaceSwitch)
	mux.HandleFunc("/api/workspaces/create", s.handleWorkspaceCreate)
	mux.HandleFunc("/api/categories", s.handleCategories)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/mcp", s.handleMCPStatus)
	mux.HandleFunc("/api/metrics", s.handleMetrics)
	mux.HandleFunc("/api/skills", s.handleSkills)
	mux.HandleFunc("/api/skills/run", s.handleSkillRun)
	mux.HandleFunc("/api/skills/import", s.handleSkillImport)
	mux.HandleFunc("/api/skills/bundled", s.handleSkillsBundled)
	mux.HandleFunc("/api/skills/install-bundled", s.handleSkillsInstallBundled)
	mux.HandleFunc("/api/skills/tree", s.handleSkillTree)
	mux.HandleFunc("/api/skills/tree/", s.handleSkillTreeSubtree)
	mux.HandleFunc("/api/skills/stats", s.handleSkillStats)
	mux.HandleFunc("/api/skills/timeline", s.handleSkillTimeline)
	mux.HandleFunc("/api/skills/analytics", s.handleSkillAnalytics)
	mux.HandleFunc("/api/skills/refine", s.handleSkillRefine)
	mux.HandleFunc("/api/skills/dependencies", s.handleSkillDependencies)
	mux.HandleFunc("/api/skills/export", s.handleSkillExport)
	mux.HandleFunc("/api/skills/import-tree", s.handleSkillTreeImport)
	mux.HandleFunc("/api/blueprint/generate", s.handleBlueprintGenerate)
	mux.HandleFunc("/api/blueprint/execute", s.handleBlueprintExecute)
	mux.HandleFunc("/api/custom-workflow/list", s.handleCustomWorkflowList)
	mux.HandleFunc("/api/custom-workflow/get", s.handleCustomWorkflowGet)
	mux.HandleFunc("/api/custom-workflow/save", s.handleCustomWorkflowSave)
	mux.HandleFunc("/api/custom-workflow/delete", s.handleCustomWorkflowDelete)
	mux.HandleFunc("/api/custom-workflow/run", s.handleCustomWorkflowRun)
	mux.HandleFunc("/api/custom-workflow/import", s.handleCustomWorkflowImport)
	mux.HandleFunc("/api/mode", s.handleMode)
	mux.HandleFunc("/api/graph/init", s.handleGraphInit)
	mux.HandleFunc("/api/graph/list", s.handleGraphList)
	mux.HandleFunc("/api/graph/get", s.handleGraphGet)
	mux.HandleFunc("/api/graph/query", s.handleGraphQuery)
	mux.HandleFunc("/api/graph/nodes", s.handleGraphNodes)
	mux.HandleFunc("/api/graph/neighbors", s.handleGraphNeighbors)
	mux.HandleFunc("/api/graph/path", s.handleGraphPath)
	mux.HandleFunc("/api/graph/data", s.handleGraphData)
	mux.HandleFunc("/api/graph/export", s.handleGraphExport)
	mux.HandleFunc("/api/clipboard/upload", s.handleClipboardUpload)
	mux.HandleFunc("/api/attachments/upload", s.handleAttachmentUpload)
	mux.HandleFunc("/api/generated-image", s.handleGeneratedImage)
	mux.HandleFunc("/api/local-image", s.handleLocalImage)
	mux.HandleFunc("/api/browser-artifact", s.handleBrowserArtifact)
	mux.HandleFunc("/api/web-sessions", s.handleWebSessions)
	mux.HandleFunc("/api/web-sessions/", s.handleWebSessionByID)
	mux.HandleFunc("/api/voice/settings", s.handleVoiceSettings)
	mux.HandleFunc("/api/voice/command", s.handleVoiceCommand)
	mux.HandleFunc("/api/voice/speak", s.handleVoiceSpeak)
	mux.HandleFunc("/api/avatar/state", s.handleAvatarState)
	mux.HandleFunc("/api/avatar/event", s.handleAvatarEvent)
	mux.HandleFunc("/api/remote-desktop/devices", s.handleRemoteDesktopDevices)
	mux.HandleFunc("/api/remote-desktop/devices/", s.handleRemoteDesktopDeviceByID)
	mux.HandleFunc("/api/remote-desktop/proxy", s.handleRemoteDesktopProxy)
	mux.HandleFunc("/api/remote-desktop/screenshot", s.handleRemoteDesktopScreenshot)
	mux.HandleFunc("/ws", s.handleWebSocket)

	srv := &http.Server{
		Addr:    s.Addr,
		Handler: s.withAuth(cors(mux)),
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("[web] Server starting on http://%s", s.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	b, _ := json.Marshal(data)
	_, _ = w.Write(b)
}

func errorResponse(w http.ResponseWriter, status int, msg string) {
	jsonResponse(w, status, map[string]string{"error": msg})
}
