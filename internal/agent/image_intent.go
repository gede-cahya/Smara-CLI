package agent

import (
	"strings"

	"github.com/gede-cahya/Smara-CLI/internal/llm"
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

	// Self-contained explanation prompts or continuation prompts (pasted code, image text, "lanjutkan", etc.)
	// do not require tool declarations on turn 0 and execute much faster without tools.
	if isCodeExplanationPrompt(prompt) || isImageExplanationPrompt(prompt) || isContinuationPrompt(prompt) {
		return nil
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
	paths := allImageAttachmentPaths(prompt)
	if len(paths) > 0 {
		return paths[0]
	}
	return ""
}

func allImageAttachmentPaths(prompt string) []string {
	var paths []string
	lower := strings.ToLower(prompt)
	searchFrom := 0
	for {
		idx := strings.Index(lower[searchFrom:], "[image:")
		if idx < 0 {
			break
		}
		start := searchFrom + idx + len("[image:")
		end := strings.Index(prompt[start:], "]")
		if end < 0 {
			break
		}
		path := strings.TrimSpace(prompt[start : start+end])
		if path != "" {
			paths = append(paths, path)
		}
		searchFrom = start + end + 1
		if len(paths) >= 3 {
			break
		}
	}
	return paths
}

func stripImageAttachmentTags(prompt string) string {
	res := prompt
	for {
		start := strings.Index(strings.ToLower(res), "[image:")
		if start < 0 {
			break
		}
		end := strings.Index(res[start:], "]")
		if end < 0 {
			break
		}
		res = res[:start] + res[start+end+1:]
	}
	return strings.TrimSpace(res)
}

func imageEditUnsupportedResponse() string {
	return "Maaf, permintaan ini adalah image editing / image-to-image karena menyertakan `[image:/path]` dan instruksi untuk mengubah gambar, tetapi sesi ini sedang tidak berada di mode image sehingga tool `edit_image` tidak dieksekusi.\n\nSilakan aktifkan mode image lalu kirim ulang prompt yang sama. Smara sudah mendukung fast-path `edit_image` untuk image-to-image dan tidak akan melakukan loop `analyze_image` -> `generate_image`."
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

func isCodeExplanationPrompt(prompt string) bool {
	p := strings.ToLower(strings.TrimSpace(prompt))
	if strings.Contains(p, "if __name__ ==") ||
		(strings.Contains(p, "def ") && (strings.Contains(p, "import ") || strings.Contains(p, "return"))) ||
		(strings.Contains(p, "package ") && strings.Contains(p, "func ")) ||
		(strings.Contains(p, "const ") && strings.Contains(p, "function")) ||
		(strings.Contains(p, "import ") && strings.Contains(p, "from ")) {
		return true
	}
	if len(p) > 300 && (strings.Contains(p, "def ") || strings.Contains(p, "import ") || strings.Contains(p, "class ") || strings.Contains(p, "function")) {
		return true
	}
	explanationKeywords := []string{
		"ini script apa", "script apa ini", "kode apa ini", "ini kode apa",
		"jelaskan script", "jelaskan kode", "jelaskan code", "maksud script",
		"maksud kode", "maksud code", "apa fungsi script", "apa fungsi kode",
		"apa fungsi code", "apa kegunaan script", "apa kegunaan kode",
		"review script", "review kode", "review code", "analisis script",
		"analisis kode", "analisis code", "apakah script ini", "apakah kode ini",
	}
	for _, kw := range explanationKeywords {
		if strings.Contains(p, kw) {
			return true
		}
	}
	return false
}

func isImageExplanationPrompt(prompt string) bool {
	p := strings.ToLower(strings.TrimSpace(prompt))
	if hasImageAttachment(p) || strings.Contains(p, "gambar terlampir") || strings.Contains(p, "lampiran:") {
		if !isImageEditRequest(prompt) && !isSoftwareImageFeaturePrompt(p) {
			return true
		}
	}
	keywords := []string{
		"ini gambar apa", "gambar apa ini", "jelaskan gambar", "maksud gambar",
		"apa isi gambar", "apa kegunaan gambar", "deskripsikan gambar",
	}
	for _, kw := range keywords {
		if strings.Contains(p, kw) {
			return true
		}
	}
	return false
}

func isContinuationPrompt(prompt string) bool {
	p := strings.ToLower(strings.TrimSpace(prompt))
	if p == "" {
		return false
	}
	keywords := []string{
		"lanjutkan", "lanjut", "teruskan", "terusskan", "sambung", "lanjutkan...", "teruskan...",
		"continue", "next", "keep going", "lanjutkan lagi", "teruskan lagi", "lanjutkan jawaban",
	}
	for _, kw := range keywords {
		if p == kw || strings.HasPrefix(p, kw+" ") || strings.HasSuffix(p, " "+kw) {
			return true
		}
	}
	return false
}
