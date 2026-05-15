// Package turso — PKCE login flow (task 8.1).
//
// This file implements TursoProvider.Login, the interactive (browser-
// based) authentication path plus the headless (env-only) shortcut.
//
// Flow summary (browser path):
//
//  1. Generate a fresh PKCE pair: a 32-byte random verifier rendered as
//     base64url, and a SHA-256(verifier) challenge, also base64url. The
//     verifier never leaves this process; only the challenge travels in
//     the authorization URL. This is the canonical RFC 7636 "S256"
//     method (the only one Turso advertises).
//
//  2. Generate a 16-byte random `state` token used as a CSRF guard. We
//     verify the redirect carries the same state value before trusting
//     any code returned from the browser.
//
//  3. Spawn a tiny local HTTP server bound to 127.0.0.1 on either the
//     caller-supplied port (LoginOptions.CallbackPort), the default
//     8765, or — if both are taken — a kernel-assigned ephemeral port
//     via `:0`. The server has exactly one route, `/callback`, and is
//     shut down once a callback has been received.
//
//  4. Build the authorization URL with `client_id=smara-cli`, the
//     PKCE challenge, the state token, and the loopback redirect_uri.
//     Open the user's default browser via `github.com/pkg/browser`.
//
//  5. Wait up to 120 seconds for the redirect. On timeout (or if the
//     user closed the tab without authorising) we return
//     `cloud.ErrLoginCancelled` so the caller can render a clean
//     "login cancelled" message instead of a stack trace.
//
//  6. CSRF-check the returned `state` against the one we generated.
//     A mismatch is a hard error; we do NOT exchange the code.
//
//  7. POST the authorization code + verifier to the Turso token
//     endpoint, parse the JSON response, and return a populated
//     `*cloud.Credentials` for the caller (typically `cmd/smara/login.go`)
//     to hand off to the CredentialStore.
//
// Headless path:
//
//   - When LoginOptions.Headless is true, we delegate immediately to
//     `cloud.LoadHeadlessOrError`. That helper returns ErrNoCredentials
//     wrapped with a message that explicitly names the missing env var
//     (SMARA_CLOUD_TOKEN), satisfying requirement 14.4.
//
// IMPORTANT — endpoint URLs:
//
//	Turso does not currently publish a public OAuth/PKCE endpoint
//	suitable for third-party CLIs. The URLs hard-coded below
//	(/v1/auth/cli/authorize, /v1/auth/cli/token) are placeholders
//	chosen to match the spec / design document so the rest of the
//	flow (PKCE pair, callback server, CSRF check, token unmarshal)
//	can be exercised end-to-end. Once Turso ships an official
//	browser-flow endpoint (or once we move to the device-code flow)
//	the constants below should be updated to the real paths and the
//	JSON schema in tursoTokenResponse adjusted to match the actual
//	response. See the spec's Algorithm 3 for the contract this code
//	implements.
//
// Requirements covered: 11.2 (provider name "turso"), 14.1 (env-mode
// short-circuit), 14.2 (env vars are the headless source), 14.4
// (missing env var → non-zero exit + named-variable error).
package turso

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/browser"

	"github.com/gede-cahya/Smara-CLI/internal/audit"
	cloud "github.com/gede-cahya/Smara-CLI/internal/memory/cloud"
)

// ----------------------------------------------------------------------------
// Constants
// ----------------------------------------------------------------------------

