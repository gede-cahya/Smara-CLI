package memory

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetDeviceIDCache clears the package-level sync.Once so each test gets
// the fresh-process behaviour `EnsureDeviceID` is documented to have.
// sync.Once has no public reset, so we replace it with a zero-valued one.
func resetDeviceIDCache(t *testing.T) {
	t.Helper()
	// Each subtest should observe pristine state. Re-initialise the package
	// vars; this is safe because the tests in this file don't run with
	// t.Parallel().
	deviceIDOnce = sync.Once{}
	deviceIDValue = ""
	deviceIDErr = nil
}

func TestEnsureDeviceID_CreatesAndPersistsUUIDv7(t *testing.T) {
	resetDeviceIDCache(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", tmpHome)
	}

	got, err := EnsureDeviceID()
	require.NoError(t, err)
	require.NotEmpty(t, got)

	// Must be a parseable UUID, version 7.
	parsed, err := uuid.Parse(got)
	require.NoError(t, err, "device id should be a valid UUID")
	assert.Equal(t, uuid.Version(7), parsed.Version(), "device id must be UUID v7")

	// File is created with mode 0600.
	path := filepath.Join(tmpHome, ".smara", "device-id")
	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		// Windows file modes don't honour POSIX bits; skip the assertion there.
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "device-id must be 0600")
	}

	// Content of file (sans trailing newline) matches the returned id.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, got, strings.TrimSpace(string(data)))
}

func TestEnsureDeviceID_ReturnsCachedValue(t *testing.T) {
	resetDeviceIDCache(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", tmpHome)
	}

	first, err := EnsureDeviceID()
	require.NoError(t, err)

	// Mutating the on-disk file should NOT change subsequent results
	// because sync.Once caches the value for the rest of the process.
	path := filepath.Join(tmpHome, ".smara", "device-id")
	require.NoError(t, os.WriteFile(path, []byte("00000000-0000-0000-0000-000000000000\n"), 0o600))

	second, err := EnsureDeviceID()
	require.NoError(t, err)
	assert.Equal(t, first, second, "EnsureDeviceID must memoize via sync.Once")
}

func TestEnsureDeviceID_LoadsExistingFile(t *testing.T) {
	resetDeviceIDCache(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", tmpHome)
	}

	// Pre-seed `~/.smara/device-id` so EnsureDeviceID hits the read path,
	// not the create path.
	dir := filepath.Join(tmpHome, ".smara")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	preexisting := uuid.Must(uuid.NewV7()).String()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "device-id"), []byte(preexisting+"\n"), 0o600))

	got, err := EnsureDeviceID()
	require.NoError(t, err)
	assert.Equal(t, preexisting, got, "must reuse existing device id verbatim")
}

func TestEnsureDeviceID_RejectsEmptyFile(t *testing.T) {
	resetDeviceIDCache(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", tmpHome)
	}

	dir := filepath.Join(tmpHome, ".smara")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "device-id"), []byte("   \n\t"), 0o600))

	_, err := EnsureDeviceID()
	require.Error(t, err, "blank device-id file must surface as an error, not a regenerated id")

	// Cached error: subsequent calls return the same failure without retrying.
	_, err2 := EnsureDeviceID()
	assert.True(t, errors.Is(err2, err) || err2.Error() == err.Error(), "error result must be cached")
}
