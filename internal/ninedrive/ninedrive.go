// Package ninedrive is a minimal client for the 9drive uploads API.
//
// Protocol (from the server): POST {base}/api/v1/uploads as multipart/form-data,
// Bearer auth, and the `filesMeta` field (with sizeBytes) MUST be written
// before the file field — the server streams against the declared size and
// rejects a file part that arrives before its meta.
package ninedrive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Client talks to a 9drive instance.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// NewClient returns a Client. baseURL defaults to http://localhost:4000 if empty.
func NewClient(baseURL, apiKey string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	baseURL = strings.TrimSuffix(baseURL, "/v1")
	baseURL = strings.TrimSuffix(baseURL, "/api")
	if baseURL == "" {
		baseURL = "http://localhost:4000"
	}
	return &Client{BaseURL: baseURL, APIKey: apiKey, HTTP: &http.Client{Timeout: 120 * time.Second}}
}

type fileMeta struct {
	FieldName string `json:"fieldName"`
	FileName  string `json:"fileName"`
	MimeType  string `json:"mimeType"`
	SizeBytes string `json:"sizeBytes"`
}

// UploadResult is the parsed server response; raw is always preserved.
type UploadResult struct {
	Raw string
}

// UploadFile uploads a single local file. mimeType is optional ("" -> octet-stream).
func (c *Client) UploadFile(ctx context.Context, path, mimeType string) (*UploadResult, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("9drive: API key kosong")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	meta := []fileMeta{{
		FieldName: "file-0",
		FileName:  filepath.Base(path),
		MimeType:  mimeType,
		SizeBytes: fmt.Sprintf("%d", st.Size()),
	}}
	metaJSON, _ := json.Marshal(meta)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	// ponytail: meta BEFORE file — required field order, server streams by size.
	if err := mw.WriteField("filesMeta", string(metaJSON)); err != nil {
		return nil, err
	}
	part, err := mw.CreateFormFile("file-0", filepath.Base(path))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/v1/uploads", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &UploadResult{Raw: string(raw)}, fmt.Errorf("9drive: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return &UploadResult{Raw: string(raw)}, nil
}

type NineDriveFile struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	MimeType  string    `json:"mimeType"`
	SizeBytes string    `json:"sizeBytes"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

type ListFilesResponse struct {
	Files []NineDriveFile `json:"files"`
}

// ListFiles lists files in 9drive, optionally filtering by name (q) or folder ID.
func (c *Client) ListFiles(ctx context.Context, q, folderID string) ([]NineDriveFile, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("9drive: API key kosong")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/v1/files", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	qParams := req.URL.Query()
	if q != "" {
		qParams.Set("q", q)
	}
	if folderID != "" {
		qParams.Set("folderId", folderID)
	}
	req.URL.RawQuery = qParams.Encode()

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("9drive: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var res ListFilesResponse
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("9drive: failed to parse response: %w", err)
	}
	return res.Files, nil
}

// DownloadFile downloads a file by ID to the local destPath.
func (c *Client) DownloadFile(ctx context.Context, fileID, destPath string) error {
	if c.APIKey == "" {
		return fmt.Errorf("9drive: API key kosong")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/v1/files/"+fileID+"/download", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("9drive: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