const (
	// tursoOAuthClientID is the public client identifier the Smara CLI
	// uses when initiating the PKCE flow. It is hard-coded (per
	// requirement 11.2 the provider identity is fixed) and may travel
	// in the authorization URL in plain text — public OAuth clients
	// do not have client secrets.
	tursoOAuthClientID = "smara-cli"

	// tursoAuthorizeURL is the placeholder authorization endpoint.
	//
	// TODO(turso-real-endpoints): Turso has not published a public
	// browser-based OAuth endpoint at the time of writing. The path
	// below is consistent with the spec's Algorithm 3 description but
	// will need to be updated once Turso ships an official one (or
	// once this CLI migrates to the device-code flow).
	tursoAuthorizeURL = "https://api.turso.tech/v1/auth/cli/authorize"

	// tursoTokenURL is the placeholder code-exchange endpoint. Same
	// caveat as tursoAuthorizeURL.
	//
	// TODO(turso-real-endpoints): replace with the real Turso token
	// endpoint once published.
	tursoTokenURL = "https://api.turso.tech/v1/auth/cli/token"

	// defaultCallbackPort is the loopback port the local PKCE server
	// binds to first. Per the spec's Algorithm 3 preconditions we
	// fall back to a kernel-assigned port (`:0`) when 8765 is busy.
	defaultCallbackPort = 8765

	// callbackTimeout is the maximum time we wait for the browser to
	// hit /callback. The 120s budget matches the spec's Algorithm 3
	// step 3 ("waitForCallback(opts.CallbackPort, timeout=120s)") and
	// gives the user enough headroom to log in via SSO / 2FA without
	// holding the local port indefinitely.
	callbackTimeout = 120 * time.Second

	// callbackPath is the single route the local server exposes. It
	// lives at /callback (not /) so unrelated probes against the port
	// (e.g. browser preflight, ad-blocker scans) cannot accidentally
	// be parsed as a valid PKCE redirect.
	callbackPath = "/callback"

	// pkceVerifierBytes is the entropy budget for the verifier. RFC
	// 7636 mandates a high-entropy random string between 43 and 128
	// chars after base64url-encoding; 32 raw bytes encode to 43 chars
	// (without padding) which is the minimum compliant length. We
	// match the spec's Algorithm 3 ("randomBase64URL(32 bytes)") for
	// determinism with the design doc.
	pkceVerifierBytes = 32

	// stateBytes is the entropy budget for the CSRF state token. 16
	// raw bytes encode to 22 base64url chars, plenty to make
	// guessing infeasible while keeping the redirect URL short.
	stateBytes = 16

	// httpServerShutdownTimeout caps the time we'll wait for the
	// callback server's graceful Shutdown to drain before returning
	// from Login. The handler has already responded by the time we
	// call Shutdown, so 5s is generous.
	httpServerShutdownTimeout = 5 * time.Second
)

// ----------------------------------------------------------------------------
// Login
// ----------------------------------------------------------------------------

// Login performs the Turso authentication flow.
//
// In interactive mode it runs the PKCE browser flow described at the
// top of this file. In headless mode (LoginOptions.Headless == true)
// it delegates to cloud.LoadHeadlessOrError so a missing env var
// surfaces as the actionable, sentinel-wrapped error from
// requirement 14.4 without ever launching a browser.
//
// The returned *cloud.Credentials is fully populated except for fields
// that the upstream identity provider does not yet supply (RefreshToken
// when the response omits it, Email when the JWT body cannot be
// decoded, etc.). Callers should treat empty optional fields as
// "unknown" rather than "absent".
func (p *TursoProvider) Login(ctx context.Context, opts cloud.LoginOptions) (*cloud.Credentials, error) {
	// Audit hook: every Login attempt is recorded — success or failure —
	// with token-shaped substrings already redacted by audit.LogCloudOp.
	// We capture both the resulting creds and the final err via locals so
	// the deferred call observes whichever return path actually fired
	// (headless short-circuit, PKCE error, token exchange failure, ...).
	//
	// Per requirement 16.1/16.3 the entry MUST NOT include the token or
	// refresh token. We record:
	//   - target = creds.Email (interactive success) or "headless"
	//     (headless mode, where the email is typically unknown),
	//   - extra.source = "interactive" | "headless" so log readers can
	//     distinguish browser flows from env-only flows,
	//   - extra.provider = "turso" so a future multi-provider audit log
	//     remains greppable per provider,
	//   - extra.error = err.Error() on failure (token-shaped substrings
	//     are redacted by the regex pass inside audit.LogCloudOp before
	//     the entry reaches disk).
	var (
		loginCreds *cloud.Credentials
		loginErr   error
	)
	defer func() {
		source := "interactive"
		if opts.Headless {
			source = "headless"
		}
		extra := map[string]any{
			"source":   source,
			"provider": p.Name(),
		}
		if loginErr != nil {
			extra["error"] = loginErr.Error()
		}
		// target preference: the authenticated email when we have one
		// (interactive flow, sometimes also headless when an env var
		// supplied it), falling back to the literal "headless" string
		// when we genuinely have no identity to report.
		target := ""
		if loginCreds != nil {
			target = loginCreds.Email
		}
		if target == "" {
			if opts.Headless {
				target = "headless"
			} else {
				target = p.Name()
			}
		}
		_ = audit.LogCloudOp("login", loginErr == nil, target, extra)
	}()

	creds, err := p.login(ctx, opts)
	loginCreds = creds
	loginErr = err
	return creds, err
}

