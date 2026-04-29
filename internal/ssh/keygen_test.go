package ssh

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateKeyPair_ED25519(t *testing.T) {
	setupTestSSHHome(t)

	pub, priv, err := GenerateKeyPair("test-ed25519", "ed25519", 0)
	require.NoError(t, err)
	assert.FileExists(t, pub)
	assert.FileExists(t, priv)

	privData, err := os.ReadFile(priv)
	require.NoError(t, err)
	assert.Contains(t, string(privData), "PRIVATE KEY")

	pubData, err := os.ReadFile(pub)
	require.NoError(t, err)
	assert.NotEmpty(t, pubData)
}

func TestGenerateKeyPair_RSA(t *testing.T) {
	setupTestSSHHome(t)

	pub, priv, err := GenerateKeyPair("test-rsa", "rsa", 2048)
	require.NoError(t, err)
	assert.FileExists(t, pub)
	assert.FileExists(t, priv)

	privData, err := os.ReadFile(priv)
	require.NoError(t, err)
	assert.Contains(t, string(privData), "RSA PRIVATE KEY")
}

func TestGenerateKeyPair_InvalidType(t *testing.T) {
	setupTestSSHHome(t)

	_, _, err := GenerateKeyPair("test", "invalid", 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tidak didukung")
}
