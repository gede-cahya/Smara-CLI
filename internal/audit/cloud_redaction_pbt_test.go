package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

const auditTokenAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"

func auditRapidToken(t *rapid.T) string {
	return rapid.StringOfN(rapid.SampledFrom([]rune(auditTokenAlphabet)), 32, 96, -1).Draw(t, "token")
}

func TestCloudAuditTokenRedactionPBT(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		token := auditRapidToken(rt)
		jwt := "eyJ" + strings.Repeat("A", 24) + "." + token[:16] + "." + token[16:]
		err := LogCloudOp("push", true, "db-token="+token, map[string]any{
			"header": "Authorization: Bearer " + token,
			"field":  "token=" + token,
			"jwt":    jwt,
		})
		require.NoError(t, err)
		body, err := os.ReadFile(filepath.Join(tmp, cloudAuditDirName, cloudAuditFileName))
		require.NoError(t, err)
		require.NotContains(t, string(body), token)
		require.NotContains(t, string(body), jwt)
		require.Contains(t, string(body), "[REDACTED]")
		lines := strings.Split(strings.TrimSpace(string(body)), "\n")
		require.NotEmpty(t, lines)
		require.Contains(t, lines[len(lines)-1], "push")
		for _, table := range config.DefaultConfig().CloudMemory.SyncTables {
			require.NotEqual(t, "audit.log", table)
			require.NotEqual(t, cloudAuditFileName, table)
		}
	})
}
