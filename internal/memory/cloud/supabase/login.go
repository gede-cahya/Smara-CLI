// Package supabase — Login flow (headless + interactive).
//
// Supabase auth is simpler than Turso PKCE OAuth:
//   - Headless: reads SMARA_CLOUD_TOKEN (service_role key) and
//     SMARA_CLOUD_ORG (project ref) from environment variables.
//   - Interactive: prompts the user for project ref and API key
//     via stdin, with optional masking.
//
// Credentials shape (mirroring cloud.Credentials):
//   - Provider = "supabase"
//   - Token    = service_role key (or anon key for read-only)
//   - OrgID    = project reference (e.g., "abcdefghijklm")
//   - Region   = not used (Supabase manages regions internally)
package supabase

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	cloud "github.com/gede-cahya/Smara-CLI/internal/memory/cloud"
)

// Login performs the Supabase auth flow.
//
// Headless mode (opts.Headless == true):
//   - Reads SMARA_CLOUD_TOKEN and SMARA_CLOUD_ORG from env.
//   - Returns credentials directly without validation (caller calls
//     ValidateCredentials separately).
//
// Interactive mode:
//   - Prompts for project reference and API key on stdin.
//   - No browser involvement (unlike Turso PKCE).
//
// Errors:
//   - cloud.ErrLoginCancelled if user sends EOF during prompt.
//   - Descriptive error if headless env vars are missing.
func (p *SupabaseProvider) Login(ctx context.Context, opts cloud.LoginOptions) (*cloud.Credentials, error) {
	if opts.Headless {
		return p.loginHeadless()
	}
	return p.loginInteractive()
}

// loginHeadless reads credentials from environment variables.
func (p *SupabaseProvider) loginHeadless() (*cloud.Credentials, error) {
	token := strings.TrimSpace(os.Getenv("SMARA_CLOUD_TOKEN"))
	orgID := strings.TrimSpace(os.Getenv("SMARA_CLOUD_ORG"))

	if token == "" {
		return nil, errors.New("supabase: login headless: SMARA_CLOUD_TOKEN is not set")
	}
	if orgID == "" {
		return nil, errors.New("supabase: login headless: SMARA_CLOUD_ORG (project ref) is not set")
	}

	return &cloud.Credentials{
		Provider: "supabase",
		Token:    token,
		OrgID:    orgID,
	}, nil
}

// loginInteractive prompts the user for Supabase project ref and API key.
func (p *SupabaseProvider) loginInteractive() (*cloud.Credentials, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Fprintln(os.Stderr, "=== Supabase Cloud Login ===")
	fmt.Fprintln(os.Stderr, "Find your project ref and API key at:")
	fmt.Fprintln(os.Stderr, "  https://supabase.com/dashboard/project/_/settings/api")
	fmt.Fprintln(os.Stderr)

	// Project reference
	fmt.Fprint(os.Stderr, "Project reference (e.g. abcdefghijklm): ")
	ref, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("%w: %v", cloud.ErrLoginCancelled, err)
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("%w: project reference is required", cloud.ErrLoginCancelled)
	}

	// API key (service_role recommended for full access)
	fmt.Fprint(os.Stderr, "API key (service_role recommended): ")
	key, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("%w: %v", cloud.ErrLoginCancelled, err)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("%w: API key is required", cloud.ErrLoginCancelled)
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "✓ Credentials collected. Validating...")

	return &cloud.Credentials{
		Provider: "supabase",
		Token:    key,
		OrgID:    ref,
	}, nil
}
