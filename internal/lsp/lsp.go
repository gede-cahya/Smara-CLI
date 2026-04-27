// Package lsp provides Language Server Protocol integration for Smara CLI.
// It enables code intelligence features: syntax checking, go-to-definition,
// and find-references before file modification.
package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Client represents an LSP client connection to a language server.
type Client struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	stderr   io.ReadCloser
	reqID    int
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	ready    bool
	language string
}

// ServerConfig holds configuration for starting a language server.
type ServerConfig struct {
	Command      string   `json:"command"`
	Args         []string `json:"args"`
	Language     string   `json:"language"`
	RootURI      string   `json:"root_uri"`
	WorkspaceDir string   `json:"workspace_dir"`
}

// Common language server configs.
var DefaultServers = map[string]ServerConfig{
	"go": {
		Command:  "gopls",
		Args:     []string{"serve", "-rpc.trace"},
		Language: "go",
	},
	"typescript": {
		Command:  "typescript-language-server",
		Args:     []string{"--stdio"},
		Language: "typescript",
	},
	"javascript": {
		Command:  "typescript-language-server",
		Args:     []string{"--stdio"},
		Language: "javascript",
	},
	"python": {
		Command:  "pylsp",
		Args:     []string{},
		Language: "python",
	},
	"rust": {
		Command:  "rust-analyzer",
		Args:     []string{},
		Language: "rust",
	},
}

// NewClient creates and starts an LSP client for a language.
func NewClient(lang, workspaceDir string) (*Client, error) {
	config, ok := DefaultServers[lang]
	if !ok {
		return nil, fmt.Errorf("tidak ada LSP server yang diketahui untuk bahasa: %s", lang)
	}

	config.WorkspaceDir = workspaceDir
	if config.RootURI == "" {
		absPath, _ := filepath.Abs(workspaceDir)
		config.RootURI = "file://" + absPath
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, config.Command, config.Args...)
	cmd.Dir = workspaceDir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("gagal membuat stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("gagal membuat stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("gagal membuat stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("gagal memulai LSP server '%s': %w", config.Command, err)
	}

	client := &Client{
		cmd:      cmd,
		stdin:    stdin,
		stdout:   stdout,
		stderr:   stderr,
		reqID:    0,
		ctx:      ctx,
		cancel:   cancel,
		language: lang,
	}

	// Initialize the LSP connection
	if err := client.initialize(config); err != nil {
		client.Close()
		return nil, fmt.Errorf("gagal inisialisasi LSP: %w", err)
	}

	client.ready = true
	return client, nil
}

// initialize sends the initialize request to the language server.
func (c *Client) initialize(config ServerConfig) error {
	params := map[string]interface{}{
		"processId": os.Getpid(),
		"rootUri":   config.RootURI,
		"capabilities": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"synchronization": map[string]interface{}{
					"dynamicRegistration": false,
					"willSave":            true,
					"willSaveWaitUntil":   true,
					"didSave":             true,
				},
				"completion": map[string]interface{}{
					"dynamicRegistration": false,
				},
				"hover": map[string]interface{}{
					"dynamicRegistration": false,
				},
				"definition": map[string]interface{}{
					"dynamicRegistration": false,
					"linkSupport":         true,
				},
				"references": map[string]interface{}{
					"dynamicRegistration": false,
				},
				"documentSymbol": map[string]interface{}{
					"dynamicRegistration": false,
				},
				"formatting": map[string]interface{}{
					"dynamicRegistration": false,
				},
				"codeAction": map[string]interface{}{
					"dynamicRegistration": false,
				},
				"diagnostic": map[string]interface{}{
					"dynamicRegistration": false,
				},
			},
		},
		"workspaceFolders": []map[string]string{
			{
				"uri":  config.RootURI,
				"name": filepath.Base(config.WorkspaceDir),
			},
		},
	}

	_, err := c.sendRequest("initialize", params)
	if err != nil {
		return err
	}

	// Send initialized notification
	return c.sendNotification("initialized", map[string]interface{}{})
}

