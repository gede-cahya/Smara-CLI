package ui

import (
	"fmt"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func ShowProviderSelector() error {
	cfg := config.Get()
	providers := llm.AvailableProviders()

	app := tview.NewApplication()
	pages := tview.NewPages()

	// Provider list view
	listView := tview.NewList()
	for name, info := range providers {
		icon := "○"
		if cfg.Provider == name {
			icon = "◉"
		}
		desc := info.Description
		if name == "custom" && cfg.CustomProviderName != "" {
			desc = fmt.Sprintf("%s (%s)", info.Description, cfg.CustomProviderName)
		}
		listView.AddItem(fmt.Sprintf("%s %s", icon, name), desc, 0, nil)
	}

	listView.SetSelectedFunc(func(index int, mainText, secondaryText string, shortcut rune) {
		providerNames := []string{"ollama", "openai", "openrouter", "anthropic", "custom"}
		if index >= 0 && index < len(providerNames) {
			selected := providerNames[index]
			showModelSelector(app, pages, selected, cfg)
		}
	})

	header := tview.NewTextView().
		SetText("🌀 Select Provider").
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true)

	footer := tview.NewTextView().
		SetText("↑↓ Navigate  Enter Select  Esc Exit").
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tview.Styles.SecondaryTextColor)

	pages.AddPage("main", tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 3, 0, false).
		AddItem(listView, 0, 1, true).
		AddItem(footer, 1, 0, false), true, true)

	app.SetRoot(pages, true)
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			app.Stop()
		}
		return event
	})

	return app.Run()
}

func showModelSelector(app *tview.Application, pages *tview.Pages, provider string, cfg *config.SmaraConfig) {
	info := llm.AvailableProviders()[provider]

	// Check if API key is needed
	if info.NeedsAPIKey {
		key := getAPIKeyForProvider(provider, cfg)
		if key == "" && provider != "custom" {
			pages.ShowPage("main")
			modal := tview.NewModal().
				SetText(fmt.Sprintf("Please login first:\nsmara login --provider %s", provider)).
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					pages.ShowPage("main")
				})
			pages.AddPage("error", modal, false, true)
			pages.ShowPage("error")
			return
		}
		if provider == "custom" && cfg.CustomAPIKey == "" {
			pages.ShowPage("main")
			modal := tview.NewModal().
				SetText("Please login first:\nsmara login --custom").
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					pages.ShowPage("main")
				})
			pages.AddPage("error", modal, false, true)
			pages.ShowPage("error")
			return
		}
	}

	var models []string
	if provider == "custom" {
		if cfg.CustomModel != "" {
			models = []string{cfg.CustomModel}
		} else {
			models = []string{"Enter model name..."}
		}
	} else {
		models = info.Models
	}

	listView := tview.NewList()
	for _, model := range models {
		listView.AddItem(model, "", 0, nil)
	}

	header := tview.NewTextView().
		SetText(fmt.Sprintf("Select Model for %s", provider)).
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true)

	footer := tview.NewTextView().
		SetText("↑↓ Navigate  Enter Select  Esc Back").
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tview.Styles.SecondaryTextColor)

	modelPage := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 3, 0, false).
		AddItem(listView, 0, 1, true).
		AddItem(footer, 1, 0, false)

	listView.SetSelectedFunc(func(index int, mainText, secondaryText string, shortcut rune) {
		selectedModel := mainText

		// Save provider and model
		config.Set("provider", provider)
		config.Set("model", selectedModel)

		// Save provider-specific model
		modelKey := modelConfigKey(provider)
		if modelKey != "" {
			config.Set(modelKey, selectedModel)
		}

		modal := tview.NewModal().
			SetText(fmt.Sprintf("✓ Provider: %s\n✓ Model: %s", provider, selectedModel)).
			AddButtons([]string{"Done"}).
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				app.Stop()
			})
		pages.AddPage("success", modal, false, true)
		pages.ShowPage("success")
	})

	pages.AddPage("models", modelPage, false, true)
	pages.ShowPage("models")
}

func getAPIKeyForProvider(name string, cfg *config.SmaraConfig) string {
	switch name {
	case "openai":
		return cfg.OpenAIAPIKey
	case "openrouter":
		return cfg.OpenRouterAPIKey
	case "anthropic":
		return cfg.AnthropicAPIKey
	case "custom":
		return cfg.CustomAPIKey
	}
	return ""
}

func modelConfigKey(name string) string {
	switch name {
	case "openai":
		return "openai_model"
	case "openrouter":
		return "openrouter_model"
	case "anthropic":
		return "anthropic_model"
	case "custom":
		return "custom_model"
	}
	return ""
}
