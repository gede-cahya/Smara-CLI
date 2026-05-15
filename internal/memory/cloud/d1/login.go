// Package d1 — Login flow (headless + interactive).
//
// Cloudflare D1 auth uses API Tokens (not OAuth/PKCE):
//   - Headless: reads SMARA_CLOUD_TOKEN (API token) and
//     SMARA_CLOUD_ORG (account ID) from environment variables.
//   - Interactive: prompts the user for account ID and API token
//     via stdin.
//
// Credentials shape (mirroring cloud.Credentials):
//   - Provider = "d1"
//   - Token    = Cloudflare API Token (with D1 read/write permissions)
//   - OrgID    = Cloudflare Account ID (32-char hex)
//   - Region   = not used (D1 is global/regional at account level)
//
// Getting a Cloudflare API Token:
//  1. Go to https://dash.cloudflare.com/profile/api-tokens
//  2. Create a token with "D1" permissions (Edit)
//  3. Account ID is in the dashboard URL: dash.cloudflare.com/<account_id>
package d1

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	cloud "github.com/gede-cahya/Smara-CLI/internal/memory/cloud"
)

// Login performs the Cloudflare D1 auth flow.
//
// Headless mode (opts.Headless == true):
//   - Reads SMARA_CLOUD_TOKEN and SMARA_CLOUD_ORG from env.
//   - Returns credentials directly (caller calls ValidateCredentials).
//
// Interactive mode:
//   - Prompts for Cloudflare Account ID and API Token on stdin.
func (p *D1Provider) Login(ctx context.Context, opts cloud.LoginOptions) (*cloud.Credentials, error) {
	if opts.Headless {
		return p.loginHeadless()
	}
	return p.loginInteractive()
}

// loginHeadless reads credentials from environment variables.
func (p *D1Provider) loginHeadless() (*cloud.Credentials, error) {
	token := strings.TrimSpace(os.Getenv("SMARA_CLOUD_TOKEN"))
	orgID := strings.TrimSpace(os.Getenv("SMARA_CLOUD_ORG"))

	if token == "" {
		return nil, errors.New("d1: login headless: SMARA_CLOUD_TOKEN is not set")
	}
	if orgID == "" {
		return nil, errors.New("d1: login headless: SMARA_CLOUD_ORG (account ID) is not set")
	}

	return &cloud.Credentials{
		Provider: "d1",
		Token:    token,
		OrgID:    orgID,
	}, nil
}

// loginInteractive prompts the user for Cloudflare account ID and API token.
func (p *D1Provider) loginInteractive() (*cloud.Credentials, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Fprintln(os.Stderr, "=== Cloudflare D1 Login ===")
	fmt.Fprintln(os.Stderr, "You need a Cloudflare API Token with D1 permissions.")
	fmt.Fprintln(os.Stderr, "  1. Go to https://dash.cloudflare.com/profile/api-tokens")
	fmt.Fprintln(os.Stderr, "  2. Create token with 'D1' edit permission")
	fmt.Fprintln(os.Stderr, "  3. Copy your Account ID from the dashboard URL")
	fmt.Fprintln(os.Stderr)

	// Account ID.
	fmt.Fprint(os.Stderr, "Cloudflare Account ID (32-char hex): ")
	accountID, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("%w: %v", cloud.ErrLoginCancelled, err)
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, fmt.Errorf("%w: account ID is required", cloud.ErrLoginCancelled)
	}

	// API Token.
	fmt.Fprint(os.Stderr, "API Token: ")
	apiToken, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("%w: %v", cloud.ErrLoginCancelled, err)
	}
	apiToken = strings.TrimSpace(apiToken)
	if apiToken == "" {
		return nil, fmt.Errorf("%w: API token is required", cloud.ErrLoginCancelled)
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "✓ Credentials collected. Validating...")

	return &cloud.Credentials{
		Provider: "d1",
		Token:    apiToken,
		OrgID:    accountID,
	}, nil
}
