package browser

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var urlRE = regexp.MustCompile(`https?://[^\s]+`)

// IsBrowserPrompt returns true when prompt likely requests browser automation.
func IsBrowserPrompt(prompt string) bool {
	p := strings.ToLower(prompt)
	keywords := []string{
		"buka browser", "gunakan browser", "browser subagent", "ambil screenshot",
		"screenshot", "e2e", "visual", "navbar", "checkout", "klik tombol",
	}
	for _, k := range keywords {
		if strings.Contains(p, k) {
			return true
		}
	}
	return false
}

// Plan converts a natural language prompt into a small deterministic task plan.
func Plan(prompt string) (Task, error) {
	u := urlRE.FindString(prompt)
	if u == "" {
		return Task{}, errors.New("URL tidak ditemukan di prompt")
	}
	u = strings.TrimRight(u, ".,)]}")
	if _, err := url.ParseRequestURI(u); err != nil {
		return Task{}, err
	}
	t := Task{Prompt: prompt, URL: u, Viewport: Viewport{Name: "desktop", Width: 1440, Height: 900}}
	p := strings.ToLower(prompt)

	if strings.Contains(p, "mobile") {
		t.Viewport = Viewport{Name: "mobile", Width: 375, Height: 812}
	}

	t.Steps = append(t.Steps, Step{Action: "goto", Target: u})

	if strings.Contains(p, "username") || strings.Contains(p, "password") || strings.Contains(p, "login") {
		user := extractQuotedAfter(prompt, "username")
		pass := extractQuotedAfter(prompt, "password")
		if user != "" {
			t.Steps = append(t.Steps, Step{Action: "fill", Target: "username", Value: user})
		}
		if pass != "" {
			t.Steps = append(t.Steps, Step{Action: "fill", Target: "password", Value: pass})
		}
		t.Steps = append(t.Steps, Step{Action: "click", Target: "Login"})
		t.Steps = append(t.Steps, Step{Action: "wait", Target: "dashboard"})
		t.Steps = append(t.Steps, Step{Action: "screenshot", Name: "dashboard"})
	} else if strings.Contains(p, "navbar") {
		t.Steps = append(t.Steps, Step{Action: "screenshot", Target: "navbar", Name: "navbar-" + t.Viewport.Name})
	} else if strings.Contains(p, "bayar") || strings.Contains(p, "checkout") {
		t.Steps = append(t.Steps, Step{Action: "click", Target: "Bayar"})
		t.Steps = append(t.Steps, Step{Action: "wait", Target: "error"})
		t.Steps = append(t.Steps, Step{Action: "screenshot", Target: "error", Name: "checkout-error"})
	} else {
		t.Steps = append(t.Steps, Step{Action: "screenshot", Name: "screenshot"})
	}
	return t, nil
}

func extractQuotedAfter(s, key string) string {
	lower := strings.ToLower(s)
	idx := strings.Index(lower, strings.ToLower(key))
	if idx < 0 {
		return ""
	}
	part := s[idx:]
	re := regexp.MustCompile(`['\"]([^'\"]+)['\"]`)
	m := re.FindStringSubmatch(part)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}
