package agent

import (
	"strings"

	"github.com/gede-cahya/Smara-CLI/pkg/llm"
)

func isDirectImageGenerationRequest(prompt string) bool {
	p := strings.ToLower(strings.TrimSpace(prompt))
	if p == "" {
		return false
	}
	if isSoftwareImageFeaturePrompt(p) {
		return false
	}

	verbs := []string{"buat", "buatkan", "bikin", "generate", "hasilkan", "create", "make", "draw", "desain", "design"}
	objects := []string{"logo", "gambar", "image", "poster", "ilustrasi", "illustration", "icon", "ikon", "banner", "sticker", "maskot", "mascot"}

	hasVerb := false
	for _, v := range verbs {
		if strings.Contains(p, v) {
			hasVerb = true
			break
		}
	}
	if !hasVerb {
		return false
	}
	for _, o := range objects {
		if strings.Contains(p, o) {
			return true
		}
	}
	return false
}

func isSoftwareImageFeaturePrompt(prompt string) bool {
	softwareTerms := []string{
		"fitur", "feature", "implement", "implementation", "kode", "code", "coding", "endpoint",
		"component", "komponen", "ui", "aplikasi", "app", "tool", "tools", "workflow",
		"upload", "api", "backend", "frontend", "integrasi", "integration", "plugin", "sdk",
	}
	for _, term := range softwareTerms {
		if strings.Contains(prompt, term) {
			return true
		}
	}
	imageToImageTerms := []string{"image to image", "image-to-image", "img2img", "edit image", "image edit", "edit gambar"}
	for _, term := range imageToImageTerms {
		if strings.Contains(prompt, term) {
			return true
		}
	}
	return false
}

// toolGroup maps tool names to their group for filtering.
// Groups: core, ssh, lsp, binary, graphify, skill, image, planning, export, memory
var toolGroup = map[string]string{
	// core — always available
	"run_command":              "core",
	"view_file":                "core",
	"read_file":                "core",
	"write_file":               "core",
	"delete_file":              "core",
	"list_dir":                 "core",
	"edit_file":                "core",
	"grep_search":              "core",
	"search_path":              "core",
	"get_cwd":                  "core",
	"analyze_workspace":        "core",
	"web_search":               "core",
	"web_fetch":                "core",
	"user_model":               "core",
	"request_iteration_budget": "core",
	"iteration_budget_status":  "core",
	// ssh
	"ssh_exec":     "ssh",
	"ssh_view_file": "ssh",
	"ssh_list_dir":  "ssh",
	"ssh_upload":    "ssh",
	"ssh_download":  "ssh",
	"ssh_manage":    "ssh",
	// lsp
	"lsp_hover":            "lsp",
	"lsp_definition":       "lsp",
	"lsp_references":       "lsp",
	"lsp_document_symbols": "lsp",
	// binary analysis
	"analyze_binary":       "binary",
	"extract_strings":      "binary",
	"scan_signature":       "binary",
	"analyze_dependencies": "binary",
	"generate_call_graph":  "binary",
	// graphify
	"graphify_init":  "graphify",
	"graphify_query": "graphify",
	// skill
	"skill_run":          "skill",
	"skill_instructions": "skill",
	"skill_create":       "skill",
	"skill_list":         "skill",
	"skill_install":      "skill",
	"skill_delete":       "skill",
	// image
	"generate_image":  "image",
	"edit_image":      "image",
	"analyze_image":   "image",
	// planning
	"planning_template": "planning",
	// export
	"export_data": "export",
	// memory
	"remember":         "memory",
	"search_memories":  "memory",
	// misc
	"serve_project":      "core",
	"connect_mcp":        "core",
	"disconnect_mcp":     "core",
	"schedule_reminder":  "core",
}

