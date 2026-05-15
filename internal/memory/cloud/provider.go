// Package cloud — Provider interface and registry.
//
// This file defines the cloud.Provider interface that abstracts every
// pluggable cloud-memory backend (Turso, Supabase, D1, Postgres, ...) and
// a thread-safe registry that maps a provider name to a factory function.
//
// Providers register themselves via init() in their own subpackage:
//
//	// internal/memory/cloud/turso/turso.go
//	func init() {
//	    cloud.Register("turso", func() cloud.Provider {
//	        return NewTursoProvider()
//	    })
//	}
//
// The CLI wires the active provider by blank-importing the subpackage and
// resolving cfg.CloudMemory.Provider through cloud.Get. An unknown name
// surfaces an error that lists every registered provider so the user can
// correct the configuration without recompiling.
package cloud

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// ----------------------------------------------------------------------------
// Provider interface
// ----------------------------------------------------------------------------

// Provider defines a pluggable cloud memory backend.
//
// Implementations are responsible for:
//
//   - Auth handshake and token validation (delegating persistent storage to
//     CredentialStore).
//   - Per-workspace remote database lifecycle (create, list, delete).
//   - Bootstrapping a libSQL embedded replica DSN that the memory store
//     opens via NewSQLiteStoreWithDSN.
//   - Explicit push/pull/status flows used by `smara memory cloud` commands;
//     ambient replication is handled by the libSQL driver.
//
// Method signatures intentionally mirror the design document
// (Section "Component 1") so that the SyncManager and CLI layers can be
// written against this interface alone.
type Provider interface {
	// Name returns the provider identifier (e.g. "turso", "supabase",
	// "d1"). The returned value MUST match the name used to register the
	// provider via Register.
	Name() string

	// Login performs the interactive (or headless) auth flow and returns
	// credentials suitable for persistence by CredentialStore. Implementations
	// MUST honor opts.Headless by skipping any browser interaction and reading
	// credentials from the environment instead.
	Login(ctx context.Context, opts LoginOptions) (*Credentials, error)

	// ValidateCredentials checks token validity without modifying any
	// remote state. It is called on cold start and before any operation
	// that would otherwise expose stale/expired credentials.
	ValidateCredentials(ctx context.Context, creds *Credentials) error

	// EnsureDatabase creates a remote database for the given workspace if
	// one does not exist, and returns connection metadata. Implementations
	// MUST be idempotent: a second call with the same workspaceName returns
	// the existing database's metadata without recreating it.
	EnsureDatabase(ctx context.Context, creds *Credentials, workspaceName string) (*DatabaseInfo, error)

	// OpenStore prepares a libSQL embedded replica at localPath that
	// mirrors the remote database described by info, and returns the DSN
	// suitable for passing to memory.NewSQLiteStoreWithDSN.
	OpenStore(ctx context.Context, info *DatabaseInfo, localPath string) (string, error)

	// Push triggers explicit replication of pending local writes to the
	// remote primary. Most replication is automatic; this exists for
	// `smara memory cloud push`.
	Push(ctx context.Context) (*SyncReport, error)

	// Pull triggers an explicit catch-up of remote frames into the local
	// replica.
	Pull(ctx context.Context) (*SyncReport, error)

	// Status returns the current sync state, lag, replica frame numbers,
	// pending counters, and quota usage.
	Status(ctx context.Context) (*SyncStatus, error)

	// ListWorkspaceDatabases returns every remote database belonging to
	// the authenticated user that matches the provider's workspace
	// naming convention.
	ListWorkspaceDatabases(ctx context.Context, creds *Credentials) ([]DatabaseInfo, error)

	// DeleteWorkspaceDatabase removes a remote database. Implementations
	// MUST be idempotent: deleting a non-existent database returns nil.
	DeleteWorkspaceDatabase(ctx context.Context, creds *Credentials, dbName string) error

	// Close releases provider-side resources (HTTP clients, replica
	// handles, ...). It MUST NOT delete data.
	Close() error
}

// ----------------------------------------------------------------------------
// Registry
// ----------------------------------------------------------------------------

// providerFactory constructs a fresh Provider instance. Factories are
// invoked lazily by Get so that providers do not perform network I/O or
// other expensive setup until they are actually requested.
type providerFactory func() Provider

// registry is the package-private store of registered provider factories.
// All access is guarded by registryMu so that Register, Get, and List are
// safe for concurrent use across goroutines.
var (
	registryMu sync.Mutex
	registry   = make(map[string]providerFactory)
)

// Register associates name with factory in the global provider registry.
//
// Register is intended to be called from a provider subpackage's init()
// function. Calling Register with an empty name or a nil factory panics
// because both indicate a programming error in the caller (registration
// happens at process start-up, well before any user input is involved).
//
// Re-registering a name overwrites the previous factory; this is useful
// for tests that swap in a fake provider via a t.Setenv-style helper.
func Register(name string, factory func() Provider) {
	if name == "" {
		panic("cloud: Register called with empty provider name")
	}
	if factory == nil {
		panic("cloud: Register called with nil factory for provider " + name)
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = providerFactory(factory)
}

// Get returns a fresh Provider instance for the given name. If no
// provider is registered under name, Get returns an error whose message
// lists every currently-registered provider name (sorted) so the caller
// can correct configuration without consulting source.
//
// Get invokes the factory while NOT holding the registry mutex so that a
// factory which itself touches the registry (e.g. for nested provider
// composition) cannot deadlock.
func Get(name string) (Provider, error) {
	registryMu.Lock()
	factory, ok := registry[name]
	if !ok {
		// Snapshot the registered names while still under the lock so
		// the error message reflects a consistent view.
		names := make([]string, 0, len(registry))
		for n := range registry {
			names = append(names, n)
		}
		registryMu.Unlock()
		sort.Strings(names)
		if len(names) == 0 {
			return nil, fmt.Errorf("cloud: provider %q is not registered (no providers are registered)", name)
		}
		return nil, fmt.Errorf("cloud: provider %q is not registered (registered providers: %v)", name, names)
	}
	registryMu.Unlock()
	return factory(), nil
}

// List returns the names of every registered provider in lexicographic
// order. The returned slice is a copy and is safe for the caller to
// mutate.
func List() []string {
	registryMu.Lock()
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	registryMu.Unlock()
	sort.Strings(names)
	return names
}
