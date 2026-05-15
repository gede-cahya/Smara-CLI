package agent

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/llm"
	smarassh "github.com/gede-cahya/Smara-CLI/internal/ssh"
	"github.com/stretchr/testify/require"
)

func TestPrintSSHPromptForReview(t *testing.T) {
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

	modeInfo := GetModeInfo(ModeRush)
	sysPrompt := modeInfo.SystemPrompt
	if hostCtx, err := smarassh.AllHosts(); err == nil && hostCtx != "(tidak ada host SSH tersimpan)" {
		sysPrompt += "\n\nHost VPS/Server yang tersimpan (gunakan saat user menyebut vps/server/remote):\n" + hostCtx
	}

	tools := GetBuiltinTools()
	var toolDescs []string
	for _, tool := range tools {
		toolDescs = append(toolDescs, fmt.Sprintf("- %s: %s", tool.Name, tool.Description))
	}
	toolsDesc := "Tools yang tersedia (gunakan via function calling):\n" + strings.Join(toolDescs, "\n")

	userPrompt := "cek status vps ubuntu dengan ssh"

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: sysPrompt},
		{Role: llm.RoleSystem, Content: toolsDesc},
		{Role: llm.RoleUser, Content: userPrompt},
	}

	var sb strings.Builder
	sb.WriteString("=== PROMPT REVIEW: SSH Context ===\n\n")
	for i, msg := range messages {
		sb.WriteString(fmt.Sprintf("--- Message %d [%s] ---\n", i, msg.Role))
		sb.WriteString(msg.Content)
		sb.WriteString("\n\n")
	}

	output := sb.String()
	t.Log(output)

	assertFile := "/tmp/smara_ssh_prompt_debug.txt"
	require.NoError(t, os.WriteFile(assertFile, []byte(output), 0644))
	t.Logf("Prompt juga disimpan ke %s", assertFile)

	require.Contains(t, output, "vps-cahya")
	require.Contains(t, output, "129.226.222.242")
	require.Contains(t, output, "ubuntu")
	require.Contains(t, output, "ssh_exec")
	require.Contains(t, output, "ssh_view_file")
	require.Contains(t, output, "ssh_list_dir")
	require.Contains(t, output, "ssh_manage")
}
