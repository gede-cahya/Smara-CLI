// Package integrations re-exports the integrations engine for public use.
package integrations

import (
	"github.com/gede-cahya/Smara-CLI/internal/integrations"
)

// Engine alias
type Engine = integrations.Engine

// NewEngine creates a new integrations engine.
var NewEngine = integrations.NewEngine
