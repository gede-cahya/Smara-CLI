package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/mcp"
	"github.com/gede-cahya/Smara-CLI/internal/ui"
)

var (
	mcpType    string
	mcpCommand string
	mcpArgs    []string
	mcpURL     string
	mcpHeaders []string
	mcpEnv     []string
	mcpEnabled bool
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Kelola MCP servers",
	Long:  `Menambah, menghapus, melihat, atau menjalankan MCP servers.`,
}

var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Jalankan Smara sebagai MCP server (stdio)",
	Long: `Menjalankan Smara CLI sebagai Model Context Protocol (MCP) server
menggunakan stdio transport, sehingga Windsurf, Cursor, atau editor lain
dapat menggunakan tools Smara (run_command, view_file, edit_file, dll).`,
	RunE: runMCP,
}

var mcpAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Tambah dan hubungkan MCP server",
	Args:  cobra.ExactArgs(1),
	RunE:  runMCPAdd,
}

var mcpRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Hapus MCP server dari config",
	Args:  cobra.ExactArgs(1),
	RunE:  runMCPRemove,
}

var mcpListCmd = &cobra.Command{
	Use:   "list",
	Short: "Daftar MCP servers dari config",
	RunE:  runMCPList,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
	mcpCmd.AddCommand(mcpServeCmd)
	mcpCmd.AddCommand(mcpAddCmd)
	mcpCmd.AddCommand(mcpRemoveCmd)
	mcpCmd.AddCommand(mcpListCmd)

	mcpAddCmd.Flags().StringVar(&mcpType, "type", "local", "Tipe koneksi: local atau remote")
	mcpAddCmd.Flags().StringVar(&mcpCommand, "command", "", "Perintah untuk menjalankan MCP server (wajib untuk type=local)")
	mcpAddCmd.Flags().StringSliceVar(&mcpArgs, "args", []string{}, "Argument untuk perintah (opsional, comma-separated)")
	mcpAddCmd.Flags().StringVar(&mcpURL, "url", "", "URL endpoint untuk remote MCP (wajib untuk type=remote)")
	mcpAddCmd.Flags().StringSliceVar(&mcpHeaders, "headers", []string{}, "HTTP headers untuk remote MCP (format: key=value, comma-separated)")
	mcpAddCmd.Flags().StringSliceVar(&mcpEnv, "env", []string{}, "Environment variables (format: key=value, comma-separated)")
	mcpAddCmd.Flags().BoolVar(&mcpEnabled, "enabled", true, "Aktifkan server setelah ditambahkan")
}

func runMCP(cmd *cobra.Command, args []string) error {
	// Initialize minimal state for tools that need DB
	cfg := config.Get()
	if cfg.DBPath != "" {
		db, err := sql.Open("sqlite3", cfg.DBPath)
		if err == nil {
			agent.BuiltinDB = db
			defer db.Close()
		}
	}

	reader := bufio.NewReader(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var req map[string]interface{}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}

		method, _ := req["method"].(string)
		idVal, hasID := req["id"]

		switch method {
		case "initialize":
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      idVal,
				"result": map[string]interface{}{
					"protocolVersion": "2024-11-05",
					"serverInfo": map[string]interface{}{
						"name":    "smara-mcp",
						"version": version,
					},
					"capabilities": map[string]interface{}{},
				},
			}
			encoder.Encode(resp)
			os.Stdout.Sync()

		case "tools/list":
			tools := agent.GetBuiltinTools()
			var mcpTools []map[string]interface{}
			for _, t := range tools {
				mcpTools = append(mcpTools, map[string]interface{}{
					"name":        t.Name,
					"description": t.Description,
					"inputSchema": t.Parameters,
				})
			}
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      idVal,
				"result": map[string]interface{}{
					"tools": mcpTools,
				},
			}
			encoder.Encode(resp)
			os.Stdout.Sync()

		case "tools/call":
			params, _ := req["params"].(map[string]interface{})
			name, _ := params["name"].(string)
			arguments, _ := params["arguments"].(map[string]interface{})

			result, err := agent.ExecuteBuiltinTool(name, arguments, nil)

			var content []map[string]interface{}
			if err != nil {
				content = append(content, map[string]interface{}{
					"type": "text",
					"text": fmt.Sprintf("Error: %v", err),
				})
			} else {
				content = append(content, map[string]interface{}{
					"type": "text",
					"text": result,
				})
			}

			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      idVal,
				"result": map[string]interface{}{
					"content": content,
					"isError": err != nil,
				},
			}
			encoder.Encode(resp)
			os.Stdout.Sync()

		case "notifications/initialized":
			// ignore

		default:
			if hasID {
				resp := map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      idVal,
					"error": map[string]interface{}{
						"code":    -32601,
						"message": "Method not found: " + method,
					},
				}
				encoder.Encode(resp)
				os.Stdout.Sync()
			}
		}
	}

	return nil
}