// toolGroupTriggers maps non-core tool groups to keywords that activate them.
// If the user's prompt contains any of these keywords, the group is included.
// Core tools are always included regardless of prompt content.
var toolGroupTriggers = map[string][]string{
	"ssh": {
		"ssh", "server", "vps", "deploy", "remote", "upload file", "download file",
		"sftp", "scp", "deployment", "host", "nginx", "apache", "systemctl",
		"service", "docker", "container", "kubernetes", "k8s", "pm2",
	},
	"lsp": {
		"lsp", "definition", "hover", "references", "symbol", "code intelligence",
		"go to definition", "find references", "type info", "dokumentasi simbol",
	},
	"binary": {
		"binary", "reverse", "disassemble", "decompile", "strings", "signature",
		"call graph", "elf", "pe ", "mach-o", "malware", "hex", "objdump",
		"radare", "ghidra", "ida", "analisis biner",
	},
	"graphify": {
		"graphify", "knowledge graph", "graf pengetahuan", "graph init", "graph query",
	},
	"skill": {
		"skill", "automate", "routine", "resep", "otomasi", "skill_run",
		"buat skill", "jalankan skill", "daftar skill",
	},
	"planning": {
		"planning", "template", "rencana", "roadmap", "plan ",
	},
	"export": {
		"export", "csv", "pdf", "tabel", "spreadsheet", "unduh data",
	},
	"memory": {
		"remember", "ingat", "memori", "recall", "simpan info", "cari memori",
		"search memories", "ingatan",
	},
	"android": {
		"android", "gradle", "apk", "adb", "emulator", "manifest.xml", "sdk", "avd",
	},
	"chrome": {
		"chrome", "browser", "devtools", "inspect", "network", "lcp", "performance",
		"console", "page", "url", "a11y", "accessibility", "lighthouse", "memory leak",
		"heap snapshot", "audit", "extension", "manifest.json",
	},
	"science": {
		"science", "biology", "dna", "rna", "protein", "gene", "uniprot", "variant",
		"dbsnp", "gnomad", "clinvar", "pubmed", "arxiv", "biorxiv", "chembl", "pubchem",
		"clinical", "trial", "structure", "pdb", "alphafold", "sequence", "alignment",
		"blast", "cloz", "fasta", "pathway", "reactome", "chem", "molecule", "interaction",
		"string-db", "string db", "ontology", "ols", "quickgo", "interpro", "jaspar",
		"unibind", "conservation", "phylo", "paper", "journal", "literature",
	},
	"antigravity": {
		"antigravity", "sdk", "agent", "multi-agent", "orchestrate", "agy",
	},
}

func getToolGroup(tool llm.ToolFunction) string {
	// First check built-in toolGroup mapping
	if group, ok := toolGroup[tool.Name]; ok {
		return group
	}

	// For MCP tools, determine their group based on tool name and description keywords
	nameLower := strings.ToLower(tool.Name)
	descLower := strings.ToLower(tool.Description)

	// Check science keywords
	scienceKeywords := []string{
		"uniprot", "dbsnp", "gnomad", "clinvar", "pubmed", "arxiv", "biorxiv",
		"chembl", "pubchem", "clinical", "trial", "pdb", "alphafold", "blast",
		"sequence", "alignment", "pathway", "reactome", "string_db", "string_database",
		"literature", "ols", "quickgo", "interpro", "jaspar", "unibind", "conservation",
		"foldseek", "pymol", "chebi", "protein", "dna", "rna", "gene", "variant",
	}
	for _, kw := range scienceKeywords {
		if strings.Contains(nameLower, kw) || strings.Contains(descLower, kw) {
			return "science"
		}
	}

	// Check chrome/devtools keywords
	chromeKeywords := []string{
		"chrome", "devtools", "a11y", "accessibility", "lighthouse", "lcp",
		"memory_leak", "heap", "screenshot", "browser", "page",
	}
	for _, kw := range chromeKeywords {
		if strings.Contains(nameLower, kw) || strings.Contains(descLower, kw) {
			return "chrome"
		}
	}

	// Check android keywords
	androidKeywords := []string{
		"android", "adb", "gradle", "apk", "emulator", "avd",
	}
	for _, kw := range androidKeywords {
		if strings.Contains(nameLower, kw) || strings.Contains(descLower, kw) {
			return "android"
		}
	}

	// Check antigravity keywords
	if strings.Contains(nameLower, "antigravity") || strings.Contains(nameLower, "agy") ||
		strings.Contains(descLower, "antigravity") || strings.Contains(descLower, "agy") {
		return "antigravity"
	}

	return "other"
}

func matchGenericTool(prompt string, tool llm.ToolFunction) bool {
	p := strings.ToLower(prompt)
	nameLower := strings.ToLower(tool.Name)

	if strings.Contains(p, nameLower) {
		return true
	}

	// Split name by delimiters and match parts
	parts := strings.FieldsFunc(tool.Name, func(r rune) bool {
		return r == '_' || r == '-' || r == '/' || r == '.'
	})

	ignoreWords := map[string]bool{
		"get": true, "set": true, "run": true, "list": true, "view": true,
		"find": true, "read": true, "write": true, "info": true, "data": true,
		"tool": true, "show": true, "exec": true, "mcp": true, "cli": true,
	}

	for _, part := range parts {
		partLower := strings.ToLower(part)
		if len(partLower) < 2 || ignoreWords[partLower] {
			continue
		}
		if strings.Contains(p, partLower) {
			return true
		}
	}

	return false
}

