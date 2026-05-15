// Package safety implements the Two-Step Safety layer (Plan Mode vs Build Mode)
// and Auto-Revert functionality for Smara CLI.
package safety

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ExecutionMode represents the current safety execution mode.
type ExecutionMode string

const (
	ModePlan  ExecutionMode = "plan"  // Read-only: no write/execute allowed
	ModeBuild ExecutionMode = "build" // Read-write: can execute after validation
)

// ActionType categorizes what kind of action a tool performs.
type ActionType string

const (
	ActionRead    ActionType = "read"
	ActionWrite   ActionType = "write"
	ActionExecute ActionType = "execute"
	ActionDelete  ActionType = "delete"
)

// ToolAction maps a tool name to its action type.
var toolActions = map[string]ActionType{
	"view_file":         ActionRead,
	"read_file":         ActionRead,
	"list_directory":    ActionRead,
	"list_dir":          ActionRead,
	"search_memories":   ActionRead,
	"remember":          ActionWrite,
	"write_file":        ActionWrite,
	"edit_file":         ActionWrite,
	"patch_file":        ActionWrite,
	"execute_command":   ActionExecute,
	"run_command":       ActionExecute,
	"run_terminal":      ActionExecute,
	"delete_file":       ActionDelete,
	"analyze_workspace": ActionRead,
	"grep_search":       ActionRead,
	"search_path":       ActionRead,
	"get_cwd":           ActionRead,
	"web_search":        ActionRead,
}

// Engine is the safety engine that enforces execution rules.
type Engine struct {
	mode         ExecutionMode
	drafts       []DraftAction
	fileBackups  map[string]*FileBackup
	mu           sync.RWMutex
	onModeChange func(ExecutionMode)
}

// DraftAction represents a proposed action in Plan Mode.
type DraftAction struct {
	ID        string                 `json:"id"`
	Tool      string                 `json:"tool"`
	Args      map[string]interface{} `json:"args"`
	Action    ActionType             `json:"action"`
	Approved  bool                   `json:"approved"`
	Timestamp time.Time              `json:"timestamp"`
}

// FileBackup stores a snapshot of a file before modification.
type FileBackup struct {
	OriginalPath string    `json:"original_path"`
	BackupPath   string    `json:"backup_path"`
	OriginalHash string    `json:"original_hash"`
	Timestamp    time.Time `json:"timestamp"`
}

// NewEngine creates a new safety engine.
func NewEngine() *Engine {
	return &Engine{
		mode:        ModePlan,
		drafts:      make([]DraftAction, 0),
		fileBackups: make(map[string]*FileBackup),
	}
}

// SetMode changes the execution mode.
func (e *Engine) SetMode(mode ExecutionMode) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.mode = mode
	if e.onModeChange != nil {
		e.onModeChange(mode)
	}
}

// GetMode returns the current execution mode.
func (e *Engine) GetMode() ExecutionMode {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.mode
}

// CanExecute checks if a tool can be executed in the current mode.
func (e *Engine) CanExecute(toolName string) (bool, string) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	action, known := toolActions[toolName]
	if !known {
		// Unknown tools: allow in build mode, block in plan mode
		if e.mode == ModeBuild {
			return true, ""
		}
		return false, fmt.Sprintf("tool '%s' tidak dikenal dan diblokir dalam Plan Mode", toolName)
	}

	switch e.mode {
	case ModePlan:
		if action == ActionRead {
			return true, ""
		}
		return false, fmt.Sprintf("tool '%s' (%s) diblokir dalam Plan Mode. Gunakan Build Mode untuk eksekusi.", toolName, action)
	case ModeBuild:
		return true, ""
	default:
		return false, "mode tidak dikenal"
	}
}

// EvaluatePolicy checks the explicit policy file without changing legacy mode behavior.
func (e *Engine) EvaluatePolicy(toolName, target string) (bool, PolicyResult, string) {
	policy, err := LoadPolicy()
	if err != nil {
		return false, PolicyResult{Decision: DecisionDeny, Risk: RiskHigh, Reason: err.Error()}, err.Error()
	}
	result := policy.Evaluate(PolicyRequest{Tool: toolName, Target: target})
	switch result.Decision {
	case DecisionAllow:
		return true, result, ""
	case DecisionAsk:
		return false, result, fmt.Sprintf("tool '%s' membutuhkan konfirmasi policy (%s)", toolName, result.Risk)
	case DecisionDeny:
		return false, result, fmt.Sprintf("tool '%s' ditolak policy (%s): %s", toolName, result.Risk, result.Reason)
	default:
		return false, result, "decision policy tidak dikenal"
	}
}

// RecordDraft records a proposed action in Plan Mode.
func (e *Engine) RecordDraft(tool string, args map[string]interface{}) *DraftAction {
	e.mu.Lock()
	defer e.mu.Unlock()

	action := ActionRead
	if a, ok := toolActions[tool]; ok {
		action = a
	}

	draft := DraftAction{
		ID:        fmt.Sprintf("draft_%d", len(e.drafts)+1),
		Tool:      tool,
		Args:      args,
		Action:    action,
		Timestamp: time.Now(),
	}
	e.drafts = append(e.drafts, draft)
	return &draft
}

