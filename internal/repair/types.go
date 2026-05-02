// Package repair provides self-repair / self-healing diagnostic and recovery tools for Smara CLI.
package repair

import "fmt"

// HealthStatus represents the result of a health check.
type HealthStatus string

const (
	StatusOK    HealthStatus = "OK"
	StatusWarn  HealthStatus = "WARN"
	StatusFail  HealthStatus = "FAIL"
)

// CheckResult is the outcome of a single diagnostic check.
type CheckResult struct {
	Module      string       `json:"module"`
	Status      HealthStatus `json:"status"`
	Message     string       `json:"message"`
	Fixable     bool         `json:"fixable"`
	Suggestion  string       `json:"suggestion,omitempty"`
}

// RepairAction describes a single repair step.
type RepairAction struct {
	Module      string
	Description string
	DryRun      bool
	Run         func() error
}

// Summary returns a human-readable summary of check results.
type Summary struct {
	OK    int
	Warn  int
	Fail  int
}

// ComputeSummary counts results by status.
func ComputeSummary(results []CheckResult) Summary {
	var s Summary
	for _, r := range results {
		switch r.Status {
		case StatusOK:
			s.OK++
		case StatusWarn:
			s.Warn++
		case StatusFail:
			s.Fail++
		}
	}
	return s
}

// String returns a color-less summary string.
func (s Summary) String() string {
	return fmt.Sprintf("%d OK, %d WARN, %d FAIL", s.OK, s.Warn, s.Fail)
}

// HasFailures returns true if any check failed.
func (s Summary) HasFailures() bool {
	return s.Fail > 0
}
