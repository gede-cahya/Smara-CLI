package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

// buildSkillContext returns a system-prompt fragment that tells the LLM
// about the skill builtin tools (skill_list, skill_create, skill_run,
// skill_delete) and lists any existing skills so the model can reuse them.
const orchestrationRuleSkillName = "parallel-orchestration-rules"

func buildSkillContext() string {
	var sb strings.Builder
	sb.WriteString("\n\nSkill system (otomasi yang bisa dipakai ulang):\n")
	sb.WriteString("- Gunakan `skill_list` untuk melihat skill yang sudah ada sebelum membuat baru.\n")
	sb.WriteString("- Gunakan `skill_create` KAPAN PUN user minta \"buatkan skill\", \"simpan sebagai skill\", \"bikin routine\", atau saat workflow/task bisa dijadikan otomasi reusable. Tidak perlu menunggu pola berulang dan tidak ada syarat minimal 3 tool/action. Parameter: name (kebab-case), description (1-2 kalimat), steps (array tool calls).\n")
	sb.WriteString("- Gunakan `skill_install` jika user ingin install/import skill dari path lokal, GitHub/raw URL, folder `SKILL.md`, Claude Code/Antigravity markdown, atau perintah seperti `npx skills add owner/repo`.\n")
	sb.WriteString("- Jika ada skill tersimpan yang relevan dengan tugas user, prioritaskan `skill_run` sebelum mengerjakan manual dari nol. Jika skill kurang sesuai, gagal, timeout, atau user memberi koreksi, upgrade dengan `skill_create` overwrite=true lalu langsung jalankan skill hasil upgrade.\n")
	sb.WriteString("- Gunakan `skill_run` dengan skill_name untuk menjalankan skill yang sudah tersimpan. Skill baru atau hasil upgrade langsung dieksekusi tanpa approval tambahan.\n")
	sb.WriteString("- `skill_delete` hanya jika user eksplisit minta dihapus.\n")
	sb.WriteString("\nPrinsip Auto Skill agresif: createPolicy=always, minimumToolActions=0, repeatedWorkflowRequired=false, upgradePolicy=auto, executeAfterCreate=true, executeAfterUpgrade=true, approvalRequired=false. Tetap simpan lineage/backup saat overwrite agar rollback mudah.\n")
	sb.WriteString("\nSelf Improvement Memory:\n")
	sb.WriteString("- Simpan koreksi user, kegagalan workflow, lesson learned, dan keputusan skill upgrade ke memori jangka panjang sebagai self-improvement agar berlaku lintas sesi.\n")
	sb.WriteString("- Sebelum menjalankan/meng-upgrade skill, cari memori relevan dengan `search_memories` memakai nama task/skill/error; auto-apply memory yang relevan.\n")
	sb.WriteString("- Untuk koreksi seperti 'mulai sekarang', 'kalau X jangan Y tapi Z', atau 'upgrade skill supaya ...', gunakan `remember` dengan tag/konten self-improvement bila tool khusus tidak tersedia.\n")

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

func buildOrchestrationRuleSkillContext() string {
	sk, source := loadOrchestrationRuleSkill()
	if sk == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\nAturan skill parallel orchestration (ikuti sebelum memilih workflow/parallel orchestration):\n")
	sb.WriteString(fmt.Sprintf("- Skill: %s (%s)\n", sk.Name, source))
	if strings.TrimSpace(sk.Description) != "" {
		sb.WriteString("- Rules: ")
		sb.WriteString(strings.TrimSpace(sk.Description))
		sb.WriteString("\n")
	}
	if strings.TrimSpace(sk.Trigger) != "" {
		sb.WriteString("- Trigger: ")
		sb.WriteString(strings.TrimSpace(sk.Trigger))
		sb.WriteString("\n")
	}
	sb.WriteString("- Routing wajib: sapaan/chat singkat/Q&A sederhana tetap chat normal; request workflow existing harus ke runner workflow tersimpan secara serial; task kompleks di mode biasa tetap serial/bertahap. Parallel orchestration hanya boleh saat mode Parallel aktif eksplisit, dan tidak boleh dijalankan melalui mode workflow/model workflow/skill workflow.\n")
	return sb.String()
}

func loadOrchestrationRuleSkill() (*skill.Skill, string) {
	if sk, err := skill.Load(orchestrationRuleSkillName); err == nil && sk != nil {
		return sk, "user skill"
	}
	dir := skill.FindBundledSkillsDir()
	if dir == "" {
		return nil, ""
	}
	data, err := os.ReadFile(filepath.Join(dir, orchestrationRuleSkillName+".json"))
	if err != nil {
		return nil, ""
	}
	sk, err := skill.FromJSON(data)
	if err != nil {
		return nil, ""
	}
	return sk, "bundled skill"
}
