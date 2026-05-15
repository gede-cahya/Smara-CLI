// Package cloud — ConflictResolver interface, factory, and per-policy stubs.
//
// Conflict resolution is the contract between SyncManager.Reconcile and the
// configured cfg.CloudMemory.ConflictPolicy: given two divergent versions of
// the same logical memory (same cloud_id, different content_hash and
// device_id), produce a single winning row plus an optional archived loser
// to preserve the discarded edit. The four policies (lww, manual,
// archive-loser, merge-content) are documented on the constants in
// config.go and in design.md Section "Component 4".
//
// This file establishes the surface area only. The per-policy logic is
// implemented in tasks 5.2-5.5; each Resolve here returns a "not yet
// implemented" error so unwired callers fail loudly rather than silently
// dropping conflicts.
//
// # Local row types: rationale
//
// internal/memory.Memory and internal/memory.MemoryVersion already model
// these rows, but importing internal/memory from internal/memory/cloud
// would close an import cycle: store_cloud.go (in internal/memory) wires
// SyncManager (in internal/memory/cloud) which would in turn import
// internal/memory for the row types. We therefore declare local
// MemoryRow and MemoryVersionRow structs that hold only the fields the
// resolver actually needs (id/cloud_id/content/device_id/version/
// updated_at for the live row; id/memory_id/content/version/changed_by/
// reason/created_at for the archived version). Translating to/from
// internal/memory.Memory happens at the SyncManager boundary
// (internal/memory/cloud/sync_manager.go in task 9), where a one-way
// dependency on internal/memory is already fine.
package cloud

import (
	"fmt"
	"time"
)

// ----------------------------------------------------------------------------
// Local row types (cycle-break)
// ----------------------------------------------------------------------------

// MemoryRow is the subset of internal/memory.Memory that ConflictResolver
// implementations need to pick a winner and produce an archived loser.
//
// Fields:
//   - ID:        local primary key (memories.id) — used by callers to
//                target the live row when persisting the resolution.
//   - CloudID:   memories.cloud_id (UUID v7). Must be identical for local
//                and remote when Resolve is invoked; the resolver uses
//                this to assert the rows really are two versions of the
//                same logical memory.
//   - Content:   memories.content snapshot. Merge policies concatenate
//                this; LWW / archive-loser policies pass it through to
//                the loser version.
//   - DeviceID:  memories.device_id of the writer. Used as the LWW
//                tiebreaker (lexicographic ascending) and embedded in
//                the merge-content separator.
//   - Version:   memories.version. Merge bumps to max+1; LWW preserves
//                the winner's version on the live row and uses the
//                loser's version when archiving.
//   - UpdatedAt: memories.updated_at. Primary LWW selection key.
type MemoryRow struct {
	ID        int64
	CloudID   string
	Content   string
	DeviceID  string
	Version   int
	UpdatedAt time.Time
}

// MemoryVersionRow is the subset of internal/memory.MemoryVersion that
// ConflictResolver returns when archiving the losing side of a conflict.
//
// Resolve constructs this in-memory; persistence happens in
// SyncManager.Reconcile (task 9.2) inside the same transaction that
// updates the live memories row, so ID and CreatedAt are typically zero
// at construction and set by the database on INSERT.
//
// Fields:
//   - ID:        memory_versions.id. Zero when returned by Resolve;
//                populated by the storage layer on INSERT.
//   - MemoryID:  memory_versions.memory_id — the live memories.id whose
//                history this row joins.
//   - Content:   the loser's content snapshot.
//   - Version:   the loser's memories.version at the time of conflict.
//   - ChangedBy: device_id of the loser's writer (for "who made the
//                edit we discarded?").
//   - Reason:    free-form annotation; resolvers use the literal
//                "conflict-loser" so SyncManager queries can filter
//                conflict-archived versions.
//   - CreatedAt: memory_versions.created_at. Zero when returned by
//                Resolve; populated by the storage layer on INSERT.
type MemoryVersionRow struct {
	ID        int64
	MemoryID  int64
	Content   string
	Version   int
	ChangedBy string
	Reason    string
	CreatedAt time.Time
}

// ----------------------------------------------------------------------------
// ConflictResolver interface
// ----------------------------------------------------------------------------

// ConflictResolver decides how to merge two divergent versions of a
// memory surfaced by SyncManager.Reconcile.
//
// Resolve takes the local and remote snapshots of a single logical row
// (matched on cloud_id by the caller) and returns:
//
//   - winner: the row that should overwrite the live memories entry.
//     Implementations MUST set winner.CloudID equal to local.CloudID
//     (which equals remote.CloudID by precondition).
//   - loser:  optional archived version that captures the discarded
//             edit. nil when no archival is needed (e.g. the manual
//             policy defers archival to the user). When non-nil, the
//             caller will INSERT it into memory_versions.
//   - err:    nil on a successful in-band resolution. Implementations
//             return ErrManualConflict to signal "do not modify the
//             live row; surface this to the user via cloud_conflicts".
//
// Resolve MUST be deterministic for the lww, archive-loser, and
// merge-content policies: identical (local, remote) inputs MUST produce
// identical (winner, loser, err) outputs across calls. This is what
// makes the eventual-consistency property (Property 3) testable via
// rapid in task 5.6.
type ConflictResolver interface {
	Resolve(local, remote MemoryRow) (winner MemoryRow, loser *MemoryVersionRow, err error)
}

