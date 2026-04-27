// Package lsp re-exports the LSP client for public use.
package lsp

import (
	"github.com/gede-cahya/Smara-CLI/internal/lsp"
)

// Re-export types
type Client = lsp.Client
type ServerConfig = lsp.ServerConfig
type Location = lsp.Location
type Range = lsp.Range
type Position = lsp.Position
type HoverInfo = lsp.HoverInfo
type DocumentSymbol = lsp.DocumentSymbol

// Default servers
var DefaultServers = lsp.DefaultServers

// NewClient creates a new LSP client.
var NewClient = lsp.NewClient

// Helper functions
var DetectLanguage = lsp.DetectLanguage
var FileToURI = lsp.FileToURI

// Manager alias
type Manager = lsp.Manager

// NewManager creates a new LSP manager.
var NewManager = lsp.NewManager
