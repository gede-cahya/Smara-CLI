// Smara CLI — Autonomous Multi-Agent Terminal
// Entry point for the smara binary.
package main

// Side-effect imports register cloud-memory providers with the
// internal/memory/cloud registry at process start, so by the time
// rootCmd parses flags `cloud.Get("turso")` resolves successfully
// without the CLI layer having to know each provider's package path.
//
// Per task 8.6 of the cloud-memory spec, the Turso provider is the
// default backend; additional providers (Supabase, D1, ...) plug in
// here as their own blank imports without further wiring.
import (
	_ "github.com/gede-cahya/Smara-CLI/internal/memory/cloud/d1"
	_ "github.com/gede-cahya/Smara-CLI/internal/memory/cloud/supabase"
	_ "github.com/gede-cahya/Smara-CLI/internal/memory/cloud/turso"
)

func main() {
	Execute()
}

// air hot reload test marker - temporary changed