// ----------------------------------------------------------------------------
// Factory
// ----------------------------------------------------------------------------

// NewResolver returns the ConflictResolver implementation for the given
// policy. An unknown policy yields an error whose message lists every
// accepted value (in canonical order) so misconfiguration surfaces a
// fixable diagnostic rather than a panic.
//
// The error path here intentionally duplicates Config.Validate's check
// on ConflictPolicy: NewResolver is also called from code paths that
// receive a policy directly (e.g. tests, future programmatic wiring)
// without going through Config, so the safety net belongs on both
// sides.
func NewResolver(policy ConflictPolicy) (ConflictResolver, error) {
	switch policy {
	case PolicyLastWriteWins:
		return lwwResolver{}, nil
	case PolicyManual:
		return manualResolver{}, nil
	case PolicyArchiveLoser:
		return archiveLoserResolver{}, nil
	case PolicyMergeContent:
		return mergeContentResolver{}, nil
	default:
		return nil, fmt.Errorf("cloud: unknown ConflictPolicy %q: must be one of %s", string(policy), validConflictPolicyNames())
	}
}

// ----------------------------------------------------------------------------
// Resolver implementations
// ----------------------------------------------------------------------------

// pickLWWWinner implements the deterministic last-write-wins selection
// rule shared by lwwResolver and archiveLoserResolver:
//
//  1. Whichever side has the more recent UpdatedAt is the winner.
//  2. When UpdatedAt is exactly equal (clock-synced writes, or a deliberate
//     test of the tiebreaker), DeviceID is compared lexicographically and
//     the smaller string wins.
//
// Determinism: the function depends only on (local, remote) field values
// — no clock reads, no map iteration, no randomness — so identical inputs
// always produce identical (winner, loser) pairs across calls. This is
// what task 5.6's Property 4 (eventual consistency) relies on.
//
// The two return values are distinct *copies*: callers may mutate
// `winner` without aliasing the input rows. Both are returned by value
// so downstream resolvers can build the archived MemoryVersionRow off of
// `loser` without an extra allocation.
func pickLWWWinner(local, remote MemoryRow) (winner, loser MemoryRow) {
	switch {
	case remote.UpdatedAt.After(local.UpdatedAt):
		return remote, local
	case local.UpdatedAt.After(remote.UpdatedAt):
		return local, remote
	// UpdatedAt is equal; tiebreak by DeviceID lexicographic ascending
	// (the smaller DeviceID wins).
	case local.DeviceID <= remote.DeviceID:
		return local, remote
	default:
		return remote, local
	}
}

// archiveLoserVersion builds the MemoryVersionRow that captures a
// discarded edit in memory_versions. The pairing with the live
// memories.id is preserved via local.ID — every conflict is between two
// rows that share a cloud_id and therefore share the same local
// memories.id.
//
// Reason is hard-coded to "conflict-loser" so SyncManager queries can
// filter conflict-archived versions from regular edit history. ID and
// CreatedAt are deliberately left zero; the storage layer fills them in
// at INSERT time (see task 9.2).
func archiveLoserVersion(localID int64, loser MemoryRow) *MemoryVersionRow {
	return &MemoryVersionRow{
		MemoryID:  localID,
		Content:   loser.Content,
		Version:   loser.Version,
		ChangedBy: loser.DeviceID,
		Reason:    "conflict-loser",
	}
}

// lwwResolver implements PolicyLastWriteWins: later UpdatedAt wins,
// DeviceID ascending tiebreaks, the loser is archived as a
// memory_versions row tagged "conflict-loser".
type lwwResolver struct{}

// Resolve picks the winner using pickLWWWinner and emits the loser as
// an archived version. The returned winner is always pinned to
// local.ID and local.CloudID: SyncManager.Reconcile uses winner.ID to
// target the live memories row, and CloudID equality is the precondition
// the caller already checked, so we re-assert it here for safety.
func (lwwResolver) Resolve(local, remote MemoryRow) (MemoryRow, *MemoryVersionRow, error) {
	winnerSrc, loserSrc := pickLWWWinner(local, remote)

	// Pin identity to the local row: the caller owns the memories.id
	// and cloud_id; the winner's content/version/timestamp/device come
	// from whichever side actually won.
	winner := MemoryRow{
		ID:        local.ID,
		CloudID:   local.CloudID,
		Content:   winnerSrc.Content,
		DeviceID:  winnerSrc.DeviceID,
		Version:   winnerSrc.Version,
		UpdatedAt: winnerSrc.UpdatedAt,
	}
	return winner, archiveLoserVersion(local.ID, loserSrc), nil
}

