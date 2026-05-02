package repair

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/mcp"
)

// CheckMCPHealth tests connectivity for all configured MCP servers.
func CheckMCPHealth() []CheckResult {
	var results []CheckResult

	servers := config.ListMCPServers()
	if len(servers) == 0 {
		results = append(results, CheckResult{
			Module:  "mcp",
			Status:  StatusOK,
			Message: "Tidak ada MCP server yang dikonfigurasi",
			Fixable: false,
		})
		return results
	}

	for _, srv := range servers {
		if !srv.Enabled {
			continue
		}
		res := checkSingleMCP(srv)
		results = append(results, res)
	}

	return results
}

func checkSingleMCP(srv config.MCPServer) CheckResult {
	res := CheckResult{
		Module:  "mcp",
		Status:  StatusOK,
		Message: fmt.Sprintf("MCP '%s' OK", srv.Name),
		Fixable: true,
	}

	cfg := mcp.MCPServerConfig{
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

	switch srv.Type {
	case "remote":
		client, err = mcp.NewRemoteClient(cfg)
	default:
		client, err = mcp.NewClient(cfg)
	}

	if err != nil {
		res.Status = StatusFail
		res.Message = fmt.Sprintf("MCP '%s' gagal connect: %v", srv.Name, err)
		res.Suggestion = "Periksa command/URL dan restart server"
		return res
	}
	defer client.Close()

	return res
}

// RepairMCP attempts to reconnect all configured MCP servers.
// Returns per-server results.
func RepairMCP() []CheckResult {
	return CheckMCPHealth()
}

// QuickHealthCheck does a lightweight HTTP GET for remote MCP URLs.
func QuickHealthCheck(url string, headers map[string]string) error {
	if url == "" {
		return fmt.Errorf("URL kosong")
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}
