package browser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func Run(ctx context.Context, task Task, opt Options) (Result, error) {
	if opt.Timeout <= 0 {
		opt.Timeout = 30 * time.Second
	}
	if opt.ArtifactRoot == "" {
		home, _ := os.UserHomeDir()
		opt.ArtifactRoot = filepath.Join(home, ".smara", "artifacts", "browser-runs")
	}
	id := time.Now().Format("20060102-150405")
	artifactDir := filepath.Join(opt.ArtifactRoot, id)
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		return Result{}, err
	}
	res := Result{ID: id, Prompt: task.Prompt, URL: task.URL, ArtifactDir: artifactDir, ReportPath: filepath.Join(artifactDir, "report.md"), Status: "pass", StartedAt: time.Now()}

	if err := CheckServer(ctx, task.URL, 5*time.Second); err != nil {
		res.Status = "fail"
		res.FinishedAt = time.Now()
		_ = WriteReport(res)
		return res, err
	}

	browser, cleanup, err := launchBrowser(opt)
	if err != nil {
		res.Status = "fail"
		res.FinishedAt = time.Now()
		_ = WriteReport(res)
		return res, err
	}
	defer cleanup()

	page := browser.MustPage("")
	if task.Viewport.Width > 0 && task.Viewport.Height > 0 {
		_ = page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{Width: task.Viewport.Width, Height: task.Viewport.Height, DeviceScaleFactor: 1, Mobile: task.Viewport.Name == "mobile"})
	}
	attachDiagnostics(page, &res)

	for _, step := range task.Steps {
		sr := StepResult{Step: step, Status: "pass"}
		if err := runStep(page, step, artifactDir, &res); err != nil {
			sr.Status = "fail"
			sr.Error = err.Error()
			res.Status = "fail"
		}
		res.Steps = append(res.Steps, sr)
		if sr.Status == "fail" && step.Action != "wait" {
			break
		}
	}
	res.FinishedAt = time.Now()
	if res.ScreenshotPath == "" {
		path := filepath.Join(artifactDir, "screenshot.png")
		if b, err := page.Screenshot(true, nil); err == nil {
			_ = os.WriteFile(path, b, 0644)
			res.ScreenshotPath = path
		}
	}
	return res, WriteReport(res)
}

func runStep(page *rod.Page, step Step, dir string, res *Result) error {
	switch step.Action {
	case "goto":
		if err := page.Navigate(step.Target); err != nil {
			return err
		}
		_ = page.WaitLoad()
		return nil
	case "fill":
		el, err := findInput(page, step.Target)
		if err != nil {
			return err
		}
		return el.Input(step.Value)
	case "click":
		var el *rod.Element
		var err error
		if step.Target == "youtube:first-video" {
			el, err = findYouTubeFirstVideo(page)
		} else {
			el, err = findByText(page, step.Target)
		}
		if err != nil {
			return err
		}
		if err := el.Click("left", 1); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		return nil
	case "wait":
		return waitText(page, step.Target, 5*time.Second)
	case "screenshot":
		name := step.Name
		if name == "" {
			name = "screenshot"
		}
		path := filepath.Join(dir, safeFileName(name)+".png")
		b, err := screenshotForTarget(page, step.Target)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, b, 0644); err != nil {
			return err
		}
		res.ScreenshotPath = path
		return nil
	default:
		return fmt.Errorf("aksi tidak dikenal: %s", step.Action)
	}
}

func attachDiagnostics(page *rod.Page, res *Result) {
	go page.EachEvent(func(e *proto.RuntimeConsoleAPICalled) {
		if e.Type == proto.RuntimeConsoleAPICalledTypeError || e.Type == proto.RuntimeConsoleAPICalledTypeAssert {
			res.ConsoleErrors = append(res.ConsoleErrors, fmt.Sprintf("%s: %s", e.Type, page.MustObjectsToJSON(e.Args)))
		}
	}, func(e *proto.NetworkLoadingFailed) {
		if e.ErrorText != "" {
			res.NetworkErrors = append(res.NetworkErrors, fmt.Sprintf("%s: %s", e.RequestID, e.ErrorText))
		}
	})()
}

func screenshotForTarget(page *rod.Page, target string) ([]byte, error) {
	t := strings.ToLower(strings.TrimSpace(target))
	var selectors []string
	switch t {
	case "navbar", "nav", "navigation":
		selectors = []string{`nav`, `[role="navigation"]`, `[class*="navbar" i]`, `[id*="navbar" i]`, `header`}
	case "error", "errors", "pesan error":
		selectors = []string{`[role="alert"]`, `[aria-live]`, `.error`, `.errors`, `.invalid-feedback`, `[class*="error" i]`, `[class*="danger" i]`, `[class*="red" i]`}
	}
	for _, sel := range selectors {
		if el, err := page.Element(sel); err == nil {
			if b, err := el.Screenshot(proto.PageCaptureScreenshotFormatPng, 90); err == nil {
				return b, nil
			}
		}
	}
	return page.Screenshot(true, nil)
}

func findInput(page *rod.Page, name string) (*rod.Element, error) {
	selectors := []string{
		fmt.Sprintf(`input[name="%s" i]`, name),
		fmt.Sprintf(`input[id="%s" i]`, name),
		fmt.Sprintf(`input[placeholder*="%s" i]`, name),
		fmt.Sprintf(`textarea[name="%s" i]`, name),
	}
	if strings.Contains(strings.ToLower(name), "password") {
		selectors = append([]string{`input[type="password"]`}, selectors...)
	}
	for _, sel := range selectors {
		if el, err := page.Element(sel); err == nil {
			return el, nil
		}
	}
	return nil, fmt.Errorf("input %s tidak ditemukan", name)
}

func findByText(page *rod.Page, text string) (*rod.Element, error) {
	if el, err := page.ElementR("button,input,a", text); err == nil {
		return el, nil
	}
	if el, err := page.ElementR("*", text); err == nil {
		return el, nil
	}
	return nil, fmt.Errorf("elemen teks %q tidak ditemukan", text)
}

func findYouTubeFirstVideo(page *rod.Page) (*rod.Element, error) {
	selectors := []string{
		`a#thumbnail[href^="/watch"]`,
		`ytd-rich-item-renderer a#thumbnail`,
		`a[href^="/watch?v="]`,
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		for _, sel := range selectors {
			els, err := page.Elements(sel)
			if err != nil || len(els) == 0 {
				continue
			}
			for _, el := range els {
				href, _ := el.Attribute("href")
				if href == nil || !strings.Contains(*href, "watch") {
					continue
				}
				_ = el.ScrollIntoView()
				return el, nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil, fmt.Errorf("video pertama YouTube tidak ditemukan")
}

func waitText(page *rod.Page, text string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	lower := strings.ToLower(text)
	for time.Now().Before(deadline) {
		if _, err := page.ElementR("*", text); err == nil {
			return nil
		}
		if lower == "error" {
			for _, sel := range []string{`[role="alert"]`, `.error`, `.invalid-feedback`, `[class*="error" i]`, `[class*="danger" i]`, `[class*="red" i]`} {
				if _, err := page.Element(sel); err == nil {
					return nil
				}
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("teks/elemen %q tidak muncul", text)
}

func safeFileName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	repl := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-")
	return repl.Replace(s)
}
