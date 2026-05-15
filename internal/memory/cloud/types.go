// Package cloud defines provider-agnostic types, interfaces, and sentinel
// errors for the Smara cloud memory subsystem.
//
// This file contains data models and enums shared across providers
// (Turso, Supabase, D1, ...), the SyncManager, and the CredentialStore.
//
// IMPORTANT (security):
//   - Credentials.Token is sensitive material. Both String() and the default
//     MarshalJSON implementation redact the token to "[REDACTED]".
//   - The internal helper marshalForStorage emits the full token and is
//     intended for use only by CredentialStore implementations that persist
//     credentials to a trusted destination (OS keyring or 0600 file).
package cloud

import (
	"encoding/json"
	"errors"
	"time"
)

// ----------------------------------------------------------------------------
// State enum
// ----------------------------------------------------------------------------

// State represents the operational state of the cloud SyncManager.
type State string

const (
	// StateIdle indicates no sync activity is in progress and the manager
	// is healthy.
	StateIdle State = "idle"
	// StateSyncing indicates a Push, Pull, or SyncNow is currently running.
	StateSyncing State = "syncing"
	// StateOffline indicates the remote endpoint is unreachable; reads and
	// writes continue against the local replica.
	StateOffline State = "offline"
	// StateError indicates the last sync attempt failed with a non-network
	// error (auth, quota, schema, ...).
	StateError State = "error"
	// StateConflict indicates one or more rows in cloud_conflicts are
	// awaiting manual resolution.
	StateConflict State = "conflict"
	// StateDisabled indicates cloud sync is turned off (post-logout, or
	// cfg.CloudMemory.Enabled == false).
	StateDisabled State = "disabled"
)

// ----------------------------------------------------------------------------
// Credentials
// ----------------------------------------------------------------------------

// Credentials holds provider-agnostic auth material.
//
// Token is sensitive and must never be written to logs, stdout/stderr, or
// any non-redacted serialization. See String() and MarshalJSON below.
type Credentials struct {
	Provider     string    `json:"provider"`                // "turso", "supabase", ...
	Token        string    `json:"token"`                   // sensitive; redacted in String() / MarshalJSON
	RefreshToken string    `json:"refresh_token,omitempty"` // sensitive; redacted in String() / MarshalJSON
	Email        string    `json:"email,omitempty"`
	OrgID        string    `json:"org_id,omitempty"`
	Region       string    `json:"region,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
}

// redactedToken is the placeholder substituted for sensitive token fields
// in any non-storage serialization or string representation of Credentials.
const redactedToken = "[REDACTED]"

// String returns a human-readable representation with the Token and
// RefreshToken fields redacted. Safe to log or print.
func (c Credentials) String() string {
	token := ""
	if c.Token != "" {
		token = redactedToken
	}
	refresh := ""
	if c.RefreshToken != "" {
		refresh = redactedToken
	}
	return "Credentials{" +
		"Provider=" + c.Provider +
		" Token=" + token +
		" RefreshToken=" + refresh +
		" Email=" + c.Email +
		" OrgID=" + c.OrgID +
		" Region=" + c.Region +
		" ExpiresAt=" + c.ExpiresAt.Format(time.RFC3339) +
		"}"
}

// credentialsRedactedJSON mirrors Credentials but with Token / RefreshToken
// rendered as redaction placeholders. Used for the default MarshalJSON path
// so that *any* accidental JSON marshalling of Credentials (logging, audit,
// API response, config dump) never leaks the real token.
type credentialsRedactedJSON struct {
	Provider     string    `json:"provider"`
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Email        string    `json:"email,omitempty"`
	OrgID        string    `json:"org_id,omitempty"`
	Region       string    `json:"region,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
}

