// Package cloud — credential storage.
//
// This file implements the CredentialStore interface and its OS-keyring
// backend. Subsequent tasks append the file-fallback store, the env-mode
// store, and the composite NewCredentialStore() chain to this file.
//
// Security notes:
//   - Tokens are persisted only via marshalForStorage (defined in types.go).
//     The default Credentials.MarshalJSON / Credentials.String redact the
//     token, so accidental logging of a *Credentials value will never leak
//     the secret.
//   - The keyring backend uses service name "smara-cloud" and account
//     "default" so that Delete() / Load() are deterministic across runs.
package cloud

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
)

// ----------------------------------------------------------------------------
// CredentialStore interface
// ----------------------------------------------------------------------------

// CredentialStore abstracts the persistence layer for cloud Credentials.
//
// Implementations must:
//   - Persist the full token via marshalForStorage (never via the redacting
//     default JSON marshaller on Credentials).
//   - Return ErrNoCredentials from Load when no credentials are available
//     in the underlying store.
//   - Be tolerant of "not found" on Delete (deleting a non-existent entry
//     must not error).
//   - Report a stable, non-empty Source() identifier so the CLI / audit
//     logger can disclose where credentials live.
//
// Concrete implementations in this package: keyringStore, fileStore,
// envStore (added by tasks 4.2 and 4.3) plus a composite store returned
// by NewCredentialStore (task 4.4).
type CredentialStore interface {
	// Save persists the supplied credentials. Implementations must use
	// marshalForStorage to serialize so the full token is preserved.
	Save(creds *Credentials) error

	// Load returns the persisted credentials. If no credentials exist in
	// the underlying store, the implementation must return ErrNoCredentials.
	Load() (*Credentials, error)

	// Delete removes the persisted credentials. Implementations must be
	// tolerant of "not found": deleting a non-existent entry returns nil.
	Delete() error

	// Source returns a short identifier for the backing store. Per
	// requirement 13.5 the value is one of "keyring" | "file" (with
	// "env" added by the headless-mode store in task 4.3).
	Source() string
}

// ----------------------------------------------------------------------------
// keyringStore — OS keyring backend
// ----------------------------------------------------------------------------

// Service / account constants for keyring entries. They are fixed so that
// Save / Load / Delete operate on the same record across CLI invocations.
const (
	keyringService = "smara-cloud"
	keyringAccount = "default"
)

// keyringStore persists credentials in the OS keyring (libsecret on Linux,
// Keychain on macOS, Credential Manager on Windows) via zalando/go-keyring.
//
// Per requirement 13.1, when the OS keyring is available the token is
// persisted under service name "smara-cloud". The store is intentionally
// stateless — the underlying keyring is the single source of truth.
type keyringStore struct{}

// newKeyringStore returns a CredentialStore backed by the OS keyring.
//
// It does not probe the keyring for availability; callers that need that
// signal (e.g. NewCredentialStore in task 4.4) should attempt a Load /
// Save and fall back on keyring.ErrUnsupportedPlatform or other errors.
func newKeyringStore() CredentialStore {
	return &keyringStore{}
}

// Save serializes the credentials with marshalForStorage (full token) and
// writes them to the OS keyring under (smara-cloud, default).
//
// A nil *Credentials is rejected so callers cannot accidentally clear the
// keyring entry by passing nil — Delete is the explicit way to remove.
func (s *keyringStore) Save(creds *Credentials) error {
	if creds == nil {
		return errors.New("cloud: keyringStore.Save: nil credentials")
	}
	body, err := marshalForStorage(creds)
	if err != nil {
		return fmt.Errorf("cloud: keyringStore.Save: marshal: %w", err)
	}
	if err := keyring.Set(keyringService, keyringAccount, string(body)); err != nil {
		return fmt.Errorf("cloud: keyringStore.Save: keyring set: %w", err)
	}
	return nil
}

// Load reads the keyring entry and decodes it via unmarshalFromStorage.
//
// A missing entry maps to ErrNoCredentials so callers can use errors.Is
// to distinguish "not logged in" from real I/O failures.
func (s *keyringStore) Load() (*Credentials, error) {
	raw, err := keyring.Get(keyringService, keyringAccount)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, ErrNoCredentials
		}
		return nil, fmt.Errorf("cloud: keyringStore.Load: keyring get: %w", err)
	}
	creds, err := unmarshalFromStorage([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("cloud: keyringStore.Load: unmarshal: %w", err)
	}
	return creds, nil
}