// sendRequest sends a JSON-RPC request and returns the result.
func (c *Client) sendRequest(method string, params interface{}) (map[string]interface{}, error) {
	c.mu.Lock()
	c.reqID++
	id := c.reqID
	c.mu.Unlock()

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// Write header + content
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := c.stdin.Write([]byte(header)); err != nil {
		return nil, err
	}
	if _, err := c.stdin.Write(data); err != nil {
		return nil, err
	}

	// For simplicity, return empty result. Full implementation would parse responses.
	return map[string]interface{}{}, nil
}

// sendNotification sends a JSON-RPC notification (no response expected).
func (c *Client) sendNotification(method string, params interface{}) error {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := c.stdin.Write([]byte(header)); err != nil {
		return err
	}
	if _, err := c.stdin.Write(data); err != nil {
		return err
	}
	return nil
}

// DidOpen notifies the server that a document is open.
func (c *Client) DidOpen(uri, languageID, text string) error {
	if !c.ready {
		return fmt.Errorf("LSP client belum siap")
	}

	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        uri,
			"languageId": languageID,
			"version":    1,
			"text":       text,
		},
	}
	return c.sendNotification("textDocument/didOpen", params)
}

// DidChange notifies the server of document changes.
func (c *Client) DidChange(uri string, version int, text string) error {
	if !c.ready {
		return fmt.Errorf("LSP client belum siap")
	}

	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":     uri,
			"version": version,
		},
		"contentChanges": []map[string]interface{}{
			{"text": text},
		},
	}
	return c.sendNotification("textDocument/didChange", params)
}

// DidSave notifies the server that a document was saved.
func (c *Client) DidSave(uri string) error {
	if !c.ready {
		return fmt.Errorf("LSP client belum siap")
	}

	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri,
		},
	}
	return c.sendNotification("textDocument/didSave", params)
}

// Definition requests the definition location of a symbol.
func (c *Client) Definition(uri string, line, character int) ([]Location, error) {
	if !c.ready {
		return nil, fmt.Errorf("LSP client belum siap")
	}

	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri,
		},
		"position": map[string]interface{}{
			"line":      line,
			"character": character,
		},
	}

	resp, err := c.sendRequest("textDocument/definition", params)
	if err != nil {
		return nil, err
	}

	// Parse response into locations
	return parseLocations(resp), nil
}

// References requests all references to a symbol.
func (c *Client) References(uri string, line, character int) ([]Location, error) {
	if !c.ready {
		return nil, fmt.Errorf("LSP client belum siap")
	}

	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri,
		},
		"position": map[string]interface{}{
			"line":      line,
			"character": character,
		},
		"context": map[string]interface{}{
			"includeDeclaration": true,
		},
	}

	resp, err := c.sendRequest("textDocument/references", params)
	if err != nil {
		return nil, err
	}

	return parseLocations(resp), nil
}

// Hover requests hover information at a position.
func (c *Client) Hover(uri string, line, character int) (*HoverInfo, error) {
	if !c.ready {
		return nil, fmt.Errorf("LSP client belum siap")
	}

	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri,
		},
		"position": map[string]interface{}{
			"line":      line,
			"character": character,
		},
	}

	resp, err := c.sendRequest("textDocument/hover", params)
	if err != nil {
		return nil, err
	}

	return parseHover(resp), nil
}

// DocumentSymbol requests all symbols in a document.
func (c *Client) DocumentSymbol(uri string) ([]DocumentSymbol, error) {
	if !c.ready {
		return nil, fmt.Errorf("LSP client belum siap")
	}

	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri,
		},
	}

	resp, err := c.sendRequest("textDocument/documentSymbol", params)
	if err != nil {
		return nil, err
	}

	return parseDocumentSymbols(resp), nil
}

// Close shuts down the LSP client.
func (c *Client) Close() error {
	c.ready = false

	// Send shutdown request
	c.sendRequest("shutdown", map[string]interface{}{})
	c.sendNotification("exit", map[string]interface{}{})

	c.cancel()

	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}

	if c.stdin != nil {
		c.stdin.Close()
	}
	if c.stdout != nil {
		c.stdout.Close()
	}
	if c.stderr != nil {
		c.stderr.Close()
	}

	return nil
}

// IsReady returns whether the client is ready.
func (c *Client) IsReady() bool {
	return c.ready
}

// --- Response Types ---

// Location represents a location in a file.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// Range represents a range in a document.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Position represents a position in a document.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// HoverInfo represents hover information.
type HoverInfo struct {
	Contents string `json:"contents"`
	Range    *Range `json:"range,omitempty"`
}

