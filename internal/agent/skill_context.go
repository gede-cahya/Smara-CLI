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
const autoSkillResultMarker = "[SMARA_AUTO_SKILL_RESULT]"

type autoSkillSelection struct {
	Skill          *skill.Skill
	Recommendation skill.Recommendation
}

func buildSkillContext() string {
	return buildSkillContextForMode(ModeAsk)
}

func buildSkillContextForMode(mode Mode) string {
	if mode == ModeWorkflow {
		return "\n\nWorkflow mode tool policy:\n- Jangan membuat skill/workflow task otomatis dari prompt chat.\n- Jangan menjalankan tool agentic umum dari mode workflow chat.\n- Eksekusi tool workflow hanya boleh melalui custom workflow node builder yang mendeklarasikan tool_name/mcp_server secara eksplisit.\n"
	}

	var sb strings.Builder
	sb.WriteString("\n\nSkill system (otomasi yang bisa dipakai ulang):\n")
	sb.WriteString("- Gunakan `skill_list` untuk melihat skill yang sudah ada sebelum membuat baru.\n")
	sb.WriteString("- Gunakan `skill_create` KAPAN PUN user minta \"buatkan skill\", \"simpan sebagai skill\", \"bikin routine\", atau saat workflow/task bisa dijadikan otomasi reusable. Tidak perlu menunggu pola berulang dan tidak ada syarat minimal 3 tool/action. Parameter: name (kebab-case), description (1-2 kalimat), steps (array tool calls).\n")
	sb.WriteString("- Gunakan `skill_install` jika user ingin install/import skill dari path lokal, GitHub/raw URL, folder `SKILL.md`, Claude Code/Antigravity markdown, atau perintah seperti `npx skills add owner/repo`.\n")
	sb.WriteString("- Jika ada skill tersimpan yang relevan dengan tugas user, prioritaskan `skill_run` sebelum mengerjakan manual dari nol. Jika skill kurang sesuai, gagal, timeout, atau user memberi koreksi, upgrade dengan `skill_create` overwrite=true lalu langsung jalankan skill hasil upgrade.\n")
	sb.WriteString("- Gunakan `skill_run` dengan skill_name untuk menjalankan skill yang sudah tersimpan. Skill baru atau hasil upgrade langsung dieksekusi tanpa approval tambahan.\n")
	sb.WriteString("- `skill_delete` hanya jika user eksplisit minta dihapus.\n")
	sb.WriteString("\nPENTING — JANGAN gunakan `skill_run` untuk tool bawaan berikut: remember, search_memories. Tool-tool ini adalah built-in tools dari [smara] tool group dan harus dipanggil LANGSUNG, bukan melalui skill_run.\n")
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

func buildSkillRecommendationContext(query string, mode Mode) string {
	if mode == ModeWorkflow || strings.TrimSpace(query) == "" {
		return ""
	}
	if strings.Contains(query, autoSkillResultMarker) {
		return "\n\nAuto skill routing: skill rekomendasi sudah dijalankan otomatis untuk prompt ini. Gunakan hasilnya dan jangan panggil `skill_run` yang sama lagi.\n"
	}
	q := strings.ToLower(strings.TrimSpace(query))
	shortChats := map[string]bool{"halo": true, "hallo": true, "hai": true, "hi": true, "hello": true, "ok": true, "oke": true, "mantap": true, "thanks": true, "terima kasih": true}
	if shortChats[q] || (len(strings.Fields(q)) <= 1 && len(q) <= 8) {
		return "\n\nAuto skill recommendation: tidak ada rekomendasi; prompt terlihat seperti sapaan/chat singkat, jawab normal tanpa skill_run.\n"
	}

	names, err := skill.List()
	if err != nil || len(names) == 0 {
		return ""
	}
	skills := make([]*skill.Skill, 0, len(names))
	for _, name := range names {
		sk, err := skill.Load(name)
		if err == nil && sk != nil {
			skills = append(skills, sk)
		}
	}
	recs := skill.RecommendSkills(query, skills, skill.RecommendationOptions{Limit: 5, LowConfidence: 25})
	if len(recs) == 0 {
		return "\n\nAuto skill recommendation: tidak ada skill yang cukup relevan; kerjakan manual atau tanya klarifikasi bila konteks kurang.\n"
	}

	var sb strings.Builder
	sb.WriteString("\n\nAuto skill recommendation untuk prompt user saat ini:\n")
	sb.WriteString("Policy: confidence high => prioritaskan panggil `skill_run`; confidence medium => panggil jika jelas cocok; confidence low/clarify => tanya klarifikasi atau kerjakan manual bila skill tidak pas. Sapaan/chat singkat jangan trigger skill.\n")
	for i, rec := range recs {
		clarify := ""
		if rec.Clarify {
			clarify = " clarify=true"
		}
		sb.WriteString(fmt.Sprintf("  %d. %s — score %.0f, confidence=%s%s", i+1, rec.SkillName, rec.Score, rec.Confidence, clarify))
		if len(rec.Reasons) > 0 {
			sb.WriteString("; alasan: ")
			sb.WriteString(strings.Join(rec.Reasons, ", "))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// selectAutoRunnableSkill returns a single unambiguous recommendation that is
// safe to execute before the model responds. Risky, parameterized, and closely
// competing recommendations remain prompt-driven so the model can clarify.
func selectAutoRunnableSkill(query string, mode Mode) *autoSkillSelection {
	if !autoSkillModeAllowed(mode) || strings.TrimSpace(query) == "" || isSkillManagementPrompt(query) {
		return nil
	}

	names, err := skill.List()
	if err != nil || len(names) == 0 {
		return nil
	}
	skills := make([]*skill.Skill, 0, len(names))
	byName := make(map[string]*skill.Skill, len(names))
	for _, name := range names {
		sk, err := skill.Load(name)
		if err != nil || sk == nil {
			continue
		}
		skills = append(skills, sk)
		byName[sk.Name] = sk
	}

	recs := skill.RecommendSkills(query, skills, skill.RecommendationOptions{Limit: 2, LowConfidence: 25})
	if len(recs) == 0 || recs[0].Confidence != "high" || recs[0].Clarify {
		return nil
	}
	if len(recs) > 1 && recs[0].Score-recs[1].Score < 15 {
		return nil
	}

	sk := byName[recs[0].SkillName]
	if sk == nil || hasUnresolvedRequiredParams(sk) || hasNestedSkillSteps(sk) {
		return nil
	}
	// In RUSH mode, the user explicitly opts into autonomous execution.
	// Do not block auto-runnable skills solely because their heuristic risk
	// would normally require approval; per-tool confirmation is also bypassed
	// by Supervisor.isCriticalCall for ModeRush.
	if mode != ModeRush && skill.AssessRisk(sk).RequiresApproval {
		return nil
	}
	return &autoSkillSelection{Skill: sk, Recommendation: recs[0]}
}

func autoSkillModeAllowed(mode Mode) bool {
	switch mode {
	case ModeAsk, ModeRush, ModePlan, ModeTest:
		return true
	default:
		return false
	}
}

func hasUnresolvedRequiredParams(sk *skill.Skill) bool {
	for _, param := range sk.Params {
		if param.Required && param.Default == nil {
			return true
		}
	}
	return false
}

func hasNestedSkillSteps(sk *skill.Skill) bool {
	for _, step := range sk.Steps {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(step.Tool)), "skill:") {
			return true
		}
	}
	return false
}

func isSkillManagementPrompt(query string) bool {
	q := strings.ToLower(query)
	for _, phrase := range []string{
		"list skill", "skill list", "daftar skill", "ngelist skill", "lihat skill",
		"skill yang tersedia", "skill apa", "buat skill", "bikin skill", "hapus skill",
		"delete skill", "install skill", "import skill", "recommend skill", "rekomendasi skill",
	} {
		if strings.Contains(q, phrase) {
			return true
		}
	}
	return false
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