// GetDrafts returns all recorded drafts.
func (e *Engine) GetDrafts() []DraftAction {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]DraftAction, len(e.drafts))
	copy(result, e.drafts)
	return result
}

// ApproveDraft approves a draft by ID.
func (e *Engine) ApproveDraft(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.drafts {
		if e.drafts[i].ID == id {
			e.drafts[i].Approved = true
			return true
		}
	}
	return false
}

// ClearDrafts removes all drafts.
func (e *Engine) ClearDrafts() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.drafts = make([]DraftAction, 0)
}

// BackupFile creates a backup of a file before modification.
func (e *Engine) BackupFile(filePath string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, store empty backup
			e.fileBackups[absPath] = &FileBackup{
				OriginalPath: absPath,
				BackupPath:   "",
				OriginalHash: "",
				Timestamp:    time.Now(),
			}
			return nil
		}
		return err
	}

	// Create backup in temp directory
	tempDir := os.TempDir()
	backupName := fmt.Sprintf("smara_backup_%d_%s", time.Now().Unix(), filepath.Base(absPath))
	backupPath := filepath.Join(tempDir, backupName)

	if err := os.WriteFile(backupPath, data, 0o644); err != nil {
		return err
	}

	e.fileBackups[absPath] = &FileBackup{
		OriginalPath: absPath,
		BackupPath:   backupPath,
		OriginalHash: hashContent(string(data)),
		Timestamp:    time.Now(),
	}
	return nil
}

// RevertFile restores a file from its backup.
func (e *Engine) RevertFile(filePath string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return err
	}

	backup, ok := e.fileBackups[absPath]
	if !ok {
		return fmt.Errorf("tidak ada backup untuk file: %s", filePath)
	}

	// If backup path is empty, the file didn't exist originally — delete it
	if backup.BackupPath == "" {
		return os.Remove(absPath)
	}

	data, err := os.ReadFile(backup.BackupPath)
	if err != nil {
		return fmt.Errorf("gagal membaca backup: %w", err)
	}

	if err := os.WriteFile(absPath, data, 0o644); err != nil {
		return fmt.Errorf("gagal merestore file: %w", err)
	}

	return nil
}

// RevertAll reverts all backed-up files to their original state.
func (e *Engine) RevertAll() []error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var errs []error
	for path := range e.fileBackups {
		backup := e.fileBackups[path]
		if backup.BackupPath == "" {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				errs = append(errs, fmt.Errorf("gagal menghapus %s: %w", path, err))
			}
			continue
		}
		data, err := os.ReadFile(backup.BackupPath)
		if err != nil {
			errs = append(errs, fmt.Errorf("gagal membaca backup %s: %w", path, err))
			continue
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			errs = append(errs, fmt.Errorf("gagal restore %s: %w", path, err))
		}
	}
	return errs
}

// GetBackups returns all active backups.
func (e *Engine) GetBackups() []FileBackup {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]FileBackup, 0, len(e.fileBackups))
	for _, b := range e.fileBackups {
		result = append(result, *b)
	}
	return result
}

// CleanBackups removes all backup files.
func (e *Engine) CleanBackups() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, b := range e.fileBackups {
		if b.BackupPath != "" {
			os.Remove(b.BackupPath)
		}
	}
	e.fileBackups = make(map[string]*FileBackup)
}

func hashContent(content string) string {
	// Simple hash for tracking
	sum := 0
	for _, c := range content {
		sum += int(c)
	}
	return fmt.Sprintf("%x", sum)
}

// SetModeChangeCallback sets a callback for mode changes.
func (e *Engine) SetModeChangeCallback(fn func(ExecutionMode)) {
	e.onModeChange = fn
}

// IsReadOnlyTool checks if a tool only performs read operations.
func IsReadOnlyTool(toolName string) bool {
	action, ok := toolActions[toolName]
	return ok && action == ActionRead
}

// IsWriteTool checks if a tool performs write operations.
func IsWriteTool(toolName string) bool {
	action, ok := toolActions[toolName]
	return ok && (action == ActionWrite || action == ActionDelete)
}

// IsExecuteTool checks if a tool performs execution.
func IsExecuteTool(toolName string) bool {
	action, ok := toolActions[toolName]
	return ok && action == ActionExecute
}

// RegisterToolAction registers a custom tool action mapping.
func RegisterToolAction(toolName string, action ActionType) {
	toolActions[toolName] = action
}

// ListBlockedTools returns tools blocked in the current mode.
func (e *Engine) ListBlockedTools() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var blocked []string
	if e.mode == ModePlan {
		for tool, action := range toolActions {
			if action != ActionRead {
				blocked = append(blocked, tool)
			}
		}
	}
	return blocked
}
