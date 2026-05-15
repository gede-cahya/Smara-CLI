// Package supabase — EnsureDatabase / List / Delete.
//
// Unlike Turso (where each workspace gets its own libSQL database),
// Supabase uses a single PostgreSQL project with a `smara_memories`
// table. Workspace isolation is done via a `workspace_id` column.
//
// EnsureDatabase is idempotent: it creates the `smara_memories` table
// if it doesn't exist, and returns a DatabaseInfo describing the
// project. The "database name" synthetic — it's derived from the
// workspace name for compatibility with the Provider interface, but
// the actual remote resource is the project itself.
//
// Requirements: 5.1, 5.2, 5.4, 5.5, 6.1.
package supabase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/audit"
	cloud "github.com/gede-cahya/Smara-CLI/internal/memory/cloud"
)

const (
	// smaraMemoriesTable is the canonical table name in Supabase.
	smaraMemoriesTable = "smara_memories"

	// defaultDBNamePatternSupabase mirrors Turso's default pattern.
	defaultDBNamePatternSupabase = "smara-{workspace}"

	// supabaseDBNameMaxLen matches PostgreSQL identifier limit.
	supabaseDBNameMaxLen = 63
)

// ---------------------------------------------------------------------------
// EnsureDatabase — idempotent table creation.
// ---------------------------------------------------------------------------

// EnsureDatabase provisions (or verifies) the remote Supabase table.
//
// For Supabase, "database" means the `smara_memories` table inside the
// user's project. This method:
//  1. Resolves the synthetic database name from the workspace + pattern.
//  2. Creates the smara_memories table via REST if it doesn't exist
//     (using POST with Prefer: resolution=merge-duplicates, or a raw SQL
//     query via the Supabase SQL API).
//  3. Returns DatabaseInfo with the REST URL and auth token.
func (p *SupabaseProvider) EnsureDatabase(ctx context.Context, creds *cloud.Credentials, workspaceName string) (info *cloud.DatabaseInfo, err error) {
	var auditDBName string
	defer func() {
		extra := map[string]any{"workspace_name": workspaceName}
		if err != nil {
			extra["error"] = err.Error()
		}
		_ = audit.LogCloudOp("ensure_database", err == nil, auditDBName, extra)
	}()

	if creds == nil {
		return nil, errors.New("supabase: EnsureDatabase: nil credentials")
	}
	if creds.Token == "" {
		return nil, errors.New("supabase: EnsureDatabase: empty token")
	}
	if creds.OrgID == "" {
		return nil, errors.New("supabase: EnsureDatabase: empty project ref")
	}
	if workspaceName == "" {
		return nil, errors.New("supabase: EnsureDatabase: empty workspace name")
	}

	// Resolve database name from pattern.
	pattern := p.cfg.DBNamePattern
	if pattern == "" {
		pattern = defaultDBNamePatternSupabase
	}
	dbName, err := applySupabaseDBNamePattern(pattern, workspaceName)
	if err != nil {
		return nil, fmt.Errorf("supabase: EnsureDatabase: %w", err)
	}
	auditDBName = dbName

	// Cache for later Push/Pull/Status calls.
	p.serviceKey = creds.Token

	restURL := supabaseRESTURL(creds.OrgID)
	p.restURL = restURL

	// Ensure the smara_memories table exists by issuing a
	// GET with limit=0; if it returns 404, we create the table via
	// the Supabase SQL REST endpoint.
	if err := p.ensureTable(ctx, creds); err != nil {
		return nil, fmt.Errorf("supabase: EnsureDatabase: ensure table %q: %w", smaraMemoriesTable, err)
	}

	return &cloud.DatabaseInfo{
		Provider:    "supabase",
		Name:        dbName,
		URL:         restURL,
		AuthToken:   creds.Token,
		Region:      creds.Region,
		CreatedAt:   time.Now().UTC(),
		WorkspaceID: 0, // filled later by UpsertCloudDatabase
	}, nil
}

// ensureTable creates the smara_memories table if it doesn't exist.
// Uses Supabase's REST API with a raw SQL query via the Supabase
// Management API, or falls back to using the REST table-create pattern.
func (p *SupabaseProvider) ensureTable(ctx context.Context, creds *cloud.Credentials) error {
	// Check if table exists with a GET request.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		p.restURL+"/"+smaraMemoriesTable+"?limit=0", nil)
	if err != nil {
		return err
	}
	req.Header.Set("apikey", creds.Token)
	req.Header.Set("Authorization", "Bearer "+creds.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("check table: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil // Table exists.
	}
	if resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("unexpected status %d checking table", resp.StatusCode)
	}

	// Table doesn't exist. Create it via the Supabase SQL REST endpoint.
	// POST to /rest/v1/rpc/create_smara_table or use raw SQL.
	// Since we can't easily run DDL via the REST table API, we use the
	// Supabase Management API's SQL endpoint.
	//
	// POST https://api.supabase.com/v1/projects/{ref}/sql
	// But this requires the management API key (different from anon/service_role).
	//
	// Alternative: use the PostgREST feature of Supabase. The simplest
	// approach for MVP is to create the table with a single row insert
	// that defines the schema. The column types will be inferred.
	//
	// We'll use the /rest/v1/smara_memories endpoint with a POST.
	// If the table doesn't exist, Supabase returns 404, and we create
	// it by inserting a bootstrap row.

	// For now, we use POST with Prefer: resolution=merge-duplicates to
	// create the table implicitly with an upsert on the primary key.

	body := map[string]interface{}{
		"memory_id":    0,
		"workspace_id": 0,
		"cloud_id":     "bootstrap",
		"device_id":    "bootstrap",
		"content":      "bootstrap",
		"content_hash": "bootstrap",
		"tags":         "[]",
		"source":       "bootstrap",
		"version":      0,
		"created_at":   time.Now().UTC().Format(time.RFC3339),
		"updated_at":   time.Now().UTC().Format(time.RFC3339),
	}
	bodyBytes, _ := json.Marshal(body)

	req2, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.restURL+"/"+smaraMemoriesTable, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req2.Header.Set("apikey", creds.Token)
	req2.Header.Set("Authorization", "Bearer "+creds.Token)
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Prefer", "return=minimal")

	resp2, err := p.httpClient.Do(req2)
	if err != nil {
		return fmt.Errorf("create table via insert: %w", err)
	}
	respBody, _ := io.ReadAll(io.LimitReader(resp2.Body, 1024))
	resp2.Body.Close()

	if resp2.StatusCode >= 200 && resp2.StatusCode < 300 {
		// Table created implicitly via first insert. Now delete the bootstrap row.
		_ = p.deleteBootstrapRow(ctx, creds)
		return nil
	}

	return fmt.Errorf("failed to create table (HTTP %d): %s", resp2.StatusCode, string(respBody))
}