// login is the inner implementation of Login; the public Login wrapper
// adds the deferred audit hook around it so every return path emits a
// single audit entry. Splitting the function this way keeps the existing
// control flow (headless short-circuit, PKCE pair, callback server,
// token exchange) unchanged while letting the defer observe the final
// error via a captured local rather than threading a named return
// through every branch.
func (p *TursoProvider) login(ctx context.Context, opts cloud.LoginOptions) (*cloud.Credentials, error) {
	// Headless short-circuit (requirement 14.1 / 14.4): in CI/CD or
	// any non-interactive environment we never spin up a callback
	// server and never open a browser. The env-mode store is the
	// authoritative credential source and missing env vars surface
	// as ErrNoCredentials wrapped with a message naming the absent
	// variable.
	if opts.Headless {
		creds, err := cloud.LoadHeadlessOrError()
		if err != nil {
			return nil, err
		}
		// Force the provider field to "turso" so callers that pass
		// a generic SMARA_CLOUD_PROVIDER override still end up with
		// a *Credentials addressed at this provider — Login is the
		// turso package's entrypoint and any other Provider value
		// would be a configuration mistake.
		creds.Provider = p.Name()
		// Region from LoginOptions overrides the env var when both
		// are set; this matches the interactive-flow contract below
		// where opts.Region is the authoritative choice for the
		// caller-requested region.
		if opts.Region != "" {
			creds.Region = opts.Region
		}
		return creds, nil
	}

	// ------------------------------------------------------------------
	// Step 1: Generate the PKCE verifier + challenge.
	// ------------------------------------------------------------------
	verifier, err := generateVerifier()
	if err != nil {
		return nil, fmt.Errorf("turso: login: generate verifier: %w", err)
	}
	challenge := computeChallenge(verifier)

	// ------------------------------------------------------------------
	// Step 2: Generate a fresh CSRF state token.
	// ------------------------------------------------------------------
	state, err := generateState()
	if err != nil {
		return nil, fmt.Errorf("turso: login: generate state: %w", err)
	}

	// ------------------------------------------------------------------
	// Step 3: Bind the local callback server.
	//
	// We try the explicit port first (LoginOptions.CallbackPort or the
	// default 8765); if it is in use, we ask the kernel for any free
	// port via `:0`. The actual port is read back from the listener
	// and embedded in the redirect_uri so the browser can find it.
	// ------------------------------------------------------------------
	preferredPort := opts.CallbackPort
	if preferredPort == 0 {
		preferredPort = defaultCallbackPort
	}
	listener, port, err := bindCallbackListener(preferredPort)
	if err != nil {
		return nil, fmt.Errorf("turso: login: bind callback listener: %w", err)
	}

	// resultCh carries the (single) callback payload from the HTTP
	// handler back to the main goroutine. Buffering by 1 lets the
	// handler return immediately even if the main goroutine has not
	// yet started selecting on the channel.
	resultCh := make(chan callbackResult, 1)
	server := startCallbackServer(listener, state, resultCh)

	// Whatever happens next, ensure the local server is closed
	// before we return so the port is freed and no goroutine leaks.
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), httpServerShutdownTimeout)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	// ------------------------------------------------------------------
	// Step 4: Build the authorization URL and open the browser.
	// ------------------------------------------------------------------
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d%s", port, callbackPath)
	authURL := buildAuthorizeURL(challenge, state, redirectURI)

	if err := browser.OpenURL(authURL); err != nil {
		// Failure to open a browser is recoverable in principle —
		// the user could paste the URL manually — but the current
		// flow does not surface the URL through any other channel.
		// Returning an error is the honest signal here; the caller
		// can render a "open this URL manually" hint with authURL
		// in a future iteration.
		return nil, fmt.Errorf("turso: login: open browser: %w", err)
	}

	// ------------------------------------------------------------------
	// Step 5: Wait for the callback (with timeout / context cancel).
	// ------------------------------------------------------------------
	cb, err := waitForCallback(ctx, resultCh, callbackTimeout)
	if err != nil {
		return nil, err
	}

	// ------------------------------------------------------------------
	// Step 6: CSRF check.
	//
	// startCallbackServer already verified the state before pushing
	// the result onto resultCh, so this assertion is defence-in-depth.
	// Keeping the explicit check here means future refactors that
	// loosen the handler-side validation cannot silently weaken the
	// guarantee.
	// ------------------------------------------------------------------
	if cb.state != state {
		return nil, fmt.Errorf("turso: login: state mismatch (CSRF check failed)")
	}
	if cb.code == "" {
		return nil, fmt.Errorf("turso: login: callback returned empty authorization code")
	}

	// ------------------------------------------------------------------
	// Step 7: Exchange the code (+ verifier) for an access token.
	// ------------------------------------------------------------------
	creds, err := p.exchangeCodeForToken(ctx, cb.code, verifier, redirectURI)
	if err != nil {
		return nil, err
	}
	creds.Provider = p.Name()
	if opts.Region != "" {
		creds.Region = opts.Region
	}
	return creds, nil
}

