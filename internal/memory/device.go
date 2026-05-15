package memory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// deviceIDFileName is the basename of the per-install device id file under
// `~/.smara/`. The file holds a single UUID v7 string and is created on
// first use of Cloud Memory features (Requirement 8.2).
const deviceIDFileName = "device-id"

// In-process cache so the file is read at most once per CLI invocation.
// Cloud-aware paths (`Save`, `BackfillCloudFields`, audit logger, etc.)
// can call `EnsureDeviceID` freely without re-hitting the filesystem.
var (
	deviceIDOnce  sync.Once
	deviceIDValue string
	deviceIDErr   error
)

// EnsureDeviceID returns the cross-device identifier stored at
// `~/.smara/device-id`. The first caller in a process generates a fresh
// UUID v7 (when the file is absent), persists it with mode 0600, and
// caches the value via sync.Once so subsequent calls are I/O-free.
//
// The identifier is what we stamp on every cloud-synced memory row's
// `device_id` column so writes can be attributed back to their origin
// device during conflict resolution and audit logging
// (Requirements 4.1 and 8.2).
//
// On error the cached state is the error itself, meaning the next call
// returns the same failure without retrying. The CLI surfaces this so
// the user can fix permissions or disk-full conditions explicitly
// rather than have us silently regenerate a different id.
func EnsureDeviceID() (string, error) {
	deviceIDOnce.Do(func() {
		deviceIDValue, deviceIDErr = loadOrCreateDeviceID()
	})
	return deviceIDValue, deviceIDErr
}

// loadOrCreateDeviceID is the uncached worker exposed for tests. Production
// callers go through `EnsureDeviceID` which wraps this in `sync.Once`.
func loadOrCreateDeviceID() (string, error) {
	path, err := deviceIDPath()
	if err != nil {
		return "", err
	}

	// Happy path: existing file. Trim whitespace because some text editors
	// append a trailing newline on save.
	if data, readErr := os.ReadFile(path); readErr == nil {
		id := strings.TrimSpace(string(data))
		if id == "" {
			return "", fmt.Errorf("device-id file kosong di %s", path)
		}
		return id, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return "", fmt.Errorf("gagal membaca device-id di %s: %w", path, readErr)
	}

	// Missing file: synthesise a fresh UUID v7. v7 is monotonic by time so
	// it sorts cleanly when used in `cloud_id`-style columns elsewhere.
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("gagal generate UUID v7 untuk device-id: %w", err)
	}

	// `~/.smara` may not exist yet on a brand-new install (e.g. a fresh
	// container). Create it with 0700 so user-only access matches the
	// 0600 permission of the secret-bearing files inside.
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o700); mkErr != nil {
		return "", fmt.Errorf("gagal membuat direktori untuk device-id: %w", mkErr)
	}

	idStr := id.String()

	// Atomic write via temp file + rename so a crash mid-write never
	// leaves an empty/partial device-id behind that the next run would
	// then reject as malformed.
	tmp := path + ".tmp"
	if writeErr := os.WriteFile(tmp, []byte(idStr+"\n"), 0o600); writeErr != nil {
		return "", fmt.Errorf("gagal menulis device-id sementara di %s: %w", tmp, writeErr)
	}
	if renErr := os.Rename(tmp, path); renErr != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("gagal commit device-id ke %s: %w", path, renErr)
	}
	// Re-assert 0600 explicitly: a non-default umask or a pre-existing
	// file with looser perms would otherwise survive the rename.
	if chErr := os.Chmod(path, 0o600); chErr != nil {
		return "", fmt.Errorf("gagal set mode 0600 pada %s: %w", path, chErr)
	}
	return idStr, nil
}

// deviceIDPath resolves to `~/.smara/device-id`. Kept as a function so
// tests can shadow `os.UserHomeDir` via `t.Setenv("HOME", ...)`.
func deviceIDPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("gagal resolve home directory untuk device-id: %w", err)
	}
	return filepath.Join(home, ".smara", deviceIDFileName), nil
}
