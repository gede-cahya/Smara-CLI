package skill

import (
	"fmt"
	"strings"
)

// InstallReviewOptions controls secure review behavior before a skill is saved.
type InstallReviewOptions struct {
	Source       string
	Skill        *Skill
	LintResult   *LintReport
	Assessment   RiskAssessment
	Approve      bool
	AllowInvalid bool
	AllowedTools []string
	BlockedTools []string
}

// InstallReviewReport summarizes a skill import/install before it is persisted.
type InstallReviewReport struct {
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	Source           string         `json:"source"`
	Tools            []string       `json:"tools"`
	ShellCommands    []string       `json:"shell_commands,omitempty"`
	FileOperations   []string       `json:"file_operations,omitempty"`
	RemoteOperations []string       `json:"remote_operations,omitempty"`
	Risk             RiskAssessment `json:"risk"`
	Lint             *LintReport    `json:"lint,omitempty"`
	RequiresApproval bool           `json:"requires_approval"`
	Approved         bool           `json:"approved"`
	CanInstall       bool           `json:"can_install"`
	BlockingReasons  []string       `json:"blocking_reasons,omitempty"`
	SecurityWarnings []string       `json:"security_warnings,omitempty"`
}

// ReviewSkillInstall performs the Phase 9 security review for imported/remote skills.
func ReviewSkillInstall(opts InstallReviewOptions) InstallReviewReport {
	sk := opts.Skill
	if opts.Assessment.Level == "" && sk != nil {
		opts.Assessment = AssessRisk(sk)
	}
	lint := opts.LintResult
	if lint == nil && sk != nil {
		res := LintSkill(sk, nil)
		lint = &res
	}
	report := InstallReviewReport{Source: opts.Source, Risk: opts.Assessment, Lint: lint, Approved: opts.Approve}
	if sk != nil {
		report.Name = sk.Name
		report.Description = sk.Description
		report.Tools = uniqueStepTools(sk)
		report.ShellCommands = collectShellCommands(sk)
		report.FileOperations = collectMatchingOperations(sk, []string{"write_file", "edit_file", "delete_file", "rm ", "rm -rf", "mv ", "cp "})
		report.RemoteOperations = collectMatchingOperations(sk, []string{"ssh", "scp", "rsync", "curl ", "wget ", "http://", "https://"})
		report.SecurityWarnings = append(report.SecurityWarnings, toolPolicyWarnings(sk, opts.AllowedTools, opts.BlockedTools)...)
	}
	report.RequiresApproval = opts.Assessment.RequiresApproval || isRemoteSource(opts.Source)
	if lint != nil && lint.HasErrors() && !opts.AllowInvalid {
		report.BlockingReasons = append(report.BlockingReasons, "skill lint/validation failed")
	}
	if len(report.SecurityWarnings) > 0 {
		report.BlockingReasons = append(report.BlockingReasons, report.SecurityWarnings...)
	}
	if report.RequiresApproval && !opts.Approve {
		report.BlockingReasons = append(report.BlockingReasons, "explicit approval required before install")
	}
	report.CanInstall = len(report.BlockingReasons) == 0
	return report
}

func FormatInstallReview(r InstallReviewReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Skill install review: %s\n", r.Name)
	fmt.Fprintf(&b, "Source: %s\n", r.Source)
	fmt.Fprintf(&b, "Description: %s\n", r.Description)
	fmt.Fprintf(&b, "Tools: %s\n", strings.Join(r.Tools, ", "))
	fmt.Fprintf(&b, "Risk: %s (approval: %t)\n", r.Risk.Level, r.RequiresApproval)
	if len(r.ShellCommands) > 0 {
		fmt.Fprintf(&b, "Shell commands: %s\n", strings.Join(r.ShellCommands, " | "))
	}
	if len(r.FileOperations) > 0 {
		fmt.Fprintf(&b, "File operations: %s\n", strings.Join(r.FileOperations, " | "))
	}
	if len(r.RemoteOperations) > 0 {
		fmt.Fprintf(&b, "Remote/network: %s\n", strings.Join(r.RemoteOperations, " | "))
	}
	if r.Lint != nil {
		fmt.Fprintf(&b, "Lint valid: %t\n", !r.Lint.HasErrors())
	}
	if len(r.BlockingReasons) > 0 {
		fmt.Fprintf(&b, "Blocked: %s\n", strings.Join(r.BlockingReasons, "; "))
	}
	fmt.Fprintf(&b, "Can install: %t\n", r.CanInstall)
	return b.String()
}

func uniqueStepTools(sk *Skill) []string {
	seen := map[string]bool{}
	var out []string
	for _, st := range sk.Steps {
		if st.Tool != "" && !seen[st.Tool] {
			seen[st.Tool] = true
			out = append(out, st.Tool)
		}
	}
	return out
}
func collectShellCommands(sk *Skill) []string {
	return collectMatchingOperations(sk, []string{"run_command", "ssh_exec", "command", "cmd"})
}
func collectMatchingOperations(sk *Skill, needles []string) []string {
	var out []string
	for _, st := range sk.Steps {
		blob := strings.ToLower(st.Tool + " " + fmt.Sprintf("%v", st.Args))
		for _, n := range needles {
			if strings.Contains(blob, strings.ToLower(n)) {
				out = append(out, fmt.Sprintf("%s %v", st.Tool, st.Args))
				break
			}
		}
	}
	return out
}
func toolPolicyWarnings(sk *Skill, allowed, blocked []string) []string {
	allow := sliceSet(allowed)
	block := sliceSet(blocked)
	var warns []string
	for _, t := range uniqueStepTools(sk) {
		tl := strings.ToLower(t)
		if block[tl] {
			warns = append(warns, "blocked tool used: "+t)
		}
		if len(allow) > 0 && !allow[tl] {
			warns = append(warns, "tool not in allowlist: "+t)
		}
	}
	return warns
}
func sliceSet(vals []string) map[string]bool {
	m := map[string]bool{}
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			m[strings.ToLower(strings.TrimSpace(v))] = true
		}
	}
	return m
}
func isRemoteSource(src string) bool {
	s := strings.ToLower(strings.TrimSpace(src))
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.Contains(s, "github.com") || strings.Contains(s, "/") && !strings.HasPrefix(s, ".")
}