// ----------------------------------------------------------------------------
// PKCE helpers
// ----------------------------------------------------------------------------

// generateVerifier returns a fresh, RFC 7636-compliant PKCE verifier
// (32 random bytes, base64url-encoded, no padding). 32 bytes is the
// minimum entropy needed to produce a 43-char encoded value, which is
// itself the spec-mandated minimum.
func generateVerifier() (string, error) {
	buf := make([]byte, pkceVerifierBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// computeChallenge returns the S256 PKCE challenge derived from the
// supplied verifier (sha256 of the verifier ASCII bytes, base64url
// encoded without padding). Per RFC 7636 §4.2 the challenge is hashed
// over the *encoded* verifier — i.e. the same string sent on the
// wire — not the raw 32 bytes. Using the encoded verifier matches the
// reference implementation in the IETF spec and keeps interop with
// any compliant authorisation server.
func computeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// generateState returns a base64url-encoded random state token used as
// a CSRF guard against malicious redirects. 16 bytes is plenty: the
// state is single-use and never persisted, so 128 bits of entropy is
// well beyond any guessing budget a network adversary could mount in
// the 120-second callback window.
func generateState() (string, error) {
	buf := make([]byte, stateBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// buildAuthorizeURL composes the authorization endpoint URL with the
// PKCE / OAuth query parameters expected by the Turso CLI flow. We use
// url.Values + Encode() rather than fmt.Sprintf so every value is
// percent-encoded correctly (the redirect_uri in particular contains a
// colon and slashes that would otherwise need manual escaping).
func buildAuthorizeURL(challenge, state, redirectURI string) string {
	q := url.Values{}
	q.Set("client_id", tursoOAuthClientID)
	q.Set("response_type", "code")
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	q.Set("redirect_uri", redirectURI)
	return tursoAuthorizeURL + "?" + q.Encode()
}

// ----------------------------------------------------------------------------
// Callback server
// ----------------------------------------------------------------------------

// callbackResult captures the data extracted from the OAuth redirect
// once it lands on the local /callback handler. error is non-nil when
// the redirect itself signalled a failure (e.g. ?error=access_denied)
// so the main goroutine can surface a tailored message.
type callbackResult struct {
	code  string
	state string
	err   error
}

// bindCallbackListener tries to bind to 127.0.0.1:preferredPort first.
// If that fails (port already in use, EACCES on a privileged port),
// it falls back to a kernel-assigned ephemeral port via `:0`. The
// returned listener is owned by the caller; close it via the
// http.Server returned from startCallbackServer.
//
// We bind to the loopback interface explicitly (rather than
// "0.0.0.0:port" or just ":port") so the local PKCE server cannot be
// reached from another host on the network — a small but real defence
// against an attacker on the same LAN spamming the /callback endpoint
// with bogus codes during the 120-second window.
func bindCallbackListener(preferredPort int) (net.Listener, int, error) {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(preferredPort))
	listener, err := net.Listen("tcp", addr)
	if err == nil {
		return listener, listenerPort(listener), nil
	}
	// Fall back to "any free port" — the kernel picks one and we
	// read it back from the listener so we can plug it into the
	// redirect URI.
	listener, fallbackErr := net.Listen("tcp", "127.0.0.1:0")
	if fallbackErr != nil {
		return nil, 0, fmt.Errorf("preferred port %d unavailable (%v) and OS-assigned fallback also failed: %w",
			preferredPort, err, fallbackErr)
	}
	return listener, listenerPort(listener), nil
}

// listenerPort returns the actual TCP port a *net.TCPListener is bound
// to. For port 0 (kernel-assigned) this is the only way to learn the
// port; for explicit ports it is a redundant but cheap sanity check.
func listenerPort(l net.Listener) int {
	if tcpAddr, ok := l.Addr().(*net.TCPAddr); ok {
		return tcpAddr.Port
	}
	return 0
}

// startCallbackServer wires the supplied listener up to a minimal
// http.Server with a single /callback handler. The handler validates
// the redirect's `state` query parameter against expectedState (CSRF
// guard) and forwards the result over resultCh. Any other path
// returns 404 so a stray probe cannot produce a fake code/state pair.
//
// The server is started in a goroutine via Serve(); the caller is
// responsible for invoking Shutdown to release the port.
func startCallbackServer(listener net.Listener, expectedState string, resultCh chan<- callbackResult) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		// Pull the (unique) code/state/error parameters off the
		// query string. We deliberately DO NOT log the query —
		// although neither value is itself a long-lived secret, the
		// authorization code is short-lived bearer credential and
		// keeping it out of stdout/stderr matches the redaction
		// posture of the wider cloud package.
		q := r.URL.Query()
		oauthErr := q.Get("error")
		state := q.Get("state")
		code := q.Get("code")

		// CSRF check: a state mismatch is treated as an attack and
		// MUST NOT be exchanged. We respond with a generic browser
		// page (so the user knows the tab can be closed) but push
		// an error onto the channel so the main goroutine surfaces
		// a typed failure.
		if state != expectedState {
			writeBrowserPage(w, http.StatusBadRequest,
				"Login failed: invalid state parameter (CSRF check). You can close this tab.")
			resultCh <- callbackResult{
				err: fmt.Errorf("turso: login: callback state mismatch"),
			}
			return
		}

		if oauthErr != "" {
			desc := q.Get("error_description")
			writeBrowserPage(w, http.StatusBadRequest,
				fmt.Sprintf("Login failed: %s. You can close this tab.", oauthErr))
			resultCh <- callbackResult{
				err: fmt.Errorf("turso: login: oauth error %q: %s", oauthErr, desc),
			}
			return
		}

		if code == "" {
			writeBrowserPage(w, http.StatusBadRequest,
				"Login failed: callback missing authorization code. You can close this tab.")
			resultCh <- callbackResult{
				err: fmt.Errorf("turso: login: callback missing code"),
			}
			return
		}

		writeBrowserPage(w, http.StatusOK,
			"Login successful. You can close this tab and return to your terminal.")
		resultCh <- callbackResult{code: code, state: state}
	})

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		// http.ErrServerClosed is the expected sentinel when
		// Shutdown is invoked from the deferred cleanup; treat it
		// as a no-op. Any other error is unusual and would already
		// have surfaced via the callback handler's resultCh push.
		_ = server.Serve(listener)
	}()
	return server
}