// Delete removes the keyring entry. Per requirement, deleting a
// non-existent entry succeeds (idempotent logout).
func (s *keyringStore) Delete() error {
	err := keyring.Delete(keyringService, keyringAccount)
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return fmt.Errorf("cloud: keyringStore.Delete: keyring delete: %w", err)
}

// Source returns the literal "keyring" per requirement 13.5.
func (s *keyringStore) Source() string {
	return "keyring"
}

// ----------------------------------------------------------------------------
// fileStore — 0600 file fallback at ~/.smara/cloud-creds.json
// ----------------------------------------------------------------------------

// fileFallbackPath is the on-disk location for the file fallback store
// (resolved against os.UserHomeDir at call time so tests that override
// $HOME via t.Setenv pick up the new location). It is exposed as a
// package-private helper so the composite store and tests can share the
// same logic without duplicating the path joining.
const (
	fileFallbackDir      = ".smara"
	fileFallbackName     = "cloud-creds.json"
	fileFallbackMode     = 0o600
	fileFallbackDirMode  = 0o700
	fileFallbackModeMask = 0o777
)

// fileFallbackWarnOnce gates the stderr warning so we only emit it once
// per process, even if the user runs many cloud subcommands inside a
// single session (the warning is informational, not actionable per call).
var fileFallbackWarnOnce sync.Once

// fileStore persists credentials in ~/.smara/cloud-creds.json with mode
// 0600. It is the headless / no-keyring fallback for environments where
// libsecret / Keychain / Credential Manager is not available.
//
// Per requirement 13.2 the file is written with mode 0600 and the mode
// is verified after every Save (umask-tolerant: a stricter umask that
// produced 0400 would still be rewritten back to 0600 via os.Chmod).
type fileStore struct {
	// path is the absolute path to cloud-creds.json. It is resolved
	// once at construction time so subsequent calls are deterministic
	// even if the caller mutates HOME mid-process.
	path string
}

// newFileStore returns a CredentialStore backed by the 0600 file
// fallback at ~/.smara/cloud-creds.json.
//
// It does not create the file or the parent directory eagerly; those
// are created on the first Save so a never-used fallback leaves no
// trace on disk.
func newFileStore() CredentialStore {
	path, err := fileFallbackPath()
	if err != nil {
		// Resolution of $HOME is effectively infallible on supported
		// platforms; if it does fail we still return a usable store
		// whose Save / Load will surface the error explicitly so the
		// caller sees a real diagnostic instead of a silent no-op.
		path = ""
		_ = err
	}
	return &fileStore{path: path}
}

// fileFallbackPath returns the absolute path to the cloud-creds.json
// fallback file. It is exported via the unexported helper so the
// composite store (task 4.4) can share the resolution logic.
func fileFallbackPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cloud: fileStore: resolve home dir: %w", err)
	}
	return filepath.Join(home, fileFallbackDir, fileFallbackName), nil
}

// Save writes the credentials to ~/.smara/cloud-creds.json with mode
// 0600. After the write it calls os.Stat and verifies that the file
// permissions are exactly 0600 (mode & 0777 == 0600); if a strict
// umask or pre-existing file produced a different mode, it issues
// os.Chmod and re-verifies. A persistent mode mismatch is a hard
// error so we never silently leave credentials world-readable.
//
// On the first successful fallback activation in a given process the
// store emits a one-time stderr warning identifying the path and the
// observed mode (requirement 13.3).
func (s *fileStore) Save(creds *Credentials) error {
	if creds == nil {
		return errors.New("cloud: fileStore.Save: nil credentials")
	}
	if s.path == "" {
		return errors.New("cloud: fileStore.Save: home directory unresolved")
	}

	body, err := marshalForStorage(creds)
	if err != nil {
		return fmt.Errorf("cloud: fileStore.Save: marshal: %w", err)
	}

	// Ensure the parent directory exists with a tight 0700 mode. We
	// deliberately do not widen permissions on an existing directory
	// so a stricter user setup is preserved.
	if err := os.MkdirAll(filepath.Dir(s.path), fileFallbackDirMode); err != nil {
		return fmt.Errorf("cloud: fileStore.Save: mkdir: %w", err)
	}

	if err := os.WriteFile(s.path, body, fileFallbackMode); err != nil {
		return fmt.Errorf("cloud: fileStore.Save: write: %w", err)
	}

	mode, err := s.verifyMode()
	if err != nil {
		return err
	}

	emitFileFallbackWarning(s.path, mode)
	return nil
}

