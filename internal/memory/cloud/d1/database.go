// Package d1 — EnsureDatabase / List / Delete.
//
// Cloudflare D1 uses API Tokens for auth. Each workspace gets its own
// D1 database (SQLite-compatible) on the Cloudflare edge.
//
// EnsureDatabase is idempotent: it creates a D1 database if one with
// the given name doesn't exist, and returns the existing one otherwise.
//
// Database names are derived from the workspace name using the
// DBNamePattern config (default: "smara-{workspace}").
//
// Requirements: 5.1, 5.2, 5.4, 5.5, 6.1.
package d1

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
	// smaraMemoriesTable is the canonical table name in D1.
	smaraMemoriesTable = "smara_memories"

	// defaultDBNamePatternD1 mirrors Turso's default pattern.
	defaultDBNamePatternD1 = "smara-{workspace}"

	// d1DBNameMaxLen is D1's database name limit.
	d1DBNameMaxLen = 255
)

// ---------------------------------------------------------------------------
// Cloudflare API response types
// ---------------------------------------------------------------------------

// cfAPIResponse wraps the Cloudflare API response envelope.
type cfAPIResponse struct {
	Success  bool              `json:"success"`
	Errors   []cfAPIError      `json:"errors"`
	Messages []string          `json:"messages"`
	Result   json.RawMessage   `json:"result"`
	ResultInfo *cfResultInfo   `json:"result_info,omitempty"`
}

type cfAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfResultInfo struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Count      int `json:"count"`
	TotalCount int `json:"total_count"`
}

