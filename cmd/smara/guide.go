package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	guideTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1).
			MarginBottom(1)

	guideStepStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#36C5F0")).
			Bold(true)

	guideCodeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A6E22E")).
			Background(lipgloss.Color("#222222")).
			Padding(0, 1)

	guideBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#444444")).
			Padding(1, 2).
			MarginBottom(1)
)

var guideCmd = &cobra.Command{
	Use:   "guide",
	Short: "Panduan interaktif (Walkthrough) fitur Smara",
	Long:  `Tampilkan panduan langkah-demi-langkah untuk memahami cara kerja Smara CLI.`,
	Run: func(cmd *cobra.Command, args []string) {
		showGuide()
	},
}

func showGuide() {
	fmt.Println(guideTitleStyle.Render(" 📖 Panduan Penggunaan Smara CLI "))

	steps := []struct {
		title string
		desc  string
		cmd   string
	}{
		{
			"1. Inisialisasi & Login",
			"Langkah pertama adalah mengatur provider LLM favoritmu (OpenRouter, OpenAI, dll).",
			"smara login --provider openrouter",
		},
		{
			"2. Memilih Model",
			"Pilih model yang ingin digunakan. Gunakan UI interaktif untuk navigasi.",
			"smara provider",
		},
		{
			"3. Memulai Sesi Interaktif",
			"Masuk ke terminal Smara untuk mulai chatting dengan agen otonom.",
			"smara start",
		},
		{
			"4. Mode Agen",
			"Gunakan flag --mode untuk mengubah perilaku agen (ask, rush, plan, test).",
			"smara start --mode test",
		},
		{
			"5. Memori & Konteks",
			"Smara secara otomatis menyimpan memori. Kamu bisa mengelola memori secara manual.",
			"smara memory list",
		},
	}

	for _, s := range steps {
		content := fmt.Sprintf("%s\n%s\n\nContoh: %s",
			guideStepStyle.Render(s.title),
			s.desc,
			guideCodeStyle.Render(s.cmd),
		)
		fmt.Println(guideBoxStyle.Render(content))
	}

	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("\nTip: Ketik 'smara help [perintah]' untuk detail lebih lanjut."))
	fmt.Println(strings.Repeat("─", 50))
}
