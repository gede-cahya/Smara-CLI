package agent

import (
	"fmt"
	"strings"

	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

// buildSkillContext returns a system-prompt fragment that tells the LLM
// about the skill builtin tools (skill_list, skill_create, skill_run,
// skill_delete) and lists any existing skills so the model can reuse them
// instead of rebuilding the same recipe, and proactively capture repeated
// workflows as new skills when the user asks to automate something.
//
// The output begins with "\n\n" so it can be concatenated onto an existing
// system prompt. It returns an empty string on any error so failures here
// never block a normal chat.
func buildSkillContext() string {
	var sb strings.Builder
	sb.WriteString("\n\nSkill system (otomatisasi yang bisa dipakai ulang):\n")
	sb.WriteString("- Gunakan `skill_list` untuk melihat skill yang sudah ada sebelum membuat baru.\n")
	sb.WriteString("- Gunakan `skill_create` KAPAN PUN user minta \"buatkan skill\", \"simpan sebagai skill\", \"bikin routine\", atau saat kamu mendeteksi pola perintah berulang (misal user sering minta hal yang sama). Parameter: name (kebab-case), description (1-2 kalimat), steps (array tool calls).\n")
	sb.WriteString("- Gunakan `skill_install` jika user ingin install/import skill dari path lokal, GitHub/raw URL, folder `SKILL.md`, Claude Code/Antigravity markdown, atau perintah seperti `npx skills add owner/repo`.\n")
	sb.WriteString("- Jika ada skill tersimpan yang relevan dengan tugas user, prioritaskan `skill_run` sebelum mengerjakan manual dari nol.\n")
	sb.WriteString("- Gunakan `skill_run` dengan skill_name untuk menjalankan skill yang sudah tersimpan.\n")
	sb.WriteString("- `skill_delete` hanya jika user eksplisit minta dihapus.\n")
	sb.WriteString("\nPrinsip proaktif: jika tugas terdiri dari 3+ tool calls yang kemungkinan besar akan diulang (monitoring service, deploy rutin, backup), tawarkan untuk menyimpannya sebagai skill. Jika user setuju atau sudah eksplisit minta, langsung panggil `skill_create` tanpa menunggu konfirmasi tambahan.\n")

	names, err := skill.List()
	if err == nil && len(names) > 0 {
		sb.WriteString("\nSkill tersimpan saat ini:\n")
		shown := 0
		for _, n := range names {
			if shown >= 80 {
				sb.WriteString(fmt.Sprintf("  ... dan %d skill lainnya\n", len(names)-shown))
				break
			}
			sk, err := skill.Load(n)
			if err != nil {
				continue
			}
			desc := sk.Description
			if len(desc) > 80 {
				desc = desc[:80] + "…"
			}
			sb.WriteString(fmt.Sprintf("  - %s — %s\n", sk.Name, desc))
			shown++
		}
	}
	return sb.String()
}