// verifyMode stats the credentials file and ensures its permission
// bits are exactly 0600. If they are not it attempts a single
// os.Chmod fix; a second mismatch is reported as an error.
func (s *fileStore) verifyMode() (os.FileMode, error) {
	info, err := os.Stat(s.path)
	if err != nil {
		return 0, fmt.Errorf("cloud: fileStore: stat after write: %w", err)
	}
	mode := info.Mode().Perm()
	if mode&fileFallbackModeMask == fileFallbackMode {
		return mode, nil
	}

	if err := os.Chmod(s.path, fileFallbackMode); err != nil {
		return mode, fmt.Errorf(
			"cloud: fileStore: chmod %s to 0600 (observed %#o): %w",
			s.path, mode, err,
		)
	}

	info, err = os.Stat(s.path)
	if err != nil {
		return 0, fmt.Errorf("cloud: fileStore: stat after chmod: %w", err)
	}
	mode = info.Mode().Perm()
	if mode&fileFallbackModeMask != fileFallbackMode {
		return mode, fmt.Errorf(
			"cloud: fileStore: %s mode is %#o after chmod, expected 0600",
			s.path, mode,
		)
	}
	return mode, nil
}

// Load reads the fallback file and decodes it via unmarshalFromStorage.
// A missing file is mapped to ErrNoCredentials so callers can use
// errors.Is to distinguish "not logged in" from real I/O failures.
func (s *fileStore) Load() (*Credentials, error) {
	if s.path == "" {
		return nil, errors.New("cloud: fileStore.Load: home directory unresolved")
	}
	body, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoCredentials
		}
		return nil, fmt.Errorf("cloud: fileStore.Load: read: %w", err)
	}
	creds, err := unmarshalFromStorage(body)
	if err != nil {
		return nil, fmt.Errorf("cloud: fileStore.Load: unmarshal: %w", err)
	}
	return creds, nil
}

// Delete removes the fallback file. A missing file is not an error
// (idempotent logout, requirement 10.1).
func (s *fileStore) Delete() error {
	if s.path == "" {
		return errors.New("cloud: fileStore.Delete: home directory unresolved")
	}
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cloud: fileStore.Delete: remove: %w", err)
	}
	return nil
}

// Source returns the literal "file" per requirement 13.5.
func (s *fileStore) Source() string {
	return "file"
}

// emitFileFallbackWarning prints a single stderr line the first time
// the file fallback is activated in the current process. The warning
// includes the absolute path and the observed permission bits so the
// operator can verify the file mode without a separate ls -l.
func emitFileFallbackWarning(path string, mode os.FileMode) {
	fileFallbackWarnOnce.Do(func() {
		fmt.Fprintf(
			os.Stderr,
			"smara: cloud credentials stored at %s (mode %#o); OS keyring unavailable, falling back to file-based storage\n",
			path, mode.Perm(),
		)
	})
}

// ----------------------------------------------------------------------------
// envStore — headless / CI-CD environment variable backend
// ----------------------------------------------------------------------------

// Environment variable names recognized by the env-mode credential store.
// They are exposed as constants so command-line code (e.g. `cmd/smara/login.go`)
// and tests can refer to the same names without string drift.
const (
	envVarToken    = "SMARA_CLOUD_TOKEN"
	envVarOrg      = "SMARA_CLOUD_ORG"
	envVarRegion   = "SMARA_CLOUD_REGION"
	envVarProvider = "SMARA_CLOUD_PROVIDER"

	// defaultEnvProvider is the Provider name used when SMARA_CLOUD_PROVIDER
	// is unset. Per requirement 11.2, "turso" is the default Provider.
	defaultEnvProvider = "turso"
)

// envStore is the headless / CI-CD credential backend. It loads credentials
// from environment variables (SMARA_CLOUD_TOKEN, SMARA_CLOUD_ORG,
// SMARA_CLOUD_REGION, SMARA_CLOUD_PROVIDER) per requirement 14.3 so that
// non-interactive deployments (Docker containers, GitHub Actions runners,
// systemd-managed bots) can authenticate without invoking the OAuth
// browser flow.
//
// The store is read-only by design: environment variables are owned by
// whoever launched the process (operator / orchestrator), so Save is a
// no-op and Delete returns an informational error pointing the operator
// at the correct way to disable env-mode (unset SMARA_CLOUD_TOKEN in the
// surrounding shell / unit / pipeline definition).
//
// Activation is gated through newEnvStore (below): only when
// SMARA_CLOUD_TOKEN is non-empty does the constructor return (store, true).
// Load re-reads the env at call time so process-level changes (or test
// helpers like t.Setenv) are picked up without recreating the store.
type envStore struct{}

