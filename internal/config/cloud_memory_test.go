package config_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"gopkg.in/yaml.v3"
)

func TestDefaultConfigCloudMemorySecretFreeRoundTrip(t *testing.T) {
	cfg := config.DefaultConfig()
	body, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal default config: %v", err)
	}
	lower := strings.ToLower(string(body))
	for _, forbidden := range []string{"cloud_memory:\n  token", "auth_token", "bearer"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("yaml contains forbidden secret marker %q:\n%s", forbidden, body)
		}
	}
	for _, table := range cfg.CloudMemory.SyncTables {
		if strings.Contains(strings.ToLower(table), "audit.log") {
			t.Fatalf("audit log must not be in cloud sync tables: %#v", cfg.CloudMemory.SyncTables)
		}
	}

	var decoded config.SmaraConfig
	if err := yaml.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal default config: %v", err)
	}
	if !reflect.DeepEqual(cfg.CloudMemory, decoded.CloudMemory) {
		t.Fatalf("cloud memory round-trip mismatch\nwant: %#v\n got: %#v", cfg.CloudMemory, decoded.CloudMemory)
	}
}
