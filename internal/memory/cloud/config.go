// Package cloud — Config struct and validation rules.
//
// This file defines cloud.Config, the in-memory representation of the
// `cloud_memory` section of internal/config.SmaraConfig. It is kept
// separate from internal/config.CloudMemoryConfig so the cloud package
// can:
//
//   - Use a richer ConflictPolicy enum type (instead of plain string)
//     while still round-tripping through the YAML config.
//   - Validate values with provider-aware rules (e.g. "is the configured
//     provider actually registered?") without dragging the global config
//     loader into every cloud-side test.
//
// FromConfig converts an internal/config.CloudMemoryConfig into this
// richer form; Validate enforces the rules listed in design.md
// Section "Model 1 / Validation Rules" and Requirements 11.4 / 15.1 / 15.6.
package cloud

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gede-cahya/Smara-CLI/internal/config"
)

// ----------------------------------------------------------------------------
// ConflictPolicy enum
// ----------------------------------------------------------------------------

// ConflictPolicy enumerates the strategies SyncManager.Reconcile uses to
// resolve divergent rows surfaced during Pull. The string values are the
// same identifiers persisted in YAML (`conflict_policy: lww`) so the
// type doubles as a self-describing config value.
type ConflictPolicy string

const (
	// PolicyLastWriteWins picks the row with the most recent updated_at,
	// using device_id lexicographic order as a stable tiebreaker. The
	// loser is preserved as a memory_versions entry. This is the default
	// because it matches user intuition for most workflows.
	PolicyLastWriteWins ConflictPolicy = "lww"

	// PolicyManual leaves the divergent row pair in cloud_conflicts and
	// surfaces ErrManualConflict to the SyncManager. The user must
	// resolve via `smara memory cloud conflicts resolve`.
	PolicyManual ConflictPolicy = "manual"

	// PolicyArchiveLoser behaves like PolicyLastWriteWins for selecting
	// the winner but additionally archives the loser's full content into
	// memory_versions regardless of timestamps.
	PolicyArchiveLoser ConflictPolicy = "archive-loser"

	// PolicyMergeContent concatenates the local and remote content with
	// a separator that records the remote device_id, bumps version, and
	// uses the more recent updated_at.
	PolicyMergeContent ConflictPolicy = "merge-content"
)

// validConflictPolicies is the canonical, ordered list of accepted
// policy values. It is the single source of truth that both Validate
// and ConflictPolicy.IsValid consult, so adding a new policy only
// requires extending this slice (and the constants above).
var validConflictPolicies = []ConflictPolicy{
	PolicyLastWriteWins,
	PolicyManual,
	PolicyArchiveLoser,
	PolicyMergeContent,
}

// IsValid reports whether p is one of the four accepted policies.
func (p ConflictPolicy) IsValid() bool {
	for _, v := range validConflictPolicies {
		if p == v {
			return true
		}
	}
	return false
}

// validConflictPolicyNames returns the accepted policy values formatted
// for inclusion in user-facing error messages: a quoted, comma-separated
// list in canonical order.
func validConflictPolicyNames() string {
	parts := make([]string, len(validConflictPolicies))
	for i, p := range validConflictPolicies {
		parts[i] = fmt.Sprintf("%q", string(p))
	}
	return strings.Join(parts, ", ")
}

// ----------------------------------------------------------------------------
// Config
// ----------------------------------------------------------------------------

// Config holds all cloud-memory settings, mirroring config.CloudMemoryConfig
// with a typed ConflictPolicy enum. Use FromConfig to convert from the
// untyped YAML representation.
//
// EncryptionKey is intentionally NOT serialized to YAML by the loader
// (see internal/config.CloudMemoryConfig); it is resolved at runtime from
// the OS keyring or the SMARA_CLOUD_ENCRYPTION_KEY env var. It lives on
// this struct so Validate can enforce the EncryptAtRest pre-condition
// without reaching into other subsystems.
type Config struct {
	Enabled         bool
	Provider        string
	DBNamePattern   string
	SyncIntervalSec int
	ConflictPolicy  ConflictPolicy
	OfflineMode     string
	EncryptAtRest   bool
	EncryptionKey   string
	MaxRowsPerHour  int
	MaxStorageMB    int
	EmbeddingsCloud bool
	SyncTables      []string
}