func runMCPAdd(cmd *cobra.Command, args []string) error {
	name := args[0]

	if mcpType != "local" && mcpType != "remote" {
		return fmt.Errorf("type harus 'local' atau 'remote'")
	}

	srv := config.MCPServer{
		Name:    name,
		Type:    mcpType,
		Enabled: mcpEnabled,
	}

	if mcpType == "local" {
		if mcpCommand == "" {
			return fmt.Errorf("--command wajib diisi untuk type=local")
		}
		srv.Command = mcpCommand
		srv.Args = mcpArgs
		srv.Env = parseKeyValueSlice(mcpEnv)
	} else {
		if mcpURL == "" {
			return fmt.Errorf("--url wajib diisi untuk type=remote")
		}
		srv.URL = mcpURL
		srv.Headers = parseKeyValueSlice(mcpHeaders)
	}

	// Try to connect immediately
	mcpCfg := mcp.MCPServerConfig{
		Name:    srv.Name,
		Type:    srv.Type,
		Command: srv.Command,
		Args:    srv.Args,
		URL:     srv.URL,
		Headers: srv.Headers,
		Env:     srv.Env,
		Enabled: srv.Enabled,
	}

	var client *mcp.Client
	var err error
	if mcpType == "remote" {
		client, err = mcp.NewRemoteClient(mcpCfg)
	} else {
		client, err = mcp.NewClient(mcpCfg)
	}
	if err != nil {
		ui.PrintError("Gagal menghubungkan MCP '%s': %v", name, err)
		return fmt.Errorf("gagal menghubungkan MCP: %w", err)
	}
	defer client.Close()

	tools, err := client.ListTools()
	if err != nil {
		ui.PrintWarning("MCP '%s' terhubung tapi gagal list tools: %v", name, err)
	} else {
		ui.PrintSuccess("MCP '%s' terhubung (%d tools)", name, len(tools))
	}

	if err := config.AddMCPServer(srv); err != nil {
		return fmt.Errorf("gagal menyimpan config: %w", err)
	}
	ui.PrintSuccess("MCP '%s' disimpan ke config", name)
	return nil
}

func runMCPRemove(cmd *cobra.Command, args []string) error {
	name := args[0]
	if err := config.RemoveMCPServer(name); err != nil {
		return fmt.Errorf("gagal menghapus MCP '%s': %w", name, err)
	}
	ui.PrintSuccess("MCP '%s' dihapus dari config", name)
	return nil
}

func runMCPList(cmd *cobra.Command, args []string) error {
	servers := config.ListMCPServers()
	if len(servers) == 0 {
		ui.PrintInfo("Belum ada MCP server tersimpan.")
		return nil
	}
	var lines []string
	for _, s := range servers {
		status := "disabled"
		if s.Enabled {
			status = "enabled"
		}
		if s.Type == "remote" {
			lines = append(lines, fmt.Sprintf("  - %s (%s, %s) → %s", s.Name, s.Type, status, s.URL))
		} else {
			lines = append(lines, fmt.Sprintf("  - %s (%s, %s) → %s %s", s.Name, s.Type, status, s.Command, strings.Join(s.Args, " ")))
		}
	}
	ui.PrintInfo("MCP Servers:\n%s", strings.Join(lines, "\n"))
	return nil
}

func parseKeyValueSlice(pairs []string) map[string]string {
	result := make(map[string]string)
	for _, p := range pairs {
		parts := strings.SplitN(p, "=", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return result
}
