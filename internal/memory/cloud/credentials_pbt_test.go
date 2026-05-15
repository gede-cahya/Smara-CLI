package cloud

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"pgregory.net/rapid"
)

const tokenAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"

func rapidToken(t *rapid.T) string {
	return rapid.StringOfN(rapid.SampledFrom([]rune(tokenAlphabet)), 32, 96, -1).Draw(t, "token")
}

func captureStdoutStderr(t *testing.T, fn func() error) (string, string, error) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, err := os.Pipe()
	require.NoError(t, err)
	rErr, wErr, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout, os.Stderr = wOut, wErr
	err = fn()
	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	var outBuf, errBuf bytes.Buffer
	_, _ = io.Copy(&outBuf, rOut)
	_, _ = io.Copy(&errBuf, rErr)
	_ = rOut.Close()
	_ = rErr.Close()
	return outBuf.String(), errBuf.String(), err
}

func TestCredentialConfidentialityPBT(t *testing.T) {
	t.Setenv(envVarToken, "")
	rapid.Check(t, func(rt *rapid.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		token := rapidToken(rt)
		creds := &Credentials{Provider: "turso", Token: token, RefreshToken: token + "r", Email: "user@example.test", OrgID: "org-test", Region: "sea"}
		store := newFileStore()
		stdout, stderr, err := captureStdoutStderr(t, func() error {
			require.NoError(t, store.Save(creds))
			loaded, err := store.Load()
			require.NoError(t, err)
			require.Equal(t, creds.Token, loaded.Token)
			fmt.Fprint(os.Stdout, loaded)
			enc, err := json.Marshal(loaded)
			require.NoError(t, err)
			fmt.Fprint(os.Stderr, string(enc))
			return nil
		})
		require.NoError(t, err)
		require.NotContains(t, stdout, token)
		require.NotContains(t, stderr, token)
		yamlBytes, err := yaml.Marshal(struct {
			Provider string `yaml:"provider"`
			Token    string `yaml:"token"`
		}{Provider: creds.Provider, Token: creds.String()})
		require.NoError(t, err)
		require.NotContains(t, string(yamlBytes), token)
		require.Contains(t, string(yamlBytes), "[REDACTED]")
		info, err := os.Stat(filepath.Join(tmp, fileFallbackDir, fileFallbackName))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		require.NoError(t, store.Delete())
		_, err = store.Load()
		require.True(t, errors.Is(err, ErrNoCredentials), "got %v", err)
	})
}

func TestEnvModeLoadsCredentialsAndRejectsEmptyTokenPBT(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		token := rapidToken(rt)
		org := "org-" + strings.ToLower(rapid.StringMatching(`[a-z0-9]{1,12}`).Draw(rt, "org"))
		region := rapid.SampledFrom([]string{"sea", "sjc", "ams", "iad"}).Draw(rt, "region")
		provider := rapid.SampledFrom([]string{"turso", "supabase", "d1"}).Draw(rt, "provider")
		t.Setenv(envVarToken, token)
		t.Setenv(envVarOrg, org)
		t.Setenv(envVarRegion, region)
		t.Setenv(envVarProvider, provider)
		store := NewCredentialStore()
		creds, err := store.Load()
		require.NoError(t, err)
		require.Equal(t, "env", store.Source())
		require.Equal(t, provider, creds.Provider)
		require.Equal(t, token, creds.Token)
		require.Equal(t, org, creds.OrgID)
		require.Equal(t, region, creds.Region)
		t.Setenv(envVarToken, "")
		_, err = LoadHeadlessOrError()
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrNoCredentials), "got %v", err)
		require.Contains(t, err.Error(), envVarToken)
	})
}
