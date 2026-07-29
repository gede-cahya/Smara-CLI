package discord

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// PRDAnswers contains the structured answers collected by the Discord PRD wizard.
type PRDAnswers struct {
	ProductName  string
	Idea         string
	ProductType  string
	TargetUser   string
	Platform     string
	Scope        string
	WorkflowPlan string
	DiagramFlow  string
	DetailLevel  string
	CreatedBy    string
	CreatedAt    time.Time
}

// GeneratePRDMarkdown produces a full PRD document in Markdown format.
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
	fmt.Fprintf(&b, "Target user (%s) membutuhkan solusi yang lebih jelas, cepat, dan terukur untuk menyelesaikan masalah utama tentang %s. Tanpa solusi ini, user berpotensi tetap menggunakan proses manual, tool terpisah, atau workflow yang sulit diskalakan.\n\n", a.TargetUser, a.Idea)
	b.WriteString("## 4. Goals\n\n")
	fmt.Fprintf(&b, "- Menyediakan pengalaman inti bagi %s untuk %s.\n", strings.ToLower(a.TargetUser), a.Idea)
	b.WriteString("- Mengurangi friction dari workflow utama.\n")
	fmt.Fprintf(&b, "- Menghasilkan value yang dapat divalidasi dalam scope %s.\n", a.Scope)
	b.WriteString("- Menyiapkan dasar iterasi produk berikutnya berdasarkan feedback dan metrik.\n\n")
	b.WriteString("## 5. Non-Goals\n\n")
	b.WriteString("- Membangun semua fitur lanjutan sebelum kebutuhan inti tervalidasi.\n")
	b.WriteString("- Mengoptimalkan skala enterprise sebelum ada sinyal penggunaan nyata.\n")
	b.WriteString("- Mengganti semua proses existing user tanpa fase migrasi/adopsi.\n\n")
	b.WriteString("## 6. Target Users\n\n")
	fmt.Fprintf(&b, "- Primary: %s\n", a.TargetUser)
	b.WriteString("- Secondary: stakeholder yang terlibat dalam evaluasi, operasional, atau pengambilan keputusan.\n\n")
	b.WriteString("## 7. User Stories\n\n")
	fmt.Fprintf(&b, "- Sebagai %s, saya ingin %s dengan cepat agar bisa mulai memakai tanpa onboarding panjang.\n", strings.ToLower(a.TargetUser), a.Idea)
	b.WriteString("- Sebagai user, saya ingin menyelesaikan workflow utama dalam beberapa langkah sederhana agar waktu kerja berkurang.\n")
	b.WriteString("- Sebagai stakeholder, saya ingin melihat output/hasil yang jelas agar bisa menilai keberhasilan produk.\n\n")
	b.WriteString("## 8. Functional Requirements\n\n")
	fmt.Fprintf(&b, "1. %s harus menyediakan entry point utama untuk memulai workflow %s.\n", a.ProductName, a.Idea)
	b.WriteString("2. Produk harus menyimpan atau menampilkan output utama secara jelas.\n")
	b.WriteString("3. Produk harus memberi feedback/error state yang mudah dipahami.\n")
	fmt.Fprintf(&b, "4. Produk harus mendukung penggunaan pada platform: %s.\n", a.Platform)
	fmt.Fprintf(&b, "5. Produk harus memiliki flow minimal yang sesuai dengan scope: %s.\n\n", a.Scope)
	b.WriteString("## 9. Non-Functional Requirements\n\n")
	b.WriteString("- Usability: user baru dapat memahami value produk dalam < 5 menit.\n")
	b.WriteString("- Reliability: fitur inti stabil untuk skenario penggunaan normal.\n")
	b.WriteString("- Performance: interaksi utama merespons secara cepat dan tidak terasa blocking.\n")
	b.WriteString("- Security/Privacy: data user diproses sesuai kebutuhan minimum dan tidak diekspos tanpa izin.\n\n")
	b.WriteString("## 10. User Flow\n\n")
	fmt.Fprintf(&b, "1. User membuka platform %s (%s).\n", a.Platform, a.ProductName)
	fmt.Fprintf(&b, "2. User mengakses fitur %s.\n", a.Idea)
	b.WriteString("3. Sistem memproses permintaan dan menyajikan UI/layanan interaktif.\n")
	b.WriteString("4. User melakukan transaksi, konfirmasi, atau peninjauan output.\n\n")

	// 11. Flow Alur Tahapan & Diagram Visual
	b.WriteString("## 11. Flow Alur Tahapan & Diagram Visual\n\n")

	diagramType := a.DiagramFlow
	if diagramType == "" {
		diagramType = "Flowchart & Sequence"
	}

	if strings.Contains(diagramType, "Flowchart") || strings.Contains(diagramType, "Semua") || diagramType == "Standard" {
		b.WriteString("### User Flowchart Diagram\n\n")
		b.WriteString("```mermaid\n")
		b.WriteString("flowchart TD\n")
		fmt.Fprintf(&b, "    A[User buka platform %s] --> B[Akses %s]\n", a.Platform, a.ProductName)
		fmt.Fprintf(&b, "    B --> C[Input detail/konteks %s]\n", a.Idea)
		b.WriteString("    C --> D[Validasi input & proses bisnis]\n")
		b.WriteString("    D --> E[Tampilkan UI / Output utama]\n")
		b.WriteString("    E --> F{User puas / konfirmasi?}\n")
		b.WriteString("    F -- Ya --> G[Selesaikan transaksi / simpan state]\n")
		b.WriteString("    F -- Tidak --> H[Edit input / sesuaikan pilihan]\n")
		b.WriteString("    H --> C\n")
		b.WriteString("```\n\n")
	}

	if strings.Contains(diagramType, "Sequence") || strings.Contains(diagramType, "Semua") {
		b.WriteString("### Sequence Diagram (System Interaction)\n\n")
		b.WriteString("```mermaid\n")
		b.WriteString("sequenceDiagram\n")
		b.WriteString("    autonumber\n")
		b.WriteString("    actor User\n")
		b.WriteString("    participant Client as Platform Client\n")
		b.WriteString("    participant Core as Core Engine\n")
		b.WriteString("    participant DB as Storage\n")
		b.WriteString("    User->>Client: Triggers action / command\n")
		b.WriteString("    Client->>Core: Sends payload & context\n")
		b.WriteString("    Core->>Core: Validate & process request\n")
		b.WriteString("    Core->>DB: Save session state\n")
		b.WriteString("    DB-->>Core: Confirm saved\n")
		b.WriteString("    Core-->>Client: Return formatted response\n")
		b.WriteString("    Client-->>User: Display result / notification\n")
		b.WriteString("```\n\n")
	}

	if strings.Contains(diagramType, "State") || strings.Contains(diagramType, "Semua") {
		b.WriteString("### State Transition Diagram\n\n")
		b.WriteString("```mermaid\n")
		b.WriteString("stateDiagram-v2\n")
		b.WriteString("    [*] --> Idle: Init Session\n")
		b.WriteString("    Idle --> CollectingContext: User Input\n")
		b.WriteString("    CollectingContext --> Validating: Submit Form / Options\n")
		b.WriteString("    Validating --> Processing: Valid Data\n")
		b.WriteString("    Validating --> CollectingContext: Validation Failed\n")
		b.WriteString("    Processing --> Completed: Execution Success\n")
		b.WriteString("    Completed --> [*]\n")
		b.WriteString("```\n\n")
	}

	b.WriteString("### Tahapan Implementasi Flow\n\n")
	fmt.Fprintf(&b, "1. **Entry point**: user membuka platform %s (%s).\n", a.Platform, a.ProductName)
	fmt.Fprintf(&b, "2. **Context collection**: sistem mengumpulkan kebutuhan konteks ide (%s).\n", a.Idea)
	b.WriteString("3. **Processing**: sistem memvalidasi input dan menjalankan bisnis logika inti.\n")
	b.WriteString("4. **Output review**: user melihat hasil/respons dan dapat melakukan koreksi.\n")
	b.WriteString("5. **Final action**: transaksi diselesaikan, data disimpan, atau dipublikasikan.\n\n")

	// 12. Workflow & Timeline Plan
	b.WriteString("## 12. Workflow & Execution Plan\n\n")

	planType := a.WorkflowPlan
	if planType == "" {
		planType = "Agile Sprints"
	}

	b.WriteString(fmt.Sprintf("Model Rencana Kerja: **%s**\n\n", planType))

	if strings.Contains(planType, "Sprint") || planType == "Agile Sprints" {
		b.WriteString("### Sprint Roadmap (Agile 2-Week Sprints)\n\n")
		b.WriteString("- **Sprint 1 — Core & Setup**: Setup environment, arsitektur dasar, dan schema data.\n")
		b.WriteString("- **Sprint 2 — MVP Features**: Implementasi workflow utama dan UI interaktif.\n")
		b.WriteString("- **Sprint 3 — Integration & Testing**: Validasi end-to-end, error handling, dan feedback loop.\n")
		b.WriteString("- **Sprint 4 — Polish & Launch**: Dokumentasi, optimasi performa, dan release V1.\n\n")
	} else if strings.Contains(planType, "Phase") || planType == "Phase-based" {
		b.WriteString("### Phased Timeline\n\n")
		b.WriteString("1. **Fase 1 (Discovery & Setup)**: Definisikan spec, validasi ide, buat mockup/architecture.\n")
		b.WriteString("2. **Fase 2 (Build Core)**: Buat komponen utama dan bisnis logika inti.\n")
		b.WriteString("3. **Fase 3 (Testing & Refinement)**: Internal testing, bug fixing, dan penyesuaian UX.\n")
		b.WriteString("4. **Fase 4 (Deployment)**: Soft launch dan monitoring penggunaan awal.\n\n")
	} else {
		b.WriteString("### Execution Checklist\n\n")
		b.WriteString("- [ ] Task 1: Setup fondasi project dan integrasi platform.\n")
		b.WriteString("- [ ] Task 2: Buat workflow input-output utama.\n")
		b.WriteString("- [ ] Task 3: Uji coba internal dan simulasikan error cases.\n")
		b.WriteString("- [ ] Task 4: Release versi awal ke user.\n\n")
	}

	b.WriteString("```mermaid\n")
	b.WriteString("gantt\n")
	b.WriteString("    title Implementation Roadmap\n")
	b.WriteString("    dateFormat YYYY-MM-DD\n")
	b.WriteString("    section Setup & Core\n")
	b.WriteString("    Architecture & Spec  :a1, 2025-01-01, 5d\n")
	b.WriteString("    Core Logic           :a2, after a1, 10d\n")
	b.WriteString("    section Development\n")
	b.WriteString("    UI / Integration     :2025-01-16, 10d\n")
	b.WriteString("    Testing & Fixes      :after a2, 5d\n")
	b.WriteString("    section Release\n")
	b.WriteString("    Deployment & Launch  :crit, 2025-02-01, 3d\n")
	b.WriteString("```\n\n")

	// 13. Chat Flow / Plain Mode Flow
	b.WriteString("## 13. Chat Flow / Plain Mode Flow\n\n")
	b.WriteString("Flow ini dipakai ketika produk berjalan dalam mode chat/plain tanpa button UI. Semua pilihan ditanyakan sebagai teks agar tetap bisa copy-paste dan berjalan di channel yang tidak mendukung button.\n\n")
	b.WriteString("```text\n")
	b.WriteString("Bot  : Halo, saya bantu buat PRD. Apa ide atau nama produknya?\n")
	fmt.Fprintf(&b, "User : %s\n", a.Idea)
	b.WriteString("Bot  : Pilih tipe produk: SaaS / Mobile App / Web App / Bot-Automation / Internal Tool\n")
	fmt.Fprintf(&b, "User : %s\n", a.ProductType)
	b.WriteString("Bot  : Siapa target user utama: Consumer / Business / Developer / Internal Team / Community?\n")
	fmt.Fprintf(&b, "User : %s\n", a.TargetUser)
	b.WriteString("Bot  : Platform utama: Web / Mobile / Desktop / Discord Bot / Multi-platform?\n")
	fmt.Fprintf(&b, "User : %s\n", a.Platform)
	b.WriteString("Bot  : Scope awal: Prototype / MVP / V1 / Enterprise?\n")
	fmt.Fprintf(&b, "User : %s\n", a.Scope)
	b.WriteString("Bot  : Rencana Kerja: Agile Sprints / Phase-based / Simple Checklist?\n")
	fmt.Fprintf(&b, "User : %s\n", a.WorkflowPlan)
	b.WriteString("Bot  : Visual Diagram: Flowchart & Sequence / Flowchart Only / State Machine / Semua Diagram?\n")
	fmt.Fprintf(&b, "User : %s\n", a.DiagramFlow)
	b.WriteString("Bot  : Detail PRD: Ringkas / Standard / Lengkap?\n")
	fmt.Fprintf(&b, "User : %s\n", a.DetailLevel)
	b.WriteString("Bot  : PRD selesai. Saya kirim Markdown untuk copy-paste dan file .md untuk download.\n")
	b.WriteString("```\n\n")

	b.WriteString("## 14. Success Metrics\n\n")
	b.WriteString("- Activation rate: persentase user yang menyelesaikan workflow pertama.\n")
	b.WriteString("- Task completion time: waktu rata-rata menyelesaikan workflow inti.\n")
	b.WriteString("- Retention/return usage: user kembali memakai produk setelah penggunaan pertama.\n")
	b.WriteString("- Qualitative feedback: user menyatakan produk membantu menyelesaikan masalah utama.\n\n")
	b.WriteString("## 15. Risks & Mitigations\n\n")
	b.WriteString("| Risk | Mitigation |\n")
	b.WriteString("| --- | --- |\n")
	b.WriteString("| Scope melebar terlalu cepat | Kunci scope " + a.Scope + " dan gunakan backlog terpisah untuk ide lanjutan |\n")
	b.WriteString("| User belum memahami value | Buat onboarding singkat dan contoh penggunaan |\n")
	b.WriteString("| Requirement belum lengkap | Validasi dengan user interview dan iterasi PRD |\n\n")
	b.WriteString("## 16. MVP Scope\n\n")
	b.WriteString("- Satu workflow utama end-to-end.\n")
	b.WriteString("- Output yang bisa digunakan/copy/share.\n")
	b.WriteString("- State sukses, kosong, dan error.\n")
	b.WriteString("- Dokumentasi penggunaan dasar.\n\n")
	if strings.EqualFold(a.DetailLevel, "Lengkap") {
		b.WriteString("## 17. Release Plan\n\n")
		b.WriteString("- Phase 1: validasi problem dan prototype.\n- Phase 2: build MVP fitur inti.\n- Phase 3: beta test dengan user terbatas.\n- Phase 4: iterasi berdasarkan feedback dan metrik.\n\n")
		b.WriteString("## 18. Open Questions\n\n")
	} else {
		b.WriteString("## 17. Open Questions\n\n")
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
