package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

// ---- Headless browser for anti-bot bypass ---------------------------------
//
// This file implements `fetchHeadless`, a rod+stealth-powered page fetcher
// that can defeat most tier-1 anti-bot mechanisms:
//
//   • Cloudflare "Just a moment..." JS challenge (5-second auto-pass)
//   • JA3/JA4 TLS fingerprinting (Chromium uses the real browser stack)
//   • navigator.webdriver detection (patched by go-rod/stealth)
//   • basic rate-limit + User-Agent sniffing
//
// It does NOT defeat:
//   • Interactive CAPTCHA (hCaptcha, reCAPTCHA v2, Cloudflare Turnstile when
//     triggered to manual mode — they wait for mouse/keyboard input)
//   • Enterprise Bot Management with behavioral scoring + residential IP
//     requirement (needs a paid proxy)
//
// Resources: a single launcher is reused across calls to avoid paying the
// Chromium startup cost (~1–2s) per request. The browser is instantiated
// lazily on the first call and torn down with the process.

var (
	sharedBrowser     *rod.Browser
	sharedBrowserErr  error
	sharedBrowserInit sync.Once
	sharedBrowserMu   sync.Mutex
)

// ensureHeadlessBrowser launches a single headless Chromium the first time
// it is called and returns the shared instance on subsequent calls.
// Returns an error only if Chromium is absent or cannot be launched.
func ensureHeadlessBrowser() (*rod.Browser, error) {
	sharedBrowserInit.Do(func() {
		sharedBrowserMu.Lock()
		defer sharedBrowserMu.Unlock()

		// Look for Chromium in the usual places. If PATH doesn't have one,
		// rod has its own downloader — but we prefer system-installed.
		bin, ok := detectChromium()
		var u string
		if ok {
			// Point rod at the system chromium so we don't download again.
			u = launcher.New().
				Bin(bin).
				Headless(true).
				NoSandbox(true). // required for root / some VPS
				Set("disable-blink-features", "AutomationControlled").
				Set("disable-dev-shm-usage").
				Set("disable-gpu").
				Set("disable-features", "IsolateOrigins,site-per-process").
				MustLaunch()
		} else {
			// Fall back to rod-downloaded Chromium. This downloads ~130 MB
			// into ~/.cache/rod on first use. Only happens if the system
			// has no chromium at all.
			u = launcher.New().
				Headless(true).
				NoSandbox(true).
				Set("disable-blink-features", "AutomationControlled").
				Set("disable-dev-shm-usage").
				Set("disable-gpu").
				MustLaunch()
		}

		browser := rod.New().ControlURL(u)
		if err := browser.Connect(); err != nil {
			sharedBrowserErr = fmt.Errorf("gagal connect ke Chromium: %w", err)
			return
		}
		sharedBrowser = browser
	})
	return sharedBrowser, sharedBrowserErr
}

// detectChromium checks common install paths.
func detectChromium() (string, bool) {
	candidates := []string{
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
		"/snap/bin/chromium",
		"/usr/bin/google-chrome",
		"/usr/bin/google-chrome-stable",
		"/opt/google/chrome/chrome",
		// macOS
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, true
		}
	}
	return "", false
}

// fetchHeadless navigates to url in a stealth headless Chromium, waits for
// the document to settle, and returns the rendered plain text (tags
// stripped). waitMillis is the extra wait after DOMContentLoaded to give
// JS challenges time to pass (default 5000ms).
//
// Returns an error if Chromium cannot start. If the page loads but is
// detected as a challenge / block page, the caller will still get some
// text — decision to retry or give up is theirs.
func fetchHeadless(url string, maxChars int, waitMillis int) (string, error) {
	if maxChars <= 0 || maxChars > 200000 {
		maxChars = 20000
	}
	if waitMillis <= 0 {
		waitMillis = 5000
	}
	if waitMillis > 30000 {
		waitMillis = 30000
	}

	browser, err := ensureHeadlessBrowser()
	if err != nil {
		return "", err
	}

	// Fresh page per request — shared context would leak cookies.
	// Apply stealth.JS to remove navigator.webdriver and other bot tells.
	page, err := stealth.Page(browser)
	if err != nil {
		return "", fmt.Errorf("gagal buat page: %w", err)
	}
	defer page.MustClose()

	// Realistic Chrome UA so server-side sniffers don't flag us.
	_ = page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
		UserAgent: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	})

	// Hard overall timeout so a stuck challenge never hangs a Telegram turn.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	page = page.Context(ctx)

	if err := page.Navigate(url); err != nil {
		return "", fmt.Errorf("gagal navigate: %w", err)
	}

	// Wait for DOMContentLoaded. Some pages never fire load (infinite
	// trackers) so we don't wait for full network idle.
	if err := page.WaitDOMStable(time.Duration(waitMillis)*time.Millisecond, 0.5); err != nil {
		// Non-fatal: dump whatever we have.
	}

	// If we're still on a Cloudflare challenge page, give it a bit more.
	html, _ := page.HTML()
	if looksLikeChallenge(html) {
		time.Sleep(time.Duration(waitMillis) * time.Millisecond)
		html, _ = page.HTML()
	}

	// Extract text from the rendered DOM. page.MustElement("body").Text()
	// would work but returns scripts/styles embedded if they set
	// visible:true — htmlToText is more consistent with web_fetch.
	text := htmlToText(html)

	info := detectPageInfo(html, url)
	return formatHeadlessResult(text, info, maxChars), nil
}

// looksLikeChallenge does a quick check for Cloudflare / Datadome markers.
func looksLikeChallenge(html string) bool {
	lower := strings.ToLower(html)
	markers := []string{
		"cf-browser-verification",
		"cf_chl_opt",
		"cf-chl-bypass",
		"checking your browser",
		"just a moment",
		"please wait while we verify",
		"datadome",
		"perimeterx",
		"_px3=",
	}
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

type pageInfo struct {
	URL       string
	Title     string
	Challenge bool
}

func detectPageInfo(html, url string) pageInfo {
	info := pageInfo{URL: url, Challenge: looksLikeChallenge(html)}
	if idx := strings.Index(strings.ToLower(html), "<title"); idx >= 0 {
		end := strings.Index(html[idx:], "</title>")
		if end > 0 {
			raw := html[idx : idx+end]
			if gt := strings.Index(raw, ">"); gt > 0 {
				info.Title = strings.TrimSpace(decodeEntities(raw[gt+1:]))
			}
		}
	}
	return info
}

func formatHeadlessResult(body string, info pageInfo, maxChars int) string {
	body = strings.TrimSpace(body)
	truncated := false
	if len(body) > maxChars {
		body = body[:maxChars]
		truncated = true
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Fetched (headless): %s\n", info.URL)
	if info.Title != "" {
		fmt.Fprintf(&sb, "Title: %s\n", info.Title)
	}
	if info.Challenge {
		sb.WriteString("⚠ Halaman terdeteksi challenge/anti-bot — content mungkin parsial.\n")
	}
	if truncated {
		fmt.Fprintf(&sb, "(dipotong ke %d karakter)\n", maxChars)
	}
	sb.WriteString("----\n")
	sb.WriteString(body)
	return sb.String()
}
