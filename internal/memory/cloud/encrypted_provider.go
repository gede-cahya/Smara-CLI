// Package cloud — EncryptedProvider: transparent content encryption wrapper.
//
// EncryptedProvider wraps any cloud.Provider and transparently encrypts
// MemoryRow.Content before it leaves the local device (Push) and decrypts
// it when pulling remote changes (Pull).
//
// Data flow (Push):
//
//	Local memories.content (plaintext)
//	  → EncryptedProvider.Push encrypts in-place for pending rows
//	  → inner.Push() sends encrypted content to cloud
//	  → EncryptedProvider decrypts the same rows back to plaintext
//
// Data flow (Pull):
//
//	Cloud returns encrypted content
//	  → inner.Pull() stores encrypted content in local memories
//	  → EncryptedProvider.Pull decrypts content back to plaintext
//
// The encrypt-before-push/decrypt-after-push cycle ensures that:
//   - Cloud storage always contains encrypted content.
//   - Local storage always contains plaintext (for search, FTS, linking).
//   - A crash between encrypt and decrypt is safe: re-running Push will
//     detect already-encrypted rows (base64 pattern) and skip them.
//
// For Turso (libSQL WAL replication): encryption is handled at the libSQL
// file level via encryption_key DSN parameter. EncryptedProvider passes
// through Push/Pull/Status unchanged for Turso.
package cloud

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// EncryptedProvider wraps a real Provider and transparently encrypts/decrypts
// MemoryRow.Content for all sync operations.
type EncryptedProvider struct {
	inner Provider
	key   []byte
	db    *sql.DB // local replica handle, set via SetReplicaDB
}

// NewEncryptedProvider creates a wrapping provider that encrypts content
// using the given AES-256 key. The key is copied so the caller can zero
// their copy after construction.
func NewEncryptedProvider(inner Provider, key []byte) *EncryptedProvider {
	if inner == nil {
		panic("cloud: NewEncryptedProvider: inner provider is nil")
	}
	if len(key) != aesKeySize {
		panic(fmt.Sprintf("cloud: NewEncryptedProvider: key must be %d bytes, got %d", aesKeySize, len(key)))
	}
	keyCopy := make([]byte, aesKeySize)
	copy(keyCopy, key)
	return &EncryptedProvider{
		inner: inner,
		key:   keyCopy,
	}
}

// SetReplicaDB forwards the DB handle to the inner provider (if it
// supports it) and keeps a reference for encrypt/decrypt operations.
func (p *EncryptedProvider) SetReplicaDB(db *sql.DB) {
	p.db = db
	if rp, ok := p.inner.(interface{ SetReplicaDB(*sql.DB) }); ok {
		rp.SetReplicaDB(db)
	}
}

// WithConfig forwards config to the inner provider.
func (p *EncryptedProvider) WithConfig(cfg Config) {
	if cp, ok := p.inner.(interface{ WithConfig(Config) }); ok {
		cp.WithConfig(cfg)
	}
}

// Name delegates to the inner provider.
func (p *EncryptedProvider) Name() string { return p.inner.Name() }

// Login delegates.
func (p *EncryptedProvider) Login(ctx context.Context, opts LoginOptions) (*Credentials, error) {
	return p.inner.Login(ctx, opts)
}

// ValidateCredentials delegates.
func (p *EncryptedProvider) ValidateCredentials(ctx context.Context, creds *Credentials) error {
	return p.inner.ValidateCredentials(ctx, creds)
}

// EnsureDatabase delegates.
func (p *EncryptedProvider) EnsureDatabase(ctx context.Context, creds *Credentials, workspaceName string) (*DatabaseInfo, error) {
	return p.inner.EnsureDatabase(ctx, creds, workspaceName)
}

// OpenStore delegates.
func (p *EncryptedProvider) OpenStore(ctx context.Context, info *DatabaseInfo, localPath string) (string, error) {
	return p.inner.OpenStore(ctx, info, localPath)
}

// Push encrypts pending content, delegates to inner provider, then
// restores plaintext locally.
func (p *EncryptedProvider) Push(ctx context.Context) (*SyncReport, error) {
	if p.db == nil {
		return p.inner.Push(ctx)
	}

	// Phase 1: Encrypt content for pending rows BEFORE push.
	encryptedIDs, err := p.encryptPendingContent(ctx)
	if err != nil {
		return nil, fmt.Errorf("cloud: EncryptedProvider.Push: encrypt phase: %w", err)
	}

	// Phase 2: Delegate to inner provider (sends encrypted content to cloud).
	report, err := p.inner.Push(ctx)

	// Phase 3: Restore plaintext locally (always, even on push error).
	if restoreErr := p.restorePlaintext(ctx, encryptedIDs); restoreErr != nil {
		if report == nil {
			report = &SyncReport{}
		}
		report.Errors = append(report.Errors, fmt.Sprintf("restore plaintext: %v", restoreErr))
		if err == nil {
			err = restoreErr
		}
	}

	return report, err
}

// Pull delegates to inner provider then decrypts any encrypted content
// that was pulled from the cloud.
func (p *EncryptedProvider) Pull(ctx context.Context) (*SyncReport, error) {
	// Phase 1: Inner provider pulls encrypted data from cloud.
	report, err := p.inner.Pull(ctx)

	// Phase 2: Decrypt any encrypted content locally.
	if p.db != nil {
		if decErr := p.decryptPulledContent(ctx); decErr != nil {
			if report == nil {
				report = &SyncReport{}
			}
			report.Errors = append(report.Errors, fmt.Sprintf("decrypt pulled: %v", decErr))
			if err == nil {
				err = decErr
			}
		}
	}

	return report, err
}

