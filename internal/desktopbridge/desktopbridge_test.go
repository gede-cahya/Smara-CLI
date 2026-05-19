package desktopbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestServiceHealthAndEmergencyStop(t *testing.T) {
	dir := t.TempDir()
	svc := New(Options{AuditLog: dir + "/audit.jsonl"})
	ts := httptest.NewServer(svc.Handler())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out response
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil || !out.OK {
		t.Fatalf("bad health err=%v out=%+v", err, out)
	}

	res, err = http.Post(ts.URL+"/stop", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	res, err = http.Post(ts.URL+"/click", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 500 {
		t.Fatalf("expected stopped status 500, got %d", res.StatusCode)
	}
	if _, err := os.Stat(dir + "/audit.jsonl"); err != nil {
		t.Fatalf("audit not written: %v", err)
	}
}

func TestRunAllowedCommandBlocksUnknown(t *testing.T) {
	if _, err := RunAllowedCommand(context.Background(), []string{"echo"}, "rm", []string{"-rf", "/"}); err == nil {
		t.Fatalf("expected allowlist block")
	}
}