// manualResolver implements PolicyManual: do not modify the live row,
// return ErrManualConflict so SyncManager.Reconcile inserts the divergent
// pair into cloud_conflicts and surfaces it via
// `smara memory cloud conflicts`.
type manualResolver struct{}

// Resolve returns (local, nil, ErrManualConflict). local is returned
// verbatim so callers that ignore err and use winner anyway leave the
// memories row untouched — defence in depth against a misuse of the
// interface.
func (manualResolver) Resolve(local, remote MemoryRow) (MemoryRow, *MemoryVersionRow, error) {
	return local, nil, ErrManualConflict
}

// archiveLoserResolver implements PolicyArchiveLoser: same winner
// selection as LWW.
//
// Note on semantics: lwwResolver already archives the loser
// unconditionally, so PolicyArchiveLoser produces the same output as
// PolicyLastWriteWins for every input. The two policies remain distinct
// at the configuration layer because they document different *intents*:
// PolicyLastWriteWins promises "loser is preserved as a side-effect",
// PolicyArchiveLoser promises "loser is preserved by contract". Keeping
// them as separate resolver types makes that contract explicit at the
// call site (NewResolver -> archiveLoserResolver) and lets us evolve
// either one independently if the semantics ever diverge.
type archiveLoserResolver struct{}

// Resolve picks the winner using pickLWWWinner and ALWAYS emits the
// archived loser, regardless of whether UpdatedAt differs. Equivalent
// in practice to lwwResolver.Resolve today; the explicit guarantee is
// the contract this policy advertises (Requirement 15.3).
func (archiveLoserResolver) Resolve(local, remote MemoryRow) (MemoryRow, *MemoryVersionRow, error) {
	winnerSrc, loserSrc := pickLWWWinner(local, remote)

	winner := MemoryRow{
		ID:        local.ID,
		CloudID:   local.CloudID,
		Content:   winnerSrc.Content,
		DeviceID:  winnerSrc.DeviceID,
		Version:   winnerSrc.Version,
		UpdatedAt: winnerSrc.UpdatedAt,
	}
	// Archival is mandatory under this policy — we do not condition on
	// timestamp ordering, content equality, or any other heuristic.
	return winner, archiveLoserVersion(local.ID, loserSrc), nil
}

// mergeContentResolver implements PolicyMergeContent: concatenate both
// sides into a single winner row with a separator that records which
// remote device contributed the merged-in content. No loser archive is
// emitted because both contents are preserved in the winner.
type mergeContentResolver struct{}

// mergeSeparator is the literal injected between local and remote
// content when policy=merge-content. The "%s" is filled with
// remote.DeviceID. Defined as a package-level constant so tests in task
// 5.6 can assert the exact substring without duplicating the format
// string.
const mergeSeparator = "\n---merged from device %s---\n"

// Resolve constructs the merged winner row. Identity (ID, CloudID,
// DeviceID) is pinned to local: the merge happens *on* the local device,
// so the resulting row is owned by the local device for subsequent
// sync. Version bumps to max(local, remote) + 1 so any concurrent
// observer can tell the merge apart from either input. UpdatedAt takes
// the later of the two inputs so causal ordering is preserved relative
// to the inputs.
//
// Returns nil for the loser — content from both sides is preserved
// inline in winner.Content, so there is nothing to archive.
func (mergeContentResolver) Resolve(local, remote MemoryRow) (MemoryRow, *MemoryVersionRow, error) {
	mergedContent := local.Content + fmt.Sprintf(mergeSeparator, remote.DeviceID) + remote.Content

	// max(local.Version, remote.Version) + 1
	nextVersion := local.Version
	if remote.Version > nextVersion {
		nextVersion = remote.Version
	}
	nextVersion++

	// max(local.UpdatedAt, remote.UpdatedAt) using After/Before so we
	// stay deterministic when the two are exactly equal (fall through
	// to local).
	mergedUpdatedAt := local.UpdatedAt
	if remote.UpdatedAt.After(mergedUpdatedAt) {
		mergedUpdatedAt = remote.UpdatedAt
	}

	winner := MemoryRow{
		ID:        local.ID,      // preserve PK
		CloudID:   local.CloudID, // preserve cloud identity (== remote.CloudID by precondition)
		Content:   mergedContent,
		DeviceID:  local.DeviceID, // the device that ran the merge
		Version:   nextVersion,
		UpdatedAt: mergedUpdatedAt,
	}
	return winner, nil, nil
}

// ----------------------------------------------------------------------------
// Compile-time interface assertions
// ----------------------------------------------------------------------------

// Compile-time assertions that every stub still satisfies ConflictResolver
// after task 5.2-5.5 swap in real bodies. Without these, switching a stub
// to a pointer receiver (or renaming Resolve) would silently break
// NewResolver's type-switch return values.
var (
	_ ConflictResolver = lwwResolver{}
	_ ConflictResolver = manualResolver{}
	_ ConflictResolver = archiveLoserResolver{}
	_ ConflictResolver = mergeContentResolver{}
)
