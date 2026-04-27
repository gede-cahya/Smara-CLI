// Package integrations wires together all new PRD features:
// Safety, Autonomy, Sandbox, LSP, Memory Compaction, and Audit Log.
package integrations

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/audit"
	"github.com/gede-cahya/Smara-CLI/internal/autonomy"
	"github.com/gede-cahya/Smara-CLI/internal/cognitive"
	"github.com/gede-cahya/Smara-CLI/internal/lsp"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/gede-cahya/Smara-CLI/internal/safety"
	"github.com/gede-cahya/Smara-CLI/internal/sandbox"
)

// Engine holds all integrated subsystems.
type Engine struct {
	Safety    *safety.Engine
	Autonomy  *autonomy.Engine
	Sandbox   *sandbox.Sandbox
	LSP       *lsp.Manager
	Compactor *memory.Compactor
	Audit     *audit.Logger
	Cognitive *cognitive.Validator
}

// NewEngine creates and initializes all integrated subsystems.
func NewEngine(memStore memory.MemoryStore) (*Engine, error) {
	// 1. Safety Engine
	safetyEngine := safety.NewEngine()

	// 2. Sandbox
	sbx := sandbox.New()
	sbx.SetTimeout(60 * time.Second)

	// 3. LSP Manager
	lspManager := lsp.NewManager()

	// 4. Audit Logger
	auditDir := filepath.Join(os.TempDir(), "smara_audit")
	auditLogger, err := audit.NewLogger(auditDir)
	if err != nil {
		return nil, fmt.Errorf("gagal inisialisasi audit logger: %w", err)
	}

	// 5. Memory Compactor
	compactor := memory.NewCompactor(memStore, memory.DefaultCompactionConfig)

	// 6. Autonomy Engine
	autonomyEngine := autonomy.NewEngine()

	// 7. Cognitive Validator
	cognitiveValidator := cognitive.NewValidator()

	// Register default checkers/executors for autonomy
	registerDefaultAutonomyHandlers(autonomyEngine, compactor)

	return &Engine{
		Safety:    safetyEngine,
		Autonomy:  autonomyEngine,
		Sandbox:   sbx,
		LSP:       lspManager,
		Compactor: compactor,
		Audit:     auditLogger,
		Cognitive: cognitiveValidator,
	}, nil
}

// Start starts all background engines.
func (e *Engine) Start(ctx context.Context) {
	e.Autonomy.Start(ctx)
}

// Stop gracefully shuts down all engines.
func (e *Engine) Stop() {
	e.Autonomy.Stop()
	e.LSP.CloseAll()
	e.Audit.Close()
}

// SetSafetyMode sets the safety execution mode.
func (e *Engine) SetSafetyMode(mode safety.ExecutionMode) {
	e.Safety.SetMode(mode)
}

// GetSafetyMode returns the current safety mode.
func (e *Engine) GetSafetyMode() safety.ExecutionMode {
	return e.Safety.GetMode()
}

// ExecuteInSandbox runs a command through the sandbox.
func (e *Engine) ExecuteInSandbox(ctx context.Context, command string, args ...string) *sandbox.Result {
	return e.Sandbox.Execute(ctx, command, args...)
}

// LogAudit records an audit entry.
func (e *Engine) LogAudit(entry audit.Entry) {
	e.Audit.Log(entry)
}

// CompactMemory triggers memory compaction.
func (e *Engine) CompactMemory() error {
	return e.Compactor.Compact()
}

// GetLSPClient gets or creates an LSP client for a language.
func (e *Engine) GetLSPClient(lang, workspaceDir string) (*lsp.Client, error) {
	return e.LSP.GetOrCreateClient(lang, workspaceDir)
}

// ValidateToolArgs validates tool arguments against registered cognitive schemas.
func (e *Engine) ValidateToolArgs(toolName string, args map[string]interface{}) cognitive.ValidationResult {
	if e.Cognitive == nil {
		return cognitive.ValidationResult{Valid: true}
	}
	return e.Cognitive.Validate(toolName, args)
}

func registerDefaultAutonomyHandlers(engine *autonomy.Engine, compactor *memory.Compactor) {
	// Memory cleanup handler
	engine.RegisterChecker("memory_cleanup", func() (bool, map[string]interface{}) {
		should := compactor.ShouldCompact()
		return should, map[string]interface{}{"reason": "memory threshold reached"}
	})
	engine.RegisterExecutor("memory_cleanup", func(ctx context.Context, context map[string]interface{}) error {
		return compactor.Compact()
	})

	// Health check handler (placeholder)
	engine.RegisterChecker("health_check", func() (bool, map[string]interface{}) {
		return true, map[string]interface{}{"status": "ok"}
	})
	engine.RegisterExecutor("health_check", func(ctx context.Context, context map[string]interface{}) error {
		return nil
	})
}