// newEnvStore returns an envStore-backed CredentialStore plus a boolean
// indicating whether env-mode is currently active. The store is active
// when (and only when) SMARA_CLOUD_TOKEN is set to a non-empty value
// per requirement 14.1.
//
// The boolean is the signal used by NewCredentialStore (task 4.4) to
// decide whether env-mode wins priority over the keyring / file backends:
// when (_, ok := newEnvStore()) is true, env-mode short-circuits the
// chain so CI/CD invocations never touch the host keyring or write a
// 0600 file.
//
// Returning (nil, false) when SMARA_CLOUD_TOKEN is unset keeps the
// caller side trivial: a single `if store, ok := newEnvStore(); ok { ... }`
// branch and no need to construct an unusable wrapper.
func newEnvStore() (CredentialStore, bool) {
	if os.Getenv(envVarToken) == "" {
		return nil, false
	}
	return &envStore{}, true
}

// Save is a no-op for env-mode: environment variables are managed by the
// process launcher, not by the CLI. Returning nil keeps the composite
// store contract simple — a successful login flow that produced fresh
// credentials can call Save unconditionally without special-casing the
// active backend.
//
// The token is *not* echoed to any log here; the audit logger
// (task 10.x) is responsible for recording the source as "env" with
// the token redacted.
func (s *envStore) Save(_ *Credentials) error {
	return nil
}

// Load reads SMARA_CLOUD_TOKEN, SMARA_CLOUD_ORG, SMARA_CLOUD_REGION, and
// SMARA_CLOUD_PROVIDER from the process environment and returns a
// populated *Credentials. The env is re-read on every call so test
// harnesses using t.Setenv and operators that mutate the env between
// CLI invocations both get fresh values.
//
// If SMARA_CLOUD_TOKEN became empty after construction (e.g. someone
// unset it mid-process) Load returns ErrNoCredentials so the caller can
// distinguish "env-mode disabled" from "env-mode misconfigured" without
// inspecting error strings.
//
// Per requirement 11.2, an unset SMARA_CLOUD_PROVIDER defaults to
// "turso" so headless deployments work out-of-the-box without having
// to enumerate every optional variable.
func (s *envStore) Load() (*Credentials, error) {
	token := os.Getenv(envVarToken)
	if token == "" {
		return nil, ErrNoCredentials
	}
	provider := os.Getenv(envVarProvider)
	if provider == "" {
		provider = defaultEnvProvider
	}
	return &Credentials{
		Provider: provider,
		Token:    token,
		OrgID:    os.Getenv(envVarOrg),
		Region:   os.Getenv(envVarRegion),
	}, nil
}

// Delete intentionally returns an informational error rather than
// silently succeeding. Env-mode credentials are owned by the process
// launcher, so a `smara memory cloud logout` invocation that would
// otherwise wipe keyring / file state has nothing local to remove and
// the user must instead unset SMARA_CLOUD_TOKEN in the surrounding
// environment (shell, systemd unit, container env, CI secret).
//
// The error is plain (errors.New, no wrapping) so the CLI can surface
// it verbatim to the operator without leaking internal package paths.
func (s *envStore) Delete() error {
	return errors.New("cloud: envStore: cannot delete env-supplied credentials; unset SMARA_CLOUD_TOKEN to disable")
}

// Source returns the literal "env" per requirement 13.5 (extended set)
// and requirement 14.5 — the audit logger uses this value to record
// `source: "env"` for every cloud op performed under env-mode.
func (s *envStore) Source() string {
	return "env"
}

// ----------------------------------------------------------------------------
// LoadHeadlessOrError — explicit headless-mode entrypoint for CLI commands
// ----------------------------------------------------------------------------

