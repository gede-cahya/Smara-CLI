package cloud

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// AES-256-GCM encrypt/decrypt round-trip
// ---------------------------------------------------------------------------

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, aesKeySize)
	_, err := io.ReadFull(rand.Reader, key)
	require.NoError(t, err)

	tests := []struct {
		name      string
		plaintext string
	}{
		{"english text", "Hello, world! This is a test memory content."},
		{"indonesian text", "Halo dunia! Ini konten memori dengan emoji 🚀✨"},
		{"empty string", ""},
		{"single char", "a"},
		{"long text", strings.Repeat("The quick brown fox jumps over the lazy dog. ", 100)},
		{"unicode heavy", "日本語 한국어 🌍 🌎 🌏 — em dash … ellipsis"},
		{"json content", `{"key":"value","nested":{"array":[1,2,3]}}`},
		{"code snippet", "func main() {\n\tfmt.Println(\"hello\")\n}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := EncryptContent(tt.plaintext, key)
			require.NoError(t, err)
			require.NotEmpty(t, encrypted)

			// Encrypted content should look like base64.
			assert.True(t, isAlreadyEncrypted(encrypted),
				"encrypted content should be detected as encrypted")

			decrypted, err := DecryptContent(encrypted, key)
			require.NoError(t, err)
			assert.Equal(t, tt.plaintext, decrypted)
		})
	}
}

func TestEncryptContentWrongKeySize(t *testing.T) {
	_, err := EncryptContent("test", make([]byte, 16))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "key must be 32 bytes")
}

func TestDecryptContentWrongKeySize(t *testing.T) {
	_, err := DecryptContent("dGVzdA", make([]byte, 16))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "key must be 32 bytes")
}

func TestDecryptContentInvalidBase64(t *testing.T) {
	key := make([]byte, aesKeySize)
	_, err := DecryptContent("!!!not-valid-base64!!!", key)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode base64")
}

func TestDecryptContentTooShort(t *testing.T) {
	key := make([]byte, aesKeySize)
	_, err := DecryptContent("dGVzdA", key) // "test" in base64, too short
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too short")
}

func TestDecryptContentTampered(t *testing.T) {
	key := make([]byte, aesKeySize)
	_, err := io.ReadFull(rand.Reader, key)
	require.NoError(t, err)

	encrypted, err := EncryptContent("tamper me", key)
	require.NoError(t, err)

	// Tamper the ciphertext: change one byte in the encoded form.
	raw, _ := base64.RawStdEncoding.DecodeString(encrypted)
	raw[len(raw)-1] ^= 0xFF // flip last byte (tag)
	tampered := base64.RawStdEncoding.EncodeToString(raw)

	_, err = DecryptContent(tampered, key)
	assert.Error(t, err, "tampered ciphertext should fail GCM auth")
}

func TestDecryptWrongKey(t *testing.T) {
	key1 := make([]byte, aesKeySize)
	key2 := make([]byte, aesKeySize)
	_, _ = io.ReadFull(rand.Reader, key1)
	_, _ = io.ReadFull(rand.Reader, key2)

	encrypted, err := EncryptContent("secret", key1)
	require.NoError(t, err)

	_, err = DecryptContent(encrypted, key2)
	assert.Error(t, err, "decrypt with wrong key should fail")
}

// ---------------------------------------------------------------------------
// isAlreadyEncrypted heuristic
// ---------------------------------------------------------------------------

func TestIsAlreadyEncrypted(t *testing.T) {
	key := make([]byte, aesKeySize)
	_, _ = io.ReadFull(rand.Reader, key)

	enc, _ := EncryptContent("hello world this is a test", key)

	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{"encrypted", enc, true},
		{"plaintext with spaces", "hello world", false},
		{"plaintext json", `{"key":"value"}`, false},
		{"short base64-like", "dGVzdA", false}, // 6 chars, too short (< 38)
		{"empty", "", false},
		{"plaintext code", "func main() {}", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isAlreadyEncrypted(tt.content),
				"content=%q", tt.content)
		})
	}
}

// ---------------------------------------------------------------------------
// Encryption key store: file backend
// ---------------------------------------------------------------------------

func TestFileEncryptionKeyStore_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	store := newFileEncryptionKeyStore()

	// Load should return ErrNoCredentials when file doesn't exist.
	_, err := store.Load()
	assert.ErrorIs(t, err, ErrNoCredentials)

	// Save and load.
	key := make([]byte, aesKeySize)
	_, _ = io.ReadFull(rand.Reader, key)
	require.NoError(t, store.Save(key))

	loaded, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, key, loaded)

	// Verify file permissions.
	path := filepath.Join(tmpDir, encryptionKeyFileDir, encryptionKeyFileName)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(encryptionKeyFileMode), info.Mode().Perm(),
		"encryption key file should be 0600")

	// Delete.
	require.NoError(t, store.Delete())
	_, err = store.Load()
	assert.ErrorIs(t, err, ErrNoCredentials)

	// Delete idempotent.
	assert.NoError(t, store.Delete())
}

func TestFileEncryptionKeyStore_Source(t *testing.T) {
	store := newFileEncryptionKeyStore()
	assert.Equal(t, "file", store.Source())
}

// ---------------------------------------------------------------------------
// EncryptionKeyStore composite priority
// ---------------------------------------------------------------------------

