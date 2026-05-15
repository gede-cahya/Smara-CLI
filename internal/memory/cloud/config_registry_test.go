package cloud_test

import (
	"context"
	"strings"
	"testing"

	cloud "github.com/gede-cahya/Smara-CLI/internal/memory/cloud"
)

type fakeProvider struct{ name string }

func (p fakeProvider) Name() string { return p.name }
func (p fakeProvider) Login(context.Context, cloud.LoginOptions) (*cloud.Credentials, error) {
	return nil, nil
}
func (p fakeProvider) ValidateCredentials(context.Context, *cloud.Credentials) error { return nil }
func (p fakeProvider) EnsureDatabase(context.Context, *cloud.Credentials, string) (*cloud.DatabaseInfo, error) {
	return nil, nil
}
func (p fakeProvider) OpenStore(context.Context, *cloud.DatabaseInfo, string) (string, error) {
	return "", nil
}
func (p fakeProvider) Push(context.Context) (*cloud.SyncReport, error)   { return nil, nil }
func (p fakeProvider) Pull(context.Context) (*cloud.SyncReport, error)   { return nil, nil }
func (p fakeProvider) Status(context.Context) (*cloud.SyncStatus, error) { return nil, nil }
func (p fakeProvider) ListWorkspaceDatabases(context.Context, *cloud.Credentials) ([]cloud.DatabaseInfo, error) {
	return nil, nil
}
func (p fakeProvider) DeleteWorkspaceDatabase(context.Context, *cloud.Credentials, string) error {
	return nil
}
func (p fakeProvider) Close() error { return nil }

func validConfig() cloud.Config {
	cloud.Register("fake-test", func() cloud.Provider { return fakeProvider{name: "fake-test"} })
	return cloud.Config{
		Provider:        "fake-test",
		DBNamePattern:   "smara-{workspace}",
		SyncIntervalSec: 30,
		ConflictPolicy:  cloud.PolicyLastWriteWins,
		MaxRowsPerHour:  100,
	}
}

func TestConfigValidate(t *testing.T) {
	base := validConfig()
	if err := base.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	tests := []struct {
		name string
		mut  func(*cloud.Config)
		want string
	}{
		{"unknown provider lists registered", func(c *cloud.Config) { c.Provider = "missing-provider" }, "fake-test"},
		{"negative interval", func(c *cloud.Config) { c.SyncIntervalSec = -1 }, "SyncIntervalSec"},
		{"missing workspace placeholder", func(c *cloud.Config) { c.DBNamePattern = "smara" }, "{workspace}"},
		{"bad conflict policy lists accepted", func(c *cloud.Config) { c.ConflictPolicy = "bad" }, "archive-loser"},
		{"rows too low", func(c *cloud.Config) { c.MaxRowsPerHour = 99 }, "MaxRowsPerHour"},
		{"encrypt needs key", func(c *cloud.Config) { c.EncryptAtRest = true }, "EncryptionKey"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			tt.mut(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.want)
			}
		})
	}
}

func TestProviderRegistry(t *testing.T) {
	cloud.Register("fake-registry", func() cloud.Provider { return fakeProvider{name: "fake-registry"} })
	p, err := cloud.Get("fake-registry")
	if err != nil {
		t.Fatalf("Get registered provider: %v", err)
	}
	if p.Name() != "fake-registry" {
		t.Fatalf("provider name = %q", p.Name())
	}
	_, err = cloud.Get("definitely-missing")
	if err == nil || !strings.Contains(err.Error(), "fake-registry") {
		t.Fatalf("unknown provider error should mention registered providers, got %v", err)
	}
}
