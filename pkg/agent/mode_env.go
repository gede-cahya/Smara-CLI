package agent

import (
	"os"
	"strings"
)

// IsRushAutoApprovalEnabled returns true when the environment explicitly opts
// into RUSH autonomous execution / auto-approval. It is used by CLI commands,
// the TUI, desktop app, gateway, and the agent core to default to RUSH mode
// and to bypass confirmation/permission gates.
func IsRushAutoApprovalEnabled() bool {
	if strings.EqualFold(os.Getenv("SMARA_RUSH_AUTO_APPROVE"), "false") || os.Getenv("SMARA_RUSH_AUTO_APPROVE") == "0" {
		return false
	}
	return strings.EqualFold(os.Getenv("SMARA_MODE"), string(ModeRush)) ||
		strings.EqualFold(os.Getenv("SMARA_AGENT_MODE"), string(ModeRush)) ||
		strings.EqualFold(os.Getenv("SMARA_RUSH"), "true") || os.Getenv("SMARA_RUSH") == "1" ||
		strings.EqualFold(os.Getenv("SMARA_RUSH_AUTO_APPROVE"), "true") || os.Getenv("SMARA_RUSH_AUTO_APPROVE") == "1" ||
		strings.EqualFold(os.Getenv("SMARA_AUTO_APPROVE"), "true") || os.Getenv("SMARA_AUTO_APPROVE") == "1" ||
		strings.EqualFold(os.Getenv("SMARA_APPROVAL_REQUIRED"), "false") || os.Getenv("SMARA_APPROVAL_REQUIRED") == "0" ||
		strings.EqualFold(os.Getenv("SMARA_BYPASS_PERMISSIONS"), "true") || os.Getenv("SMARA_BYPASS_PERMISSIONS") == "1"
}