// LoadHeadlessOrError returns the credentials sourced from the process
// environment, or an explicit error that names the missing variable when
// the required SMARA_CLOUD_TOKEN is unset / empty.
//
// It is the helper that `cmd/smara/login.go --headless` (and any other
// non-interactive entrypoint, e.g. doctor / serve / mcp under bot
// orchestration) calls to fail fast with an actionable diagnostic
// instead of falling through to the interactive PKCE flow.
//
// Semantics (requirement 14.1, 14.3, 14.4):
//
//   - When SMARA_CLOUD_TOKEN is non-empty, returns the same *Credentials
//     that envStore.Load would yield (Provider defaults to "turso",
//     OrgID / Region / Provider read verbatim from env).
//
//   - When SMARA_CLOUD_TOKEN is empty, returns ErrNoCredentials wrapped
//     with a message that explicitly names the missing env var so the
//     CLI can render it directly to stderr. errors.Is(err, ErrNoCredentials)
//     remains true so the caller can still branch on the sentinel for
//     non-string-based handling (testing, structured logging).
//
// The helper does not look at the keyring or the file fallback by design
// — its only job is to disambiguate "headless mode requested, env
// missing" from "interactive mode, no credentials yet". Code paths that
// want the full priority chain should call NewCredentialStore().Load()
// (task 4.4) instead.
func LoadHeadlessOrError() (*Credentials, error) {
	store, ok := newEnvStore()
	if !ok {
		return nil, fmt.Errorf(
			"%w: headless mode requires %s to be set in the environment",
			ErrNoCredentials, envVarToken,
		)
	}
	return store.Load()
}

// ----------------------------------------------------------------------------
// compositeStore — priority chain wiring envStore / keyringStore / fileStore
// ----------------------------------------------------------------------------

// compositeStore wires the three backends behind CredentialStore into a
// single, deterministic priority chain so call sites only need to invoke
// NewCredentialStore() and let the package decide where credentials
// actually live.
//
// Priority chain (highest → lowest):
//
//  1. envStore — selected exclusively when SMARA_CLOUD_TOKEN is non-empty.
//     Per requirements 14.1 / 14.3, env-mode is meant for CI/CD and bot
//     deployments where the launcher (Docker, systemd, GitHub Actions,
//     Kubernetes secret mounts) owns the token. When env-mode is active
//     we short-circuit the entire chain: keyring and file are NEVER read
//     or written, so a CI runner with leftover keyring entries from a
//     prior interactive session cannot accidentally override the
//     orchestrator-supplied token, and a stray write to the file fallback
//     cannot leak the env-supplied token to disk. envStore.Save being a
//     no-op (and Delete returning an informational error) means callers
//     can use the same code path in both modes without branching.
//
//  2. keyringStore — the preferred persistent backend on workstations
//     with libsecret (Linux), Keychain (macOS), or Credential Manager
//     (Windows). Per requirement 13.1 the entry lives under service name
//     "smara-cloud" / account "default" so Save / Load / Delete are
//     deterministic across runs.
//
//  3. fileStore — the headless fallback at ~/.smara/cloud-creds.json
//     (mode 0600). Per requirement 13.2, when the keyring is unavailable
//     we degrade gracefully to a file with a one-time stderr warning so
//     the operator knows a less-secure path is in use.
//
// Failure mode mapping:
//
//   - keyring backend reports keyring.ErrUnsupportedPlatform → fall
//     through to the file backend transparently. This is the canonical
//     "keyring not available" signal from the zalando/go-keyring fallback
//     provider on platforms without a system secret service (typical on
//     bare Linux servers, minimal Docker images, WSL without dbus).
//
//   - keyring backend reports any *other* error (libsecret crashed, dbus
//     timed out, Credential Manager refused) → fall through to the file
//     backend as well, but the original error is logged to stderr via
//     emitKeyringFallbackWarning so the operator can investigate. We
//     prefer "credentials still work via file" over "credentials silently
//     fail" because the latter would force the user back through the
//     interactive PKCE flow on every CLI invocation.
//
//   - file backend errors (disk full, permissions) propagate verbatim;
//     there is no further fallback below file storage.
//
// Composite Delete semantics (requirement 10.5): logout must clean up
// every persistent location, so Delete fans out to BOTH the keyring AND
// the file backend regardless of which Source() returned the most recent
// Load. This guards against the edge case where a user logged in once
// without a keyring (file-only), then later installed libsecret and
// logged in again (keyring), leaving stale credentials in the file. The
// returned error (when non-nil) names the affected sources so the CLI
// can render the deterministic logout summary required by 10.5.
type compositeStore struct {
	// env is the env-mode store, populated only when SMARA_CLOUD_TOKEN
	// was set at NewCredentialStore() time. When non-nil, the composite
	// short-circuits to env exclusively and the keyring/file backends
	// are never consulted.
	env CredentialStore

	// keyring is the OS keyring backend. Always non-nil for non-env
	// composites; whether it actually responds depends on the host
	// platform (see compositeLoad / compositeSave failure mode mapping).
	keyring CredentialStore

	// file is the 0600 fallback backend. Always non-nil for non-env
	// composites and used both as the keyring fallback AND as a Delete
	// fan-out target so logout cleans every persistent location.
	file CredentialStore

	// activeSource records which underlying store responded to the most
	// recent successful Load, so Source() can report the *actual* origin
	// of the credentials currently in use (requirement 13.5). It is
	// guarded by mu because Source() may be called concurrently with
	// Load (e.g. from a status panel).
	activeSource string
	mu           sync.RWMutex
}

