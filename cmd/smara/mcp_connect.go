package main

import (
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/mcp"
	"github.com/gede-cahya/Smara-CLI/internal/ui"
)

const (
	mcpStartupPerServerTimeout = 15 * time.Second
	mcpStartupOverallTimeout   = 30 * time.Second
	mcpPreflightDialTimeout    = 2 * time.Second
)

type mcpConnResult struct {
	Name   string
	Client *mcp.Client
	Tools  []mcp.Tool
	Err    error
}

func connectMCPServersForStartup(supervisor *agent.Supervisor, enabledConfigs []mcp.MCPServerConfig) {
	if len(enabledConfigs) == 0 {
		return
	}

	ui.PrintInfo("Menghubungkan %d MCP server secara paralel...", len(enabledConfigs))

	results := make(chan mcpConnResult, len(enabledConfigs))
	pending := make(map[string]bool, len(enabledConfigs))
	var pendingMu sync.Mutex

	for _, mcpCfg := range enabledConfigs {
		pending[mcpCfg.Name] = true
		go connectMCPServerWithTimeout(mcpCfg, results)
	}

	overallTimer := time.NewTimer(mcpStartupOverallTimeout)
	defer overallTimer.Stop()

	remaining := len(enabledConfigs)
	for remaining > 0 {
		select {
		case res := <-results:
			remaining--
			pendingMu.Lock()
			delete(pending, res.Name)
			pendingMu.Unlock()
			registerMCPStartupResult(supervisor, res)
		case <-overallTimer.C:
			pendingMu.Lock()
			names := make([]string, 0, len(pending))
			for name := range pending {
				names = append(names, name)
			}
			pendingMu.Unlock()
			for _, name := range names {
				ui.PrintWarning("MCP '%s' dilewati: timeout startup setelah %s", name, mcpStartupOverallTimeout)
			}
			return
		}
	}
}

// preflightCheckMCP performs fast sanity checks before attempting a full
// MCP handshake. Returns a human-readable reason to skip, or "" if OK.
func preflightCheckMCP(cfg mcp.MCPServerConfig) string {
	switch cfg.Type {
	case "remote":
		return preflightRemote(cfg)
	default:
		return preflightLocal(cfg)
	}
}

// preflightRemote does a fast TCP dial to the remote URL's host:port.
// If the port is unreachable, we know the handshake will fail anyway.
func preflightRemote(cfg mcp.MCPServerConfig) string {
	if cfg.URL == "" {
		return "URL remote kosong"
	}
	parsed, err := url.Parse(cfg.URL)
	if err != nil {
		return fmt.Sprintf("URL tidak valid: %v", err)
	}
	host := parsed.Host
	if host == "" {
		return "host remote kosong"
	}
	// Ensure we have a port for the dial.
	if !strings.Contains(host, ":") {
		switch parsed.Scheme {
		case "https":
			host += ":443"
		default:
			host += ":80"
		}
	}
	conn, err := net.DialTimeout("tcp", host, mcpPreflightDialTimeout)
	if err != nil {
		return fmt.Sprintf("server tidak aktif di %s (dial: %v)", cfg.URL, err)
	}
	_ = conn.Close()
	return ""
}

// preflightLocal checks that the command binary exists before spawning it.
func preflightLocal(cfg mcp.MCPServerConfig) string {
	if cfg.Command == "" {
		return "command kosong"
	}
	if _, err := exec.LookPath(cfg.Command); err != nil {
		return fmt.Sprintf("command '%s' tidak ditemukan di PATH", cfg.Command)
	}
	return ""
}

func connectMCPServerWithTimeout(cfg mcp.MCPServerConfig, results chan<- mcpConnResult) {
	// Pre-flight check: skip early if clearly unreachable
	if reason := preflightCheckMCP(cfg); reason != "" {
		results <- mcpConnResult{
			Name: cfg.Name,
			Err:  fmt.Errorf("%s", reason),
		}
		return
	}

	resCh := make(chan mcpConnResult, 1)
	abandoned := make(chan struct{})

	go func() {
		res := connectMCPServer(cfg)
		select {
		case resCh <- res:
		case <-abandoned:
			if res.Client != nil {
				_ = res.Client.Close()
			}
		}
	}()

	timer := time.NewTimer(mcpStartupPerServerTimeout)
	defer timer.Stop()

	select {
	case res := <-resCh:
		results <- res
	case <-timer.C:
		close(abandoned)
		results <- mcpConnResult{
			Name: cfg.Name,
			Err:  fmt.Errorf("timeout startup setelah %s (server lambat startup?)", mcpStartupPerServerTimeout),
		}
	}
}

func connectMCPServer(cfg mcp.MCPServerConfig) mcpConnResult {
	var client *mcp.Client
	var err error

	switch cfg.Type {
	case "remote":
		client, err = mcp.NewRemoteClient(cfg)
	default:
		client, err = mcp.NewClient(cfg)
	}
	if err != nil {
		return mcpConnResult{Name: cfg.Name, Err: err}
	}

	tools, err := client.ListTools()
	if err != nil {
		_ = client.Close()
		return mcpConnResult{Name: cfg.Name, Err: err}
	}
	return mcpConnResult{Name: cfg.Name, Client: client, Tools: tools}
}

func registerMCPStartupResult(supervisor *agent.Supervisor, res mcpConnResult) {
	if res.Err != nil {
		ui.PrintWarning("Gagal menghubungkan MCP '%s': %v", res.Name, res.Err)
		return
	}
	supervisor.RegisterMCPClient(res.Name, res.Client)
	if len(res.Tools) > 0 {
		supervisor.UpdateMCPInfo(res.Name, res.Tools)
		ui.PrintSuccess("MCP '%s' terhubung (%d tools)", res.Name, len(res.Tools))
	} else {
		ui.PrintSuccess("MCP '%s' terhubung", res.Name)
	}
}
