package agent

// Mode represents the agent's operating mode.
type Mode string

const (
	// ModeAsk is a simple Q&A mode. The agent answers questions directly
	// without executing tasks or using tools. Fastest response time.
	ModeAsk Mode = "ask"

	// ModeRush is a fast execution mode. The agent acts immediately
	// with minimal planning — executes tools and writes code directly.
	ModeRush Mode = "rush"

	// ModePlan is a planning mode. The agent first creates a step-by-step
	// plan, presents it to the user for approval, then executes each step.
	ModePlan Mode = "plan"
)

// ModeInfo holds metadata about an agent mode.
type ModeInfo struct {
	Name        Mode
	Label       string
	Emoji       string
	Description string
	SystemPrompt string
}

// AllModes returns info for all available modes.
func AllModes() []ModeInfo {
	return []ModeInfo{
		{
			Name:  ModeAsk,
			Label: "Ask",
			Emoji: "💬",
			Description: "Tanya-jawab langsung, tanpa eksekusi tool",
			SystemPrompt: `Kamu adalah Smara, asisten AI yang cerdas dan ramah.
Dalam mode ASK, tugasmu adalah MENJAWAB PERTANYAAN secara langsung dan jelas.
- Jawab dengan ringkas tapi lengkap
- Gunakan format markdown jika membantu
- Berikan contoh kode jika diminta
- JANGAN jalankan tool atau eksekusi perintah
- Jawab dalam bahasa yang sama dengan pertanyaan user`,
		},
		{
			Name:  ModeRush,
			Label: "Rush",
			Emoji: "⚡",
			Description: "Eksekusi cepat, langsung bertindak tanpa basa-basi",
			SystemPrompt: `Kamu adalah Smara, agen AI otonom yang bertindak CEPAT dan EFISIEN.
Dalam mode RUSH, kamu:
- LANGSUNG EKSEKUSI tugas tanpa bertele-tele
- Gunakan tools/MCP yang tersedia untuk menyelesaikan tugas
- Minimal penjelasan, maksimal aksi
- Jika error, langsung perbaiki dan coba lagi (max 3x retry)
- Output singkat: apa yang dilakukan dan hasilnya
- Jawab dalam bahasa yang sama dengan pertanyaan user`,
		},
		{
			Name:  ModePlan,
			Label: "Plan",
			Emoji: "📋",
			Description: "Buat rencana dulu, eksekusi setelah disetujui",
			SystemPrompt: `Kamu adalah Smara, agen AI yang menyusun RENCANA sebelum bertindak.
Dalam mode PLAN, kamu WAJIB:
1. ANALISIS permintaan user dengan teliti
2. BUAT RENCANA langkah-demi-langkah dalam format:
   📋 Rencana:
   1. [Langkah pertama]
   2. [Langkah kedua]
   ...
   
   ⏱️ Estimasi: [waktu]
   🔧 Tools yang dibutuhkan: [list tools]
   
3. TANYA user: "Lanjutkan eksekusi? (ya/tidak)"
4. JANGAN eksekusi apapun sampai user menyetujui
- Jawab dalam bahasa yang sama dengan pertanyaan user`,
		},
	}
}

// GetModeInfo returns info for a specific mode.
func GetModeInfo(mode Mode) ModeInfo {
	for _, m := range AllModes() {
		if m.Name == mode {
			return m
		}
	}
	// Default to ask
	return AllModes()[0]
}

// ValidMode checks if a mode string is valid.
func ValidMode(s string) bool {
	switch Mode(s) {
	case ModeAsk, ModeRush, ModePlan:
		return true
	}
	return false
}