// DocumentSymbol represents a symbol in a document.
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// --- Parsers ---

func parseLocations(resp map[string]interface{}) []Location {
	var locations []Location
	if result, ok := resp["result"]; ok {
		switch v := result.(type) {
		case []interface{}:
			for _, item := range v {
				if loc, ok := item.(map[string]interface{}); ok {
					locations = append(locations, parseLocation(loc))
				}
			}
		case map[string]interface{}:
			locations = append(locations, parseLocation(v))
		}
	}
	return locations
}

func parseLocation(m map[string]interface{}) Location {
	loc := Location{}
	if uri, ok := m["uri"].(string); ok {
		loc.URI = uri
	}
	if r, ok := m["range"].(map[string]interface{}); ok {
		loc.Range = parseRange(r)
	}
	return loc
}

func parseRange(m map[string]interface{}) Range {
	r := Range{}
	if start, ok := m["start"].(map[string]interface{}); ok {
		r.Start = parsePosition(start)
	}
	if end, ok := m["end"].(map[string]interface{}); ok {
		r.End = parsePosition(end)
	}
	return r
}

func parsePosition(m map[string]interface{}) Position {
	p := Position{}
	if line, ok := m["line"].(float64); ok {
		p.Line = int(line)
	}
	if char, ok := m["character"].(float64); ok {
		p.Character = int(char)
	}
	return p
}

func parseHover(resp map[string]interface{}) *HoverInfo {
	hover := &HoverInfo{}
	if result, ok := resp["result"].(map[string]interface{}); ok {
		if contents, ok := result["contents"]; ok {
			switch v := contents.(type) {
			case string:
				hover.Contents = v
			case map[string]interface{}:
				if value, ok := v["value"].(string); ok {
					hover.Contents = value
				}
			}
		}
	}
	return hover
}

func parseDocumentSymbols(resp map[string]interface{}) []DocumentSymbol {
	var symbols []DocumentSymbol
	if result, ok := resp["result"].([]interface{}); ok {
		for _, item := range result {
			if s, ok := item.(map[string]interface{}); ok {
				symbols = append(symbols, parseDocumentSymbol(s))
			}
		}
	}
	return symbols
}

func parseDocumentSymbol(m map[string]interface{}) DocumentSymbol {
	s := DocumentSymbol{}
	if name, ok := m["name"].(string); ok {
		s.Name = name
	}
	if detail, ok := m["detail"].(string); ok {
		s.Detail = detail
	}
	if kind, ok := m["kind"].(float64); ok {
		s.Kind = int(kind)
	}
	if r, ok := m["range"].(map[string]interface{}); ok {
		s.Range = parseRange(r)
	}
	return s
}

// --- Helpers ---

// DetectLanguage detects the programming language from a file path.
func DetectLanguage(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".c", ".h":
		return "c"
	case ".cpp", ".hpp", ".cc":
		return "cpp"
	default:
		return ""
	}
}

// FileToURI converts a file path to an LSP URI.
func FileToURI(path string) string {
	abs, _ := filepath.Abs(path)
	return "file://" + abs
}

// Manager manages multiple LSP clients for different languages.
type Manager struct {
	clients map[string]*Client
	mu      sync.RWMutex
}

// NewManager creates a new LSP manager.
func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]*Client),
	}
}

// GetOrCreateClient gets or creates an LSP client for a language.
func (m *Manager) GetOrCreateClient(lang, workspaceDir string) (*Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if client, ok := m.clients[lang]; ok && client.IsReady() {
		return client, nil
	}

	client, err := NewClient(lang, workspaceDir)
	if err != nil {
		return nil, err
	}

	m.clients[lang] = client
	return client, nil
}

// GetClient gets an existing client for a language.
func (m *Manager) GetClient(lang string) (*Client, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	client, ok := m.clients[lang]
	return client, ok
}

// CloseAll closes all managed LSP clients.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, client := range m.clients {
		client.Close()
	}
	m.clients = make(map[string]*Client)
}

// CloseClient closes a specific language client.
func (m *Manager) CloseClient(lang string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if client, ok := m.clients[lang]; ok {
		client.Close()
		delete(m.clients, lang)
	}
}
