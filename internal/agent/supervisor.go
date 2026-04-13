package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cahya/smara/internal/llm"
	"github.com/cahya/smara/internal/mcp"
	"github.com/cahya/smara/internal/memory"
)

// Supervisor orchestrates multi-agent task execution.
type Supervisor struct {
	provider    llm.Provider
	memStore    memory.MemoryStore
	mcpClients  map[string]*mcp.Client
	workers     []*Worker
	taskCh      chan Task
	resultCh    chan TaskResult
	mu          sync.RWMutex
	maxWorkers  int
	mode        Mode
	history     []llm.Message // conversation history for context
}

// NewSupervisor creates a new supervisor agent.
func NewSupervisor(provider llm.Provider, memStore memory.MemoryStore) *Supervisor {
	return &Supervisor{
		provider:   provider,
		memStore:   memStore,
		mcpClients: make(map[string]*mcp.Client),
		taskCh:     make(chan Task, 100),
		resultCh:   make(chan TaskResult, 100),
		maxWorkers: 4,
		mode:       ModeAsk, // default mode
		history:    make([]llm.Message, 0),
	}
}

// SetMode changes the agent's operating mode.
func (s *Supervisor) SetMode(mode Mode) {
	s.mode = mode
	// Clear conversation history on mode change
	s.history = make([]llm.Message, 0)
}

// GetMode returns the current agent mode.
func (s *Supervisor) GetMode() Mode {
	return s.mode
}

// RegisterMCPClient adds an MCP server connection.
func (s *Supervisor) RegisterMCPClient(name string, client *mcp.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mcpClients[name] = client
}

// GetMCPClient retrieves an MCP client by name.
func (s *Supervisor) GetMCPClient(name string) (*mcp.Client, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.mcpClients[name]
	return c, ok
}

// ListMCPServers returns names of all connected MCP servers.
func (s *Supervisor) ListMCPServers() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.mcpClients))
	for name := range s.mcpClients {
		names = append(names, name)
	}
	return names
}

// ProcessPrompt handles a user prompt using the current agent mode.
func (s *Supervisor) ProcessPrompt(ctx context.Context, userPrompt string) (string, error) {
	modeInfo := GetModeInfo(s.mode)

	// 1. Search memory for relevant context
	var memContext string
	if s.memStore != nil {
		embedding, err := s.provider.GenerateEmbedding(userPrompt)
		if err == nil && len(embedding) > 0 {
			results, err := s.memStore.Search(embedding, 3)
			if err == nil && len(results) > 0 {
				var parts []string
				for _, r := range results {
					parts = append(parts, fmt.Sprintf("- %s (relevansi: %.2f)", r.Memory.Content, r.Similarity))
				}
				memContext = "Konteks dari memori:\n" + strings.Join(parts, "\n")
			}
		}
	}

	// 2. Build messages with mode-specific system prompt
	messages := []llm.Message{
		{
			Role:    llm.RoleSystem,
			Content: modeInfo.SystemPrompt,
		},
	}

	// Add MCP tools context (only relevant for rush and plan modes)
	if s.mode == ModeRush || s.mode == ModePlan {
		mcpServers := s.ListMCPServers()
		if len(mcpServers) > 0 {
			toolsDesc := "MCP Servers yang tersedia: " + strings.Join(mcpServers, ", ")
			messages = append(messages, llm.Message{
				Role:    llm.RoleSystem,
				Content: toolsDesc,
			})
		}
	}

	// Add memory context
	if memContext != "" {
		messages = append(messages, llm.Message{
			Role:    llm.RoleSystem,
			Content: memContext,
		})
	}

	// Add conversation history (keep last 10 exchanges for context)
	maxHistory := 20 // 10 pairs of user+assistant
	if len(s.history) > maxHistory {
		s.history = s.history[len(s.history)-maxHistory:]
	}
	messages = append(messages, s.history...)

	// Add user prompt
	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: userPrompt,
	})

	// 3. Call LLM
	resp, err := s.provider.Chat(messages)
	if err != nil {
		return "", fmt.Errorf("gagal mendapatkan response dari LLM: %w", err)
	}

	// 4. Update conversation history
	s.history = append(s.history,
		llm.Message{Role: llm.RoleUser, Content: userPrompt},
		llm.Message{Role: llm.RoleAssistant, Content: resp.Content},
	)

	// 5. Save interaction to memory
	if s.memStore != nil {
		tag := fmt.Sprintf("mode:%s", s.mode)
		content := fmt.Sprintf("Q: %s\nA: %s", userPrompt, truncate(resp.Content, 500))
		embedding, _ := s.provider.GenerateEmbedding(content)
		s.memStore.Save(content, tag, "supervisor", embedding)
	}

	return resp.Content, nil
}

// Close shuts down all MCP client connections.
func (s *Supervisor) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, client := range s.mcpClients {
		client.Close()
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ExecuteTask runs a single task with timeout.
func (s *Supervisor) ExecuteTask(ctx context.Context, task Task) TaskResult {
	timeout := 60 * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resultCh := make(chan TaskResult, 1)

	go func() {
		worker := NewWorker(s.provider, s.mcpClients)
		result := worker.Execute(ctx, task)
		resultCh <- result
	}()

	select {
	case result := <-resultCh:
		return result
	case <-ctx.Done():
		return TaskResult{
			TaskID: task.ID,
			Status: TaskFailed,
			Error:  "task timeout",
		}
	}
}
