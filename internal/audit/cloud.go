package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// VERIFY: cfg.CloudMemory.SyncTables (default) does NOT include "audit.log".
// Full assertion lives in PBT (Property 7/13 — token redaction & sync-table
// allowlist) per Requirement 16.3.

// tokenPatterns are compiled once at package init and reused for every
// redaction pass. Three shapes are covered:
//
//  1. HTTP "Authorization: Bearer <token>" header — preserve the prefix and
//     redact only the token portion via the capture group.
//  2. Inline "token=<...>" / "token: <...>" assignments with at least 16
//     characters of token-shaped payload — redact the whole match.
//  3. JWT triple-segment values (header.payload.signature) — redact the whole
//     match.
var tokenPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization:\s*bearer\s+)\S+`),
	regexp.MustCompile(`(?i)token["\s:=]+["']?[A-Za-z0-9._\-]{16,}["']?`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_\-]{20,}\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+`),
}

const (
	cloudAuditDirName  = ".smara"
	cloudAuditFileName = "audit.log"
	cloudAuditFileMode = 0o600
	cloudAuditDirMode  = 0o700
)

// cloudAuditMu serialises writes to the audit log so concurrent LogCloudOp
// callers cannot interleave partial JSON records on the same line.
var cloudAuditMu sync.Mutex

// cloudAuditEntry is the on-disk JSONL shape for cloud-memory audit records.
// Token-shaped substrings are redacted before serialisation; row content is
// never included.
type cloudAuditEntry struct {
	Timestamp string          `json:"timestamp"`
	Action    string          `json:"action"`
	Success   bool            `json:"success"`
	TargetDB  string          `json:"target_db,omitempty"`
	Source    string          `json:"source,omitempty"`
	Error     string          `json:"error,omitempty"`
	Extra     json.RawMessage `json:"extra,omitempty"`
}

// redactTokens returns s with credential-shaped substrings replaced by the
// literal string "[REDACTED]". The compiled patterns in tokenPatterns are
// applied in order; pattern 1 preserves its prefix capture group while
// patterns 2 and 3 collapse the whole match.
func redactTokens(s string) string {
	if s == "" {
		return s
	}
	s = tokenPatterns[0].ReplaceAllString(s, "${1}[REDACTED]")
	for _, re := range tokenPatterns[1:] {
		s = re.ReplaceAllString(s, "[REDACTED]")
	}
	return s
}

// cloudAuditPath resolves the absolute path of the audit log file under the
// user's home directory and ensures the parent directory exists with mode
// 0o700.
func cloudAuditPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("audit: resolve home dir: %w", err)
	}
	dir := filepath.Join(home, cloudAuditDirName)
	if err := os.MkdirAll(dir, cloudAuditDirMode); err != nil {
		return "", fmt.Errorf("audit: create %s: %w", dir, err)
	}
	return filepath.Join(dir, cloudAuditFileName), nil
}

// LogCloudOp appends a single JSONL record describing a cloud-memory
// operation to ~/.smara/audit.log (mode 0600 on creation). The record shape
// is:
//
//	{timestamp, action, success, target_db, source, error, extra}
//
// Token-shaped substrings are redacted from every text field and from the
// marshaled extra payload before write. The "source" field is sourced from
// extra["source"] when present and otherwise inferred from the
// SMARA_CLOUD_TOKEN environment variable (Requirement 14.5: env-mode loads
// must record source="env"). The "error" field is sourced from
// extra["error"] when present.
//
// Returns an error only when the log file cannot be opened or written; the
// caller is expected to surface (or swallow) the error per its own policy.
func LogCloudOp(action string, success bool, target string, extra map[string]any) error {
	path, err := cloudAuditPath()
	if err != nil {
		return err
	}

	source, errStr := extractAuditMetadata(extra)
	if source == "" && os.Getenv("SMARA_CLOUD_TOKEN") != "" {
		source = "env"
	}

	extraRaw, err := marshalRedactedExtra(extra)
	if err != nil {
		return err
	}

	entry := cloudAuditEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Action:    redactTokens(action),
		Success:   success,
		TargetDB:  redactTokens(target),
		Source:    source,
		Error:     redactTokens(errStr),
		Extra:     extraRaw,
	}

	body, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("audit: marshal entry: %w", err)
	}
	// Defensive final pass: every text field was redacted individually, but
	// run the patterns over the marshaled record so any token that slipped in
	// through a non-redacted code path is still masked before reaching disk.
	body = []byte(redactTokens(string(body)))

	cloudAuditMu.Lock()
	defer cloudAuditMu.Unlock()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, cloudAuditFileMode)
	if err != nil {
		return fmt.Errorf("audit: open %s: %w", path, err)
	}
	defer f.Close()
	// Re-assert the mode in case the file pre-existed with looser permissions
	// (umask-tolerant); ignore the error since the open succeeded above and
	// chmod failures are non-fatal for log durability.
	_ = os.Chmod(path, cloudAuditFileMode)

	if _, err := f.Write(append(body, '\n')); err != nil {
		return fmt.Errorf("audit: write %s: %w", path, err)
	}
	return nil
}

// extractAuditMetadata pulls the "source" and "error" hints from the extra
// map without removing them — callers may legitimately want both surfaced as
// top-level fields and inside the extra payload for downstream tooling.
func extractAuditMetadata(extra map[string]any) (source, errStr string) {
	if extra == nil {
		return "", ""
	}
	if v, ok := extra["source"].(string); ok {
		source = v
	}
	if v, ok := extra["error"].(string); ok {
		errStr = v
	}
	return source, errStr
}

// marshalRedactedExtra serialises the extra map to JSON, applies the token
// patterns to the resulting string, and re-embeds it as a json.RawMessage so
// the final log line stays valid JSON. If redaction breaks the syntax (e.g.
// an unexpected pattern produced unbalanced delimiters), fall back to
// embedding the redacted form as a plain JSON string so the audit record
// remains parseable.
func marshalRedactedExtra(extra map[string]any) (json.RawMessage, error) {
	if len(extra) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(extra)
	if err != nil {
		return nil, fmt.Errorf("audit: marshal extra: %w", err)
	}
	redacted := redactTokens(string(raw))
	if json.Valid([]byte(redacted)) {
		return json.RawMessage(redacted), nil
	}
	fallback, fbErr := json.Marshal(redacted)
	if fbErr != nil {
		return nil, fmt.Errorf("audit: re-marshal redacted extra: %w", fbErr)
	}
	return json.RawMessage(fallback), nil
}