// cfD1Database is the shape of a D1 database in Cloudflare API responses.
type cfD1Database struct {
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// cfD1QueryResult is the shape of the D1 query API response.
type cfD1QueryResult struct {
	Success  bool             `json:"success"`
	Errors   []cfAPIError     `json:"errors"`
	Messages []string         `json:"messages"`
	Result   []cfD1QueryMeta  `json:"result"`
}

type cfD1QueryMeta struct {
	Results []map[string]interface{} `json:"results"`
	Success bool                     `json:"success"`
	Meta    cfD1Meta                 `json:"meta"`
}

type cfD1Meta struct {
	ChangedDB   bool    `json:"changed_db"`
	Changes     int     `json:"changes"`
	Duration    float64 `json:"duration"`
	LastRowID   int64   `json:"last_row_id"`
	RowsRead    int64   `json:"rows_read"`
	RowsWritten int64   `json:"rows_written"`
}

// ---------------------------------------------------------------------------
// EnsureDatabase — idempotent D1 database creation.
// ---------------------------------------------------------------------------

// EnsureDatabase provisions (or verifies) a remote D1 database.
//
// Flow:
//  1. Resolve the database name from the workspace + pattern.
//  2. Check if a D1 database with that name already exists via LIST.
//  3. If found, return its UUID as DatabaseInfo.
//  4. If not found, create a new D1 database via POST.
//  5. Create the smara_memories table via D1 query API.
func (p *D1Provider) EnsureDatabase(ctx context.Context, creds *cloud.Credentials, workspaceName string) (info *cloud.DatabaseInfo, err error) {
	var auditDBName string
	defer func() {
		extra := map[string]any{"workspace_name": workspaceName}
		if err != nil {
			extra["error"] = err.Error()
		}
		_ = audit.LogCloudOp("ensure_database", err == nil, auditDBName, extra)
	}()

	if creds == nil {
		return nil, errors.New("d1: EnsureDatabase: nil credentials")
	}
	if creds.Token == "" {
		return nil, errors.New("d1: EnsureDatabase: empty token")
	}
	if creds.OrgID == "" {
		return nil, errors.New("d1: EnsureDatabase: empty account ID")
	}
	if workspaceName == "" {
		return nil, errors.New("d1: EnsureDatabase: empty workspace name")
	}

	// Resolve database name from pattern.
	pattern := p.cfg.DBNamePattern
	if pattern == "" {
		pattern = defaultDBNamePatternD1
	}
	dbName, err := applyD1DBNamePattern(pattern, workspaceName)
	if err != nil {
		return nil, fmt.Errorf("d1: EnsureDatabase: %w", err)
	}
	auditDBName = dbName

	// Cache for later Push/Pull/Status calls.
	p.apiToken = creds.Token
	p.accountID = creds.OrgID

	// Check if a database with this name already exists.
	existing, err := p.findDatabaseByName(ctx, dbName)
	if err != nil {
		return nil, fmt.Errorf("d1: EnsureDatabase: list databases: %w", err)
	}

	if existing != nil {
		// Already exists — cache the ID and return.
		p.databaseID = existing.UUID

		// Ensure the smara_memories table exists.
		if err := p.ensureTable(ctx); err != nil {
			return nil, fmt.Errorf("d1: EnsureDatabase: ensure table: %w", err)
		}

		return &cloud.DatabaseInfo{
			Provider:  "d1",
			Name:      existing.Name,
			URL:       cfAPIBase + "/accounts/" + p.accountID + "/d1/database/" + existing.UUID,
			AuthToken: creds.Token,
			Region:    creds.Region,
			CreatedAt: parseCFTime(existing.CreatedAt),
		}, nil
	}

	// Create a new D1 database.
	created, err := p.createDatabase(ctx, dbName)
	if err != nil {
		return nil, fmt.Errorf("d1: EnsureDatabase: create database: %w", err)
	}
	p.databaseID = created.UUID

	// Create the smara_memories table in the new database.
	if err := p.ensureTable(ctx); err != nil {
		return nil, fmt.Errorf("d1: EnsureDatabase: ensure table: %w", err)
	}

	return &cloud.DatabaseInfo{
		Provider:  "d1",
		Name:      created.Name,
		URL:       cfAPIBase + "/accounts/" + p.accountID + "/d1/database/" + created.UUID,
		AuthToken: creds.Token,
		Region:    creds.Region,
		CreatedAt: parseCFTime(created.CreatedAt),
	}, nil
}

// findDatabaseByName searches for an existing D1 database by name.
// Returns nil if not found.
func (p *D1Provider) findDatabaseByName(ctx context.Context, name string) (*cfD1Database, error) {
	if p.accountID == "" || p.apiToken == "" {
		return nil, errors.New("d1: not authenticated")
	}

	// List all D1 databases and search by name.
	endpoint := cfAPIBase + "/accounts/" + p.accountID + "/d1/database?per_page=100"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp cfAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if !apiResp.Success {
		return nil, cfAPIErrors(apiResp.Errors)
	}

	var dbs []cfD1Database
	if err := json.Unmarshal(apiResp.Result, &dbs); err != nil {
		return nil, fmt.Errorf("decode databases: %w", err)
	}

	for _, db := range dbs {
		if db.Name == name {
			return &db, nil
		}
	}
	return nil, nil
}

// createDatabase creates a new D1 database via the Cloudflare API.
func (p *D1Provider) createDatabase(ctx context.Context, name string) (*cfD1Database, error) {
	body := map[string]string{"name": name}
	bodyBytes, _ := json.Marshal(body)

	endpoint := cfAPIBase + "/accounts/" + p.accountID + "/d1/database"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp cfAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if !apiResp.Success {
		return nil, cfAPIErrors(apiResp.Errors)
	}

	var db cfD1Database
	if err := json.Unmarshal(apiResp.Result, &db); err != nil {
		return nil, fmt.Errorf("decode database: %w", err)
	}
	return &db, nil
}

// ensureTable creates the smara_memories table in the D1 database if
// it doesn't exist.
func (p *D1Provider) ensureTable(ctx context.Context) error {
	sqlDDL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			memory_id    INTEGER PRIMARY KEY,
			workspace_id INTEGER NOT NULL DEFAULT 0,
			cloud_id     TEXT UNIQUE NOT NULL,
			device_id    TEXT NOT NULL DEFAULT '',
			content      TEXT NOT NULL DEFAULT '',
			content_hash TEXT NOT NULL DEFAULT '',
			tags         TEXT NOT NULL DEFAULT '[]',
			source       TEXT NOT NULL DEFAULT '',
			version      INTEGER NOT NULL DEFAULT 1,
			created_at   TEXT NOT NULL DEFAULT '',
			updated_at   TEXT NOT NULL DEFAULT ''
		)
	`, smaraMemoriesTable)

	return p.execD1Query(ctx, sqlDDL)
}

// ---------------------------------------------------------------------------
// ListWorkspaceDatabases — list all D1 databases.
// ---------------------------------------------------------------------------

// ListWorkspaceDatabases returns every D1 database in the account.
func (p *D1Provider) ListWorkspaceDatabases(ctx context.Context, creds *cloud.Credentials) ([]cloud.DatabaseInfo, error) {
	if creds == nil {
		return nil, errors.New("d1: ListWorkspaceDatabases: nil credentials")
	}
	if creds.Token == "" {
		return nil, errors.New("d1: ListWorkspaceDatabases: empty token")
	}
	if creds.OrgID == "" {
		return nil, errors.New("d1: ListWorkspaceDatabases: empty account ID")
	}

	// Temporarily set auth for the request if not already cached.
	savedToken := p.apiToken
	savedAccount := p.accountID
	p.apiToken = creds.Token
	p.accountID = creds.OrgID
	defer func() {
		p.apiToken = savedToken
		p.accountID = savedAccount
	}()

	endpoint := cfAPIBase + "/accounts/" + p.accountID + "/d1/database?per_page=100"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp cfAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}
	if !apiResp.Success {
		return nil, cfAPIErrors(apiResp.Errors)
	}

	var dbs []cfD1Database
	if err := json.Unmarshal(apiResp.Result, &dbs); err != nil {
		return nil, err
	}

	var infos []cloud.DatabaseInfo
	for _, db := range dbs {
		infos = append(infos, cloud.DatabaseInfo{
			Provider:  "d1",
			Name:      db.Name,
			URL:       cfAPIBase + "/accounts/" + p.accountID + "/d1/database/" + db.UUID,
			AuthToken: creds.Token,
			CreatedAt: parseCFTime(db.CreatedAt),
		})
	}
	return infos, nil
}

// ---------------------------------------------------------------------------
// DeleteWorkspaceDatabase — delete a D1 database.
// ---------------------------------------------------------------------------

// DeleteWorkspaceDatabase removes a D1 database by name.
// Idempotent: deleting a non-existent database returns nil.
func (p *D1Provider) DeleteWorkspaceDatabase(ctx context.Context, creds *cloud.Credentials, dbName string) error {
	if creds == nil {
		return errors.New("d1: DeleteWorkspaceDatabase: nil credentials")
	}
	if creds.Token == "" {
		return errors.New("d1: DeleteWorkspaceDatabase: empty token")
	}
	if creds.OrgID == "" {
		return errors.New("d1: DeleteWorkspaceDatabase: empty account ID")
	}

	// Find the database by name.
	savedToken := p.apiToken
	savedAccount := p.accountID
	p.apiToken = creds.Token
	p.accountID = creds.OrgID
	defer func() {
		p.apiToken = savedToken
		p.accountID = savedAccount
	}()

	db, err := p.findDatabaseByName(ctx, dbName)
	if err != nil {
		return err
	}
	if db == nil {
		return nil // Not found — idempotent.
	}

	endpoint := cfAPIBase + "/accounts/" + p.accountID + "/d1/database/" + db.UUID
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var apiResp cfAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return err
	}
	if !apiResp.Success {
		// If the database doesn't exist, treat as success.
		for _, e := range apiResp.Errors {
			if strings.Contains(strings.ToLower(e.Message), "not found") {
				return nil
			}
		}
		return cfAPIErrors(apiResp.Errors)
	}

	_ = audit.LogCloudOp("delete_database", true, dbName, nil)
	return nil
}

// ---------------------------------------------------------------------------
// Pattern substitution (mirrors Turso/Supabase applyDBNamePattern).
// ---------------------------------------------------------------------------

func applyD1DBNamePattern(pattern, workspace string) (string, error) {
	if !strings.Contains(pattern, "{workspace}") {
		return "", fmt.Errorf("invalid DBNamePattern %q: must contain {workspace} placeholder", pattern)
	}
	if workspace == "" {
		return "", errors.New("workspace name must not be empty")
	}

	name := strings.ReplaceAll(pattern, "{workspace}", workspace)
	if len(name) > d1DBNameMaxLen {
		return "", fmt.Errorf("resolved database name %q exceeds %d characters (got %d)",
			name, d1DBNameMaxLen, len(name))
	}
	return name, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// cfAPIErrors formats Cloudflare API error messages.
func cfAPIErrors(errs []cfAPIError) error {
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = fmt.Sprintf("[%d] %s", e.Code, e.Message)
	}
	return errors.New("d1: Cloudflare API: " + strings.Join(msgs, "; "))
}

// parseCFTime parses a Cloudflare timestamp string.
func parseCFTime(s string) time.Time {
	if s == "" {
		return time.Now().UTC()
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Now().UTC()
	}
	return t
}

// ---------------------------------------------------------------------------
// D1 Query API — low-level query execution.
// ---------------------------------------------------------------------------

// execD1Query executes a SQL statement on the D1 database via the
// Cloudflare REST API and ignores the result rows (DDL/DML).
func (p *D1Provider) execD1Query(ctx context.Context, sql string) error {
	body := map[string]string{"sql": sql}
	bodyBytes, _ := json.Marshal(body)

	endpoint := cfAPIBase + "/accounts/" + p.accountID + "/d1/database/" + p.databaseID + "/query"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bodyBytes2, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var queryResp cfD1QueryResult
	if err := json.Unmarshal(bodyBytes2, &queryResp); err != nil {
		return fmt.Errorf("d1 query: decode: %w (HTTP %d)", err, resp.StatusCode)
	}
	if !queryResp.Success {
		return cfAPIErrors(queryResp.Errors)
	}
	if len(queryResp.Result) > 0 && !queryResp.Result[0].Success {
		return errors.New("d1 query: statement failed")
	}
	return nil
}

// queryD1Rows executes a SELECT query and returns the result rows.
func (p *D1Provider) queryD1Rows(ctx context.Context, sql string) ([]map[string]interface{}, error) {
	body := map[string]string{"sql": sql}
	bodyBytes, _ := json.Marshal(body)

	endpoint := cfAPIBase + "/accounts/" + p.accountID + "/d1/database/" + p.databaseID + "/query"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var queryResp cfD1QueryResult
	if err := json.NewDecoder(resp.Body).Decode(&queryResp); err != nil {
		return nil, fmt.Errorf("d1 query: decode: %w", err)
	}
	if !queryResp.Success {
		return nil, cfAPIErrors(queryResp.Errors)
	}
	if len(queryResp.Result) == 0 {
		return nil, nil
	}
	return queryResp.Result[0].Results, nil
}

// countD1Rows returns the total number of rows in a table.
func (p *D1Provider) countD1Rows(ctx context.Context, table string) (int64, error) {
	rows, err := p.queryD1Rows(ctx, "SELECT COUNT(*) AS cnt FROM "+table)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	cnt, _ := rows[0]["cnt"].(float64)
	return int64(cnt), nil
}
