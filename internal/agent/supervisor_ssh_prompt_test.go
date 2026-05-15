package agent

import (
	"testing"

	smarassh "github.com/gede-cahya/Smara-CLI/internal/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModePrompts_ContainSSHInstructions(t *testing.T) {
	modes := AllModes()
	require.Len(t, modes, 5)

	for _, m := range modes {
		if m.Name == ModeTest || m.Name == ModeWorkflow {
			continue
		}
		assert.Contains(t, m.SystemPrompt, "SSH", "mode %s should mention SSH", m.Name)
		assert.Contains(t, m.SystemPrompt, "ssh_exec", "mode %s should mention ssh_exec tool", m.Name)
		assert.Contains(t, m.SystemPrompt, "vps", "mode %s should mention vps keyword", m.Name)
	}
}

func TestBuiltinTools_ContainSSHTools(t *testing.T) {
	tools := GetBuiltinTools()
	require.NotEmpty(t, tools)

	hasSSHExec := false
	hasSSHView := false
	hasSSHList := false
	hasSSHManage := false

	for _, tool := range tools {
		switch tool.Name {
		case "ssh_exec":
			hasSSHExec = true
			assert.Contains(t, tool.Description, "SSH")
			params := tool.Parameters["properties"].(map[string]interface{})
			assert.Contains(t, params, "host")
			assert.Contains(t, params, "command")
		case "ssh_view_file":
			hasSSHView = true
			assert.Contains(t, tool.Description, "SSH")
			params := tool.Parameters["properties"].(map[string]interface{})
			assert.Contains(t, params, "host")
			assert.Contains(t, params, "path")
		case "ssh_list_dir":
			hasSSHList = true
			assert.Contains(t, tool.Description, "SSH")
			params := tool.Parameters["properties"].(map[string]interface{})
			assert.Contains(t, params, "host")
		case "ssh_manage":
			hasSSHManage = true
			assert.Contains(t, tool.Description, "SSH")
			params := tool.Parameters["properties"].(map[string]interface{})
			assert.Contains(t, params, "action")
		}
	}

	assert.True(t, hasSSHExec, "ssh_exec should be in builtin tools")
	assert.True(t, hasSSHView, "ssh_view_file should be in builtin tools")
	assert.True(t, hasSSHList, "ssh_list_dir should be in builtin tools")
	assert.True(t, hasSSHManage, "ssh_manage should be in builtin tools")
}

func TestSupervisor_ToolList_ContainsSSH(t *testing.T) {
	s := NewSupervisor(nil, nil)
	tools := s.ConvertMCPToolsToToolFunctions()
	require.NotEmpty(t, tools)

	hasSSH := false
	for _, tool := range tools {
		if tool.Name == "ssh_exec" || tool.Name == "ssh_view_file" ||
			tool.Name == "ssh_list_dir" || tool.Name == "ssh_manage" {
			hasSSH = true
		}
	}
	assert.True(t, hasSSH, "supervisor tool list should contain SSH tools")
}

func TestSupervisor_PromptWithHostContext(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	require.NoError(t, smarassh.EnsureDir())

	host := smarassh.Host{
		Name:    "vps-cahya",
		Address: "129.226.222.242",
		User:    "ubuntu",
		Port:    "22",
		KeyPath: "/home/cahya/Downloads/vpsCahya.pem",
	}
	require.NoError(t, smarassh.SaveHost(host))

	hostCtx, err := smarassh.AllHosts()
	require.NoError(t, err)
	assert.Contains(t, hostCtx, "vps-cahya")
	assert.Contains(t, hostCtx, "129.226.222.242")
	assert.Contains(t, hostCtx, "ubuntu")
}
