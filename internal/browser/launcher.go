package browser

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

func launchBrowser(opt Options) (*rod.Browser, func(), error) {
	launch := launcher.New().Headless(!opt.Headful)
	u, err := launch.Launch()
	if err != nil {
		return nil, func() {}, err
	}
	browser := rod.New().ControlURL(u).Timeout(opt.Timeout)
	if err := browser.Connect(); err != nil {
		return nil, func() {}, err
	}
	return browser, func() { _ = browser.Close() }, nil
}