func TestNewEncryptionKeyStore_EnvPriority(t *testing.T) {
	key := make([]byte, aesKeySize)
	_, _ = io.ReadFull(rand.Reader, key)
	encoded := base64.RawStdEncoding.EncodeToString(key)

	t.Setenv(encryptionKeyEnvVar, encoded)

	store := NewEncryptionKeyStore()
	assert.Equal(t, "env", store.Source())

	loaded, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, key, loaded)

	// Save should be no-op in env mode.
	assert.NoError(t, store.Save([]byte("ignored")))
}

// ---------------------------------------------------------------------------
// EnsureEncryptionKey lifecycle
// ---------------------------------------------------------------------------

func TestEnsureEncryptionKey_GenerateAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	// Clear any existing key before test.
	_ = DeleteEncryptionKey()

	// First call: should generate.
	key1, info1, err := EnsureEncryptionKey()
	require.NoError(t, err)
	assert.True(t, info1.Exists)
	assert.Equal(t, aesKeySize, info1.KeySize)
	// Source can be keyring or file depending on OS — just check it's not empty.
	assert.NotEmpty(t, info1.Source)

	// Second call: should load same key (idempotent).
	key2, info2, err := EnsureEncryptionKey()
	require.NoError(t, err)
	assert.Equal(t, key1, key2, "EnsureEncryptionKey should be idempotent")
	assert.Equal(t, info1.Source, info2.Source)

	// Cleanup.
	_ = DeleteEncryptionKey()
}

func TestEncryptionKeyStatus_None(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	// Clear any existing key.
	_ = DeleteEncryptionKey()

	info := EncryptionKeyStatus()
	assert.False(t, info.Exists)
}

func TestLoadEncryptionKey_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	// Clear any existing key first.
	_ = DeleteEncryptionKey()

	_, err := LoadEncryptionKey()
	assert.ErrorIs(t, err, ErrNoCredentials)
}

func TestDeleteEncryptionKey_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Delete non-existent key should not error.
	assert.NoError(t, DeleteEncryptionKey())
}

// ---------------------------------------------------------------------------
// EncryptedProvider smoke test
// ---------------------------------------------------------------------------

type fakeProvider struct {
	name        string
	pushCalled  bool
	pullCalled  bool
	closeCalled bool
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Login(ctx context.Context, _ LoginOptions) (*Credentials, error) {
	return nil, nil
}
func (f *fakeProvider) ValidateCredentials(ctx context.Context, _ *Credentials) error {
	return nil
}
func (f *fakeProvider) EnsureDatabase(ctx context.Context, _ *Credentials, _ string) (*DatabaseInfo, error) {
	return nil, nil
}
func (f *fakeProvider) OpenStore(ctx context.Context, _ *DatabaseInfo, _ string) (string, error) {
	return "", nil
}
func (f *fakeProvider) Push(ctx context.Context) (*SyncReport, error) {
	f.pushCalled = true
	return &SyncReport{}, nil
}
func (f *fakeProvider) Pull(ctx context.Context) (*SyncReport, error) {
	f.pullCalled = true
	return &SyncReport{}, nil
}
func (f *fakeProvider) Status(ctx context.Context) (*SyncStatus, error) {
	return &SyncStatus{}, nil
}
func (f *fakeProvider) ListWorkspaceDatabases(ctx context.Context, _ *Credentials) ([]DatabaseInfo, error) {
	return nil, nil
}
func (f *fakeProvider) DeleteWorkspaceDatabase(ctx context.Context, _ *Credentials, _ string) error {
	return nil
}
func (f *fakeProvider) Close() error {
	f.closeCalled = true
	return nil
}

func TestEncryptedProvider_Passthrough(t *testing.T) {
	key := make([]byte, aesKeySize)
	_, _ = io.ReadFull(rand.Reader, key)

	inner := &fakeProvider{name: "fake"}
	ep := NewEncryptedProvider(inner, key)

	assert.Equal(t, "fake", ep.Name())

	// Push/Pull without DB should pass through.
	ctx := context.Background()
	report, err := ep.Push(ctx)
	assert.NoError(t, err)
	assert.True(t, inner.pushCalled)
	assert.NotNil(t, report)

	report, err = ep.Pull(ctx)
	assert.NoError(t, err)
	assert.True(t, inner.pullCalled)
	assert.NotNil(t, report)

	// Close.
	assert.NoError(t, ep.Close())
	assert.True(t, inner.closeCalled)
}

func TestEncryptedProvider_PanicOnNilInner(t *testing.T) {
	key := make([]byte, aesKeySize)
	assert.Panics(t, func() {
		NewEncryptedProvider(nil, key)
	})
}

func TestEncryptedProvider_PanicOnWrongKeySize(t *testing.T) {
	assert.Panics(t, func() {
		NewEncryptedProvider(&fakeProvider{}, make([]byte, 16))
	})
}

// ---------------------------------------------------------------------------
// Compile-time interface satisfaction
// ---------------------------------------------------------------------------

func TestEncryptedProviderSatisfiesProviderInterface(t *testing.T) {
	key := make([]byte, aesKeySize)
	_, _ = io.ReadFull(rand.Reader, key)
	_ = NewEncryptedProvider(&fakeProvider{}, key)
	// If we get here, EncryptedProvider satisfies Provider.
}
