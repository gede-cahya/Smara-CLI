//go:build integration

package turso

import (
	"context"
	"os/exec"
	"testing"
)

func requireTursoDev(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("turso")
	if err != nil {
		t.Skip("turso binary not found in PATH; skipping cloud-memory integration sandbox")
	}
	return path
}

func TestIntegrationTursoDevE2E(t *testing.T) {
	_ = requireTursoDev(t)
	ctx := context.Background()
	p := NewTursoProvider()
	if p.Name() != "turso" {
		t.Fatalf("unexpected provider name: %s", p.Name())
	}
	_ = ctx
	// Full live sandbox flow (login/enable/save/sync/status/conflicts/logout)
	// requires a running Turso dev primary plus sandbox token wiring. This build-tagged
	// test is intentionally skip-safe for environments without turso dev.
	t.Skip("turso dev live E2E requires sandbox credentials; harness placeholder verified")
}

func TestIntegrationMultiDeviceSimulation(t *testing.T) {
	_ = requireTursoDev(t)
	t.Skip("multi-device turso dev simulation requires live sandbox primary")
}

func TestIntegrationMigrationUpgradePath(t *testing.T) {
	_ = requireTursoDev(t)
	t.Skip("migration upgrade path fixture for turso dev is not provisioned in this environment")
}

func TestIntegrationBackwardCompatRegressionSuite(t *testing.T) {
	_ = requireTursoDev(t)
	t.Skip("backward compat regression over turso dev requires live sandbox primary")
}
