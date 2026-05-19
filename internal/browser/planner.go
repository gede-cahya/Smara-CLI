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
		"klik ", "click ", "tunggu ", "wait ", "open http", "buka http",
		"video pertama", "salah satu video",
	}
	for _, k := range keywords {
		if strings.Contains(p, k) {
			return true
		}
	}
	return urlRE.MatchString(prompt) && (strings.Contains(p, "buka") || strings.Contains(p, "open"))
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

	customSteps := extractInteractionSteps(prompt)
	if len(customSteps) > 0 {
		t.Steps = append(t.Steps, customSteps...)
		if wantsScreenshot(p) && !hasStepAction(customSteps, "screenshot") && !hasStepAction(customSteps, "error-check") {
			t.Steps = append(t.Steps, Step{Action: "screenshot", Name: "screenshot"})
		}
		return t, nil
	}

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
		if wantsResponsiveCheck(p) {
			t.Steps = append(t.Steps, Step{Action: "visual-check", Target: "navbar", Name: "navbar-responsive"})
		} else {
			t.Steps = append(t.Steps, Step{Action: "screenshot", Target: "navbar", Name: "navbar-" + t.Viewport.Name})
		}
	} else if strings.Contains(p, "bayar") || strings.Contains(p, "checkout") {
		t.Steps = append(t.Steps, Step{Action: "click", Target: "Bayar"})
		t.Steps = append(t.Steps, Step{Action: "error-check", Target: "checkout-error", Name: "checkout-error"})
	} else {
		t.Steps = append(t.Steps, Step{Action: "screenshot", Name: "screenshot"})
	}
	return t, nil
}

func extractInteractionSteps(prompt string) []Step {
	var steps []Step
	p := strings.ToLower(prompt)
	if strings.Contains(p, "salah satu video") || strings.Contains(p, "video pertama") || strings.Contains(p, "first video") {
		steps = append(steps, Step{Action: "click", Target: "youtube:first-video"})
	}
	if strings.Contains(p, "bayar") || strings.Contains(p, "checkout") {
		steps = append(steps, Step{Action: "click", Target: "Bayar"})
		steps = append(steps, Step{Action: "error-check", Target: "checkout-error", Name: "checkout-error"})
		return steps
	}
	for _, target := range extractCommandTargets(prompt, []string{"klik tombol", "klik", "click button", "click"}) {
		steps = append(steps, Step{Action: "click", Target: target})
	}
	for _, target := range extractCommandTargets(prompt, []string{"tunggu teks", "tunggu", "wait for", "wait"}) {
		steps = append(steps, Step{Action: "wait", Target: target})
	}
	return steps
}

func extractCommandTargets(prompt string, commands []string) []string {
	var out []string
	lower := strings.ToLower(prompt)
	stops := []string{" lalu ", " dan ", " kemudian ", " setelah itu ", " ambil ", " screenshot", ",", ".", " then ", " and ", " wait ", " tunggu ", " klik ", " click "}
	for _, cmd := range commands {
		searchFrom := 0
		for {
			idx := strings.Index(lower[searchFrom:], cmd)
			if idx < 0 {
				break
			}
			idx += searchFrom
			start := idx + len(cmd)
			if start < len(prompt) && prompt[start] != ' ' && prompt[start] != '\t' && prompt[start] != ':' {
				searchFrom = start
				continue
			}
			rest := strings.TrimLeft(prompt[start:], " \t:-=\"'`")
			restLower := strings.ToLower(rest)
			end := len(rest)
			for _, stop := range stops {
				if pos := strings.Index(restLower, stop); pos >= 0 && pos < end {
					end = pos
				}
			}
			target := strings.Trim(rest[:end], " \t\n\r`'\".,;:!?()[]{}")
			if target != "" && !looksLikeURL(target) {
				out = append(out, target)
			}
			searchFrom = start
		}
	}
	return dedupeStrings(out)
}

func wantsScreenshot(p string) bool {
	return strings.Contains(p, "screenshot") || strings.Contains(p, "tangkapan layar") || strings.Contains(p, "ambil gambar")
}

func wantsResponsiveCheck(p string) bool {
	return strings.Contains(p, "responsif") || strings.Contains(p, "responsive") || strings.Contains(p, "mobile") || strings.Contains(p, "tablet") || strings.Contains(p, "desktop")
}

func hasStepAction(steps []Step, action string) bool {
	for _, s := range steps {
		if s.Action == action {
			return true
		}
	}
	return false
}

func looksLikeURL(s string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(s)), "http")
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		key := strings.ToLower(v)
		if !seen[key] {
			seen[key] = true
			out = append(out, v)
		}
	}
	return out
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
