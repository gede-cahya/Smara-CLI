package discord

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// PRDAnswers contains the structured answers collected by the Discord PRD wizard.
type PRDAnswers struct {
	ProductName string
	Idea        string
	ProductType string
	TargetUser  string
	Platform    string
	Scope       string
	DetailLevel string
	CreatedBy   string
	CreatedAt   time.Time
}

// GeneratePRDMarkdown renders a copy-paste friendly Product Requirements Document.
func GeneratePRDMarkdown(a PRDAnswers) string {
	if strings.TrimSpace(a.ProductName) == "" {
		a.ProductName = inferProductName(a.Idea)
	}
	if strings.TrimSpace(a.Idea) == "" {
		a.Idea = "Produk baru yang membutuhkan validasi kebutuhan, scope MVP, dan rencana delivery."
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# PRD: %s\n\n", a.ProductName)
	b.WriteString("## 1. Overview\n\n")
	fmt.Fprintf(&b, "%s adalah %s untuk %s di platform %s. Dokumen ini menjelaskan masalah, tujuan, scope, requirement, metrik keberhasilan, risiko, dan pertanyaan terbuka untuk membantu tim membangun produk secara terarah.\n\n", a.ProductName, strings.ToLower(a.ProductType), strings.ToLower(a.TargetUser), strings.ToLower(a.Platform))
	b.WriteString("## 2. Product Idea\n\n")
	fmt.Fprintf(&b, "%s\n\n", a.Idea)
	b.WriteString("## 3. Problem Statement\n\n")
	fmt.Fprintf(&b, "Target user (%s) membutuhkan solusi yang lebih jelas, cepat, dan terukur untuk menyelesaikan masalah utama yang dijelaskan pada ide produk. Tanpa solusi ini, user berpotensi tetap menggunakan proses manual, tool terpisah, atau workflow yang sulit diskalakan.\n\n", a.TargetUser)
	b.WriteString("## 4. Goals\n\n")
	b.WriteString("- Menyediakan pengalaman inti yang mudah dipahami oleh user.\n")
	b.WriteString("- Mengurangi friction dari workflow utama.\n")
	b.WriteString("- Menghasilkan value yang dapat divalidasi dalam scope " + a.Scope + ".\n")
	b.WriteString("- Menyiapkan dasar iterasi produk berikutnya berdasarkan feedback dan metrik.\n\n")
	b.WriteString("## 5. Non-Goals\n\n")
	b.WriteString("- Membangun semua fitur lanjutan sebelum kebutuhan inti tervalidasi.\n")
	b.WriteString("- Mengoptimalkan skala enterprise sebelum ada sinyal penggunaan nyata.\n")
	b.WriteString("- Mengganti semua proses existing user tanpa fase migrasi/adopsi.\n\n")
	b.WriteString("## 6. Target Users\n\n")
	fmt.Fprintf(&b, "- Primary: %s\n", a.TargetUser)
	b.WriteString("- Secondary: stakeholder yang terlibat dalam evaluasi, operasional, atau pengambilan keputusan.\n\n")
	b.WriteString("## 7. User Stories\n\n")
	b.WriteString("- Sebagai user, saya ingin memahami fungsi utama produk dengan cepat agar bisa mulai memakai tanpa onboarding panjang.\n")
	b.WriteString("- Sebagai user, saya ingin menyelesaikan workflow utama dalam beberapa langkah sederhana agar waktu kerja berkurang.\n")
	b.WriteString("- Sebagai stakeholder, saya ingin melihat output/hasil yang jelas agar bisa menilai keberhasilan produk.\n\n")
	b.WriteString("## 8. Functional Requirements\n\n")
	b.WriteString("1. Produk harus menyediakan entry point utama untuk memulai workflow.\n")
	b.WriteString("2. Produk harus menyimpan atau menampilkan output utama secara jelas.\n")
	b.WriteString("3. Produk harus memberi feedback/error state yang mudah dipahami.\n")
	b.WriteString("4. Produk harus mendukung penggunaan pada platform: " + a.Platform + ".\n")
	b.WriteString("5. Produk harus memiliki flow minimal yang sesuai dengan scope: " + a.Scope + ".\n\n")
	b.WriteString("## 9. Non-Functional Requirements\n\n")
	b.WriteString("- Usability: user baru dapat memahami value produk dalam < 5 menit.\n")
	b.WriteString("- Reliability: fitur inti stabil untuk skenario penggunaan normal.\n")
	b.WriteString("- Performance: interaksi utama merespons secara cepat dan tidak terasa blocking.\n")
	b.WriteString("- Security/Privacy: data user diproses sesuai kebutuhan minimum dan tidak diekspos tanpa izin.\n\n")
	b.WriteString("## 10. User Flow\n\n")
	b.WriteString("1. User membuka produk atau menjalankan command utama.\n")
	b.WriteString("2. User memasukkan kebutuhan/konteks awal.\n")
	b.WriteString("3. Produk memproses input dan menampilkan hasil.\n")
	b.WriteString("4. User meninjau, menyimpan, membagikan, atau mengulangi proses.\n\n")
	b.WriteString("## 11. Flow Alur Tahapan\n\n")
	b.WriteString("```mermaid\n")
	b.WriteString("flowchart TD\n")
	b.WriteString("    A[User mulai / membuka produk] --> B[Input kebutuhan atau konteks]\n")
	b.WriteString("    B --> C[Validasi input]\n")
	b.WriteString("    C --> D[Proses fitur inti]\n")
	b.WriteString("    D --> E[Tampilkan output utama]\n")
	b.WriteString("    E --> F{User puas?}\n")
	b.WriteString("    F -- Ya --> G[Simpan / download / share]\n")
	b.WriteString("    F -- Tidak --> H[Edit input / ulangi proses]\n")
	b.WriteString("    H --> B\n")
	b.WriteString("```\n\n")
	b.WriteString("### Tahapan Implementasi Flow\n\n")
	b.WriteString("1. **Entry point**: user masuk melalui tombol, command, form, atau menu utama.\n")
	b.WriteString("2. **Context collection**: sistem mengumpulkan kebutuhan minimum dari user.\n")
	b.WriteString("3. **Processing**: sistem memvalidasi input dan menjalankan fitur inti.\n")
	b.WriteString("4. **Output review**: user melihat hasil dan bisa memberi koreksi.\n")
	b.WriteString("5. **Final action**: user menyimpan, mengunduh, membagikan, atau mengulang workflow.\n\n")
	b.WriteString("## 12. Chat Flow / Plain Mode Flow\n\n")
	b.WriteString("Flow ini dipakai ketika produk berjalan dalam mode chat/plain tanpa button UI. Semua pilihan ditanyakan sebagai teks agar tetap bisa copy-paste dan berjalan di channel yang tidak mendukung button.\n\n")
	b.WriteString("```text\n")
	b.WriteString("Bot  : Halo, saya bantu buat PRD. Apa ide atau nama produknya?\n")
	b.WriteString("User : <menjelaskan ide produk>\n")
	b.WriteString("Bot  : Pilih tipe produk: SaaS / Mobile App / Web App / Bot-Automation / Internal Tool\n")
	b.WriteString("User : <memilih salah satu>\n")
	b.WriteString("Bot  : Siapa target user utama: Consumer / Business / Developer / Internal Team / Community?\n")
	b.WriteString("User : <memilih salah satu>\n")
	b.WriteString("Bot  : Platform utama: Web / Mobile / Desktop / Discord Bot / Multi-platform?\n")
	b.WriteString("User : <memilih salah satu>\n")
	b.WriteString("Bot  : Scope awal: Prototype / MVP / V1 / Enterprise?\n")
	b.WriteString("User : <memilih salah satu>\n")
	b.WriteString("Bot  : Detail PRD: Ringkas / Standard / Lengkap?\n")
	b.WriteString("User : <memilih salah satu>\n")
	b.WriteString("Bot  : PRD selesai. Saya kirim Markdown untuk copy-paste dan file .md untuk download.\n")
	b.WriteString("```\n\n")
	b.WriteString("### Plain Mode State\n\n")
	b.WriteString("- `collect_idea` → mengumpulkan nama/ide produk.\n")
	b.WriteString("- `select_product_type` → memilih tipe produk.\n")
	b.WriteString("- `select_target_user` → memilih target user.\n")
	b.WriteString("- `select_platform` → memilih platform.\n")
	b.WriteString("- `select_scope` → memilih scope awal.\n")
	b.WriteString("- `select_detail_level` → memilih kedalaman PRD.\n")
	b.WriteString("- `generate_markdown` → membuat PRD final dalam Markdown.\n\n")
	b.WriteString("## 13. Success Metrics\n\n")
	b.WriteString("- Activation rate: persentase user yang menyelesaikan workflow pertama.\n")
	b.WriteString("- Task completion time: waktu rata-rata menyelesaikan workflow inti.\n")
	b.WriteString("- Retention/return usage: user kembali memakai produk setelah penggunaan pertama.\n")
	b.WriteString("- Qualitative feedback: user menyatakan produk membantu menyelesaikan masalah utama.\n\n")
	b.WriteString("## 14. Risks & Mitigations\n\n")
	b.WriteString("| Risk | Mitigation |\n")
	b.WriteString("| --- | --- |\n")
	b.WriteString("| Scope melebar terlalu cepat | Kunci scope " + a.Scope + " dan gunakan backlog terpisah untuk ide lanjutan |\n")
	b.WriteString("| User belum memahami value | Buat onboarding singkat dan contoh penggunaan |\n")
	b.WriteString("| Requirement belum lengkap | Validasi dengan user interview dan iterasi PRD |\n\n")
	b.WriteString("## 15. MVP Scope\n\n")
	b.WriteString("- Satu workflow utama end-to-end.\n")
	b.WriteString("- Output yang bisa digunakan/copy/share.\n")
	b.WriteString("- State sukses, kosong, dan error.\n")
	b.WriteString("- Dokumentasi penggunaan dasar.\n\n")
	if strings.EqualFold(a.DetailLevel, "Lengkap") {
		b.WriteString("## 16. Release Plan\n\n")
		b.WriteString("- Phase 1: validasi problem dan prototype.\n- Phase 2: build MVP fitur inti.\n- Phase 3: beta test dengan user terbatas.\n- Phase 4: iterasi berdasarkan feedback dan metrik.\n\n")
		b.WriteString("## 17. Open Questions\n\n")
	} else {
		b.WriteString("## 16. Open Questions\n\n")
	}
	b.WriteString("- Apa masalah paling mahal/sering yang dialami target user?\n")
	b.WriteString("- Integrasi apa yang wajib ada pada versi awal?\n")
	b.WriteString("- Apa definisi sukses untuk 2-4 minggu pertama setelah launch?\n\n")
	fmt.Fprintf(&b, "---\nGenerated by Smara Discord PRD Wizard on %s", a.CreatedAt.Format("2006-01-02 15:04 MST"))
	if a.CreatedBy != "" {
		fmt.Fprintf(&b, " for %s", a.CreatedBy)
	}
	b.WriteString(".\n")
	return b.String()
}

func PRDFileName(productName string) string {
	slug := strings.ToLower(strings.TrimSpace(productName))
	if slug == "" {
		slug = "prd"
	}
	slug = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "prd"
	}
	return "PRD_" + slug + ".md"
}

func inferProductName(idea string) string {
	words := strings.Fields(strings.TrimSpace(idea))
	if len(words) == 0 {
		return "Produk Baru"
	}
	if len(words) > 5 {
		words = words[:5]
	}
	return strings.Title(strings.Join(words, " "))
}
