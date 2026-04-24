package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HTTPTransport manages JSON-RPC communication over HTTP for remote MCP servers.
// Supports both standard JSON responses and SSE (Server-Sent Events) streams
// as per MCP Streamable HTTP transport specification.
type HTTPTransport struct {
	url     string
	headers map[string]string
	client  *http.Client
	mu      sync.Mutex
	respCh  chan *Response
}

// NewHTTPTransport creates a new HTTP-based transport.
func NewHTTPTransport(url string, headers map[string]string) (*HTTPTransport, error) {
	return &HTTPTransport{
		url:     url,
		headers: headers,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
		respCh: make(chan *Response, 10),
	}, nil
}

// Send writes a JSON-RPC request via HTTP POST and queues the response.
func (t *HTTPTransport) Send(req *Request) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("gagal marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", t.url, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("gagal membuat HTTP request: %w", err)
	}

	// Required headers for MCP Streamable HTTP transport
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	// Add custom headers (API keys, auth, etc.)
	for k, v := range t.headers {
		httpReq.Header.Set(k, v)
	}

	httpResp, err := t.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("gagal mengirim HTTP request: %w", err)
	}

	contentType := httpResp.Header.Get("Content-Type")

	// Handle based on response content type
	if strings.Contains(contentType, "text/event-stream") {
		// SSE response — parse Server-Sent Events
		go t.handleSSE(httpResp)
		return nil
	}

	// Standard JSON response
	defer httpResp.Body.Close()
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return fmt.Errorf("gagal membaca response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP error %d: %s", httpResp.StatusCode, string(body))
	}

	var resp Response
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("gagal parse JSON-RPC response: %w", err)
	}

	t.respCh <- &resp
	return nil
}

// handleSSE parses a Server-Sent Events stream for JSON-RPC responses.
func (t *HTTPTransport) handleSSE(httpResp *http.Response) {
	defer httpResp.Body.Close()

	scanner := bufio.NewScanner(httpResp.Body)
	var dataBuffer strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "data: ") {
			dataBuffer.WriteString(strings.TrimPrefix(line, "data: "))
		} else if line == "" && dataBuffer.Len() > 0 {
			// Empty line = end of SSE event, process accumulated data
			rawData := dataBuffer.String()
			dataBuffer.Reset()

			var resp Response
			if err := json.Unmarshal([]byte(rawData), &resp); err == nil {
				t.respCh <- &resp
			}
		}
	}

	// Handle any remaining data
	if dataBuffer.Len() > 0 {
		var resp Response
		if err := json.Unmarshal([]byte(dataBuffer.String()), &resp); err == nil {
			t.respCh <- &resp
		}
	}
}

// Receive reads a queued JSON-RPC response.
func (t *HTTPTransport) Receive() (*Response, error) {
	select {
	case resp, ok := <-t.respCh:
		if !ok {
			return nil, fmt.Errorf("transport closed")
		}
		return resp, nil
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("timeout menunggu response")
	}
}

// Close cleans up the HTTP transport.
func (t *HTTPTransport) Close() error {
	// Don't close respCh here to avoid panic from pending SSE goroutines
	return nil
}
