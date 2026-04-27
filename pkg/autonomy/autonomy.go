// Package autonomy re-exports the autonomy engine for public use.
package autonomy

import (
	"github.com/gede-cahya/Smara-CLI/internal/autonomy"
)

// Re-export types
type LoopState = autonomy.LoopState
type Action = autonomy.Action
type Timeframe = autonomy.Timeframe

const (
	StateObserving = autonomy.StateObserving
	StateThinking  = autonomy.StateThinking
	StateActing    = autonomy.StateActing
	StateHolding   = autonomy.StateHolding
	StateIdle      = autonomy.StateIdle
)

const (
	ActionExecute = autonomy.ActionExecute
	ActionHold    = autonomy.ActionHold
	ActionAlert   = autonomy.ActionAlert
)

// Engine alias
type Engine = autonomy.Engine

// NewEngine creates a new autonomy engine.
var NewEngine = autonomy.NewEngine

// Default timeframes
var DefaultTimeframes = autonomy.DefaultTimeframes

// Function types
type ConditionChecker = autonomy.ConditionChecker
type ActionExecutor = autonomy.ActionExecutor