// writeBrowserPage emits a tiny HTML body so the user sees a friendly
// message in the browser tab once the redirect lands. We do not link
// to any external resources or run any JavaScript; the response is
// fully self-contained and harmless even if the page were viewed
// from a different origin.
func writeBrowserPage(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	body := `<!doctype html><html><head><meta charset="utf-8"><title>Smara CLI</title></head>` +
		`<body style="font-family:sans-serif;padding:2rem"><p>` + htmlEscape(message) + `</p></body></html>`
	_, _ = io.WriteString(w, body)
}

// htmlEscape provides the minimum escaping needed for plain-text
// messages embedded in the browser page. The full html/template
// package would be overkill for a fixed-string status line and would
// pull in additional imports for what is effectively a trivial
// substitution.
func htmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return r.Replace(s)
}

// waitForCallback blocks until one of three things happens:
//
//   - the local callback server pushes a result onto resultCh (success
//     or oauth-error, both surfaced as typed errors to the caller),
//   - the supplied context is cancelled (caller-driven cancellation,
//     e.g. SIGINT), or
//   - the 120-second timeout elapses with no callback received.
//
// The two cancellation paths both map to cloud.ErrLoginCancelled so
// the caller can render a single "login cancelled" message regardless
// of which signal fired. The internal cause (ctx.Err / "timeout") is
// wrapped via fmt.Errorf so debug logs can still pinpoint the source.
func waitForCallback(ctx context.Context, resultCh <-chan callbackResult, timeout time.Duration) (callbackResult, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case res := <-resultCh:
		if res.err != nil {
			return callbackResult{}, res.err
		}
		return res, nil
	case <-ctx.Done():
		return callbackResult{}, fmt.Errorf("%w: %v", cloud.ErrLoginCancelled, ctx.Err())
	case <-timer.C:
		return callbackResult{}, fmt.Errorf("%w: timed out after %s waiting for browser callback",
			cloud.ErrLoginCancelled, timeout)
	}
}

