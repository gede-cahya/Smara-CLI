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
	TechStack    string
	Scope        string
	Priority     string
	WorkflowPlan string
	DiagramFlow  string
	DetailLevel  string
	CreatedBy    string
	CreatedAt    time.Time
}

type domainContext struct {
	OverviewDesc        string
	ProblemDesc         string
	Goals               []string
	NonGoals            []string
	PrimaryUser         string
	SecondaryUser       string
	UserStories         []string
	FunctionalReqs      []string
	NonFunctionalReqs   []string
	UserFlowSteps       []string
	FlowchartMermaid    string
	SequenceMermaid     string
	StateMermaid        string
	TahapanFlow         []string
	SprintRoadmap       []string
	PhasedRoadmap       []string
	ChecklistRoadmap    []string
	GanttMermaid        string
	SuccessMetrics      []string
	RisksAndMitigations [][2]string
	MVPScope            []string
	ReleasePlan         []string
	OpenQuestions       []string
}

// GeneratePRDMarkdown produces a full PRD document in Markdown format.
func GeneratePRDMarkdown(a PRDAnswers) string {
	if strings.TrimSpace(a.ProductName) == "" || isGenericProductName(a.ProductName) {
		a.ProductName = inferProductName(a.Idea)
	}
	if strings.TrimSpace(a.Idea) == "" {
		a.Idea = "Produk baru yang membutuhkan validasi kebutuhan, scope MVP, dan rencana delivery."
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}

	ctx := buildDomainContext(a)

	var b strings.Builder
	fmt.Fprintf(&b, "# PRD: %s\n\n", a.ProductName)
	b.WriteString("## 1. Overview\n\n")
	b.WriteString(ctx.OverviewDesc)
	b.WriteString("\n\n")

	b.WriteString("## 2. Product Idea\n\n")
	fmt.Fprintf(&b, "%s\n\n", a.Idea)

	b.WriteString("## 3. Problem Statement\n\n")
	b.WriteString(ctx.ProblemDesc)
	b.WriteString("\n\n")

	b.WriteString("## 4. Goals\n\n")
	for _, g := range ctx.Goals {
		fmt.Fprintf(&b, "- %s\n", g)
	}
	b.WriteString("\n")

	b.WriteString("## 5. Non-Goals\n\n")
	for _, ng := range ctx.NonGoals {
		fmt.Fprintf(&b, "- %s\n", ng)
	}
	b.WriteString("\n")

	b.WriteString("## 6. Target Users\n\n")
	fmt.Fprintf(&b, "- Primary: %s\n", ctx.PrimaryUser)
	fmt.Fprintf(&b, "- Secondary: %s\n\n", ctx.SecondaryUser)

	b.WriteString("## 7. User Stories\n\n")
	for _, us := range ctx.UserStories {
		fmt.Fprintf(&b, "- %s\n", us)
	}
	b.WriteString("\n")

	b.WriteString("## 8. Functional Requirements\n\n")
	for i, fr := range ctx.FunctionalReqs {
		fmt.Fprintf(&b, "%d. %s\n", i+1, fr)
	}
	b.WriteString("\n")

	b.WriteString("## 9. Non-Functional Requirements\n\n")
	for _, nfr := range ctx.NonFunctionalReqs {
		fmt.Fprintf(&b, "- %s\n", nfr)
	}
	b.WriteString("\n")

	b.WriteString("## 10. User Flow\n\n")
	for i, uf := range ctx.UserFlowSteps {
		fmt.Fprintf(&b, "%d. %s\n", i+1, uf)
	}
	b.WriteString("\n")

	// 11. Flow Alur Tahapan & Diagram Visual
	b.WriteString("## 11. Flow Alur Tahapan & Diagram Visual\n\n")

	diagramType := a.DiagramFlow
	if diagramType == "" {
		diagramType = "Flowchart & Sequence"
	}

	if strings.Contains(diagramType, "Flowchart") || strings.Contains(diagramType, "Semua") || diagramType == "Standard" {
		b.WriteString("### User Flowchart Diagram\n\n")
		b.WriteString("```mermaid\n")
		b.WriteString(ctx.FlowchartMermaid)
		b.WriteString("\n```\n\n")
	}

	if strings.Contains(diagramType, "Sequence") || strings.Contains(diagramType, "Semua") {
		b.WriteString("### Sequence Diagram (System Interaction)\n\n")
		b.WriteString("```mermaid\n")
		b.WriteString(ctx.SequenceMermaid)
		b.WriteString("\n```\n\n")
	}

	if strings.Contains(diagramType, "State") || strings.Contains(diagramType, "Semua") {
		b.WriteString("### State Transition Diagram\n\n")
		b.WriteString("```mermaid\n")
		b.WriteString(ctx.StateMermaid)
		b.WriteString("\n```\n\n")
	}

	b.WriteString("### Tahapan Implementasi Flow\n\n")
	for i, tf := range ctx.TahapanFlow {
		fmt.Fprintf(&b, "%d. %s\n", i+1, tf)
	}
	b.WriteString("\n")

	// 12. Workflow & Timeline Plan
	b.WriteString("## 12. Workflow & Execution Plan\n\n")

	planType := a.WorkflowPlan
	if planType == "" {
		planType = "Agile Sprints"
	}

	fmt.Fprintf(&b, "Model Rencana Kerja: **%s**\n\n", planType)

	if strings.Contains(planType, "Sprint") || planType == "Agile Sprints" {
		b.WriteString("### Sprint Roadmap (Agile 2-Week Sprints)\n\n")
		for _, s := range ctx.SprintRoadmap {
			fmt.Fprintf(&b, "- %s\n", s)
		}
		b.WriteString("\n")
	} else if strings.Contains(planType, "Phase") || planType == "Phase-based" {
		b.WriteString("### Phased Timeline\n\n")
		for i, p := range ctx.PhasedRoadmap {
			fmt.Fprintf(&b, "%d. %s\n", i+1, p)
		}
		b.WriteString("\n")
	} else {
		b.WriteString("### Execution Checklist\n\n")
		for _, c := range ctx.ChecklistRoadmap {
			fmt.Fprintf(&b, "- [ ] %s\n", c)
		}
		b.WriteString("\n")
	}

	b.WriteString("```mermaid\n")
	b.WriteString(ctx.GanttMermaid)
	b.WriteString("\n```\n\n")

	// 13. Chat Flow / Plain Mode Flow
	b.WriteString("## 13. Chat Flow / Plain Mode Flow\n\n")
	b.WriteString("Flow ini dipakai ketika produk berjalan dalam mode chat/plain tanpa button UI. Semua pilihan ditanyakan sebagai teks agar tetap bisa copy-paste dan berjalan di channel yang tidak mendukung button.\n\n")
	b.WriteString("```text\n")
	b.WriteString("Bot  : Halo, saya bantu buat PRD. Apa ide atau nama produknya?\n")
	fmt.Fprintf(&b, "User : %s\n", a.Idea)
	b.WriteString("Bot  : Pilih tipe produk: SaaS / Mobile App / Web App / Bot-Automation / Internal Tool\n")
	fmt.Fprintf(&b, "User : %s\n", defaultStr(a.ProductType, "Web App"))
	b.WriteString("Bot  : Siapa target user utama: Consumer / Business / Developer / Internal Team / Community?\n")
	fmt.Fprintf(&b, "User : %s\n", defaultStr(a.TargetUser, "Consumer"))
	b.WriteString("Bot  : Platform utama: Web / Mobile / Desktop / Discord Bot / Multi-platform?\n")
	fmt.Fprintf(&b, "User : %s\n", defaultStr(a.Platform, "Web"))
	b.WriteString("Bot  : Preferensi Tech Stack: Fullstack JS/TS / Go REST API / Python / AI / Mobile Native / Auto-Recommend?\n")
	fmt.Fprintf(&b, "User : %s\n", defaultStr(a.TechStack, "Auto-Recommend"))
	b.WriteString("Bot  : Scope awal: Prototype / MVP / V1 / Enterprise?\n")
	fmt.Fprintf(&b, "User : %s\n", defaultStr(a.Scope, "MVP"))
	b.WriteString("Bot  : Prioritas Utama MVP: Speed-to-Market / Security & Data / High Scalability / Premium UX & Design?\n")
	fmt.Fprintf(&b, "User : %s\n", defaultStr(a.Priority, "Speed-to-Market"))
	b.WriteString("Bot  : Rencana Kerja: Agile Sprints / Phase-based / Simple Checklist?\n")
	fmt.Fprintf(&b, "User : %s\n", defaultStr(a.WorkflowPlan, "Agile Sprints"))
	b.WriteString("Bot  : Visual Diagram: Flowchart & Sequence / Flowchart Only / State Machine / Semua Diagram?\n")
	fmt.Fprintf(&b, "User : %s\n", defaultStr(a.DiagramFlow, "Flowchart & Sequence"))
	b.WriteString("Bot  : Detail PRD: Ringkas / Standard / Lengkap?\n")
	fmt.Fprintf(&b, "User : %s\n", defaultStr(a.DetailLevel, "Lengkap"))
	b.WriteString("Bot  : PRD selesai. Saya kirim Markdown untuk copy-paste dan file .md untuk download.\n")
	b.WriteString("```\n\n")

	b.WriteString("## 14. Success Metrics\n\n")
	for _, sm := range ctx.SuccessMetrics {
		fmt.Fprintf(&b, "- %s\n", sm)
	}
	b.WriteString("\n")

	b.WriteString("## 15. Risks & Mitigations\n\n")
	b.WriteString("| Risk | Mitigation |\n")
	b.WriteString("| --- | --- |\n")
	for _, rm := range ctx.RisksAndMitigations {
		fmt.Fprintf(&b, "| %s | %s |\n", rm[0], rm[1])
	}
	b.WriteString("\n")

	b.WriteString("## 16. MVP Scope\n\n")
	for _, ms := range ctx.MVPScope {
		fmt.Fprintf(&b, "- %s\n", ms)
	}
	b.WriteString("\n")

	if strings.EqualFold(a.DetailLevel, "Lengkap") || a.DetailLevel == "" {
		b.WriteString("## 17. Release Plan\n\n")
		for _, rp := range ctx.ReleasePlan {
			fmt.Fprintf(&b, "- %s\n", rp)
		}
		b.WriteString("\n")

		b.WriteString("## 18. Open Questions\n\n")
		for _, oq := range ctx.OpenQuestions {
			fmt.Fprintf(&b, "- %s\n", oq)
		}
		b.WriteString("\n")
	} else {
		b.WriteString("## 17. Open Questions\n\n")
		for _, oq := range ctx.OpenQuestions {
			fmt.Fprintf(&b, "- %s\n", oq)
		}
		b.WriteString("\n")
	}

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

func isGenericProductName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(lower, "buatkan ") || strings.HasPrefix(lower, "bantu buat") || strings.HasPrefix(lower, "tolong buat") || strings.HasPrefix(lower, "bikin ") || lower == "produk baru"
}

