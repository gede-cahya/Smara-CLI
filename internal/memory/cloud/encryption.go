// Package cloud — encryption-at-rest key management and AES-256-GCM
// encrypt/decrypt helpers.
//
// This file implements the encryption key lifecycle (generate, store,
// load, delete) using the same three-tier credential infrastructure as
// cloud tokens (keyring → file → env), plus the symmetric encrypt/decrypt
// functions used by EncryptedProvider to protect content before it
// leaves the local device.
//
// Key storage hierarchy (mirrors credentials.go):
//
//  1. Env:   SMARA_ENCRYPTION_KEY (headless/CI, read-only)
//  2. Keyring: service "smara-cloud-encryption", account "default"
//  3. File:  ~/.smara/cloud-encryption-key (mode 0600)
//
// Key generation: on first use when EncryptAtRest=true and no key exists,
// EnsureEncryptionKey generates a cryptographically random 32-byte key
// (AES-256) and persists it through the composite store. Subsequent calls
// are idempotent — they load the existing key without regenerating.
//
// Crypto: AES-256-GCM with random 12-byte nonce. The wire format is
// nonce (12 bytes) || ciphertext || tag (16 bytes), base64-encoded for
// safe storage in SQLite TEXT / JSON string columns. AES-256-GCM is
// chosen because:
//   - It is an AEAD mode that provides both confidentiality and integrity.
//   - It is hardware-accelerated on amd64 (AES-NI) and arm64 (PMULL).
//   - It is the de-facto standard for at-rest content encryption.
//   - The Go standard library implementation (crypto/aes, crypto/cipher)
//     is constant-time and well-audited.
//
// Security notes:
//   - The encryption key itself is protected by the OS keyring (or the
//     0600 file fallback). An attacker who can read the keyring can
//     also read the cloud token, so the threat model assumes local
//     compromise is game-over.
//   - The nonce is randomly generated per encryption operation, never
//     reused with the same key. AES-GCM safety requires unique
//     (key, nonce) pairs; random 96-bit nonces satisfy this with
//     negligible collision probability for the expected volume of
//     memory rows.
//   - The base64 encoding is standard (RFC 4648) without padding, chosen
//     to produce compact, URL-safe strings that fit cleanly into JSON
//     and SQLite TEXT columns without escaping issues.
package cloud

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/zalando/go-keyring"
)

// ---------------------------------------------------------------------------
// Encryption key store constants
// ---------------------------------------------------------------------------

const (
	encryptionKeyringService = "smara-cloud-encryption"
	encryptionKeyringAccount = "default"

	encryptionKeyFileDir  = ".smara"
	encryptionKeyFileName = "cloud-encryption-key"
	encryptionKeyFileMode = 0o600
	encryptionKeyDirMode  = 0o700

	encryptionKeyEnvVar = "SMARA_ENCRYPTION_KEY"

	// AES-256 key size in bytes.
	aesKeySize = 32
	// AES-GCM nonce size in bytes (recommended: 12).
	aesNonceSize = 12
)

// ---------------------------------------------------------------------------
// EncryptionKeyStore interface
// ---------------------------------------------------------------------------

// EncryptionKeyStore abstracts persistence of the AES-256 encryption key.
// The three backends (env, keyring, file) implement this interface and
// are wired into a composite chain by NewEncryptionKeyStore.
type EncryptionKeyStore interface {
	Save(key []byte) error
	Load() ([]byte, error)
	Delete() error
	Source() string
}

// ---------------------------------------------------------------------------
// Env-backed encryption key store
// ---------------------------------------------------------------------------

// envEncryptionKeyStore reads the encryption key from SMARA_ENCRYPTION_KEY.
// Read-only by design: Save is a no-op, Delete returns an informational error.
type envEncryptionKeyStore struct{}

func newEnvEncryptionKeyStore() (EncryptionKeyStore, bool) {
	if os.Getenv(encryptionKeyEnvVar) == "" {
		return nil, false
	}
	return &envEncryptionKeyStore{}, true
}

func (s *envEncryptionKeyStore) Save(_ []byte) error { return nil }
func (s *envEncryptionKeyStore) Source() string      { return "env" }

