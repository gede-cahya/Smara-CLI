package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gede-cahya/Smara-CLI/pkg/agent"
	"github.com/gede-cahya/Smara-CLI/pkg/config"
	"github.com/gede-cahya/Smara-CLI/pkg/llm"
	"github.com/gede-cahya/Smara-CLI/pkg/memory"
	"github.com/gede-cahya/Smara-CLI/pkg/session"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx        context.Context
	supervisor *agent.Supervisor
	memStore   memory.MemoryStore
	sessStore  session.Store
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.initSmara()
}

func (a *App) initSmara() {
	// 1. Load Config
	if err := config.Init(""); err != nil {
		fmt.Fprintf(os.Stderr, "Error config: %v\n", err)
		return
	}
	cfg := config.Get()

	// 2. Initialize LLM Provider
	providerCfg := llm.ProviderConfig{
		Name:   cfg.Provider,
		Model:  cfg.Model,
		Host:   cfg.OllamaHost,
		APIKey: "",
	}

	switch cfg.Provider {
	case "openai":
		providerCfg.APIKey = cfg.OpenAIAPIKey
		providerCfg.Host = cfg.OpenAIBaseURL
	case "openrouter":
		providerCfg.APIKey = cfg.OpenRouterAPIKey
		if cfg.Model == "" {
			providerCfg.Model = cfg.OpenRouterModel
		}
	case "anthropic":
		providerCfg.APIKey = cfg.AnthropicAPIKey
		if cfg.Model == "" {
			providerCfg.Model = cfg.AnthropicModel
		}
	case "custom":
		providerCfg.APIKey = cfg.CustomAPIKey
		providerCfg.Host = cfg.CustomBaseURL
	}

	provider, err := llm.NewProvider(providerCfg)
	if err != nil {
		runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "LLM Error",
			Message: fmt.Sprintf("Gagal inisialisasi LLM: %v", err),
		})
		return
	}

	// 3. Initialize Memory Store
	a.memStore, err = memory.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "DB Error",
			Message: fmt.Sprintf("Gagal membuka database: %v", err),
		})
		return
	}

	// 4. Initialize Supervisor
	a.supervisor = agent.NewSupervisorWithConfig(provider, providerCfg, a.memStore)

	// Set Workspace
	w, _ := a.memStore.GetWorkspaceByName(cfg.ActiveWorkspace)
	if w != nil {
		a.supervisor.SetWorkspaceID(w.ID)
	}

	// Set Session Store
	a.sessStore, _ = session.NewSQLiteStore(cfg.DBPath)
	if a.sessStore != nil {
		a.supervisor.SetSessionStore(a.sessStore)
		_ = a.supervisor.InitializeSessions()
		
		// Auto-connect to last session
		lastSess, _ := a.supervisor.GetLastActiveSession()
		if lastSess != nil {
			_ = a.supervisor.SwitchSession(lastSess.ID)
		} else {
			// Create new one if none
			_, _ = a.supervisor.CreateSession(agent.SessionConfig{
				Name: "Desktop Session",
				Mode: string(agent.ModeAsk),
			})
		}
	}

	// Setup Stream Callback
	a.supervisor.SetCallback(agent.AgenticCallback{
		OnStream: func(chunk string, isThinking bool) {
			runtime.EventsEmit(a.ctx, "stream_chunk", map[string]interface{}{
				"chunk":       chunk,
				"is_thinking": isThinking,
			})
		},
	})
}

// Ask sends a prompt to Smara and returns the final response
func (a *App) Ask(prompt string) (string, error) {
	if a.supervisor == nil {
		return "", fmt.Errorf("supervisor belum siap")
	}

	res, err := a.supervisor.ProcessPrompt(a.ctx, prompt)
	if err != nil {
		return "", err
	}

	return res.Response, nil
}

// GetWorkspaces returns all available workspaces
func (a *App) GetWorkspaces() ([]memory.Workspace, error) {
	if a.memStore == nil {
		return nil, fmt.Errorf("memStore belum siap")
	}
	return a.memStore.ListWorkspaces()
}

// GetSessions returns all sessions for the active workspace
func (a *App) GetSessions() ([]session.Session, error) {
	if a.sessStore == nil {
		return nil, fmt.Errorf("sessStore belum siap")
	}
	wID := a.supervisor.GetWorkspaceID()
	return a.sessStore.ListSessionsByWorkspace(wID)
}

// SwitchSession switches to a different session
func (a *App) SwitchSession(id string) error {
	if a.supervisor == nil {
		return fmt.Errorf("supervisor belum siap")
	}
	return a.supervisor.SwitchSession(id)
}

// SetWorkspace switches the active workspace
func (a *App) SetWorkspace(id int64) error {
	if a.supervisor == nil {
		return fmt.Errorf("supervisor belum siap")
	}
	a.supervisor.SetWorkspaceID(id)
	// After switching workspace, we should probably switch to the last active session in that workspace
	lastSess, _ := a.supervisor.GetLastActiveSession()
	if lastSess != nil {
		return a.supervisor.SwitchSession(lastSess.ID)
	}
	return nil
}

// GetSessionHistory returns message history for the active session
func (a *App) GetSessionHistory() ([]llm.Message, error) {
	if a.supervisor == nil {
		return nil, fmt.Errorf("supervisor belum siap")
	}
	sess := a.supervisor.GetCurrentSession()
	if sess == nil {
		return nil, fmt.Errorf("tidak ada session aktif")
	}
	return sess.History, nil
}

// CreateSession creates a new session and switches to it
func (a *App) CreateSession(name string) (string, error) {
	if a.supervisor == nil {
		return "", fmt.Errorf("supervisor belum siap")
	}
	
	sess, err := a.supervisor.CreateSession(agent.SessionConfig{
		Name: name,
		Mode: string(agent.ModeAsk), // Default to Ask for now
	})
	if err != nil {
		return "", err
	}
	
	return sess.ID, nil
}

// GetConfig returns the current configuration
func (a *App) GetConfig() (*config.SmaraConfig, error) {
	return config.Get(), nil
}

// UpdateConfig updates the configuration and saves it
func (a *App) UpdateConfig(newCfg config.SmaraConfig) error {
	// Update local config
	curr := config.Get()
	*curr = newCfg
	
	// Save to file
	if err := config.Save(); err != nil {
		return err
	}
	
	// Re-initialize LLM and Supervisor to apply changes
	a.initSmara()
	return nil
}

// GetTools returns all available tools from the supervisor
func (a *App) GetTools() ([]map[string]interface{}, error) {
	if a.supervisor == nil {
		return nil, fmt.Errorf("supervisor belum siap")
	}

	// 1. Get Builtin Tools
	builtin := agent.GetBuiltinTools()
	
	// 2. Get MCP Tools
	mcpInfo := a.supervisor.GetMCPInfo()
	
	var result []map[string]interface{}
	
	// Process Builtin
	for _, t := range builtin {
		result = append(result, map[string]interface{}{
			"name":        t.Name,
			"description": t.Description,
			"source":      "builtin",
		})
	}
	
	// Process MCP
	for server, info := range mcpInfo {
		if !info.Connected {
			continue
		}
		for _, t := range info.Tools {
			result = append(result, map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"source":      server,
			})
		}
	}
	
	return result, nil
}

// Greet returns a greeting for the given name (Legacy)
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}
