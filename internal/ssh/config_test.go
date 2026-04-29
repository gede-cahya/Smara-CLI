package ssh

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestSSHHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func TestEnsureDir(t *testing.T) {
	setupTestSSHHome(t)
	require.NoError(t, EnsureDir())
}

func TestSaveAndLoadHosts(t *testing.T) {
	setupTestSSHHome(t)

	h1 := Host{Name: "web1", Address: "192.168.1.1", Port: "22", User: "root", KeyPath: "/tmp/key"}
	h2 := Host{Name: "web2", Address: "192.168.1.2", Port: "22", User: "admin"}

	require.NoError(t, SaveHost(h1))
	require.NoError(t, SaveHost(h2))

	hosts, err := LoadHosts()
	require.NoError(t, err)
	require.Len(t, hosts, 2)

	assert.Equal(t, "web1", hosts[0].Name)
	assert.Equal(t, "192.168.1.1", hosts[0].Address)
	assert.Equal(t, "root", hosts[0].User)
	assert.Equal(t, "/tmp/key", hosts[0].KeyPath)

	assert.Equal(t, "web2", hosts[1].Name)
	assert.Empty(t, hosts[1].KeyPath)
	assert.Equal(t, "admin", hosts[1].User)
}

func TestSaveHost_Update(t *testing.T) {
	setupTestSSHHome(t)

	h := Host{Name: "web1", Address: "1.1.1.1", Port: "22", User: "root"}
	require.NoError(t, SaveHost(h))

	h.Address = "2.2.2.2"
	require.NoError(t, SaveHost(h))

	hosts, err := LoadHosts()
	require.NoError(t, err)
	require.Len(t, hosts, 1)
	assert.Equal(t, "2.2.2.2", hosts[0].Address)
}

func TestGetHost(t *testing.T) {
	setupTestSSHHome(t)

	require.NoError(t, SaveHost(Host{Name: "web1", Address: "1.1.1.1", User: "root"}))

	found, err := GetHost("web1")
	require.NoError(t, err)
	assert.Equal(t, "1.1.1.1", found.Address)

	_, err = GetHost("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tidak ditemukan")
}

func TestRemoveHost(t *testing.T) {
	setupTestSSHHome(t)

	require.NoError(t, SaveHost(Host{Name: "web1", Address: "1.1.1.1", User: "root"}))
	require.NoError(t, SaveHost(Host{Name: "web2", Address: "2.2.2.2", User: "root"}))

	require.NoError(t, RemoveHost("web1"))

	hosts, err := LoadHosts()
	require.NoError(t, err)
	require.Len(t, hosts, 1)
	assert.Equal(t, "web2", hosts[0].Name)
}

func TestFindHost_Exact(t *testing.T) {
	setupTestSSHHome(t)

	require.NoError(t, SaveHost(Host{Name: "web1", Address: "1.1.1.1", User: "root"}))

	found, _, err := FindHost("web1")
	require.NoError(t, err)
	assert.Equal(t, "1.1.1.1", found.Address)
}

func TestFindHost_UserAtAddress(t *testing.T) {
	setupTestSSHHome(t)

	require.NoError(t, SaveHost(Host{Name: "web1", Address: "1.1.1.1", User: "root"}))

	found, _, err := FindHost("root@1.1.1.1")
	require.NoError(t, err)
	assert.Equal(t, "web1", found.Name)
}

func TestFindHost_Multiple(t *testing.T) {
	setupTestSSHHome(t)

	require.NoError(t, SaveHost(Host{Name: "web1", Address: "1.1.1.1", User: "root"}))
	require.NoError(t, SaveHost(Host{Name: "web2", Address: "2.2.2.2", User: "root"}))

	_, matches, err := FindHost("web")
	assert.Error(t, err)
	assert.Len(t, matches, 2)
}

func TestFindHost_NoMatch(t *testing.T) {
	setupTestSSHHome(t)

	require.NoError(t, SaveHost(Host{Name: "web1", Address: "1.1.1.1", User: "root"}))

	_, _, err := FindHost("xyz")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tidak ada host")
}

func TestAllHosts(t *testing.T) {
	setupTestSSHHome(t)

	result, err := AllHosts()
	require.NoError(t, err)
	assert.Equal(t, "(tidak ada host SSH tersimpan)", result)

	require.NoError(t, SaveHost(Host{Name: "web1", Address: "1.1.1.1", User: "root", Port: "22"}))

	result, err = AllHosts()
	require.NoError(t, err)
	assert.Contains(t, result, "web1: root@1.1.1.1:22")
}
