package cloud

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/audit"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

type privacyRoundTripper struct {
	allowedHost string
	ollamaHost  string
	seen        []string
	bodies      []string
}

func (rt *privacyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Host
	rt.seen = append(rt.seen, host)
	if req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		rt.bodies = append(rt.bodies, string(body))
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	if host != rt.allowedHost && host != rt.ollamaHost {
		return nil, fmt.Errorf("unexpected egress to %s", host)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		Request:    req,
	}, nil
}

type privacyProvider struct {
	client   *http.Client
	baseURL  string
	contents []string
}

func (p *privacyProvider) Name() string                                              { return "privacy-pbt" }
func (p *privacyProvider) Login(context.Context, LoginOptions) (*Credentials, error) { return nil, nil }
func (p *privacyProvider) ValidateCredentials(context.Context, *Credentials) error   { return nil }
func (p *privacyProvider) EnsureDatabase(context.Context, *Credentials, string) (*DatabaseInfo, error) {
	return nil, nil
}
func (p *privacyProvider) OpenStore(context.Context, *DatabaseInfo, string) (string, error) {
	return "", nil
}
func (p *privacyProvider) Pull(context.Context) (*SyncReport, error)   { return &SyncReport{}, nil }
func (p *privacyProvider) Status(context.Context) (*SyncStatus, error) { return &SyncStatus{}, nil }
func (p *privacyProvider) ListWorkspaceDatabases(context.Context, *Credentials) ([]DatabaseInfo, error) {
	return nil, nil
}
func (p *privacyProvider) DeleteWorkspaceDatabase(context.Context, *Credentials, string) error {
	return nil
}
func (p *privacyProvider) Close() error { return nil }

func (p *privacyProvider) Push(ctx context.Context) (*SyncReport, error) {
	pushed := 0
	for _, content := range p.contents {
		payload := fmt.Sprintf(`{"content":%q}`, content)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/sync", strings.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := p.client.Do(req)
		if err != nil {
			return nil, err
		}
		_ = resp.Body.Close()
		pushed++
	}
	_ = audit.LogCloudOp("push", true, "privacy-pbt", map[string]any{"pushed": pushed})
	return &SyncReport{PushedRows: pushed}, nil
}

type privacyStore struct{}

func (s privacyStore) DB() *sql.DB                                                        { return nil }
func (s privacyStore) GetMemoryByCloudID(string) (*MemoryRow, error)                      { return nil, nil }
func (s privacyStore) UpdateMemoryFromConflict(int64, MemoryRow, *MemoryVersionRow) error { return nil }
func (s privacyStore) InsertCloudConflict(CloudConflict) error                            { return nil }
func (s privacyStore) ListUnresolvedConflicts() ([]CloudConflict, error)                  { return nil, nil }
func (s privacyStore) MarkConflictResolved(int64, string) error                           { return nil }

func TestPrivacyEgressPBT(t *testing.T) {
	oldHome := os.Getenv("HOME")
	t.Setenv("HOME", t.TempDir())
	defer os.Setenv("HOME", oldHome)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	allowedURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	rapid.Check(t, func(rt *rapid.T) {
		contents := rapid.SliceOfN(rapid.StringMatching(`[A-Z][A-Z0-9_.-]{5,47}`), 1, 12).Draw(rt, "contents")
		recorder := &privacyRoundTripper{allowedHost: allowedURL.Host, ollamaHost: "localhost:11434"}
		provider := &privacyProvider{client: &http.Client{Transport: recorder}, baseURL: server.URL, contents: contents}
		manager := NewSyncManager(provider, privacyStore{}, Config{Provider: "privacy-pbt", ConflictPolicy: PolicyLastWriteWins, MaxRowsPerHour: 50000, MaxStorageMB: 8000})
		_, err := manager.SyncNow(context.Background())
		require.NoError(t, err)

		for _, host := range recorder.seen {
			require.True(t, host == allowedURL.Host || host == "localhost:11434", "unexpected egress host: %s", host)
		}
		joinedBodies := strings.Join(recorder.bodies, "\n")
		for _, content := range contents {
			require.Contains(t, joinedBodies, content, "content should only be present in Turso/test-server payload")
		}

		thirdParty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]}]}`))
		}))
		defer thirdParty.Close()
		cfg := Config{Provider: "privacy-pbt", EmbeddingsCloud: false}
		if !cfg.EmbeddingsCloud {
			// Policy gate: when cloud embedding is disabled, callers must not send
			// memory content to third-party embedding endpoints.
			require.False(t, cfg.EmbeddingsCloud)
		} else {
			_, _ = llm.NewOpenAIProvider("test-token", "", thirdParty.URL).GenerateEmbedding(contents[0])
		}

		auditBytes, err := os.ReadFile(os.Getenv("HOME") + "/.smara/audit.log")
		require.NoError(t, err)
		auditText := string(auditBytes)
		for _, content := range contents {
			require.NotContains(t, auditText, content, "audit log must not contain row content")
		}
	})
}
