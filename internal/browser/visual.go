package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

var responsiveViewports = []Viewport{
	{Name: "mobile", Width: 375, Height: 812},
	{Name: "tablet", Width: 768, Height: 1024},
	{Name: "desktop", Width: 1440, Height: 900},
}

type VisualCheck struct {
	Name               string   `json:"name"`
	Target             string   `json:"target"`
	Viewport           Viewport `json:"viewport"`
	ScreenshotPath     string   `json:"screenshot_path"`
	FoundTarget        bool     `json:"found_target"`
	HorizontalOverflow bool     `json:"horizontal_overflow"`
	Status             string   `json:"status"`
	Error              string   `json:"error,omitempty"`
}

func runResponsiveVisualCheck(page *rod.Page, step Step, dir string, res *Result) error {
	target := step.Target
	if target == "" {
		target = "navbar"
	}
	checks := make([]VisualCheck, 0, len(responsiveViewports))
	for _, vp := range responsiveViewports {
		_ = page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{Width: vp.Width, Height: vp.Height, DeviceScaleFactor: 1, Mobile: vp.Name == "mobile"})
		_ = page.WaitLoad()
		check := VisualCheck{Name: target + "-" + vp.Name, Target: target, Viewport: vp, Status: "pass"}
		b, found, err := screenshotTargetWithFound(page, target)
		if err != nil {
			check.Status = "fail"
			check.Error = err.Error()
		} else {
			check.FoundTarget = found
			check.HorizontalOverflow = hasHorizontalOverflow(page)
			if !found || check.HorizontalOverflow {
				check.Status = "needs_review"
			}
			path := filepath.Join(dir, safeFileName(check.Name)+".png")
			if werr := os.WriteFile(path, b, 0644); werr != nil {
				check.Status = "fail"
				check.Error = werr.Error()
			} else {
				check.ScreenshotPath = path
				res.ScreenshotPath = path
			}
		}
		checks = append(checks, check)
		if check.Status == "fail" {
			res.Status = "fail"
		} else if check.Status == "needs_review" && res.Status == "pass" {
			res.Status = "needs_review"
		}
	}
	res.VisualChecks = append(res.VisualChecks, checks...)
	path := filepath.Join(dir, "visual-check.json")
	data, err := json.MarshalIndent(checks, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	res.VisualCheckPath = path
	return nil
}

func screenshotTargetWithFound(page *rod.Page, target string) ([]byte, bool, error) {
	for _, sel := range selectorsForTarget(target) {
		if el, err := page.Element(sel); err == nil {
			if b, err := el.Screenshot(proto.PageCaptureScreenshotFormatPng, 90); err == nil {
				return b, true, nil
			}
		}
	}
	b, err := page.Screenshot(true, nil)
	return b, false, err
}

func selectorsForTarget(target string) []string {
	switch normalizeTarget(target) {
	case "navbar", "nav", "navigation":
		return []string{`nav`, `[role="navigation"]`, `[class*="navbar" i]`, `[id*="navbar" i]`, `[class*="nav" i]`, `[id*="nav" i]`, `header`}
	case "error", "errors", "pesan error":
		return []string{`[role="alert"]`, `[aria-live]`, `.error`, `.errors`, `.invalid-feedback`, `[class*="error" i]`, `[class*="danger" i]`, `[class*="red" i]`}
	default:
		return nil
	}
}

func normalizeTarget(target string) string { return strings.ToLower(strings.TrimSpace(target)) }

func hasHorizontalOverflow(page *rod.Page) bool {
	obj, err := page.Eval(`() => document.documentElement.scrollWidth > window.innerWidth || document.body.scrollWidth > window.innerWidth`)
	if err != nil || obj == nil {
		return false
	}
	return obj.Value.Bool()
}