// NewCredentialStore returns the package's canonical CredentialStore — a
// composite that wires envStore, keyringStore, and fileStore into a
// fixed priority chain (env → keyring → file).
//
// Behavioral contract (matching requirements 10.1, 10.2, 10.5, 13.1,
// 13.2, 14.3):
//
//   - Save: if env-mode is active, delegates to envStore (no-op). Otherwise
//     attempts keyring first; on keyring.ErrUnsupportedPlatform OR any
//     other keyring error, falls through to fileStore so a brittle
//     keyring never blocks login.
//
//   - Load: if env-mode is active, returns envStore.Load() exclusively
//     (so a CI/CD runner cannot accidentally pick up keyring leftovers).
//     Otherwise tries keyring; on ErrNoCredentials or
//     ErrUnsupportedPlatform falls through to file. Returns
//     ErrNoCredentials only when *every* applicable store reports it
//     missing, so callers using errors.Is can distinguish "not logged
//     in" from real I/O errors.
//
//   - Delete: if env-mode is active, delegates to envStore (which returns
//     an informational error pointing the operator at SMARA_CLOUD_TOKEN).
//     Otherwise fans out to BOTH keyring and file so logout is total —
//     no source is left holding stale credentials. Errors from individual
//     stores are aggregated with the affected source names so the CLI
//     can render the per-source summary mandated by requirement 10.5.
//
//   - Source: reports the backing store that satisfied the most recent
//     Load. Before any Load has occurred, falls back to the highest
//     priority store that exists (env if active, else keyring) so status
//     panels rendered at startup still produce a sensible answer.
//
// The returned CredentialStore is safe for concurrent use; the
// underlying stores either keep no mutable state (keyringStore,
// envStore) or rely on the OS to serialize file writes (fileStore).
func NewCredentialStore() CredentialStore {
	if envS, ok := newEnvStore(); ok {
		return &compositeStore{
			env:          envS,
			activeSource: envS.Source(),
		}
	}
	return &compositeStore{
		keyring:      newKeyringStore(),
		file:         newFileStore(),
		activeSource: "", // resolved on first Load / Save
	}
}

// envActive reports whether this composite is operating in env-mode.
// When true, every operation short-circuits to the env backend and the
// keyring / file backends are never consulted.
func (c *compositeStore) envActive() bool {
	return c.env != nil
}

// Save persists the supplied credentials through the priority chain.
//
// In env-mode (envActive), delegation is unconditional: envStore.Save is
// a documented no-op and the keyring / file backends are not touched.
// This keeps callers trivially symmetric — a successful login flow that
// produced fresh credentials can call Save without inspecting the
// active backend.
//
// In keyring/file mode, we try keyring first. A successful keyring write
// records "keyring" as the active source and returns. A keyring error is
// classified:
//
//   - keyring.ErrUnsupportedPlatform → expected fallback signal; we
//     emit a one-time stderr warning (so the operator sees the
//     degradation) and write to file instead.
//
//   - any other error → keyring is present but malfunctioning. We still
//     fall through to file (so the user is never locked out), but we
//     emit a stderr warning that names the original keyring error to
//     aid debugging.
func (c *compositeStore) Save(creds *Credentials) error {
	if c.envActive() {
		// envStore.Save is a no-op by design. We still update
		// activeSource so a subsequent Source() call reports "env".
		c.setActiveSource(c.env.Source())
		return c.env.Save(creds)
	}

	if err := c.keyring.Save(creds); err != nil {
		emitKeyringFallbackWarning("save", err)
		if ferr := c.file.Save(creds); ferr != nil {
			return fmt.Errorf(
				"cloud: NewCredentialStore.Save: keyring failed (%v) and file fallback failed: %w",
				err, ferr,
			)
		}
		c.setActiveSource(c.file.Source())
		return nil
	}

	c.setActiveSource(c.keyring.Source())
	return nil
}

