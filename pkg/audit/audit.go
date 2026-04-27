// Package audit re-exports the audit logger for public use.
package audit

import (
	"github.com/gede-cahya/Smara-CLI/internal/audit"
)

// Re-export types
type EntryType = audit.EntryType
type Entry = audit.Entry

const (
	EntryPrompt      = audit.EntryPrompt
	EntryToolCall    = audit.EntryToolCall
	EntryToolResult  = audit.EntryToolResult
	EntryFileRead    = audit.EntryFileRead
	EntryFileWrite   = audit.EntryFileWrite
	EntryFileDelete  = audit.EntryFileDelete
	EntryModeChange  = audit.EntryModeChange
	EntrySession     = audit.EntrySession
	EntryMemory      = audit.EntryMemory
	EntryError       = audit.EntryError
	EntryDecision    = audit.EntryDecision
	EntrySafetyCheck = audit.EntrySafetyCheck
)

// Logger alias
type Logger = audit.Logger

// NewLogger creates a new audit logger.
var NewLogger = audit.NewLogger

// FilterEntries filters audit entries.
var FilterEntries = audit.FilterEntries
