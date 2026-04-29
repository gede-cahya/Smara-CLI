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
)

var mcpCmd = &cobra.Command{
	Use:    "mcp",
	Short:  "Jalankan Smara sebagai MCP server (stdio)",
	Long: `Menjalankan Smara CLI sebagai Model Context Protocol (MCP) server
menggunakan stdio transport, sehingga Windsurf, Cursor, atau editor lain
dapat menggunakan tools Smara (run_command, view_file, edit_file, dll).`,
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
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
