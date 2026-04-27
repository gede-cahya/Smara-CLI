// Package safety re-exports the safety engine for public use.
package safety

import (
	"github.com/gede-cahya/Smara-CLI/internal/safety"
)

// Re-export types
type ExecutionMode = safety.ExecutionMode
type ActionType = safety.ActionType
type DraftAction = safety.DraftAction
type FileBackup = safety.FileBackup

const (
	ModePlan  = safety.ModePlan
	ModeBuild = safety.ModeBuild
)

const (
	ActionRead    = safety.ActionRead
	ActionWrite   = safety.ActionWrite
	ActionExecute = safety.ActionExecute
	ActionDelete  = safety.ActionDelete
)

// Engine alias
type Engine = safety.Engine

// NewEngine creates a new safety engine.
var NewEngine = safety.NewEngine

// Helper functions
var IsReadOnlyTool = safety.IsReadOnlyTool
var IsWriteTool = safety.IsWriteTool
var IsExecuteTool = safety.IsExecuteTool
var RegisterToolAction = safety.RegisterToolAction
