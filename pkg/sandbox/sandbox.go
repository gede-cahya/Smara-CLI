// Package sandbox re-exports the sandbox for public use.
package sandbox

import (
	"github.com/gede-cahya/Smara-CLI/internal/sandbox"
)

// Re-export types
type Level = sandbox.Level
type Result = sandbox.Result

const (
	LevelStrict     = sandbox.LevelStrict
	LevelNormal     = sandbox.LevelNormal
	LevelPermissive = sandbox.LevelPermissive
)

// Sandbox alias
type Sandbox = sandbox.Sandbox

// New creates a new sandbox.
var New = sandbox.New

// Helper functions
var IsSafePath = sandbox.IsSafePath
var WrapCommand = sandbox.WrapCommand