// ----------------------------------------------------------------------------
// Token exchange
// ----------------------------------------------------------------------------

// tursoTokenResponse is the JSON shape we expect from the Turso token
// endpoint. The field names match the OAuth 2.0 token response shape
// (RFC 6749 §5.1) plus a few Turso-specific extensions (email,
// org_id) we use to populate cloud.Credentials.
//
// TODO(turso-real-endpoints): this schema is provisional. Once the
// real Turso endpoint is published the field names / nesting may
// need to be adjusted; tests in this package should be updated in
// lockstep.
type tursoTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"` // seconds until expiry
	RefreshToken string `json:"refresh_token,omitempty"`
	Email        string `json:"email,omitempty"`
	OrgID        string `json:"org_id,omitempty"`
	Region       string `json:"region,omitempty"`

	// Optional error fields populated when the server returns a
	// non-2xx response with a structured error body.
	Error            string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// exchangeCodeForToken POSTs the authorization code + PKCE verifier
// to the Turso token endpoint and returns a populated *cloud.Credentials
// (sans Provider, which the caller fills in).
//
// The request body is form-encoded (`application/x-www-form-urlencoded`)
// per OAuth 2.0 conventions, NOT JSON. The endpoint URL itself uses
// HTTPS so the verifier never travels in clear text.
func (p *TursoProvider) exchangeCodeForToken(ctx context.Context, code, verifier, redirectURI string) (*cloud.Credentials, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("code_verifier", verifier)
	form.Set("client_id", tursoOAuthClientID)
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tursoTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("turso: login: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("turso: login: token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB safety cap
	if err != nil {
		return nil, fmt.Errorf("turso: login: read token response: %w", err)
	}

	var parsed tursoTokenResponse
	// We attempt to decode regardless of status code so a 4xx body
	// with an `error` / `error_description` field can still be
	// surfaced verbatim to the user. Decoding failures on a non-2xx
	// status fall through to a generic "status N" error.
	_ = json.Unmarshal(body, &parsed)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if parsed.Error != "" {
			return nil, fmt.Errorf("turso: login: token endpoint returned %d %s: %s",
				resp.StatusCode, parsed.Error, parsed.ErrorDescription)
		}
		return nil, fmt.Errorf("turso: login: token endpoint returned %d: %s",
			resp.StatusCode, truncateForError(string(body)))
	}

	if parsed.AccessToken == "" {
		// The spec's Algorithm 3 step 4 ("ASSERT token.AccessToken ≠ ∅")
		// is enforced here: a 2xx response without an access_token
		// field is malformed and we refuse to mint a *Credentials
		// the caller would have no use for.
		return nil, errors.New("turso: login: token endpoint returned no access_token")
	}

	expiresAt := time.Time{}
	if parsed.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(parsed.ExpiresIn) * time.Second)
	}

	return &cloud.Credentials{
		Token:        parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
		Email:        parsed.Email,
		OrgID:        parsed.OrgID,
		Region:       parsed.Region,
		ExpiresAt:    expiresAt,
	}, nil
}

// truncateForError keeps error messages bounded when a server returns
// an unexpectedly large body (HTML error page, debug dump, ...) so we
// never blow up the user's terminal with kilobytes of unstructured
// content. 256 chars is enough for a representative snippet.
func truncateForError(s string) string {
	const max = 256
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