func inferProductName(idea string) string {
	clean := strings.TrimSpace(idea)
	if clean == "" {
		return "Produk Baru"
	}

	// Strip common request lead-in prefixes
	prefixes := []string{
		"tolong buatkan ", "tolong buat ", "tolong bikin ",
		"bantu buatkan ", "bantu buat ", "bantu bikin ",
		"buatkan ", "buat ", "bikin ", "membuat ",
		"create a ", "create ", "build a ", "build ", "develop a ", "develop ",
	}
	lower := strings.ToLower(clean)
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			clean = clean[len(p):]
			lower = strings.ToLower(clean)
			break
		}
	}

	typePrefixes := []string{"website ", "web app ", "aplikasi ", "app ", "sistem ", "platform ", "bot "}
	leadType := ""
	for _, tp := range typePrefixes {
		if strings.HasPrefix(lower, tp) {
			leadType = strings.TrimSpace(tp)
			clean = clean[len(tp):]
			lower = strings.ToLower(clean)
			break
		}
	}

	words := strings.Fields(clean)
	if len(words) == 0 {
		if leadType != "" {
			return strings.Title(leadType)
		}
		return "Produk Baru"
	}

	if len(words) > 6 {
		words = words[:6]
	}
	name := strings.Title(strings.Join(words, " "))
	if leadType != "" {
		name = strings.Title(leadType) + " " + name
	}

	if strings.Contains(strings.ToLower(idea), "mie") && !strings.Contains(strings.ToLower(name), "mie") {
		name += " (Mie)"
	}
	return name
}

func defaultStr(val, fallback string) string {
	if strings.TrimSpace(val) == "" {
		return fallback
	}
	return val
}