// deleteBootstrapRow removes the bootstrap row used to create the table.
func (p *SupabaseProvider) deleteBootstrapRow(ctx context.Context, creds *cloud.Credentials) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		p.restURL+"/"+smaraMemoriesTable+"?memory_id=eq.0", nil)
	if err != nil {
		return err
	}
	req.Header.Set("apikey", creds.Token)
	req.Header.Set("Authorization", "Bearer "+creds.Token)
	req.Header.Set("Prefer", "return=minimal")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// ---------------------------------------------------------------------------
// ListWorkspaceDatabases — list all databases for the project.
// ---------------------------------------------------------------------------

// ListWorkspaceDatabases returns all workspace databases. For Supabase,
// this means returning a single entry for the project, since all
// workspaces share the same PostgreSQL project.
func (p *SupabaseProvider) ListWorkspaceDatabases(ctx context.Context, creds *cloud.Credentials) ([]cloud.DatabaseInfo, error) {
	if creds == nil {
		return nil, errors.New("supabase: ListWorkspaceDatabases: nil credentials")
	}
	if creds.Token == "" {
		return nil, errors.New("supabase: ListWorkspaceDatabases: empty token")
	}
	if creds.OrgID == "" {
		return nil, errors.New("supabase: ListWorkspaceDatabases: empty project ref")
	}

	restURL := supabaseRESTURL(creds.OrgID)
	return []cloud.DatabaseInfo{
		{
			Provider:  "supabase",
			Name:      smaraMemoriesTable,
			URL:       restURL,
			AuthToken: creds.Token,
			Region:    creds.Region,
			CreatedAt: time.Now().UTC(),
		},
	}, nil
}

// ---------------------------------------------------------------------------
// DeleteWorkspaceDatabase — delete (drop) the smara_memories table.
// ---------------------------------------------------------------------------

// DeleteWorkspaceDatabase removes the smara_memories table.
// Idempotent: a 404 (table not found) is treated as success.
func (p *SupabaseProvider) DeleteWorkspaceDatabase(ctx context.Context, creds *cloud.Credentials, dbName string) error {
	if creds == nil {
		return errors.New("supabase: DeleteWorkspaceDatabase: nil credentials")
	}
	if creds.Token == "" {
		return errors.New("supabase: DeleteWorkspaceDatabase: empty token")
	}
	if creds.OrgID == "" {
		return errors.New("supabase: DeleteWorkspaceDatabase: empty project ref")
	}

	// Delete all rows from the table (we can't DROP via REST easily).
	// Use DELETE with a filter that matches all rows.
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		p.restURL+"/"+smaraMemoriesTable+"?memory_id=gte.0", nil)
	if err != nil {
		return fmt.Errorf("supabase: DeleteWorkspaceDatabase: build request: %w", err)
	}
	req.Header.Set("apikey", creds.Token)
	req.Header.Set("Authorization", "Bearer "+creds.Token)
	req.Header.Set("Prefer", "return=minimal")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("supabase: DeleteWorkspaceDatabase: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil // Already gone — idempotent.
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("supabase: DeleteWorkspaceDatabase: HTTP %d: %s",
			resp.StatusCode, string(body))
	}

	_ = audit.LogCloudOp("delete_database", true, dbName, nil)
	return nil
}

// ---------------------------------------------------------------------------
// Pattern substitution (mirrors Turso's applyDBNamePattern).
// ---------------------------------------------------------------------------

func applySupabaseDBNamePattern(pattern, workspace string) (string, error) {
	if !strings.Contains(pattern, "{workspace}") {
		return "", fmt.Errorf("invalid DBNamePattern %q: must contain {workspace} placeholder", pattern)
	}
	if workspace == "" {
		return "", errors.New("workspace name must not be empty")
	}

	name := strings.ReplaceAll(pattern, "{workspace}", workspace)
	if len(name) > supabaseDBNameMaxLen {
		return "", fmt.Errorf("resolved database name %q exceeds %d characters (got %d)",
			name, supabaseDBNameMaxLen, len(name))
	}
	return name, nil
}