func filterToolsForPromptIntent(tools []llm.ToolFunction, prompt string, mode Mode) []llm.ToolFunction {
	p := strings.ToLower(strings.TrimSpace(prompt))

	// Determine which non-core groups should be active based on prompt keywords
	activeGroups := map[string]bool{"core": true}

	// Image mode always enables image tools
	if mode == ModeImage {
		activeGroups["image"] = true
	}

	// Scan prompt for trigger keywords
	for group, keywords := range toolGroupTriggers {
		for _, kw := range keywords {
			if strings.Contains(p, kw) {
				activeGroups[group] = true
				break
			}
		}
	}

	// If prompt is empty or very short, include all tools
	if len(p) < 5 {
		return tools
	}

	filtered := make([]llm.ToolFunction, 0, len(tools))
	for _, tool := range tools {
		group := getToolGroup(tool)
		if group == "core" {
			filtered = append(filtered, tool)
			continue
		}
		if group == "other" {
			// Fallback: match generic tool name or keywords in prompt
			if matchGenericTool(p, tool) {
				filtered = append(filtered, tool)
			}
			continue
		}
		if activeGroups[group] {
			filtered = append(filtered, tool)
		}
	}

	// Safety: if image mode but software prompt, hide generate_image
	if mode == ModeImage && isSoftwareImageFeaturePrompt(p) {
		var safe []llm.ToolFunction
		for _, t := range filtered {
			if t.Name != "generate_image" {
				safe = append(safe, t)
			}
		}
		return safe
	}

	return filtered
}

func hasImageAttachment(prompt string) bool {
	return strings.Contains(strings.ToLower(prompt), "[image:")
}

func isImageEditRequest(prompt string) bool {
	p := strings.ToLower(strings.TrimSpace(prompt))
	if p == "" {
		return false
	}

	editSignals := []string{
		"ubah", "edit", "jadikan", "jadi", "transform", "convert", "konversi", "ganti", "modif", "modifikasi",
		"style", "stylize", "restyle", "retouch", "replace", "remove", "hapus", "tambahkan", "add",
	}
	styleSignals := []string{"kartun", "cartoon", "carton", "anime", "manga", "ghibli", "pixar", "disney", "vector", "vektor", "sketsa", "sketch", "watercolor", "painting"}

	for _, s := range editSignals {
		if strings.Contains(p, s) {
			return true
		}
	}
	if hasImageAttachment(p) {
		for _, s := range styleSignals {
			if strings.Contains(p, s) {
				return true
			}
		}
	}
	return false
}

func firstImageAttachmentPath(prompt string) string {
	lower := strings.ToLower(prompt)
	start := strings.Index(lower, "[image:")
	if start < 0 {
		return ""
	}
	start += len("[image:")
	end := strings.Index(prompt[start:], "]")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(prompt[start : start+end])
}

func imageEditUnsupportedResponse() string {
	return "Maaf, permintaan ini adalah image editing / image-to-image karena menyertakan `[image:/path]` dan instruksi untuk mengubah gambar. Tool `edit_image` tidak tersedia pada sesi/provider ini, jadi belum bisa memakai gambar input sebagai referensi/edit.\n\nSaya tidak akan memanggil `analyze_image` atau mengulang `generate_image` supaya tidak terjadi loop sampai batas iterasi. Solusi teknisnya: tambahkan tool `edit_image(input_image_path, prompt, output_path, size, quality)` atau perluas provider image dengan endpoint image edit."
}

func enhanceImagePrompt(prompt string) string {
	p := strings.TrimSpace(prompt)
	if p == "" {
		return p
	}
	lower := strings.ToLower(p)
	if strings.Contains(lower, "logo") && len([]rune(p)) < 120 {
		return p + ". Buat sebagai logo brand profesional: modern, elegan, minimalis, rapi, mudah dikenali, komposisi seimbang, vektor-style, high quality, clean typography jika ada teks, warna harmonis, latar belakang sederhana. Hindari watermark, mockup, foto realistis, dan elemen berantakan."
	}
	return p
}