func (s *envEncryptionKeyStore) Load() ([]byte, error) {
	encoded := os.Getenv(encryptionKeyEnvVar)
	if encoded == "" {
		return nil, ErrNoCredentials
	}
	key, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("cloud: env encryption key: decode base64: %w", err)
	}
	if len(key) != aesKeySize {
		return nil, fmt.Errorf("cloud: env encryption key: expected %d bytes, got %d", aesKeySize, len(key))
	}
	return key, nil
}

func (s *envEncryptionKeyStore) Delete() error {
	return errors.New("cloud: cannot delete env-supplied encryption key; unset SMARA_ENCRYPTION_KEY to disable")
}

// ---------------------------------------------------------------------------
// Keyring-backed encryption key store
// ---------------------------------------------------------------------------

type keyringEncryptionKeyStore struct{}

func newKeyringEncryptionKeyStore() EncryptionKeyStore {
	return &keyringEncryptionKeyStore{}
}

func (s *keyringEncryptionKeyStore) Save(key []byte) error {
	encoded := base64.RawStdEncoding.EncodeToString(key)
	return keyring.Set(encryptionKeyringService, encryptionKeyringAccount, encoded)
}

func (s *keyringEncryptionKeyStore) Load() ([]byte, error) {
	encoded, err := keyring.Get(encryptionKeyringService, encryptionKeyringAccount)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, ErrNoCredentials
		}
		return nil, fmt.Errorf("cloud: keyring encryption key: %w", err)
	}
	key, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("cloud: keyring encryption key: decode: %w", err)
	}
	if len(key) != aesKeySize {
		return nil, fmt.Errorf("cloud: keyring encryption key: expected %d bytes, got %d", aesKeySize, len(key))
	}
	return key, nil
}

func (s *keyringEncryptionKeyStore) Delete() error {
	err := keyring.Delete(encryptionKeyringService, encryptionKeyringAccount)
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

func (s *keyringEncryptionKeyStore) Source() string { return "keyring" }

// ---------------------------------------------------------------------------
// File-backed encryption key store (0600 fallback)
// ---------------------------------------------------------------------------

type fileEncryptionKeyStore struct {
	path string
}

func newFileEncryptionKeyStore() EncryptionKeyStore {
	home, err := os.UserHomeDir()
	path := ""
	if err == nil {
		path = filepath.Join(home, encryptionKeyFileDir, encryptionKeyFileName)
	}
	return &fileEncryptionKeyStore{path: path}
}

func (s *fileEncryptionKeyStore) Source() string { return "file" }

func (s *fileEncryptionKeyStore) Save(key []byte) error {
	if s.path == "" {
		return errors.New("cloud: file encryption key: home dir unresolved")
	}
	encoded := base64.RawStdEncoding.EncodeToString(key)
	if err := os.MkdirAll(filepath.Dir(s.path), encryptionKeyDirMode); err != nil {
		return err
	}
	if err := os.WriteFile(s.path, []byte(encoded), encryptionKeyFileMode); err != nil {
		return err
	}
	// Verify mode.
	info, err := os.Stat(s.path)
	if err != nil {
		return err
	}
	if info.Mode().Perm() != encryptionKeyFileMode {
		_ = os.Chmod(s.path, encryptionKeyFileMode)
	}
	return nil
}

func (s *fileEncryptionKeyStore) Load() ([]byte, error) {
	if s.path == "" {
		return nil, ErrNoCredentials
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoCredentials
		}
		return nil, err
	}
	key, err := base64.RawStdEncoding.DecodeString(string(data))
	if err != nil {
		return nil, fmt.Errorf("cloud: file encryption key: decode: %w", err)
	}
	if len(key) != aesKeySize {
		return nil, fmt.Errorf("cloud: file encryption key: expected %d bytes, got %d", aesKeySize, len(key))
	}
	return key, nil
}

