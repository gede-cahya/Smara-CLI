package browser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

type ErrorCheck struct {
	Name           string   `json:"name"`
	Target         string   `json:"target"`
	ScreenshotPath string   `json:"screenshot_path"`
	FoundError     bool     `json:"found_error"`
	HasRedStyle    bool     `json:"has_red_style"`
	Text           string   `json:"text,omitempty"`
	Selectors      []string `json:"selectors,omitempty"`
	Status         string   `json:"status"`
	Error          string   `json:"error,omitempty"`
}

func runErrorCheck(page *rod.Page, step Step, dir string, res *Result) error {
	target := step.Target
	if target == "" {
		target = "error"
	}
	check := ErrorCheck{Name: safeFileName(target), Target: target, Status: "pass"}

	el, selector, err := findErrorElement(page, 3*time.Second)
	if err != nil {
		check.Status = "fail"
		check.Error = err.Error()
		res.Status = "fail"
		path := filepath.Join(dir, safeFileName(target)+"-not-found.png")
		if b, serr := page.Screenshot(true, nil); serr == nil {
			_ = os.WriteFile(path, b, 0644)
			check.ScreenshotPath = path
			res.ScreenshotPath = path
		}
	} else {
		check.FoundError = true
		check.Selectors = []string{selector}
		check.Text = strings.TrimSpace(el.MustText())
		check.HasRedStyle = elementLooksRed(page, el)
		if !check.HasRedStyle {
			check.Status = "needs_review"
			if res.Status == "pass" {
				res.Status = "needs_review"
			}
		}
		path := filepath.Join(dir, safeFileName(target)+".png")
		if b, serr := el.Screenshot(proto.PageCaptureScreenshotFormatPng, 90); serr == nil {
			_ = os.WriteFile(path, b, 0644)
			check.ScreenshotPath = path
			res.ScreenshotPath = path
		} else if b, serr := page.Screenshot(true, nil); serr == nil {
			_ = os.WriteFile(path, b, 0644)
			check.ScreenshotPath = path
			res.ScreenshotPath = path
		}
	}

	res.ErrorChecks = append(res.ErrorChecks, check)
	path := filepath.Join(dir, "error-check.json")
	data, err := json.MarshalIndent(res.ErrorChecks, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	res.ErrorCheckPath = path
	return nil
}

func findErrorElement(page *rod.Page, timeout time.Duration) (*rod.Element, string, error) {
	selectors := selectorsForTarget("error")
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, sel := range selectors {
			els, err := page.Elements(sel)
			if err != nil {
				continue
			}
			for _, el := range els {
				visible, _ := el.Visible()
				text := strings.TrimSpace(el.MustText())
				if visible && (text != "" || strings.Contains(strings.ToLower(sel), "error") || strings.Contains(strings.ToLower(sel), "red")) {
					return el, sel, nil
				}
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil, "", fmt.Errorf("pesan error tidak ditemukan")
}

func elementLooksRed(page *rod.Page, el *rod.Element) bool {
	obj, err := page.Evaluate(rod.Eval(`(el) => {
		const cs = window.getComputedStyle(el);
		const color = (cs.color || '') + ' ' + (cs.backgroundColor || '') + ' ' + (cs.borderColor || '') + ' ' + (el.className || '');
		return /red|danger|invalid|error|rgb\(\s*(1[5-9][0-9]|2[0-5][0-9])\s*,\s*([0-9]{1,2}|1[0-2][0-9])\s*,\s*([0-9]{1,2}|1[0-2][0-9])\s*\)/i.test(color);
	}`, el))
	if err != nil || obj == nil {
		return false
	}
	return obj.Value.Bool()
}