func buildDomainContext(a PRDAnswers) domainContext {
	pName := a.ProductName
	pType := defaultStr(a.ProductType, "Web App")
	pTypeLower := strings.ToLower(pType)
	tUser := defaultStr(a.TargetUser, "Consumer")
	tUserLower := strings.ToLower(tUser)
	platform := defaultStr(a.Platform, "Web")
	scope := defaultStr(a.Scope, "MVP")
	techStack := defaultStr(a.TechStack, "Auto-Recommend")
	priority := defaultStr(a.Priority, "Speed-to-Market")
	idea := a.Idea
	ideaLower := strings.ToLower(idea)

	techInfo := fmt.Sprintf(" Serta dibangun di atas rekomendasi arsitektur **%s** dengan fokus utama pada **%s**.", techStack, priority)

	// Detect domain type: Food/E-Commerce, Bot/Automation, SaaS/Management, Community, General
	isFood := strings.Contains(ideaLower, "makanan") || strings.Contains(ideaLower, "mie") || strings.Contains(ideaLower, "kuliner") || strings.Contains(ideaLower, "instan") || strings.Contains(ideaLower, "toko") || strings.Contains(ideaLower, "resto") || strings.Contains(ideaLower, "katalog")
	isBot := strings.Contains(ideaLower, "bot") || strings.Contains(ideaLower, "discord") || strings.Contains(ideaLower, "telegram") || strings.Contains(ideaLower, "automation") || strings.Contains(ideaLower, "webhook")
	isSaaS := strings.Contains(ideaLower, "dashboard") || strings.Contains(ideaLower, "crm") || strings.Contains(ideaLower, "analytics") || strings.Contains(ideaLower, "manajemen") || strings.Contains(ideaLower, "saas") || strings.Contains(ideaLower, "finance")

	if isFood {
		return domainContext{
			OverviewDesc: fmt.Sprintf("%s adalah %s yang dirancang khusus untuk memfasilitasi penjelajahan, pemesanan, dan transaksi produk makanan instan (seperti produk mie) bagi %s melalui platform %s.%s Dokumen PRD ini merinci skema produk, kebutuhan fungsional, arsitektur flow, dan rencana eksekusi.", pName, pTypeLower, tUserLower, platform, techInfo),
			ProblemDesc:  fmt.Sprintf("Konsumen (%s) sering menghadapi hambatan dalam mengakses katalog lengkap produk makanan instan, informasi ketersediaan varian rasa mie secara realtime, serta alur checkout belanja yang praktis. Tanpa platform digital yang terstruktur, pemesanan produk rentan terkendala oleh saluran manual dan informasi yang kurang transparan.", tUser),
			Goals: []string{
				fmt.Sprintf("Menyediakan katalog produk makanan instan yang komprehensif dan intuitif bagi %s.", tUserLower),
				"Mempermudah penjelajahan varian rasa, penyaringan produk, serta proses pemesanan dalam beberapa langkah sederhana.",
				fmt.Sprintf("Menghasilkan pengalaman belanja digital yang dapat divalidasi secara cepat dalam scope %s.", scope),
				"Menyediakan sistem manajemen stok dan rincian pesanan yang siap dikembangkan pada iterasi berikutnya.",
			},
			NonGoals: []string{
				"Membangun pabrik manufaktur atau sistem logistik fisik internal dalam rilis rintisan.",
				"Mendukung integrasi ke seluruh saluran pembayaran internasional sebelum MVP teruji di pasar domestik.",
				"Mengganti seluruh mekanisme distribusi offline secara mendadak tanpa tahap adopsi gradual.",
			},
			PrimaryUser:   fmt.Sprintf("%s (Pembeli / penikmat produk makanan instan mie)", tUser),
			SecondaryUser: "Admin Toko, Tim Pengelola Stok, dan Stakeholder Operasional Penjualan",
			UserStories: []string{
				fmt.Sprintf("Sebagai %s, saya ingin melihat katalog varian produk mie instan beserta harga dan deskripsi rasa agar dapat menentukan pilihan belanja dengan tepat.", tUserLower),
				fmt.Sprintf("Sebagai %s, saya ingin menambahkan varian pilihan ke keranjang belanja dan melakukan checkout digital agar transaksi selesai dengan cepat.", tUserLower),
				"Sebagai Admin Toko, saya ingin mengelola katalog produk dan memantau status pesanan masuk agar dapat memproses pengiriman dengan akurat.",
			},
			FunctionalReqs: []string{
				fmt.Sprintf("%s harus menyediakan entry point berupa katalog produk makanan instan dengan fitur pencarian dan filter varian (rasa, harga, paket).", pName),
				"Sistem harus menyediakan modul keranjang belanja (shopping cart) dengan kalkulasi otomatis item, subtotal, dan opsi pengiriman.",
				"Sistem harus mendukung proses checkout, pengisian alamat tujuan, serta integrasi saluran pembayaran digital.",
				"Sistem harus menerbitkan ringkasan pesanan dan memberikan status pelacakan transaksi (Menunggu Pembayaran, Diproses, Dikirim, Selesai).",
				fmt.Sprintf("Sistem harus dioptimalkan untuk pengoperasian pada platform %s sesuai scope %s.", platform, scope),
			},
			NonFunctionalReqs: []string{
				"Usability: Antarmuka belanja yang ramah pengguna sehingga konsumen dapat memilih dan memesan produk dalam waktu < 3 menit.",
				"Performance: Waktu muat halaman katalog dan pencarian produk mie < 1.5 detik.",
				"Reliability: Ketersediaan platform mencapai 99.9% termasuk saat lonjakan promo penjualan.",
				"Security/Privacy: Perlindungan data transaksi pelanggan dan enkripsi proses checkout (HTTPS/TLS).",
			},
			UserFlowSteps: []string{
				fmt.Sprintf("User membuka platform %s (%s).", platform, pName),
				"User menjelajahi katalog produk makanan instan dan memilih varian mie yang diinginkan.",
				"User memasukkan produk pilihan ke dalam keranjang belanja dan menuju halaman checkout.",
				"User mengisi alamat pengiriman, memilih metode pembayaran, dan mengonfirmasi transaksi.",
				"Sistem mengonfirmasi pembayaran, memperbarui stok, dan menampilkan resi transaksi serta pelacakan pesanan.",
			},
			FlowchartMermaid: fmt.Sprintf(`flowchart TD
    A[User Buka Platform %s] --> B[Jelajahi Katalog Produk Mie Instan]
    B --> C[Filter / Cari Varian Rasa & Paket]
    C --> D[Pilih Varian & Tambah ke Keranjang]
    D --> E[Review Keranjang & Isi Alamat Pengiriman]
    E --> F[Pilih Metode Pembayaran Digital]
    F --> G{Pembayaran Diverifikasi?}
    G -- Ya --> H[Sistem Buat Resi & Kirim Notifikasi Pesanan]
    G -- Tidak --> I[Tampilkan Informasi Error & Opsi Ulangi]
    I --> F
    H --> J[Tim Operasional Memproses Pengiriman Produk]`, platform),
			SequenceMermaid: `sequenceDiagram
    autonumber
    actor Consumer as Consumer User
    participant Web as Web Frontend
    participant API as Catalog & Order API
    participant Pay as Payment Gateway
    participant DB as Database Storage

    Consumer->>Web: Akses Katalog Produk Mie Instan
    Web->>API: Request Catalog & Stock List
    API->>DB: Fetch Products & Variants Data
    DB-->>API: Return Active Products & Stock
    API-->>Web: Render Product Grid UI
    Consumer->>Web: Pilih Varian & Klik Checkout
    Web->>API: Create Order Payload
    API->>Pay: Initiate Payment Transaction
    Pay-->>Consumer: Tampilkan Prompt Pembayaran
    Consumer->>Pay: Selesaikan Pembayaran
    Pay-->>API: Payment Verification Callback
    API->>DB: Update Order Status & Deduct Stock
    API-->>Web: Return Order Success Status
    Web-->>Consumer: Tampilkan Resi & Pelacakan Pesanan`,
			StateMermaid: `stateDiagram-v2
    [*] --> Idle: User Session Start
    Idle --> BrowsingCatalog: Browse Products & Variants
    BrowsingCatalog --> CartUpdated: Add Item to Shopping Cart
    CartUpdated --> CheckoutPending: Submit Checkout Details
    CheckoutPending --> ProcessingPayment: Initiate Payment Gateway
    ProcessingPayment --> PaymentFailed: Payment Timed Out / Rejected
    PaymentFailed --> CheckoutPending: Retry Payment Step
    ProcessingPayment --> OrderPaid: Payment Confirmed
    OrderPaid --> InFulfillment: Packing & Shipping Process
    InFulfillment --> Completed: Delivered to Customer
    Completed --> [*]`,
			TahapanFlow: []string{
				fmt.Sprintf("**Entry point**: User mengakses platform %s dan disambut oleh etalase katalog produk makanan instan.", platform),
				"**Context collection**: Sistem mengumpulkan item pilihan, jumlah kuantitas, dan data lokasi pengiriman.",
				"**Processing**: Sistem memvalidasi ketersediaan stok produk mie, menghitung biaya, dan memproses kanal pembayaran.",
				"**Output review**: User melihat rincian nota transaksi, konfirmasi pembayaran, serta estimasi waktu tiba.",
				"**Final action**: Pesanan diteruskan ke sistem operasional toko untuk dikemas dan dikirimkan.",
			},
			SprintRoadmap: []string{
				"**Sprint 1 — Core Architecture & Catalog DB**: Inisialisasi skema database produk, varian rasa mie instan, dan backend API.",
				"**Sprint 2 — Shopping Cart & Checkout Flow**: Pengembangan UI etalase, filter pencarian rasa, keranjang belanja, dan halaman checkout.",
				"**Sprint 3 — Payment Gateway & Order Tracking**: Integrasi kanal pembayaran digital, pembuatan nota resi, dan status pelacakan pesanan.",
				"**Sprint 4 — Admin Dashboard & Launch**: Dashboard manajemen stok operasional, pengujian end-to-end, dan peluncuran produk.",
			},
			PhasedRoadmap: []string{
				"**Fase 1 (Discovery & Setup)**: Penyusunan spesifikasi katalog varian mie, perancangan database produk, dan wireframe UI.",
				"**Fase 2 (Build Core)**: Pengembangan komponen katalog interaktif, modul keranjang belanja, dan API pesanan.",
				"**Fase 3 (Testing & Refinement)**: Integrasi payment gateway, pengujian alur pesanan, dan perbaikan UX checkout.",
				"**Fase 4 (Deployment & Launch)**: Peluncuran versi rintisan (MVP) di platform Web dan pemantauan transaksi awal.",
			},
			ChecklistRoadmap: []string{
				"Task 1: Setup fondasi arsitektur project dan struktur database produk makanan instan.",
				"Task 2: Buat tampilan etalase katalog produk, fitur filter varian, dan halaman detail produk.",
				"Task 3: Implementasikan keranjang belanja serta alur pengisian alamat checkout.",
				"Task 4: Integrasikan kanal pembayaran digital dan halaman resi transaksi pesanan.",
				"Task 5: Jalankan pengujian transaksi akhir dan rilis versi rintisan.",
			},
			GanttMermaid: fmt.Sprintf(`gantt
    title Implementation Roadmap - %s
    dateFormat YYYY-MM-DD
    section Setup & Database
    Architecture & Catalog Schema :a1, 2025-01-01, 6d
    Core Catalog & Order API      :a2, after a1, 8d
    section Frontend Development
    Catalog & Filter UI           :b1, 2025-01-08, 7d
    Cart & Checkout Integration   :b2, after b1, 8d
    section Release & Launch
    Payment Gateway & QA          :c1, after a2, 7d
    Deployment & Launch           :crit, after b2, 4d`, pName),
			SuccessMetrics: []string{
				"Checkout Conversion Rate: Persentase pengunjung katalog yang berhasil menyelesaikan transaksi checkout (> 15%).",
				"Cart Abandonment Rate: Rasio pembatalan di keranjang belanja terpelihara di bawah 25%.",
				"Average Order Value (AOV): Nilai rata-rata transaksi pembelian varian produk mie instan per pesanan.",
				"Customer Satisfaction (CSAT): Tingkat kepuasan pengguna terhadap kemudahan pemesanan produk (> 4.5/5).",
			},
			RisksAndMitigations: [][2]string{
				{"Stok varian produk mie habis saat checkout", "Implementasi reservasi stok sementara (stock holding) selama 15 menit saat pemesanan diproses."},
				{"Kegagalan transaksi pada payment gateway", "Penyediaan opsi pembayaran cadangan serta fitur ulangi bayar tanpa memasukkan ulang pesanan."},
				{"Lonjakan trafik katalog saat sesi promosi", "Penerapan caching data katalog dan pengujian beban (load testing) sebelum promo."},
			},
			MVPScope: []string{
				"Etalase katalog interaktif untuk penjelajahan varian produk makanan instan mie.",
				"Keranjang belanja dan form pengisian data pengiriman.",
				"Integrasi pembayaran digital standar (Transfer / E-Wallet).",
				"Pelacakan status transaksi dasar (Pending, Diproses, Selesai).",
			},
			ReleasePlan: []string{
				"Fase 1: Alpha testing internal & uji validasi skema database produk.",
				"Fase 2: Beta release terbatas untuk kelompok pengguna terdaftar.",
				"Fase 3: Peluncuran publik MVP di platform Web.",
				"Fase 4: Iterasi pengembangan fitur rekomendasi varian dan sistem reward.",
			},
			OpenQuestions: []string{
				"Metode pembayaran digital apa yang paling disukai oleh konsumen target makanan instan?",
				"Apakah integrasi otomatis dengan penyedia jasa kurir instan diperlukan pada rilis rintisan?",
				"Bagaimana alur penanganan pesanan jika terjadi kehabisan stok varian mie secara tiba-tiba?",
			},
		}
	}

	if isBot {
		return domainContext{
			OverviewDesc: fmt.Sprintf("%s adalah %s yang berjalan di platform %s untuk mengotomatisasikan workflow dan pengiriman notifikasi bagi %s. Dokumen ini menjelaskan spesifikasi produk, diagram interaksi, dan rencana eksekusi.", pName, pTypeLower, platform, tUserLower),
			ProblemDesc:  fmt.Sprintf("User (%s) sering menghadapi inefisiensi akibat penanganan aktivitas manual, pemantauan update terpisah, dan respon lambat. Bot ini hadir untuk memberikan otomatisasi cerdas, pengiriman notifikasi terstruktur, dan kemudahan eksekusi command.", tUser),
			Goals: []string{
				fmt.Sprintf("Menyediakan antarmuka bot yang responsif dan mudah dioperasikan bagi %s.", tUserLower),
				"Mengurangi pekerjaan manual melalui pengolahan pesan dan perintah otomatis.",
				fmt.Sprintf("Memastikan keandalan penyampaian pesan dalam scope %s.", scope),
				"Menyiapkan modul integrasi webhook dan perintah kustom untuk iterasi mendatang.",
			},
			NonGoals: []string{
				"Membangun infrastruktur chat platform kustom di luar sistem platform utama yang didukung.",
				"Memproses eksekusi komputasi berat tanpa arsitektur worker terpisah.",
			},
			PrimaryUser:   fmt.Sprintf("%s (Pengguna aktif bot/otomasi)", tUser),
			SecondaryUser: "Administrator Server & Tim Pengembang Bot",
			UserStories: []string{
				fmt.Sprintf("Sebagai %s, saya ingin menjalankan command bot secara intuitif agar mendapatkan hasil dengan cepat.", tUserLower),
				fmt.Sprintf("Sebagai %s, saya ingin menerima notifikasi otomatis saat terjadi event penting agar tidak tertinggal update.", tUserLower),
				"Sebagai Admin Server, saya ingin mengonfigurasi opsi bot agar sesuai dengan aturan komunitas/tim.",
			},
			FunctionalReqs: []string{
				fmt.Sprintf("%s harus menyediakan pendaftaran perintah (slash command / text command) yang ramah pengguna.", pName),
				"Bot harus mampu memproses payload incoming event dan memberikan rincian respon yang terformat.",
				"Bot harus memiliki mekanisme penanganan error dan pesan feedback yang informatif.",
				fmt.Sprintf("Bot harus berjalan dengan stabil di platform %s.", platform),
			},
			NonFunctionalReqs: []string{
				"Responsiveness: Waktu respon terhadap command < 800ms.",
				"Reliability: Uptime operasional bot mencapai 99.9%.",
				"Security: Penyimpanan token API dan credential terenkripsi dengan aman.",
			},
			UserFlowSteps: []string{
				fmt.Sprintf("User mengundang atau mengakses bot %s di platform %s.", pName, platform),
				"User menginput perintah (command) atau memicu event tertentu.",
				"Bot mengesahkan hak akses dan memproses argumen perintah.",
				"Bot mengeksekusi logika bisnis dan mengirimkan pesan balasan/file output.",
			},
			FlowchartMermaid: fmt.Sprintf(`flowchart TD
    A[User Trigger Command / Event on %s] --> B[Bot Core Authenticate Payload]
    B --> C{Argument Valid?}
    C -- Ya --> D[Process Business Logic / Event Payload]
    C -- Tidak --> E[Return Error Message / Usage Guide]
    D --> F[Send Formatted Response Message]
    F --> G[Log Transaction Audit]`, platform),
			SequenceMermaid: `sequenceDiagram
    autonumber
    actor User
    participant Bot as Bot Adapter
    participant Core as Engine Logic
    participant Ext as External Service / DB

    User->>Bot: Send Slash Command / Mention
    Bot->>Core: Forward Event Context
    Core->>Ext: Fetch / Update Data
    Ext-->>Core: Return Operation Result
    Core-->>Bot: Format Markdown / Attachment
    Bot-->>User: Reply Channel Message`,
			StateMermaid: `stateDiagram-v2
    [*] --> Standby: Bot Ready
    Standby --> EventReceived: Payload Incoming
    EventReceived --> Validating: Check Permission & Args
    Validating --> Executing: Logic Execution
    Executing --> Replied: Send Output Message
    Replied --> Standby`,
			TahapanFlow: []string{
				"**Entry point**: User mengetikkan slash command atau memicu trigger event.",
				"**Context collection**: Bot menguraikan konteks pesan, user ID, channel ID, dan parameter.",
				"**Processing**: Core engine menjalankan validasi dan pemrosesan bisnis.",
				"**Output review**: User menerima pesan balasan interaktif atau lampiran dokumen.",
			},
			SprintRoadmap: []string{
				"**Sprint 1 — Bot Skeleton & Platform Gateway**: Setup koneksi API platform Discord/Chat dan handler command dasar.",
				"**Sprint 2 — Business Logic & Commands**: Implementasi fungsi utama, komposting pesan, dan interaksi button.",
				"**Sprint 3 — Error Handling & Webhooks**: Pemantauan event otomatis, retry mechanism, dan logging.",
				"**Sprint 4 — Polish & Release**: Testing beban pesan, pengayaan dokumentasi command, dan peluncuran.",
			},
			PhasedRoadmap: []string{
				"**Fase 1 (Setup)**: Registrasi bot application, penyusunan arsitektur event handler.",
				"**Fase 2 (Build)**: Pengembangan fitur utama dan integrasi payload handler.",
				"**Fase 3 (Test)**: Testing skenario konkurensi pesan dan pengujian error handling.",
				"**Fase 4 (Deploy)**: Deployment ke server produksi dan pemantauan ubi-uptime.",
			},
			ChecklistRoadmap: []string{
				"Task 1: Setup project bot gateway & registrasi API token.",
				"Task 2: Buat handler slash command dan parser argumen.",
				"Task 3: Hubungkan engine pemrosesan dan format tampilan pesan balasan.",
				"Task 4: Rilis bot dan aktifkan logging audit.",
			},
			GanttMermaid: fmt.Sprintf(`gantt
    title Implementation Roadmap - %s
    dateFormat YYYY-MM-DD
    section Bot Core
    Bot Architecture & Gateway  :a1, 2025-01-01, 5d
    Command Handlers & Logic    :a2, after a1, 8d
    section Deployment & QA
    Testing & Error Handling    :b1, after a2, 5d
    Production Launch           :crit, after b1, 3d`, pName),
			SuccessMetrics: []string{
				"Command Execution Rate: Jumlah perintah yang berhasil dieksekusi per hari.",
				"Latency Time: Rata-rata waktu respon bot terhadap trigger user (< 1 detik).",
				"Error Rate: Persentase kegagalan penanganan pesan di bawah 0.5%.",
			},
			RisksAndMitigations: [][2]string{
				{"Rate limit API dari platform host", "Penerapan queue manager dan throttling pengiriman pesan."},
				{"Koneksi gateway terputus secara mendadak", "Fitur auto-reconnect dengan exponential backoff policy."},
			},
			MVPScope: []string{
				"Set perintah inti (slash commands) terdaftar.",
				"Satu flow utama pemrosesan pesan & balasan terformat.",
				"Handling error dasar jika perintah tidak valid.",
			},
			ReleasePlan: []string{
				"Fase 1: Testing privat di server pengembang.",
				"Fase 2: Beta launch di server komunitas terbatas.",
				"Fase 3: Rilis publik dan pendaftaran bot.",
			},
			OpenQuestions: []string{
				"Apakah bot memerlukan izin administratif khusus saat diinstal di server?",
				"Bagaimana skenario fallback jika layanan eksternal mengalami downtime?",
			},
		}
	}

	if isSaaS {
		return domainContext{
			OverviewDesc: fmt.Sprintf("%s adalah platform %s yang dirancang untuk mendukung pemantauan, pengelolaan, dan analisis data bagi %s di platform %s. PRD ini mendefinisikan arsitektur sistem, requirement fungsional, dan peta jalan implementasi.", pName, pTypeLower, tUserLower, platform),
			ProblemDesc:  fmt.Sprintf("User (%s) mengalami kesulitan dalam mengonsolidasikan data bisnis, mengelola alur kerja terpusat, dan membuat keputusan berbasis data secara realtime. Platform ini menyediakan solusi dashboard terpadu yang terstruktur dan terukur.", tUser),
			Goals: []string{
				fmt.Sprintf("Menyediakan dashboard manajemen terintegrasi untuk %s.", tUserLower),
				"Mengoptimalkan visualisasi metrik dan otomatisasi laporan operasional.",
				fmt.Sprintf("Menjamin keamanan data dan manajemen akses dalam skala %s.", scope),
			},
			NonGoals: []string{
				"Mengembangkan modul kustom di luar cakupan alur kerja inti pada rilis awal.",
			},
			PrimaryUser:   fmt.Sprintf("%s (Pengguna utama dashboard & fitur analitik)", tUser),
			SecondaryUser: "Executive Stakeholders, System Administrator",
			UserStories: []string{
				fmt.Sprintf("Sebagai %s, saya ingin melihat ringkasan metrik utama pada dashboard agar dapat memantau perkembangan terkini.", tUserLower),
				fmt.Sprintf("Sebagai %s, saya ingin mengekspor laporan terformat agar dapat dibagikan kepada stakeholder terkait.", tUserLower),
			},
			FunctionalReqs: []string{
				fmt.Sprintf("%s harus memiliki modul autentikasi dan manajemen hak akses pengguna.", pName),
				"Sistem harus menampilkan ringkasan analitik dan grafik data secara realtime.",
				"Sistem harus menyediakan fitur ekspor laporan ke format dokumen standar (PDF/CSV).",
				fmt.Sprintf("Sistem harus berjalan optimal pada platform %s.", platform),
			},
			NonFunctionalReqs: []string{
				"Usability: Kemudahan navigasi dashboard dengan kurva belajar minimal.",
				"Performance: Waktu muat widget data dashboard < 2 detik.",
				"Security: Enkripsi data sensitif (AES-256) dan role-based access control (RBAC).",
			},
			UserFlowSteps: []string{
				fmt.Sprintf("User login ke platform %s (%s).", platform, pName),
				"User membuka dashboard utama dan memilih menu analitik / laporan.",
				"User mengatur filter tanggal atau parameter data yang ingin dianalisis.",
				"Sistem menampilkan sajian visual data dan menyediakan opsi ekspor hasil.",
			},
			FlowchartMermaid: `flowchart TD
    A[User Login ke Dashboard] --> B[Akses Menu Analitik / Laporan]
    B --> C[Terapkan Filter Data & Periode]
    C --> D[Sistem Olah Data & Render Grafik]
    D --> E{User Butuh Ekspor?}
    E -- Ya --> F[Generasi Dokumentasi PDF / CSV]
    E -- Tidak --> G[Tinjau Metrik di Screen]`,
			SequenceMermaid: `sequenceDiagram
    autonumber
    actor User
    participant UI as Web Dashboard
    participant API as Analytics API
    participant DB as Data Warehouse

    User->>UI: Login & Request Dashboard View
    UI->>API: Query Aggregated Metrics
    API->>DB: Execute Analytics Query
    DB-->>API: Return Metrics Data
    API-->>UI: Send JSON Data Payload
    UI-->>User: Render Interactive Charts`,
			StateMermaid: `stateDiagram-v2
    [*] --> Unauthenticated: Open App
    Unauthenticated --> Authenticated: Login Success
    Authenticated --> ViewingDashboard: Load Main View
    ViewingDashboard --> ExportingReport: Trigger Download
    ExportingReport --> ViewingDashboard: Export Completed`,
			TahapanFlow: []string{
				"**Entry point**: User melakukan otentikasi masuk ke platform SaaS.",
				"**Context collection**: Sistem memuat profil pengguna dan preferensi filter dashboard.",
				"**Processing**: Engine mengeksekusi aggregasi data dan pembuatan visualisasi grafik.",
				"**Output review**: User melihat sajian ringkasan metrik dan laporan bisnis.",
			},
			SprintRoadmap: []string{
				"**Sprint 1 — Authentication & Database Schema**: Setup arsitektur database, tabel pengguna, dan sistem auth.",
				"**Sprint 2 — Core Analytics & Dashboard UI**: Pembuatan komponen widget, grafik, dan API pengolah data.",
				"**Sprint 3 — Reporting & Export Engine**: Fitur filter data dinamis dan modul ekspor laporan PDF/CSV.",
				"**Sprint 4 — Security Audit & Launch**: Pengujian hak akses (RBAC), optimasi query, dan release.",
			},
			PhasedRoadmap: []string{
				"**Fase 1 (Architecture)**: Perancangan skema data warehouse dan desain UI/UX dashboard.",
				"**Fase 2 (Development)**: Pembangunan fitur analitik inti dan manajemen akun pengguna.",
				"**Fase 3 (Testing)**: Audit keamanan data, load testing, dan penyempurnaan UI.",
				"**Fase 4 (Deployment)**: Peluncuran ke lingkungan produksi SaaS.",
			},
			ChecklistRoadmap: []string{
				"Task 1: Setup fondasi project SaaS dan modul otentikasi.",
				"Task 2: Buat tampilan dashboard analitik dan komponen visualisasi.",
				"Task 3: Hubungkan generator laporan dan fitur ekspor.",
				"Task 4: Peluncuran aplikasi ke pengguna.",
			},
			GanttMermaid: fmt.Sprintf(`gantt
    title Implementation Roadmap - %s
    dateFormat YYYY-MM-DD
    section SaaS Foundation
    Auth & Schema Setup        :a1, 2025-01-01, 7d
    Core Dashboard & API       :a2, after a1, 9d
    section Analytics & Launch
    Reporting & Security Audit :b1, after a2, 7d
    SaaS Production Release    :crit, after b1, 4d`, pName),
			SuccessMetrics: []string{
				"Daily Active Users (DAU): Tingkat keaktifan pengguna dalam mengakses dashboard harian.",
				"Report Generation Rate: Frekuensi pembuatan dan pengunduhan laporan analitik.",
				"System Uptime: Menjaga ketersediaan portal sebesar 99.9%.",
			},
			RisksAndMitigations: [][2]string{
				{"Query data lambat saat dataset membesar", "Penerapan query indexing, materialized views, dan caching layer."},
				{"Kebocoran akses data antar tenant", "Implementasi isolasi data tingkat database dan audit log ketat."},
			},
			MVPScope: []string{
				"Otentikasi login dan manajemen profil dasar.",
				"Dashboard visualisasi metrik bisnis utama.",
				"Ekspor data ke format CSV.",
			},
			ReleasePlan: []string{
				"Fase 1: Closed alpha untuk pemangku kepentingan internal.",
				"Fase 2: Open beta rintisan bagi pengguna terbatas.",
				"Fase 3: Rilis komersial v1.0.",
			},
			OpenQuestions: []string{
				"Integrasi data dari sumber mana yang paling mutlak dibutuhkan pada versi awal?",
				"Apakah diperlukan kustomisasi widget dashboard oleh pengguna?",
			},
		}
	}

	// General / Fallback Domain
	cleanIdeaSummary := idea
	if len(cleanIdeaSummary) > 120 {
		cleanIdeaSummary = cleanIdeaSummary[:117] + "..."
	}

	return domainContext{
		OverviewDesc: fmt.Sprintf("%s adalah %s yang dikembangkan untuk memfasilitasi kebutuhan %s bagi %s melalui platform %s. Dokumen ini menyajikan gambaran solusi, spesifikasi kebutuhan, flow arsitektur, dan peta eksekusi rilis.", pName, pTypeLower, cleanIdeaSummary, tUserLower, platform),
		ProblemDesc:  fmt.Sprintf("Target user (%s) membutuhkan solusi digital yang lebih efisien, terstruktur, dan terukur untuk menangani tantangan terkait %s. Tanpa platform ini, alur kerja berisiko tetap bergantung pada proses manual yang tidak efisien.", tUser, cleanIdeaSummary),
		Goals: []string{
			fmt.Sprintf("Menyediakan antarmuka inti yang intuitif bagi %s untuk memproses %s.", tUserLower, cleanIdeaSummary),
			"Mengurangi hambatan dan waktu operasional dalam alur kerja utama.",
			fmt.Sprintf("Menghasilkan value yang dapat tervalidasi secara terukur dalam scope %s.", scope),
			"Membangun dasar arsitektur yang fleksibel untuk pengembangan iterasi fitur berikutnya.",
		},
		NonGoals: []string{
			"Mengimplementasikan fitur sekunder sebelum fungsionalitas inti tervalidasi.",
			"Mengoptimalkan skala enterprise penuh sebelum ada sinyal adopsi nyata dari pengguna.",
		},
		PrimaryUser:   fmt.Sprintf("%s (Pengguna utama fitur produk)", tUser),
		SecondaryUser: "Stakeholder Operasional, Evaluator Sistem, dan Tim Administrator",
		UserStories: []string{
			fmt.Sprintf("Sebagai %s, saya ingin mengakses modul utama dengan cepat agar dapat menyelesaikan workflow tanpa hambatan.", tUserLower),
			fmt.Sprintf("Sebagai %s, saya ingin melihat luaran/output yang jelas agar bisa menilai hasil pemrosesan secara akurat.", tUserLower),
			"Sebagai Admin, saya ingin melihat log aktivitas dan status sistem untuk memastikan keandalan operasi.",
		},
		FunctionalReqs: []string{
			fmt.Sprintf("%s harus menyediakan titik masuk (entry point) utama untuk memulai workflow bisnis.", pName),
			"Produk harus menyimpan dan menyajikan output utama secara jelas dan terstruktur.",
			"Produk harus memberikan pesan umpan balik (feedback) dan penanganan error yang mudah dipahami.",
			fmt.Sprintf("Produk harus mendukung pengoperasian optimal pada platform %s.", platform),
			fmt.Sprintf("Produk harus memenuhi standar fitur minimal sesuai scope %s.", scope),
		},
		NonFunctionalReqs: []string{
			"Usability: Antarmuka mudah dipahami sehingga pengguna baru dapat menguasainya dalam waktu < 5 menit.",
			"Reliability: Fungsionalitas inti berjalan stabil pada skenario penggunaan normal.",
			"Performance: Waktu respon interaksi utama berlangsung cepat dan tidak blocking.",
			"Security/Privacy: Data pengguna diproses secara aman dan sesuai prinsip privasi minimum.",
		},
		UserFlowSteps: []string{
			fmt.Sprintf("User membuka aplikasi pada platform %s (%s).", platform, pName),
			"User memasukkan masukan/konteks kebutuhan awal.",
			"Sistem memproses masukan dan menjalankan logika bisnis inti.",
			"User meninjau output, melakukan penyesuaian jika diperlukan, dan menyelesaikan transaksi.",
		},
		FlowchartMermaid: fmt.Sprintf(`flowchart TD
    A[User Akses Platform %s] --> B[Input Kebutuhan / Konteks Awal]
    B --> C[Validasi Input Data]
    C --> D[Proses Fitur Inti Engine]
    D --> E[Tampilkan Output / Hasil Utama]
    E --> F{User Puas / Konfirmasi?}
    F -- Ya --> G[Simpan Output / Finalisasi Transaksi]
    F -- Tidak --> H[Edit Input / Ulangi Pemrosesan]
    H --> B`, platform),
		SequenceMermaid: `sequenceDiagram
    autonumber
    actor User
    participant Client as Platform Client
    participant Core as Core Engine
    participant DB as Storage System

    User->>Client: Triggers Action / Command
    Client->>Core: Sends Payload & Context Data
    Core->>Core: Validate & Process Request
    Core->>DB: Save Session State
    DB-->>Core: Confirm Data Saved
    Core-->>Client: Return Formatted Output Response
    Client-->>User: Display Result / Notification`,
		StateMermaid: `stateDiagram-v2
    [*] --> Idle: Session Init
    Idle --> CollectingContext: User Input Action
    CollectingContext --> Validating: Submit Form / Selection
    Validating --> Processing: Data Validated
    Validating --> CollectingContext: Validation Failed
    Processing --> Completed: Execution Success
    Completed --> [*]`,
		TahapanFlow: []string{
			"**Entry point**: User masuk melalui menu atau perintah utama platform.",
			"**Context collection**: Sistem mengumpulkan parameter kebutuhan minimum dari user.",
			"**Processing**: Sistem memvalidasi data dan mengeksekusi logika pemrosesan.",
			"**Output review**: User melihat ringkasan hasil dan diberikan opsi koreksi.",
			"**Final action**: User menyimpan, mengunduh, atau mengonfirmasi alur produk.",
		},
		SprintRoadmap: []string{
			"**Sprint 1 — Core & Setup**: Setup environment, arsitektur dasar, dan perancangan skema data.",
			"**Sprint 2 — MVP Features**: Implementasi alur pemrosesan utama dan penyusunan antarmuka.",
			"**Sprint 3 — Integration & Testing**: Validasi alur end-to-end, penanganan error, dan refinement UX.",
			"**Sprint 4 — Polish & Release**: Dokumemtasi, optimasi performa, dan peluncuran versi rintisan.",
		},
		PhasedRoadmap: []string{
			"**Fase 1 (Discovery & Setup)**: Penyusunan spesifikasi spec, validasi kebutuhan ide, dan mockup arsitektur.",
			"**Fase 2 (Build Core)**: Pembuatan komponen utama dan logika bisnis inti.",
			"**Fase 3 (Testing & Refinement)**: Pengujian internal, bug fixing, dan penyesuaian UX.",
			"**Fase 4 (Deployment)**: Peluncuran terbatas (soft launch) dan monitoring awal.",
		},
		ChecklistRoadmap: []string{
			"Task 1: Setup fondasi arsitektur project dan integrasi platform.",
			"Task 2: Buat alur input-output utama produk.",
			"Task 3: Jalankan uji coba internal dan simulasikan penanganan error.",
			"Task 4: Release versi pertama ke pengguna.",
		},
		GanttMermaid: fmt.Sprintf(`gantt
    title Implementation Roadmap - %s
    dateFormat YYYY-MM-DD
    section Setup & Core
    Architecture & Spec  :a1, 2025-01-01, 5d
    Core Business Logic  :a2, after a1, 9d
    section Development & Release
    UI Integration & QA  :b1, 2025-01-15, 8d
    Deployment & Launch  :crit, after b1, 4d`, pName),
		SuccessMetrics: []string{
			"Activation Rate: Persentase pengguna yang berhasil menyelesaikan workflow pertama.",
			"Task Completion Time: Menurunnya waktu rata-rata dalam menyelesaikan proses inti.",
			"User Retention: Tingkat penggunaan kembali produk setelah sesi pertama.",
			"Qualitative Feedback: Evaluasi positif dari pengguna terkait efisiensi solusi.",
		},
		RisksAndMitigations: [][2]string{
			{"Pengembangan scope melebar terlalu cepat", fmt.Sprintf("Fokus ketat pada scope %s dan tampung ide lanjutan di backlog terpisah.", scope)},
			{"Pengguna membutuhkan panduan awal", "Penyediaan tur singkat (onboarding) dan dokumen panduan penggunaan."},
		},
		MVPScope: []string{
			"Satu alur kerja utama terintegrasi dari awal hingga akhir.",
			"Output hasil pemrosesan yang siap dipakai atau dibagikan.",
			"Penanganan state sukses, kosong (empty state), dan error.",
			"Dokumentasi penggunaan dasar.",
		},
		ReleasePlan: []string{
			"Phase 1: Validasi konsep dan pembuatan prototype.",
			"Phase 2: Pembangunan MVP fitur inti.",
			"Phase 3: Beta testing terpandu dengan kelompok pengguna terbatas.",
			"Phase 4: Release publik dan iterasi berbasis metrik.",
		},
		OpenQuestions: []string{
			"Integrasi layanan pihak ketiga apa yang paling kritikal untuk ada pada rilis awal?",
			"Apa indikator keberhasilan utama dalam 2-4 minggu pertama setelah peluncuran?",
		},
	}
}