func (s *fileEncryptionKeyStore) Delete() error {
	if s.path == "" {
		return errors.New("cloud: file encryption key: home dir unresolved")
	}
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// Composite encryption key store (env → keyring → file)
// ---------------------------------------------------------------------------

// compositeEncryptionKeyStore wires the three backends into a priority chain
// matching the cloud credentials composite: env (if active) exclusive,
// otherwise keyring → file fallback.
type compositeEncryptionKeyStore struct {
	env     EncryptionKeyStore
	keyring EncryptionKeyStore
	file    EncryptionKeyStore

	activeSource string
	mu           sync.RWMutex
}

// NewEncryptionKeyStore returns the canonical encryption key store.
// Priority: env (if SMARA_ENCRYPTION_KEY is set) → keyring → file.
func NewEncryptionKeyStore() EncryptionKeyStore {
	if envStore, ok := newEnvEncryptionKeyStore(); ok {
		return &compositeEncryptionKeyStore{
			env:          envStore,
			activeSource: "env",
		}
	}
	return &compositeEncryptionKeyStore{
		keyring: newKeyringEncryptionKeyStore(),
		file:    newFileEncryptionKeyStore(),
	}
}

func (c *compositeEncryptionKeyStore) envActive() bool { return c.env != nil }

func (c *compositeEncryptionKeyStore) Save(key []byte) error {
	if c.envActive() {
		c.setSource(c.env.Source())
		return c.env.Save(key)
	}

	if err := c.keyring.Save(key); err != nil {
		emitKeyringFallbackWarning("save-encryption-key", err)
		if ferr := c.file.Save(key); ferr != nil {
			return fmt.Errorf("cloud: encryption key save: keyring failed (%v) and file fallback: %w", err, ferr)
		}
		c.setSource(c.file.Source())
		return nil
	}
	c.setSource(c.keyring.Source())
	return nil
}

func (c *compositeEncryptionKeyStore) Load() ([]byte, error) {
	if c.envActive() {
		key, err := c.env.Load()
		if err == nil {
			c.setSource(c.env.Source())
		}
		return key, err
	}

	key, err := c.keyring.Load()
	if err == nil {
		c.setSource(c.keyring.Source())
		return key, nil
	}
	if !errors.Is(err, ErrNoCredentials) && !isKeyringUnsupported(err) {
		emitKeyringFallbackWarning("load-encryption-key", err)
	}

	key, ferr := c.file.Load()
	if ferr == nil {
		c.setSource(c.file.Source())
		return key, nil
	}
	if errors.Is(ferr, ErrNoCredentials) {
		return nil, ErrNoCredentials
	}
	return nil, fmt.Errorf("cloud: encryption key load: file fallback: %w", ferr)
}

func (c *compositeEncryptionKeyStore) Delete() error {
	if c.envActive() {
		return c.env.Delete()
	}
	// Fan out to both persistent stores.
	var errs []error
	for _, s := range []EncryptionKeyStore{c.keyring, c.file} {
		if err := s.Delete(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("cloud: encryption key delete: %v", errs)
	}
	c.setSource("")
	return nil
}

func (c *compositeEncryptionKeyStore) Source() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.activeSource != "" {
		return c.activeSource
	}
	if c.envActive() {
		return c.env.Source()
	}
	return c.keyring.Source()
}

func (c *compositeEncryptionKeyStore) setSource(src string) {
	c.mu.Lock()
	c.activeSource = src
	c.mu.Unlock()
}

// ---------------------------------------------------------------------------
// EnsureEncryptionKey — idempotent key lifecycle
// ---------------------------------------------------------------------------

// EncryptionKeyInfo describes the current state of the encryption key.
type EncryptionKeyInfo struct {
	Exists  bool   `json:"exists"`
	Source  string `json:"source"`   // "env", "keyring", "file", or "" if none
	KeySize int    `json:"key_size"` // bytes (32 for AES-256)
}

// EnsureEncryptionKey returns the active AES-256 key, generating and
// persisting a new one if none exists. It is idempotent: repeated calls
// return the same key without regenerating.
//
// When env-mode is active (SMARA_ENCRYPTION_KEY set), the key is loaded
// from the environment and never generated or persisted.
//
// The returned EncryptionKeyInfo describes the source and state for
// CLI reporting (smara memory cloud encryption status).
func EnsureEncryptionKey() (key []byte, info EncryptionKeyInfo, err error) {
	store := NewEncryptionKeyStore()

	key, err = store.Load()
	if err == nil && len(key) == aesKeySize {
		return key, EncryptionKeyInfo{
			Exists:  true,
			Source:  store.Source(),
			KeySize: len(key),
		}, nil
	}

	if !errors.Is(err, ErrNoCredentials) {
		// Real error (not "not found") — surface it.
		return nil, EncryptionKeyInfo{}, err
	}

	// No key exists and not in env-mode — generate a new one.
	// Only generate if we're NOT in env-mode (env store is read-only).
	if cs, ok := store.(*compositeEncryptionKeyStore); ok && cs.envActive() {
		return nil, EncryptionKeyInfo{Exists: false}, fmt.Errorf(
			"cloud: encryption key: %s is set but could not be loaded; check the value is a valid base64-encoded 32-byte key",
			encryptionKeyEnvVar,
		)
	}

	newKey := make([]byte, aesKeySize)
	if _, err := io.ReadFull(rand.Reader, newKey); err != nil {
		return nil, EncryptionKeyInfo{}, fmt.Errorf("cloud: generate encryption key: %w", err)
	}

	if err := store.Save(newKey); err != nil {
		return nil, EncryptionKeyInfo{}, fmt.Errorf("cloud: persist encryption key: %w", err)
	}

	return newKey, EncryptionKeyInfo{
		Exists:  true,
		Source:  store.Source(),
		KeySize: len(newKey),
	}, nil
}

// LoadEncryptionKey returns the stored encryption key, or ErrNoCredentials
// if none exists. Unlike EnsureEncryptionKey, it never generates a new key.
func LoadEncryptionKey() ([]byte, error) {
	return NewEncryptionKeyStore().Load()
}

// DeleteEncryptionKey removes the encryption key from all persistent stores.
func DeleteEncryptionKey() error {
	return NewEncryptionKeyStore().Delete()
}

// EncryptionKeyStatus reports whether an encryption key exists and where.
func EncryptionKeyStatus() EncryptionKeyInfo {
	store := NewEncryptionKeyStore()
	key, err := store.Load()
	if err != nil || len(key) != aesKeySize {
		return EncryptionKeyInfo{Exists: false}
	}
	return EncryptionKeyInfo{
		Exists:  true,
		Source:  store.Source(),
		KeySize: len(key),
	}
}

// ---------------------------------------------------------------------------
// AES-256-GCM encrypt / decrypt
// ---------------------------------------------------------------------------

// EncryptContent encrypts plaintext with the given AES-256 key using
// AES-GCM with a random 12-byte nonce. Returns the base64-encoded wire
// format: nonce (12) || ciphertext || tag (16).
//
// The plaintext can be any valid UTF-8 string (memory content). Empty
// plaintext is encrypted normally (an empty plaintext still produces a
// valid ciphertext with authentication tag).
func EncryptContent(plaintext string, key []byte) (string, error) {
	if len(key) != aesKeySize {
		return "", fmt.Errorf("cloud: encrypt: key must be %d bytes, got %d", aesKeySize, len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("cloud: encrypt: new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("cloud: encrypt: new GCM: %w", err)
	}

	nonce := make([]byte, aesNonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("cloud: encrypt: generate nonce: %w", err)
	}

	// Seal appends the ciphertext to nonce and returns nonce || ciphertext || tag.
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawStdEncoding.EncodeToString(ciphertext), nil
}

// DecryptContent decrypts a base64-encoded ciphertext produced by
// EncryptContent. Returns the original plaintext string.
//
// Any tampering with the ciphertext (bit flips, truncation) is detected
// by the GCM authentication tag and surfaced as an error.
func DecryptContent(encoded string, key []byte) (string, error) {
	if len(key) != aesKeySize {
		return "", fmt.Errorf("cloud: decrypt: key must be %d bytes, got %d", aesKeySize, len(key))
	}

	ciphertext, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("cloud: decrypt: decode base64: %w", err)
	}

	if len(ciphertext) < aesNonceSize {
		return "", errors.New("cloud: decrypt: ciphertext too short")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("cloud: decrypt: new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("cloud: decrypt: new GCM: %w", err)
	}

	nonce := ciphertext[:aesNonceSize]
	encrypted := ciphertext[aesNonceSize:]

	plaintext, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", fmt.Errorf("cloud: decrypt: %w", err)
	}

	return string(plaintext), nil
}