// Status delegates.
func (p *EncryptedProvider) Status(ctx context.Context) (*SyncStatus, error) {
	return p.inner.Status(ctx)
}

// ListWorkspaceDatabases delegates.
func (p *EncryptedProvider) ListWorkspaceDatabases(ctx context.Context, creds *Credentials) ([]DatabaseInfo, error) {
	return p.inner.ListWorkspaceDatabases(ctx, creds)
}

// DeleteWorkspaceDatabase delegates.
func (p *EncryptedProvider) DeleteWorkspaceDatabase(ctx context.Context, creds *Credentials, dbName string) error {
	return p.inner.DeleteWorkspaceDatabase(ctx, creds, dbName)
}

// Close zeroes the encryption key and closes the inner provider.
func (p *EncryptedProvider) Close() error {
	for i := range p.key {
		p.key[i] = 0
	}
	return p.inner.Close()
}

// ---------------------------------------------------------------------------
// Internal encrypt/decrypt helpers
// ---------------------------------------------------------------------------

// encryptPendingContent encrypts the content of all pending memories
// in-place. Returns the list of memory IDs that were encrypted so they
// can be restored after push.
//
// Already-encrypted rows (detected via base64-like pattern) are skipped.
func (p *EncryptedProvider) encryptPendingContent(ctx context.Context) ([]int64, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT m.id, m.content
		FROM sync_log sl
		JOIN memories m ON m.id = sl.memory_id
		WHERE sl.status = 'pending'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type pendingRow struct {
		id      int64
		content string
	}
	var pending []pendingRow
	for rows.Next() {
		var pr pendingRow
		if err := rows.Scan(&pr.id, &pr.content); err != nil {
			return nil, err
		}
		pending = append(pending, pr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(pending) == 0 {
		return nil, nil
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var encryptedIDs []int64
	for _, pr := range pending {
		if isAlreadyEncrypted(pr.content) {
			continue // already encrypted from a previous interrupted push
		}
		encrypted, err := EncryptContent(pr.content, p.key)
		if err != nil {
			return encryptedIDs, fmt.Errorf("encrypt memory %d: %w", pr.id, err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE memories SET content = ? WHERE id = ?`, encrypted, pr.id); err != nil {
			return encryptedIDs, err
		}
		encryptedIDs = append(encryptedIDs, pr.id)
	}

	return encryptedIDs, tx.Commit()
}

// restorePlaintext decrypts the specified memory rows back to plaintext.
// It queries the current (encrypted) content and restores the original
// plaintext via decryption.
func (p *EncryptedProvider) restorePlaintext(ctx context.Context, memIDs []int64) error {
	if len(memIDs) == 0 {
		return nil
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, id := range memIDs {
		var content string
		if err := tx.QueryRowContext(ctx, `SELECT content FROM memories WHERE id = ?`, id).Scan(&content); err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return err
		}
		// Only decrypt if it looks encrypted.
		if !isAlreadyEncrypted(content) {
			continue
		}
		plaintext, err := DecryptContent(content, p.key)
		if err != nil {
			return fmt.Errorf("decrypt memory %d: %w", id, err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE memories SET content = ? WHERE id = ?`, plaintext, id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// decryptPulledContent scans the memories table for rows that appear to
// have encrypted content (pulled from remote) and decrypts them in-place.
func (p *EncryptedProvider) decryptPulledContent(ctx context.Context) error {
	rows, err := p.db.QueryContext(ctx, `SELECT id, content FROM memories WHERE LENGTH(content) > 20`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type memRow struct {
		id      int64
		content string
	}
	var candidates []memRow
	for rows.Next() {
		var mr memRow
		if err := rows.Scan(&mr.id, &mr.content); err != nil {
			return err
		}
		if isAlreadyEncrypted(mr.content) {
			candidates = append(candidates, mr)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(candidates) == 0 {
		return nil
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, mr := range candidates {
		plaintext, err := DecryptContent(mr.content, p.key)
		if err != nil {
			// Not actually encrypted with our key — skip.
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE memories SET content = ? WHERE id = ?`, plaintext, mr.id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isAlreadyEncrypted returns true if the content looks like it's already
// encrypted (base64-encoded ciphertext). Encrypted content produced by
// EncryptContent is always base64-encoded and lacks spaces/newlines
// typical of plaintext.
//
// Heuristic: base64 RawStdEncoding charset only, no spaces, length >= 38
// (minimum: 12-byte nonce + 16-byte tag = 28 bytes = 38 base64 chars for
// empty plaintext). Returns false for empty or short strings.
func isAlreadyEncrypted(content string) bool {
	if len(content) < 38 {
		return false
	}
	// base64 RawStdEncoding uses: A-Za-z0-9+/
	for _, c := range content {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '+' || c == '/') {
			return false
		}
	}
	// Additional heuristic: plaintext almost always has spaces.
	return !strings.Contains(content, " ")
}

// compile-time interface checks
var (
	_ Provider = (*EncryptedProvider)(nil)
)