// Load returns the credentials from the highest-priority store that has
// them.
//
// In env-mode the call is delegated to envStore exclusively: a CI/CD
// runner with leftover keyring or file credentials must NOT silently
// override the orchestrator-supplied token. envStore.Load returns
// ErrNoCredentials if SMARA_CLOUD_TOKEN was unset between construction
// and the call (e.g. test harness using t.Setenv mid-process), which
// preserves the sentinel-based semantics callers rely on.
//
// In keyring/file mode the chain is keyring → file. A keyring
// ErrNoCredentials or ErrUnsupportedPlatform falls through to file
// (these are both "not authoritative for this entry" signals). Any
// other keyring error is *also* treated as a fallback signal so a
// transient libsecret outage cannot lock an authenticated user out of
// their own file-stored credentials, but we surface the original error
// via emitKeyringFallbackWarning for debuggability.
//
// When neither keyring nor file holds credentials, the function returns
// ErrNoCredentials so callers can use errors.Is to branch on
// "definitely not logged in" without parsing error strings.
func (c *compositeStore) Load() (*Credentials, error) {
	if c.envActive() {
		creds, err := c.env.Load()
		if err == nil {
			c.setActiveSource(c.env.Source())
		}
		return creds, err
	}

	creds, err := c.keyring.Load()
	if err == nil {
		c.setActiveSource(c.keyring.Source())
		return creds, nil
	}
	if !errors.Is(err, ErrNoCredentials) && !isKeyringUnsupported(err) {
		// Real keyring failure (not a "missing entry" or "no keyring on
		// this platform" signal). Surface it once to stderr so the
		// operator can diagnose, but still try the file fallback so the
		// user keeps working.
		emitKeyringFallbackWarning("load", err)
	}

	creds, ferr := c.file.Load()
	if ferr == nil {
		c.setActiveSource(c.file.Source())
		return creds, nil
	}
	if errors.Is(ferr, ErrNoCredentials) {
		// Both stores agree: nothing persisted. Return the sentinel so
		// callers using errors.Is(err, ErrNoCredentials) keep working
		// without inspecting which underlying store reported it.
		return nil, ErrNoCredentials
	}
	return nil, fmt.Errorf("cloud: NewCredentialStore.Load: file fallback: %w", ferr)
}

// Delete removes credentials from every persistent location.
//
// In env-mode the call delegates to envStore, which returns an
// informational error pointing the operator at SMARA_CLOUD_TOKEN.
// Callers should display that error verbatim — it is not a "logout
// failed" signal but rather instructions for unsetting the env var.
//
// In keyring/file mode Delete fans out to BOTH backends regardless of
// which one currently holds credentials. Per requirement 10.5 the
// returned error (when non-nil) names which sources held credentials
// before the call and which sources errored, so the CLI can render the
// per-source logout summary. Successful deletion of a non-existent
// entry is not an error (idempotent logout, requirement 10.1).
//
// The composite tracks which sources actually had credentials by
// calling Load on each backend before Delete, so the CLI can report
// "removed credentials from keyring and file" vs. "no stored
// credentials found" without further bookkeeping. The detection is
// best-effort: a backend that errors on Load is still asked to Delete
// (idempotency makes this safe), but it is not counted as "had
// credentials" in the summary.
func (c *compositeStore) Delete() error {
	if c.envActive() {
		// activeSource transitions back to env so a subsequent Source()
		// call still reports the controlling backend.
		c.setActiveSource(c.env.Source())
		return c.env.Delete()
	}

	type result struct {
		source  string
		hadData bool
		err     error
	}

	results := make([]result, 0, 2)
	for _, store := range []CredentialStore{c.keyring, c.file} {
		had := storeHasCredentials(store)
		err := store.Delete()
		results = append(results, result{
			source:  store.Source(),
			hadData: had,
			err:     err,
		})
	}

	// Reset activeSource: every persistent location has been wiped.
	c.setActiveSource("")

	var (
		affected []string
		failures []string
	)
	for _, r := range results {
		if r.err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", r.source, r.err))
			continue
		}
		if r.hadData {
			affected = append(affected, r.source)
		}
	}

	if len(failures) > 0 {
		// Even if some deletes failed, list the sources that succeeded
		// so the CLI can report partial progress.
		msg := fmt.Sprintf("cloud: NewCredentialStore.Delete: %s", strings.Join(failures, "; "))
		if len(affected) > 0 {
			msg = fmt.Sprintf("%s (cleared: %s)", msg, strings.Join(affected, ", "))
		}
		return errors.New(msg)
	}
	if len(affected) == 0 {
		// Successful no-op (requirement 10.1: deleting non-existent
		// entries is not an error). We return nil rather than a
		// sentinel so the CLI's logout flow always exits cleanly.
		return nil
	}
	// Successful deletion that actually cleared something. The CLI
	// inspects this informational error via errors.As / string match
	// to render the per-source summary required by 10.5.
	return &DeleteSummary{Sources: affected}
}