// FromConfig converts an internal/config.CloudMemoryConfig into a
// cloud.Config. The conversion is field-for-field except for
// ConflictPolicy, which is wrapped in the typed enum. EncryptionKey is
// left empty because it is not part of the YAML config (see the comment
// on Config.EncryptionKey); callers that enable EncryptAtRest are
// responsible for populating EncryptionKey from the keyring before
// calling Validate.
func FromConfig(c config.CloudMemoryConfig) Config {
	tables := append([]string(nil), c.SyncTables...)
	return Config{
		Enabled:         c.Enabled,
		Provider:        c.Provider,
		DBNamePattern:   c.DBNamePattern,
		SyncIntervalSec: c.SyncIntervalSec,
		ConflictPolicy:  ConflictPolicy(c.ConflictPolicy),
		OfflineMode:     c.OfflineMode,
		EncryptAtRest:   c.EncryptAtRest,
		// EncryptionKey is resolved from the keyring at runtime; it is
		// not part of the YAML config and is therefore left zero here.
		EncryptionKey:   "",
		MaxRowsPerHour:  c.MaxRowsPerHour,
		MaxStorageMB:    c.MaxStorageMB,
		EmbeddingsCloud: c.EmbeddingsCloud,
		SyncTables:      tables,
	}
}

// ----------------------------------------------------------------------------
// Validation
// ----------------------------------------------------------------------------

// workspacePlaceholder is the substring that DBNamePattern must contain
// so the per-workspace database name can be derived by simple string
// substitution. Defined as a constant so the error message and the
// check itself stay in sync.
const workspacePlaceholder = "{workspace}"

// Validate enforces the rules listed in design.md Section "Model 1":
//
//   - Provider must be a name registered in the provider registry. The
//     error lists every currently-registered provider so the user can
//     fix their configuration without consulting source.
//   - SyncIntervalSec must be non-negative. Zero is allowed and disables
//     the periodic ticker; negative values are nonsensical.
//   - DBNamePattern must contain "{workspace}" so a per-workspace name
//     can be derived deterministically.
//   - ConflictPolicy must equal one of the four exported PolicyXxx
//     constants. The error explicitly enumerates the accepted values.
//   - MaxRowsPerHour must be at least 100; lower than that defeats the
//     point of the cloud feature (Requirement 9.5).
//   - When EncryptAtRest is true, EncryptionKey must be non-empty so the
//     libSQL replica can actually encrypt the on-disk WAL.
//
// Validate returns nil when every rule passes. The first failing rule
// produces the returned error.
func (c Config) Validate() error {
	// Provider registration check. We deliberately do NOT instantiate
	// the provider here (Get's factory may perform expensive setup) —
	// we only need to confirm the name resolves to *some* factory.
	if _, err := Get(c.Provider); err != nil {
		// Get already produces a message that lists registered
		// providers via List(); prefix it so the user sees the field
		// name in the surfaced error.
		return fmt.Errorf("invalid Provider: %w", err)
	}

	if c.SyncIntervalSec < 0 {
		return fmt.Errorf("invalid SyncIntervalSec: %d (must be >= 0; 0 disables the periodic ticker)", c.SyncIntervalSec)
	}

	if !strings.Contains(c.DBNamePattern, workspacePlaceholder) {
		return fmt.Errorf("invalid DBNamePattern %q: must contain %q substring so per-workspace database names can be derived", c.DBNamePattern, workspacePlaceholder)
	}

	if !c.ConflictPolicy.IsValid() {
		return fmt.Errorf("invalid ConflictPolicy %q: must be one of %s", string(c.ConflictPolicy), validConflictPolicyNames())
	}

	if c.MaxRowsPerHour < 100 {
		return fmt.Errorf("invalid MaxRowsPerHour: %d (must be >= 100; lower throttle defeats cloud sync)", c.MaxRowsPerHour)
	}

	if c.EncryptAtRest && c.EncryptionKey == "" {
		return errors.New("invalid configuration: EncryptAtRest=true requires a non-empty EncryptionKey (resolve from keyring before Validate)")
	}

	return nil
}
