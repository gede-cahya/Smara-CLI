package repair

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gede-cahya/Smara-CLI/internal/config"
)

// DoctorOptions controls diagnostic behavior.
type DoctorOptions struct {
	JSON   bool
	Module string // filter by module, empty = all
}

// RunDoctor executes all diagnostic checks and returns results.
func RunDoctor(opts DoctorOptions) ([]CheckResult, error) {
	var allResults []CheckResult
	cfg := config.Get()

	// DB module
	if opts.Module == "" || opts.Module == "db" {
		dbRes := CheckDBHealth(cfg.DBPath)
		allResults = append(allResults, dbRes)
	}

	// Config module
	if opts.Module == "" || opts.Module == "config" {
		configRes := CheckConfigHealth("")
		allResults = append(allResults, configRes)
	}

	// MCP module
	if opts.Module == "" || opts.Module == "mcp" {
		mcpResults := CheckMCPHealth()
		allResults = append(allResults, mcpResults...)
	}

	// Session module
	if opts.Module == "" || opts.Module == "session" {
		sessionResults := CheckSessionHealth()
		allResults = append(allResults, sessionResults...)
	}

	// Disk module
	if opts.Module == "" || opts.Module == "disk" {
		diskResults := CheckDiskHealth()
		allResults = append(allResults, diskResults...)
	}

	if opts.JSON {
		data, err := json.MarshalIndent(allResults, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("gagal marshal JSON: %w", err)
		}
		fmt.Println(string(data))
	} else {
		printResults(allResults)
	}

	return allResults, nil
}

func printResults(results []CheckResult) {
	for _, r := range results {
		icon := "[OK]  "
		if r.Status == StatusWarn {
			icon = "[WARN]"
		} else if r.Status == StatusFail {
			icon = "[FAIL]"
		}
		fmt.Printf("%s %-10s %s\n", icon, r.Module, r.Message)
		if r.Suggestion != "" {
			fmt.Printf("       → %s\n", r.Suggestion)
		}
	}
	summary := ComputeSummary(results)
	fmt.Println(strings.Repeat("-", 40))
	fmt.Printf("Summary: %s\n", summary.String())
}