// Source returns the identifier of the backing store that satisfied the
// most recent successful Load (or, before any Load, the highest-priority
// store that exists). Possible values: "env", "keyring", "file", or "" if
// the composite has been fully Deleted and never reloaded.
func (c *compositeStore) Source() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.activeSource != "" {
		return c.activeSource
	}
	if c.envActive() {
		return c.env.Source()
	}
	// Default to the highest-priority persistent backend so status
	// panels rendered before any Load still produce a meaningful answer.
	return c.keyring.Source()
}

// setActiveSource records the source that satisfied the most recent
// operation. Concurrent-safe via the composite's RWMutex.
func (c *compositeStore) setActiveSource(src string) {
	c.mu.Lock()
	c.activeSource = src
	c.mu.Unlock()
}

// storeHasCredentials reports whether the given store currently holds
// retrievable credentials. Errors other than ErrNoCredentials are
// treated as "unknown" (returns false) to avoid double-counting a
// transient I/O failure as "had data" in the Delete summary.
func storeHasCredentials(s CredentialStore) bool {
	creds, err := s.Load()
	if err != nil {
		return false
	}
	return creds != nil
}

// isKeyringUnsupported reports whether err indicates that the host
// platform has no usable secret service (the canonical "fall back to
// file" signal). It uses errors.Is against the zalando/go-keyring
// sentinel so wrapped errors are recognized.
func isKeyringUnsupported(err error) bool {
	return errors.Is(err, keyring.ErrUnsupportedPlatform)
}

// keyringFallbackWarnOnce gates the stderr warning so a flaky keyring
// does not flood output across many cloud subcommands in a single
// session. Like fileFallbackWarnOnce above, the warning is informational
// and not actionable per call.
var keyringFallbackWarnOnce sync.Once

// emitKeyringFallbackWarning prints a single stderr line the first time
// the keyring backend returns a non-"missing entry" error in the
// current process. The op argument names the operation that triggered
// the fallback ("save" / "load") so the operator can correlate the
// warning with their CLI invocation.
func emitKeyringFallbackWarning(op string, err error) {
	keyringFallbackWarnOnce.Do(func() {
		fmt.Fprintf(
			os.Stderr,
			"smara: cloud credentials %s via keyring failed (%v); falling back to file storage at ~/%s/%s\n",
			op, err, fileFallbackDir, fileFallbackName,
		)
	})
}

// DeleteSummary is the informational error returned by the composite
// Delete when one or more backends successfully cleared credentials.
// The CLI inspects it via errors.As to render the per-source summary
// required by requirement 10.5 ("the CLI SHALL report which storage
// location had credentials removed").
//
// It is an error type rather than a plain return value so the existing
// CredentialStore.Delete() error contract is preserved (callers that do
// not care about the summary keep working unchanged), and so it
// composes naturally with errors.Is / errors.As at higher layers (e.g.
// the audit logger can detect the summary case and record it).
type DeleteSummary struct {
	// Sources is the list of backend identifiers that had credentials
	// before Delete was invoked, in priority order ("keyring" before
	// "file"). It is never empty: the composite returns a nil error
	// rather than an empty summary when no source held credentials.
	Sources []string
}

// Error implements the error interface. The string form is the
// human-readable per-source summary the CLI prints to stdout on a
// successful logout.
func (d *DeleteSummary) Error() string {
	if d == nil || len(d.Sources) == 0 {
		return "cloud: no credentials removed"
	}
	if len(d.Sources) == 1 {
		return fmt.Sprintf("cloud: removed credentials from %s", d.Sources[0])
	}
	return fmt.Sprintf("cloud: removed credentials from %s", strings.Join(d.Sources, " and "))
}