// MarshalJSON implements json.Marshaler with token redaction. This is the
// default serialization for Credentials and is intentionally lossy: the
// real token is replaced by "[REDACTED]" so structs that embed or include
// Credentials can never accidentally leak the token.
//
// To persist credentials with the full token (e.g. into the OS keyring or
// the 0600 cloud-creds.json fallback), use marshalForStorage.
func (c Credentials) MarshalJSON() ([]byte, error) {
	out := credentialsRedactedJSON{
		Provider:  c.Provider,
		Email:     c.Email,
		OrgID:     c.OrgID,
		Region:    c.Region,
		ExpiresAt: c.ExpiresAt,
	}
	if c.Token != "" {
		out.Token = redactedToken
	}
	if c.RefreshToken != "" {
		out.RefreshToken = redactedToken
	}
	return json.Marshal(out)
}

// credentialsStorageJSON is the on-disk / in-keyring representation. It
// preserves the full token so the credential can be re-loaded later. It is
// a separate type so that the field tags are explicit and so that
// MarshalJSON on Credentials cannot accidentally be invoked through this
// path.
type credentialsStorageJSON struct {
	Provider     string    `json:"provider"`
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Email        string    `json:"email,omitempty"`
	OrgID        string    `json:"org_id,omitempty"`
	Region       string    `json:"region,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
}

// marshalForStorage serializes Credentials with the full Token and
// RefreshToken intact. It is intended only for CredentialStore
// implementations that persist credentials to a trusted destination
// (OS keyring or a 0600 file). All other call sites should use the
// default MarshalJSON which redacts the token.
func marshalForStorage(c *Credentials) ([]byte, error) {
	if c == nil {
		return nil, errors.New("cloud: marshalForStorage: nil credentials")
	}
	return json.Marshal(credentialsStorageJSON{
		Provider:     c.Provider,
		Token:        c.Token,
		RefreshToken: c.RefreshToken,
		Email:        c.Email,
		OrgID:        c.OrgID,
		Region:       c.Region,
		ExpiresAt:    c.ExpiresAt,
	})
}

// unmarshalFromStorage is the inverse of marshalForStorage. It exists
// alongside marshalForStorage so CredentialStore implementations have a
// symmetric, package-private codec for persisted credentials.
func unmarshalFromStorage(data []byte) (*Credentials, error) {
	var raw credentialsStorageJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return &Credentials{
		Provider:     raw.Provider,
		Token:        raw.Token,
		RefreshToken: raw.RefreshToken,
		Email:        raw.Email,
		OrgID:        raw.OrgID,
		Region:       raw.Region,
		ExpiresAt:    raw.ExpiresAt,
	}, nil
}

// ----------------------------------------------------------------------------
// DatabaseInfo
// ----------------------------------------------------------------------------

// DatabaseInfo describes a remote database after EnsureDatabase.
//
// AuthToken is intentionally tagged json:"-" so that DatabaseInfo can be
// safely logged or persisted (e.g. into cloud_databases) without leaking
// per-database tokens. Tokens flow through Credentials and the
// libSQL DSN at runtime instead.
type DatabaseInfo struct {
	Provider    string    `json:"provider"`
	Name        string    `json:"name"` // e.g. "smara-default"
	URL         string    `json:"url"`  // libsql://smara-default-xxx.turso.io
	AuthToken   string    `json:"-"`    // never serialized (sensitive)
	Region      string    `json:"region"`
	SizeBytes   int64     `json:"size_bytes"`
	RowsRead    int64     `json:"rows_read"`    // current month, free-tier accounting
	RowsWritten int64     `json:"rows_written"` // current month
	CreatedAt   time.Time `json:"created_at"`
	WorkspaceID int64     `json:"workspace_id"` // FK to local workspaces.id
}

// ----------------------------------------------------------------------------
// LoginOptions
// ----------------------------------------------------------------------------

// LoginOptions controls the Provider.Login flow.
//
// Headless skips browser interaction and expects credentials to be supplied
// out-of-band (e.g. SMARA_CLOUD_TOKEN / SMARA_CLOUD_ORG / SMARA_CLOUD_REGION
// environment variables). CallbackPort is the local TCP port to bind for the
// PKCE redirect; 0 means "let the OS pick".
type LoginOptions struct {
	Provider     string `json:"provider"`
	Region       string `json:"region,omitempty"`
	Headless     bool   `json:"headless"`
	CallbackPort int    `json:"callback_port,omitempty"`
}

// ----------------------------------------------------------------------------
// SyncStatus / SyncReport / ReconcileReport / QuotaInfo
// ----------------------------------------------------------------------------

// QuotaInfo summarises remote-side resource usage so the SyncManager can
// throttle writes ahead of provider-imposed hard limits.
type QuotaInfo struct {
	StorageBytes      int64   `json:"storage_bytes"`
	StorageLimitBytes int64   `json:"storage_limit_bytes"` // 9 GB for Turso free
	RowsReadMonth     int64   `json:"rows_read_month"`
	RowsReadLimit     int64   `json:"rows_read_limit"` // 1B for Turso free
	PercentUsed       float64 `json:"percent_used"`
}

// SyncStatus is the snapshot returned by Provider.Status / SyncManager.Status.
type SyncStatus struct {
	State               State     `json:"state"`
	LastSyncAt          time.Time `json:"last_sync_at"`
	LagSeconds          int       `json:"lag_seconds"`    // est. lag vs primary
	LocalFrameNo        int64     `json:"local_frame_no"` // libSQL replica frame
	RemoteFrameNo       int64     `json:"remote_frame_no"`
	PendingPush         int       `json:"pending_push"` // local rows not yet replicated
	PendingPull         int       `json:"pending_pull"` // remote frames not yet applied
	UnresolvedConflicts int       `json:"unresolved_conflicts"`
	LastError           string    `json:"last_error,omitempty"`
	Quota               QuotaInfo `json:"quota"`
}

// SyncReport summarises the result of a single Push, Pull, or SyncNow op.
type SyncReport struct {
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	PushedRows   int       `json:"pushed_rows"`
	PulledFrames int       `json:"pulled_frames"`
	Conflicts    int       `json:"conflicts"`
	Errors       []string  `json:"errors,omitempty"`
}

// ReconcileReport extends SyncReport with conflict-resolution counters.
//
// Invariant: ResolvedAuto + DeferredManual + ArchivedLosers == count of
// processed conflicts.
type ReconcileReport struct {
	SyncReport
	ResolvedAuto   int `json:"resolved_auto"`
	DeferredManual int `json:"deferred_manual"`
	ArchivedLosers int `json:"archived_losers"`
}

// ----------------------------------------------------------------------------
// Sentinel errors
// ----------------------------------------------------------------------------

// Sentinel errors returned by cloud package and provider implementations.
// Use errors.Is to compare against these values.
var (
	// ErrNoCredentials is returned by CredentialStore.Load when no
	// credentials are available from any source (env, keyring, file).
	ErrNoCredentials = errors.New("cloud: no credentials available")

	// ErrManualConflict is returned by ConflictResolver implementations
	// configured with the "manual" policy. The caller is expected to
	// insert the divergent row pair into cloud_conflicts and surface it
	// via `smara memory cloud conflicts`.
	ErrManualConflict = errors.New("cloud: manual conflict resolution required")

	// ErrBudgetExceeded is returned by SyncManager.checkBudgetBeforeWrite
	// when an attempted write would push the workspace past the
	// configured per-hour or storage budget.
	ErrBudgetExceeded = errors.New("cloud: write budget exceeded")

	// ErrLoginCancelled is returned by Provider.Login when the user
	// cancels the OAuth/PKCE flow or the local callback server times out
	// waiting for the redirect.
	ErrLoginCancelled = errors.New("cloud: login cancelled")

	// ErrQuotaExceeded is returned by Provider.EnsureDatabase when the
	// remote provider rejects creation due to organisation-level quota
	// limits (HTTP 402 / equivalent).
	ErrQuotaExceeded = errors.New("cloud: provider quota exceeded")

	// ErrUnreachable is returned by provider operations when the remote
	// endpoint cannot be reached. Read/write paths must remain operable
	// against the local replica when this error surfaces.
	ErrUnreachable = errors.New("cloud: remote endpoint unreachable")
)
