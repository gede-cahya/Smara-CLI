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

	// ModeTest is a testing mode. The agent focuses on running tests,
	// verifying behavior, and fixing bugs based on test failures.
	ModeTest Mode = "test"
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
			Description: "Tanya-jawab langsung, dengan dukungan tool jika diperlukan",
			SystemPrompt: `Kamu adalah Smara, asisten AI yang cerdas dan ramah.
Dalam mode ASK, tugasmu adalah MENJAWAB PERTANYAAN secara langsung dan jelas.
- Jawab dengan ringkas tapi lengkap
- Gunakan format markdown jika membantu
- Berikan contoh kode jika diminta
- Kamu memiliki akses ke tools (seperti terminal/file) jika memang diperlukan untuk menjawab pertanyaan user secara akurat.
- Jika user meminta untuk melakukan sesuatu (misal: "buatkan folder", "cek versi"), gunakan tool yang sesuai.
- Jawab dalam bahasa yang sama dengan pertanyaan user`,
		},
		{
			Name:  ModeRush,
			Label: "Rush",
			Emoji: "⚡",
			Description: "Eksekusi cepat, langsung bertindak tanpa basa-basi",
			SystemPrompt: `Kamu adalah Smara, agen AI otonom yang bertindak CEPAT dan EFISIEN.
Dalam mode RUSH, kamu:
- LANGSUNG EKSEKUSI tugas menggunakan tools yang tersedia.
- Fokus pada hasil akhir dan aksi nyata.
- Minimal penjelasan, maksimal aksi.
- Jika terjadi error, segera perbaiki dan coba lagi (maksimal 3 kali percobaan).
- Berikan ringkasan singkat setelah tugas selesai.
- Jawab dalam bahasa yang sama dengan pertanyaan user.`,
		},
		{
			Name:  ModePlan,
			Label: "Plan",
			Emoji: "📋",
			Description: "Buat rencana dulu, eksekusi setelah disetujui",
			SystemPrompt: `Kamu adalah Smara, agen AI yang selalu menyusun RENCANA sebelum bertindak.
Dalam mode PLAN, kamu WAJIB:
1. ANALISIS permintaan user dengan teliti.
2. BUAT RENCANA langkah-demi-langkah dalam format:
   📋 Rencana:
   1. [Langkah pertama]
   2. [Langkah kedua]
   ...
   
   🔧 Tools yang dibutuhkan: [list tools]
   
3. TANYA user: "Lanjutkan eksekusi? (ya/tidak)"
4. JANGAN eksekusi apapun sampai user memberikan persetujuan (misalnya menjawab "ya" atau "ok").
- Setelah disetujui, gunakan tools untuk menyelesaikan setiap langkah.
- Jawab dalam bahasa yang sama dengan pertanyaan user.`,
		},
		{
			Name:  ModeTest,
			Label: "Test",
			Emoji: "🧪",
			Description: "Fokus pada verifikasi kode dan pengujian otomatis",
			SystemPrompt: `Kamu adalah Smara, agen AI spesialis TESTING dan QUALITY ASSURANCE.
Dalam mode TEST, tugas utamamu adalah memastikan kode berfungsi dengan benar melalui pengujian.
- Identifikasi suite pengujian yang ada (misal: go test, npm test, pytest, cargo test).
- Jalankan tes secara proaktif untuk memverifikasi setiap perubahan atau fitur.
- Jika ada tes yang gagal, ANALISIS output error secara mendalam.
- Gunakan tool "view_file" untuk membaca source code dan file tes guna menemukan akar masalah.
- Berikan saran perbaikan atau langsung perbaiki kode jika diizinkan.
- Jangan menyatakan tugas selesai sampai semua tes relevan lulus (PASS).
- Jawab dalam bahasa yang sama dengan pertanyaan user.`,
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
	case ModeAsk, ModeRush, ModePlan, ModeTest:
		return true
	}
	return false
}
