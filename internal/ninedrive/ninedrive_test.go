package ninedrive

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The one rule that matters: filesMeta must arrive before the file part,
// carry the real sizeBytes, and the Bearer token must be set.
func TestUploadFile_FieldOrderAndAuth(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(fp, []byte("hello world!"), 0o644); err != nil {
		t.Fatal(err)
	}

	var gotBody, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "9d_test_key")
	if _, err := c.UploadFile(context.Background(), fp, "text/plain"); err != nil {
		t.Fatalf("upload: %v", err)
	}

	if gotAuth != "Bearer 9d_test_key" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	metaIdx := strings.Index(gotBody, "filesMeta")
	fileIdx := strings.Index(gotBody, `name="file-0"`)
	if metaIdx < 0 || fileIdx < 0 || metaIdx > fileIdx {
		t.Fatalf("filesMeta must precede file-0 (meta=%d file=%d)", metaIdx, fileIdx)
	}
	if !strings.Contains(gotBody, `"sizeBytes":"12"`) {
		t.Fatalf("sizeBytes not 12 in body:\n%s", gotBody)
	}
}
