package agent

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/graphify"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/nudge"
	"github.com/gede-cahya/Smara-CLI/internal/skill"
	smarassh "github.com/gede-cahya/Smara-CLI/internal/ssh"
	"github.com/gede-cahya/Smara-CLI/internal/ui/clipboard"
	"github.com/gede-cahya/Smara-CLI/internal/workspace"
)

// BuiltinDB is set by the Supervisor so built-in tools can access SQLite.
var BuiltinDB *sql.DB

// activeBudgetController exposes just the budget hooks tools need so they
// don't take a hard dependency on *Supervisor (avoids import cycles in
// tests that build slim fakes).
type activeBudgetController interface {
	RequestIterationExtension(amount int, reason string) ExtensionRequest
	IterationBudgetSnapshot() (BudgetSnapshot, bool)
}

// SetActiveBudgetController is called by Supervisor at the start of
// RunAgenticLoop to grant tools (request_iteration_budget,
// iteration_budget_status) access to the running budget. Cleared at end.
var (
	activeBudgetCtrlMu sync.RWMutex
	activeBudgetCtrl   activeBudgetController
)

func SetActiveBudgetController(c activeBudgetController) {
	activeBudgetCtrlMu.Lock()
	activeBudgetCtrl = c
	activeBudgetCtrlMu.Unlock()
}

func getActiveBudgetController() activeBudgetController {
	activeBudgetCtrlMu.RLock()
	defer activeBudgetCtrlMu.RUnlock()
	return activeBudgetCtrl
}

const builtinMCPServerName = "builtin"

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
	"ssh_exec":      "ssh",
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
	// skill reuse — always available so the model can choose an existing
	// recipe even when the user does not explicitly say "skill".
	"skill_run":          "core",
	"skill_instructions": "core",
	"skill_list":         "core",
	// skill management
	"skill_create":  "skill",
	"skill_install": "skill",
	"skill_delete":  "skill",
	// image
	"generate_image": "image",
	"edit_image":     "image",
	"analyze_image":  "image",
	// planning
	"planning_template": "planning",
	// export
	"export_data": "export",
	// memory
	"remember":        "memory",
	"search_memories": "memory",
	// misc
	"serve_project":     "core",
	"connect_mcp":       "core",
	"disconnect_mcp":    "core",
	"schedule_reminder": "core",
}

// GetBuiltinTools returns the standard OS and file manipulation tools
func GetBuiltinTools() []llm.ToolFunction {
	return getBuiltinToolsFiltered(nil)
}

// GetBuiltinToolsFiltered returns builtin tools with disabled groups removed.
func GetBuiltinToolsFiltered(disabledGroups []string) []llm.ToolFunction {
	return getBuiltinToolsFiltered(disabledGroups)
}

func getBuiltinToolsFiltered(disabledGroups []string) []llm.ToolFunction {
	disabled := make(map[string]bool, len(disabledGroups))
	for _, g := range disabledGroups {
		disabled[strings.ToLower(strings.TrimSpace(g))] = true
	}

	all := allBuiltinTools()
	if len(disabled) == 0 {
		return all
	}

	filtered := make([]llm.ToolFunction, 0, len(all))
	for _, t := range all {
		group, ok := toolGroup[t.Name]
		if !ok {
			group = "core" // unknown tools default to core (always included)
		}
		if disabled[group] {
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered
}

// allBuiltinTools returns the full unfiltered list.
func allBuiltinTools() []llm.ToolFunction {
	return []llm.ToolFunction{
		{
			Name:        "run_command",
			Description: "Menjalankan perintah shell atau bash (misal: npm install, git clone, mkdir). Gunakan ini untuk operasi terminal.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "Perintah shell lengkap yang akan dieksekusi",
					},
					"timeout_sec": map[string]interface{}{
						"type":        "integer",
						"description": "Timeout maksimal dalam detik. Default 30. Gunakan nilai lebih besar untuk build, deploy, release, upload, atau install dependency.",
					},
				},
				"required": []string{"command"},
			},
		},
		{
			Name:        "view_file",
			Description: "Melihat isi file dengan nomor baris. Sangat berguna untuk menganalisis kode sebelum melakukan pengeditan.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path relatif atau absolut ke file",
					},
					"start_line": map[string]interface{}{
						"type":        "integer",
						"description": "Baris awal untuk mulai membaca (1-indexed)",
					},
					"end_line": map[string]interface{}{
						"type":        "integer",
						"description": "Baris akhir untuk selesai membaca",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "read_file",
			Description: "Membaca isi file mentah di sistem lokal.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path relatif atau absolut ke file",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "write_file",
			Description: "Membuat file baru atau menimpa file yang sudah ada.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path relatif atau absolut ke file",
					},
					"content": map[string]interface{}{
						"type":        "string",
						"description": "Isi kode atau teks yang akan dituliskan ke file",
					},
				},
				"required": []string{"path", "content"},
			},
		},
		{
			Name:        "delete_file",
			Description: "Menghapus file dari sistem.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path relatif atau absolut ke file",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "list_dir",
			Description: "Melihat isi dari sebuah direktori (folder).",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path direktori",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "generate_image",
			Description: "Membuat/generate gambar dari prompt teks. Untuk permintaan gambar/logo/desain, panggil tool ini langsung maksimal satu kali; jangan hanya menulis rencana/prompt dan jangan ulangi call yang sama.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"prompt": map[string]interface{}{
						"type":        "string",
						"description": "Prompt final yang sangat detail untuk image model. Jika user cuma menulis singkat seperti 'buatkan logo smara', kembangkan sendiri menjadi brief visual lengkap sebelum memanggil tool.",
					},
					"model": map[string]interface{}{
						"type":        "string",
						"description": "Model gambar, default dari config image_model",
					},
					"output_path": map[string]interface{}{
						"type":        "string",
						"description": "Path output opsional, misalnya /tmp/logo.png",
					},
					"size": map[string]interface{}{
						"type":        "string",
						"description": "Ukuran gambar opsional, misalnya 1024x1024",
					},
					"quality": map[string]interface{}{
						"type":        "string",
						"description": "Kualitas gambar opsional: low, medium, high, auto",
					},
				},
				"required": []string{"prompt"},
			},
		},
		{
			Name:        "edit_image",
			Description: "Mengedit gambar / image-to-image dari file input dan instruksi teks. Gunakan ini bila user menyertakan [image:/path] atau image_path dan meminta ubah gaya, edit, atau transformasi gambar.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"image_path": map[string]interface{}{
						"type":        "string",
						"description": "Path gambar input yang akan diedit, misalnya /tmp/input.png",
					},
					"prompt": map[string]interface{}{
						"type":        "string",
						"description": "Instruksi edit/style transfer yang detail untuk image model.",
					},
					"model": map[string]interface{}{
						"type":        "string",
						"description": "Model image edit, default dari config image_model",
					},
					"output_path": map[string]interface{}{
						"type":        "string",
						"description": "Path output opsional, misalnya /tmp/edited.png",
					},
					"size": map[string]interface{}{
						"type":        "string",
						"description": "Ukuran gambar opsional, misalnya 1024x1024",
					},
					"quality": map[string]interface{}{
						"type":        "string",
						"description": "Kualitas gambar opsional: low, medium, high, auto",
					},
				},
				"required": []string{"image_path", "prompt"},
			},
		},
		{
			Name:        "analyze_workspace",
			Description: "Menganalisis struktur proyek saat ini untuk mendapatkan gambaran umum file dan folder penting.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"depth": map[string]interface{}{
						"type":        "integer",
						"description": "Kedalaman scan direktori (default: 2)",
					},
				},
			},
		},
		{
			Name:        "edit_file",
			Description: "Mengubah bagian spesifik dari sebuah file. Gunakan view_file terlebih dahulu untuk mendapatkan nomor baris dan konten yang tepat.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path ke file yang akan diubah",
					},
					"old_content": map[string]interface{}{
						"type":        "string",
						"description": "Teks asli yang ingin diganti (harus persis sama)",
					},
					"new_content": map[string]interface{}{
						"type":        "string",
						"description": "Teks pengganti",
					},
					"start_line": map[string]interface{}{
						"type":        "integer",
						"description": "Optional: Baris awal target perubahan (untuk akurasi)",
					},
					"end_line": map[string]interface{}{
						"type":        "integer",
						"description": "Optional: Baris akhir target perubahan",
					},
				},
				"required": []string{"path", "old_content", "new_content"},
			},
		},
		{
			Name:        "grep_search",
			Description: "Mencari string atau teks tertentu di dalam file di sebuah direktori secara rekursif.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Teks yang ingin dicari",
					},
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Direktori pencarian (default: .)",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "search_path",
			Description: "Mencari file atau direktori berdasarkan nama di seluruh workspace atau path tertentu. Gunakan ini jika anda kehilangan jejak file atau ingin mencari di folder parent.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Nama file atau folder yang dicari (bisa sebagian)",
					},
					"root": map[string]interface{}{
						"type":        "string",
						"description": "Path awal pencarian (default: current directory). Gunakan '..' untuk mencari di folder parent, atau '/' untuk pencarian sistem (hati-hati).",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "get_cwd",
			Description: "Mendapatkan path absolut dari direktori kerja saat ini.",
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "web_search",
			Description: "Mencari informasi di internet secara anonim (menggunakan DuckDuckGo). Gunakan ini jika anda membutuhkan data terbaru atau informasi di luar workspace.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Kata kunci pencarian",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "web_fetch",
			Description: "Mengunduh konten satu halaman web (URL) dan membersihkannya menjadi teks biasa (HTML tag dihapus, navigasi/script di-strip). Gunakan ini setelah `web_search` untuk membaca detail dari link yang ditemukan — misalnya untuk mengambil data terstruktur dari sebuah artikel, daftar, atau tabel HTML. Maksimum 200 KB per halaman, di-truncate kalau lebih besar. Kalau situs pakai Cloudflare / anti-bot dan response dicurigai challenge page, tool otomatis switch ke mode headless Chromium.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "URL lengkap (https://...) yang akan di-fetch",
					},
					"max_chars": map[string]interface{}{
						"type":        "integer",
						"description": "Batas karakter teks yang dikembalikan setelah cleanup. Default 20000.",
					},
					"render": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"auto", "never", "always"},
						"description": "Kontrol headless Chromium: 'auto' (default) = coba HTTP dulu, fallback ke headless kalau kena challenge; 'always' = langsung headless (lebih lambat ~3-8 detik tapi handle JS challenges); 'never' = HTTP saja, gagal kalau situs pakai Cloudflare.",
					},
					"wait_ms": map[string]interface{}{
						"type":        "integer",
						"description": "Berapa lama menunggu JS challenge pass saat mode headless (default 5000ms).",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			Name:        "export_data",
			Description: "Menyimpan data terstruktur ke file lokal dalam format CSV, JSON, Markdown table, atau PDF. Gunakan ini setelah mengumpulkan data dari web_search/web_fetch untuk menyerahkan hasil ke user dalam format yang mudah dipakai. Untuk PDF dibutuhkan `pandoc` atau `wkhtmltopdf` terinstall di sistem — kalau tidak ada, tool akan fallback ke Markdown dan memberitahu user.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"format": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"csv", "json", "md", "markdown", "pdf"},
						"description": "Format output. csv = spreadsheet, json = mesin-readable, md = tabel Markdown, pdf = dokumen siap print.",
					},
					"data": map[string]interface{}{
						"type":        "array",
						"description": "Array of objects (baris tabel). Contoh: [{\"name\":\"Budi\",\"role\":\"Menteri X\"}, ...]. Semua object pakai keys yang sama jadi kolom.",
						"items": map[string]interface{}{
							"type": "object",
						},
					},
					"columns": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Urutan kolom eksplisit. Kalau kosong, otomatis dari keys object pertama.",
					},
					"title": map[string]interface{}{
						"type":        "string",
						"description": "Judul dokumen (muncul di PDF/MD). Opsional.",
					},
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path file output. Kalau kosong, otomatis ke /tmp/export-<timestamp>.<ext>.",
					},
				},
				"required": []string{"format", "data"},
			},
		},
		{
			Name:        "request_iteration_budget",
			Description: "Minta tambahan iterasi tool-call untuk turn yang sedang berjalan ketika kamu yakin task butuh lebih banyak langkah (mis. roadmap panjang, refactor multi-file, deploy bertahap). Sistem punya safety: maksimum 5 grant per turn, hard cap tidak boleh > 3x nilai awal. Berikan alasan eksplisit yang jelas. Tool ini mengembalikan info granted/denied beserta limit baru.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"amount": map[string]interface{}{
						"type":        "integer",
						"description": "Jumlah iterasi tambahan yang diminta (>0). Sistem akan clamp ke headroom yang tersisa.",
					},
					"reason": map[string]interface{}{
						"type":        "string",
						"description": "Alasan singkat kenapa butuh tambahan (wajib). Contoh: 'menyelesaikan Phase B (7 task tersisa)', 'multi-file refactor 12 file Go'.",
					},
				},
				"required": []string{"amount", "reason"},
			},
		},
		{
			Name:        "iteration_budget_status",
			Description: "Ambil snapshot status iterasi turn saat ini: nilai limit aktif, hard cap, jumlah iterasi yang sudah terpakai pola-pola, sisa kuota request_iteration_budget. Pakai sebelum minta tambahan untuk menilai apakah benar-benar perlu.",
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "remember",
			Description: "Menyimpan informasi penting ke memori jangka panjang agar bisa diingat di sesi atau percakapan berikutnya (lintas sesi).",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"content": map[string]interface{}{
						"type":        "string",
						"description": "Informasi atau fakta yang ingin diingat (misal: 'User suka tema gelap', 'Nama project ini adalah X')",
					},
				},
				"required": []string{"content"},
			},
		},
		{
			Name:        "search_memories",
			Description: "Mencari informasi yang pernah disimpan sebelumnya di memori jangka panjang.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Kata kunci atau topik pencarian memori",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "ssh_exec",
			Description: "Menjalankan perintah shell di remote VPS/Server melalui SSH. Gunakan ini untuk deployment, update, monitoring, atau konfigurasi server.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"host": map[string]interface{}{
						"type":        "string",
						"description": "Nama host yang tersimpan atau format user@address",
					},
					"command": map[string]interface{}{
						"type":        "string",
						"description": "Perintah shell yang akan dijalankan di remote server",
					},
				},
				"required": []string{"host", "command"},
			},
		},
		{
			Name:        "ssh_view_file",
			Description: "Melihat isi file di remote VPS/Server melalui SSH. Gunakan untuk membaca log, konfigurasi, atau file lain sebelum melakukan perbaikan.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"host": map[string]interface{}{
						"type":        "string",
						"description": "Nama host yang tersimpan atau format user@address",
					},
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path absolut file di remote server (misal: /var/log/syslog, /etc/nginx/nginx.conf)",
					},
				},
				"required": []string{"host", "path"},
			},
		},
		{
			Name:        "ssh_list_dir",
			Description: "Melihat isi direktori di remote VPS/Server melalui SSH. Gunakan untuk eksplorasi struktur file sebelum melakukan perbaikan.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"host": map[string]interface{}{
						"type":        "string",
						"description": "Nama host yang tersimpan atau format user@address",
					},
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path direktori di remote server (default: /home/user)",
					},
				},
				"required": []string{"host"},
			},
		},
		{
			Name:        "ssh_upload",
			Description: "Upload file ke remote VPS/Server melalui SSH (SFTP/SCP). Gunakan untuk deploy file, konfigurasi, atau asset ke server.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"host": map[string]interface{}{
						"type":        "string",
						"description": "Nama host yang tersimpan atau format user@address",
					},
					"local_path": map[string]interface{}{
						"type":        "string",
						"description": "Path file lokal yang akan diupload",
					},
					"remote_path": map[string]interface{}{
						"type":        "string",
						"description": "Path tujuan di remote server (misal: /opt/app.tar.gz, /var/www/index.html)",
					},
					"method": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"sftp", "scp"},
						"description": "Metode transfer: sftp (default) atau scp",
					},
				},
				"required": []string{"host", "local_path", "remote_path"},
			},
		},
		{
			Name:        "ssh_download",
			Description: "Download file dari remote VPS/Server melalui SSH (SFTP/SCP). Gunakan untuk mengambil log, backup, atau file konfigurasi dari server.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"host": map[string]interface{}{
						"type":        "string",
						"description": "Nama host yang tersimpan atau format user@address",
					},
					"remote_path": map[string]interface{}{
						"type":        "string",
						"description": "Path file di remote server yang akan didownload",
					},
					"local_path": map[string]interface{}{
						"type":        "string",
						"description": "Path tujuan lokal (default: nama file yang sama di cwd)",
					},
					"method": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"sftp", "scp"},
						"description": "Metode transfer: sftp (default) atau scp",
					},
				},
				"required": []string{"host", "remote_path"},
			},
		},
		{
			Name:        "ssh_manage",
			Description: "Mengelola konfigurasi host SSH (VPS). Bisa menambah, menghapus, atau melihat daftar host tersimpan.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action": map[string]interface{}{
						"type":        "string",
						"description": "Aksi yang diinginkan: add, remove, list",
						"enum":        []string{"add", "remove", "list"},
					},
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Nama identifier host (wajib untuk add/remove)",
					},
					"address": map[string]interface{}{
						"type":        "string",
						"description": "Alamat IP atau hostname (wajib untuk add)",
					},
					"user": map[string]interface{}{
						"type":        "string",
						"description": "Username SSH (wajib untuk add, default: root)",
					},
					"key_path": map[string]interface{}{
						"type":        "string",
						"description": "Path ke private key file (opsional)",
					},
					"password": map[string]interface{}{
						"type":        "string",
						"description": "Password SSH (opsional, tidak direkomendasikan)",
					},
					"port": map[string]interface{}{
						"type":        "string",
						"description": "Port SSH (default: 22)",
					},
				},
				"required": []string{"action"},
			},
		},
		{
			Name:        "lsp_hover",
			Description: "Mendapatkan dokumentasi atau tipe dari simbol (fungsi, variabel, struct) di baris/kolom tertentu (Code Intelligence).",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file_path": map[string]interface{}{
						"type":        "string",
						"description": "Path relatif/absolut file",
					},
					"line": map[string]interface{}{
						"type":        "integer",
						"description": "Nomor baris (1-indexed)",
					},
					"character": map[string]interface{}{
						"type":        "integer",
						"description": "Posisi karakter (0-indexed)",
					},
				},
				"required": []string{"file_path", "line", "character"},
			},
		},
		{
			Name:        "lsp_definition",
			Description: "Lompat ke definisi dari simbol (fungsi, variabel, struct) (Code Intelligence).",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file_path": map[string]interface{}{
						"type":        "string",
						"description": "Path relatif/absolut file tempat simbol dipanggil",
					},
					"line": map[string]interface{}{
						"type":        "integer",
						"description": "Nomor baris (1-indexed)",
					},
					"character": map[string]interface{}{
						"type":        "integer",
						"description": "Posisi karakter (0-indexed)",
					},
				},
				"required": []string{"file_path", "line", "character"},
			},
		},
		{
			Name:        "lsp_references",
			Description: "Mencari semua tempat di mana sebuah simbol (fungsi, variabel) digunakan/dipanggil (Code Intelligence).",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file_path": map[string]interface{}{
						"type":        "string",
						"description": "Path relatif/absolut file",
					},
					"line": map[string]interface{}{
						"type":        "integer",
						"description": "Nomor baris (1-indexed)",
					},
					"character": map[string]interface{}{
						"type":        "integer",
						"description": "Posisi karakter (0-indexed)",
					},
				},
				"required": []string{"file_path", "line", "character"},
			},
		},
		{
			Name:        "lsp_document_symbols",
			Description: "Mendapatkan semua daftar fungsi, class, variabel global dalam satu file untuk mengetahui strukturnya (Code Intelligence).",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file_path": map[string]interface{}{
						"type":        "string",
						"description": "Path relatif/absolut file",
					},
				},
				"required": []string{"file_path"},
			},
		},
		{
			Name:        "user_model",
			Description: "Membaca atau memperbarui profil adaptif user (verbosity, risk tolerance, domains, bahasa, custom patterns). Gunakan saat user menyatakan preferensi seperti 'jangan panjang-panjang' atau 'saya suka mode agresif'.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"read", "update"},
						"description": "Aksi: read = baca profil, update = ubah preferensi",
					},
					"key": map[string]interface{}{
						"type":        "string",
						"description": "Nama preferensi (verbosity, risk_tolerance, primary_domains, preferred_languages, custom_patterns)",
					},
					"value": map[string]interface{}{
						"type":        "string",
						"description": "Nilai baru (wajib untuk action=update)",
					},
				},
				"required": []string{"action"},
			},
		},
		{
			Name:        "skill_run",
			Description: "Menjalankan skill otomasi tersimpan yang telah dibuat user. Skill adalah resep multi-step tool calls.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"skill_name": map[string]interface{}{
						"type":        "string",
						"description": "Nama skill yang tersimpan (misal: deploy-react)",
					},
				},
				"required": []string{"skill_name"},
			},
		},
		{
			Name:        "skill_instructions",
			Description: "Internal: membaca instruksi dari skill folder Codex-style (SKILL.md) agar agent bisa melanjutkan workflow dengan tools yang tersedia.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"skill_name": map[string]interface{}{"type": "string"},
					"skill_dir":  map[string]interface{}{"type": "string"},
					"trigger":    map[string]interface{}{"type": "string"},
					"instructions": map[string]interface{}{
						"type":        "string",
						"description": "Isi instruksi SKILL.md",
					},
				},
				"required": []string{"skill_name", "instructions"},
			},
		},
		{
			Name:        "skill_create",
			Description: "Membuat atau meng-upgrade skill otomasi (resep tool calls) ke ~/.smara/skills/. Gunakan untuk auto-create pada workflow pertama tanpa syarat minimal action, dan untuk auto-upgrade dengan overwrite=true saat skill perlu perbaikan. Skill hasil create/upgrade langsung bisa dan sebaiknya dijalankan via skill_run tanpa approval tambahan.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Nama skill dalam kebab-case atau snake_case tanpa spasi (misal: cek-service-vps, deploy-react). Wajib.",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "Deskripsi 1-2 kalimat: apa yang dilakukan skill ini dan kapan dipakai. Wajib.",
					},
					"steps": map[string]interface{}{
						"type":        "array",
						"description": "Urutan tool calls yang akan dijalankan. Setiap step berisi {\"tool\": \"<nama_tool>\", \"args\": {...}}. Nama tool harus salah satu builtin tool Smara (misal run_command, ssh_exec, read_file, edit_file). Wajib minimal 1 step.",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"tool": map[string]interface{}{
									"type":        "string",
									"description": "Nama tool, misal run_command atau ssh_exec",
								},
								"args": map[string]interface{}{
									"type":        "object",
									"description": "Argumen tool. Gunakan __PARAM__nama untuk placeholder yang akan diisi runtime.",
								},
							},
							"required": []string{"tool", "args"},
						},
					},
					"tags": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "1-3 tag kategori, misal [\"vps\", \"monitoring\"]. Opsional.",
					},
					"params": map[string]interface{}{
						"type":        "array",
						"description": "Parameter yang bisa diisi runtime. Placeholder di args pakai format __PARAM__<name>. Opsional.",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"name":        map[string]interface{}{"type": "string"},
								"type":        map[string]interface{}{"type": "string", "enum": []string{"string", "number", "boolean"}},
								"description": map[string]interface{}{"type": "string"},
								"required":    map[string]interface{}{"type": "boolean"},
								"default":     map[string]interface{}{"type": "string"},
							},
						},
					},
					"overwrite": map[string]interface{}{
						"type":        "boolean",
						"description": "Jika true, skill dengan nama sama akan ditimpa (lineage versi lama otomatis direkam). Default: false.",
					},
					"parent": map[string]interface{}{
						"type":        "string",
						"description": "Nama skill induk di hierarchy tree. Gunakan jika skill baru ini adalah perpanjangan/spesialisasi dari skill lain yang sudah ada. Opsional.",
					},
					"category_path": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Jalur kategori untuk skill tree. Contoh: [\"monitoring\", \"vps\"]. Opsional.",
					},
					"dependencies": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Nama-nama skill lain yang dibutuhkan skill ini. Ditampilkan sebagai edge di hierarchy/constellation. Opsional.",
					},
				},
				"required": []string{"name", "description", "steps"},
			},
		},
		{
			Name:        "skill_list",
			Description: "Daftar semua skill yang tersimpan di ~/.smara/skills/. Gunakan untuk mengecek apakah skill tertentu sudah ada sebelum membuat yang baru, atau untuk menunjukkan pilihan ke user.",
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "skill_install",
			Description: "Menginstall/import skill dari path lokal, file Markdown/JSON, folder SKILL.md, repo GitHub, raw URL, atau perintah kompatibel seperti 'npx skills add owner/repo'. Mendukung Smara skill native dan markdown instruction skill gaya Codex/Claude Code/Antigravity tanpa menjalankan script eksternal.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"source": map[string]interface{}{
						"type":        "string",
						"description": "Path/URL/repo/perintah install. Contoh: ~/.agents/skills/graphify, owner/repo, https://.../SKILL.md, atau npx skills add owner/repo",
					},
					"alias": map[string]interface{}{
						"type":        "string",
						"description": "Nama skill pengganti. Hanya untuk satu skill.",
					},
					"overwrite": map[string]interface{}{
						"type":        "boolean",
						"description": "Timpa skill dengan nama sama.",
					},
				},
				"required": []string{"source"},
			},
		},
		{
			Name:        "planning_template",
			Description: "Menghasilkan scaffold markdown read-only untuk planning terstruktur. Gunakan melalui planning skills atau langsung saat user meminta rencana, review risiko, test plan, atau Agile/Minsky planning.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"kind": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"clarify-requirements", "implementation-plan", "risk-review", "test-plan", "agile-minsky"},
						"description": "Jenis template planning yang diinginkan.",
					},
					"goal": map[string]interface{}{
						"type":        "string",
						"description": "Tujuan, fitur, bug, atau keputusan yang ingin direncanakan.",
					},
					"context": map[string]interface{}{
						"type":        "string",
						"description": "Konteks tambahan seperti repo, batasan, deadline, stakeholder, atau catatan teknis.",
					},
				},
				"required": []string{"kind", "goal"},
			},
		},
		{
			Name:        "skill_delete",
			Description: "Menghapus skill tersimpan. Gunakan hanya jika user eksplisit minta skill dihapus.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"skill_name": map[string]interface{}{
						"type":        "string",
						"description": "Nama skill yang akan dihapus",
					},
				},
				"required": []string{"skill_name"},
			},
		},
		{
			Name:        "schedule_reminder",
			Description: "Menyimpan reminder/nudge periodik yang akan ditampilkan saat user buka Smara berikutnya.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"prompt_text": map[string]interface{}{
						"type":        "string",
						"description": "Teks perintah yang akan diingatkan, misal 'cek wa di vps'",
					},
					"when": map[string]interface{}{
						"type":        "string",
						"description": "Format waktu: 'hourly', 'daily at 09:00', 'every 30 minutes'",
					},
				},
				"required": []string{"prompt_text"},
			},
		},
		{
			Name:        "connect_mcp",
			Description: "Menghubungkan MCP server secara manual (local atau remote) dan menyimpannya ke config.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Nama identifier untuk MCP server",
					},
					"type": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"local", "remote"},
						"description": "Tipe koneksi: 'local' (stdio) atau 'remote' (HTTP)",
					},
					"command": map[string]interface{}{
						"type":        "string",
						"description": "Perintah untuk menjalankan MCP server (wajib untuk type=local)",
					},
					"args": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Argument untuk perintah (opsional)",
					},
					"url": map[string]interface{}{
						"type":        "string",
						"description": "URL endpoint untuk remote MCP (wajib untuk type=remote)",
					},
					"headers": map[string]interface{}{
						"type":        "object",
						"description": "HTTP headers untuk remote MCP (opsional)",
					},
					"env": map[string]interface{}{
						"type":        "object",
						"description": "Environment variables untuk local MCP (opsional)",
					},
				},
				"required": []string{"name", "type"},
			},
		},
		{
			Name:        "disconnect_mcp",
			Description: "Memutuskan koneksi MCP server dan menghapusnya dari config.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Nama MCP server yang akan diputuskan",
					},
				},
				"required": []string{"name"},
			},
		},
		// ─── Reverse Engineering Tools ────────────────────────────
		{
			Name:        "analyze_binary",
			Description: "Menganalisis file binary (firmware, executable, library) secara read-only untuk mendeteksi format, arsitektur, entropy, dan packer indicators. Tidak pernah mengeksekusi file target.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file_path": map[string]interface{}{
						"type":        "string",
						"description": "Path absolut atau relatif ke file binary yang akan dianalisis",
					},
				},
				"required": []string{"file_path"},
			},
		},
		{
			Name:        "extract_strings",
			Description: "Mengekstrak strings ASCII/Unicode dari file binary atau teks. Berguna untuk menemukan hardcoded URL, API keys, nama fungsi, dan pesan debug.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file_path": map[string]interface{}{
						"type":        "string",
						"description": "Path ke file yang akan diekstrak strings-nya",
					},
					"min_length": map[string]interface{}{
						"type":        "integer",
						"description": "Panjang minimum string (default: 4)",
					},
					"max_results": map[string]interface{}{
						"type":        "integer",
						"description": "Jumlah maksimum string yang dikembalikan (default: 500, cap: 2000)",
					},
				},
				"required": []string{"file_path"},
			},
		},
		{
			Name:        "scan_signature",
			Description: "Melakukan pattern/signature matching (YARA-lite) terhadap byte sequence file. Mendukung hex patterns dan regex sederhana untuk mendeteksi known signatures.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file_path": map[string]interface{}{
						"type":        "string",
						"description": "Path ke file yang akan di-scan",
					},
					"patterns": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Daftar pattern: hex (e.g., '48 89 E5'), regex (e.g., 'regex:https?://.*'), atau plain string",
					},
				},
				"required": []string{"file_path", "patterns"},
			},
		},
		{
			Name:        "analyze_dependencies",
			Description: "Menganalisis tree source code (Go, JavaScript/TypeScript, Python) untuk memetakan imports, internal package dependencies, dan external libraries.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"source_path": map[string]interface{}{
						"type":        "string",
						"description": "Path ke direktori root source code",
					},
					"language": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"go", "javascript", "typescript", "python", "auto"},
						"description": "Bahasa pemrograman (default: auto-detect)",
					},
				},
				"required": []string{"source_path"},
			},
		},
		{
			Name:        "generate_call_graph",
			Description: "Membuat static call-graph outline dari source code. Melakukan scan sederhana berbasis AST/token/regex untuk memetakan function definitions dan callers. Bukan full compiler.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"source_path": map[string]interface{}{
						"type":        "string",
						"description": "Path ke direktori root source code",
					},
					"language": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"go", "javascript", "typescript", "python", "auto"},
						"description": "Bahasa pemrograman (default: auto-detect)",
					},
					"max_depth": map[string]interface{}{
						"type":        "integer",
						"description": "Kedalaman maksimum call chain (default: 3)",
					},
				},
				"required": []string{"source_path"},
			},
		},
		{
			Name:        "serve_project",
			Description: "Menjalankan HTTP server lokal untuk preview project web (static HTML, React dev server, dll). Server berjalan di background dan bisa diakses via browser.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project_dir": map[string]interface{}{
						"type":        "string",
						"description": "Path direktori project yang akan di-serve (default: direktori project terakhir atau cwd)",
					},
					"port": map[string]interface{}{
						"type":        "integer",
						"description": "Port untuk HTTP server (default: auto-assign 8000-8999)",
					},
				},
			},
		},
		{
			Name:        "graphify_init",
			Description: "Membuat knowledge graph dari direktori source code Go untuk analisis struktur codebase.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path direktori codebase yang akan di-parse (default: cwd)",
					},
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Nama graph (default: nama direktori)",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "graphify_query",
			Description: "Mencari knowledge graph yang sudah dibuat untuk menemukan fungsi, type, atau hubungan antar komponen codebase.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Kata kunci pencarian (misal: 'auth', 'router', 'Database')",
					},
					"graph_name": map[string]interface{}{
						"type":        "string",
						"description": "Nama graph yang akan dicari (default: graph terakhir)",
					},
					"depth": map[string]interface{}{
						"type":        "integer",
						"description": "Kedalaman neighborhood (default: 2)",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name: "analyze_image",
			Description: "Analisa file gambar (PNG/JPG/WEBP). Otomatis pilih backend terbaik: " +
				"(1) OCR via tesseract jika terpasang untuk ekstraksi teks; " +
				"(2) metadata file (dimensi, ukuran, format) selalu tersedia. " +
				"Pakai untuk screenshot, foto dokumen, diagram, dll. " +
				"Path bisa absolut atau relatif terhadap cwd. Format `[image:/path]` di prompt user " +
				"juga otomatis dikenali.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path ke file gambar (.png, .jpg, .jpeg, .webp, .gif, .bmp). Bisa juga raw `[image:/path]` token.",
					},
					"ocr_lang": map[string]interface{}{
						"type":        "string",
						"description": "Bahasa OCR (default: 'eng+ind' untuk dokumen Indonesia/Inggris). Pakai 'eng' untuk Inggris saja, 'ind' untuk Bahasa.",
					},
					"include_metadata": map[string]interface{}{
						"type":        "boolean",
						"description": "Sertakan metadata file (default: true)",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "clip_paste_image",
			Description: "Ambil gambar dari clipboard sistem dan simpan ke file PNG. Mengembalikan path file yang bisa langsung dipakai oleh analyze_image. Jalan di Linux (X11/Wayland), macOS, dan Windows.",
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "clip_copy_image",
			Description: "Salin file gambar ke clipboard sistem. Berguna kalau agen perlu kasih hasil generate gambar/diagram ke user untuk paste di app lain.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path ke file gambar yang akan di-copy",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name: "read_document",
			Description: "Ekstrak teks dari dokumen biner (PDF, DOCX, ODT, RTF) dan teks polos (TXT, MD, JSON, CSV). " +
				"Jangan pakai read_file untuk PDF — ia akan mengembalikan byte mentah yang merusak konteks LLM. " +
				"Otomatis pilih backend: pdftotext (poppler) untuk PDF, pandoc untuk DOCX/ODT/RTF, " +
				"baca langsung untuk teks polos. Format `[file:/path]` di prompt user juga otomatis dikenali.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path ke file dokumen. Bisa absolut, relatif terhadap cwd, atau raw `[file:/path]` token.",
					},
					"max_chars": map[string]interface{}{
						"type":        "number",
						"description": "Batasi output (default 20000). Untuk dokumen besar, agen bisa minta nilai lebih kecil dulu untuk preview.",
					},
				},
				"required": []string{"path"},
			},
		},
	}
}

// activeServers tracks running background HTTP servers spawned by serve_project.
var activeServers = make(map[string]*exec.Cmd)
var activeServersMu sync.Mutex

const builtinToolProgressRole = "tool_progress"

type builtinToolProgressEvent struct {
	Tool    string                 `json:"tool"`
	Event   string                 `json:"event"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

func emitBuiltinProgress(logCallback func(role, content string), tool, event, message string, details map[string]interface{}) {
	if logCallback == nil {
		return
	}
	ev := builtinToolProgressEvent{
		Tool:    tool,
		Event:   event,
		Message: message,
		Details: compactBuiltinProgressDetails(details),
	}
	data, err := json.Marshal(ev)
	if err != nil {
		logCallback("system", message)
		return
	}
	logCallback(builtinToolProgressRole, string(data))
}

func compactBuiltinProgressDetails(details map[string]interface{}) map[string]interface{} {
	if len(details) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(details))
	for k, v := range details {
		lower := strings.ToLower(k)
		if strings.Contains(lower, "token") ||
			strings.Contains(lower, "secret") ||
			strings.Contains(lower, "password") ||
			strings.Contains(lower, "api_key") ||
			strings.Contains(lower, "apikey") {
			out[k] = "[redacted]"
			continue
		}
		if s, ok := v.(string); ok && len(s) > 500 {
			out[k] = s[:500] + "...[truncated]"
			continue
		}
		out[k] = v
	}
	return out
}

// ExecuteBuiltinTool eksekusi fungsi tool built-in tanpa harus melewati koneksi MCP
func ExecuteBuiltinTool(toolName string, args map[string]interface{}, logCallback func(role, content string)) (result string, err error) {
	return ExecuteBuiltinToolWithContext(context.Background(), toolName, args, logCallback)
}

func builtinRunCommandTimeout(args map[string]interface{}) time.Duration {
	const defaultTimeout = 30 * time.Second
	if len(args) == 0 {
		return defaultTimeout
	}
	if timeoutSec, ok := numericArg(args["timeout_sec"]); ok && timeoutSec > 0 {
		return time.Duration(timeoutSec) * time.Second
	}
	if timeoutMs, ok := numericArg(args["timeout_ms"]); ok && timeoutMs > 0 {
		return time.Duration(timeoutMs) * time.Millisecond
	}
	return defaultTimeout
}

func numericArg(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		return int64(v), true
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		if v > math.MaxInt64 {
			return 0, false
		}
		return int64(v), true
	case float32:
		return int64(v), true
	case float64:
		return int64(v), true
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i, true
		}
		if f, err := v.Float64(); err == nil {
			return int64(f), true
		}
	case string:
		i, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return i, err == nil
	}
	return 0, false
}

// ExecuteBuiltinToolWithContext eksekusi tool built-in dengan context cancellation untuk tool yang mendukungnya.
func ExecuteBuiltinToolWithContext(ctx context.Context, toolName string, args map[string]interface{}, logCallback func(role, content string)) (result string, err error) {
	progress := func(event, message string, details map[string]interface{}) {
		emitBuiltinProgress(logCallback, toolName, event, message, details)
	}

	// Recover dari panic di handler tool: jangan sampai bug di satu tool
	// menjatuhkan TUI / WebSocket / supervisor secara keseluruhan. Kembalikan
	// sebagai error biasa supaya agent bisa mencoba pendekatan lain.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("tool %q panic: %v", toolName, r)
			result = ""
		}
	}()

	switch toolName {
	case "generate_image":
		return executeGenerateImageTool(ctx, args, logCallback)
	case "edit_image":
		return executeEditImageTool(ctx, args, logCallback)

	case "request_iteration_budget":
		progress("tool_progress", "Memproses permintaan tambahan iterasi.", map[string]interface{}{"amount": args["amount"]})
		ctrl := getActiveBudgetController()
		if ctrl == nil {
			return "", fmt.Errorf("budget controller tidak aktif (request_iteration_budget hanya valid saat ProcessPrompt sedang berjalan)")
		}
		amountF, _ := args["amount"].(float64)
		amount := int(amountF)
		// Some providers may pass amount as int directly via JSON unmarshal.
		if amount == 0 {
			if a, ok := args["amount"].(int); ok {
				amount = a
			}
		}
		reason, _ := args["reason"].(string)
		req := ctrl.RequestIterationExtension(amount, reason)
		var sb strings.Builder
		if req.Granted {
			fmt.Fprintf(&sb, "✓ Iterasi diperbanyak %d (limit: %d, hard cap: %d). Sisa kuota request: %d/%d.\n",
				req.GrantedAmount, req.NewLimit, req.NewHardCap, req.RemainingGrant, MaxManualExtRequests)
			if req.GrantedAmount < amount {
				fmt.Fprintf(&sb, "Catatan: diminta %d, hanya %d yang bisa dikabulkan (sisanya kena ceiling 3x hard cap awal).\n",
					amount, req.GrantedAmount)
			}
			fmt.Fprintf(&sb, "Alasan tercatat: %s", req.Reason)
		} else {
			fmt.Fprintf(&sb, "✗ Permintaan ditolak: %s\nLimit saat ini: %d, hard cap: %d, sisa kuota request: %d/%d.",
				req.Denial, req.NewLimit, req.NewHardCap, req.RemainingGrant, MaxManualExtRequests)
		}
		return sb.String(), nil

	case "iteration_budget_status":
		progress("tool_progress", "Membaca status budget iterasi.", nil)
		ctrl := getActiveBudgetController()
		if ctrl == nil {
			return "", fmt.Errorf("budget controller tidak aktif")
		}
		snap, ok := ctrl.IterationBudgetSnapshot()
		if !ok {
			return "Tidak ada prompt aktif.", nil
		}
		return fmt.Sprintf(
			"Mode: %s\nBase: %d\nLimit aktif: %d\nHard cap: %d\nManual extensions: %d/%d (total +%d iterasi)\nWindow tool calls: %d unik dari %d terakhir\nStuck repeats: %d",
			snap.Mode, snap.Base, snap.Current, snap.HardCap,
			snap.ManualExtCount, MaxManualExtRequests, snap.ManualExtTotal,
			snap.UniqueRecent, snap.WindowSize, snap.StuckRepeats,
		), nil

	case "run_command":
		cmdStr, ok := args["command"].(string)
		if !ok {
			return "", fmt.Errorf("argumen 'command' tidak valid")
		}
		timeout := builtinRunCommandTimeout(args)
		progress("tool_progress", "Menyiapkan proses shell.", map[string]interface{}{"command": cmdStr})

		cmd := exec.Command("sh", "-c", cmdStr)
		setProcessGroup(cmd)

		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()

		var fullOutput strings.Builder
		var mu sync.Mutex

		if err := cmd.Start(); err != nil {
			return "", fmt.Errorf("gagal memulai perintah: %w", err)
		}
		progress("tool_progress", "Proses shell berjalan.", map[string]interface{}{"pid": cmd.Process.Pid})

		// Baca stdout dan stderr secara konkuren
		var wg sync.WaitGroup
		wg.Add(2)

		readPipe := func(pipe io.ReadCloser) {
			defer wg.Done()
			scanner := bufio.NewScanner(pipe)
			for scanner.Scan() {
				line := scanner.Text()
				mu.Lock()
				fullOutput.WriteString(line + "\n")
				if logCallback != nil {
					logCallback("Terminal", line)
				}
				mu.Unlock()
			}
		}

		go readPipe(stdout)
		go readPipe(stderr)

		// Timeout untuk mencegah hang jika background process menahan pipe
		done := make(chan error, 1)
		go func() {
			err := cmd.Wait()
			// Tutup pipe agar goroutine reader keluar jika process group masih hidup
			stdout.Close()
			stderr.Close()
			done <- err
		}()

		var waitErr error
		select {
		case waitErr = <-done:
			// Proses selesai, tunggu reader goroutine
			wg.Wait()
			progress("tool_progress", "Proses shell selesai.", nil)
		case <-ctx.Done():
			// Context cancellation: bunuh seluruh process group (termasuk background processes)
			_ = killProcessGroup(cmd.Process.Pid)
			wg.Wait()
			waitErr = ctx.Err()
			progress("tool_timeout", "Proses shell dibatalkan oleh context dan dihentikan.", map[string]interface{}{"pid": cmd.Process.Pid})
		case <-time.After(timeout):
			// Timeout: bunuh seluruh process group (termasuk background processes)
			_ = killProcessGroup(cmd.Process.Pid)
			wg.Wait()
			waitErr = fmt.Errorf("timeout setelah %d detik", int(timeout.Seconds()))
			progress("tool_timeout", fmt.Sprintf("Proses shell timeout setelah %d detik dan dihentikan.", int(timeout.Seconds())), map[string]interface{}{"pid": cmd.Process.Pid})
		}

		if waitErr != nil {
			output := fullOutput.String()
			if output == "" {
				output = "(tidak ada output)"
			}
			return output, fmt.Errorf("eksekusi gagal: %w\nOutput: %s", waitErr, output)
		}

		result := strings.TrimSpace(fullOutput.String())
		if result == "" {
			result = "Perintah berhasil dieksekusi tanpa output (exit code 0)."
		}

		// Batasi output jika terlalu panjang (max 10,000 karakter)
		maxOutputLen := 10000
		if len(result) > maxOutputLen {
			result = result[:maxOutputLen] + "\n... (output dipotong karena terlalu panjang)"
		}

		return result, nil

	case "view_file":
		path, ok := args["path"].(string)
		if !ok {
			return "", fmt.Errorf("argumen 'path' tidak valid")
		}
		progress("tool_progress", "Membaca file dengan nomor baris.", map[string]interface{}{"path": path})

		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("gagal membaca file: %w", err)
		}

		lines := strings.Split(string(data), "\n")
		startLine := 1
		if sl, ok := args["start_line"].(float64); ok {
			startLine = int(sl)
		}
		endLine := len(lines)
		if el, ok := args["end_line"].(float64); ok {
			endLine = int(el)
		}

		if startLine < 1 {
			startLine = 1
		}
		if endLine > len(lines) {
			endLine = len(lines)
		}
		if endLine < startLine {
			return "", fmt.Errorf("range tidak valid: end_line (%d) < start_line (%d)", endLine, startLine)
		}

		var sb strings.Builder
		for i := startLine; i <= endLine; i++ {
			sb.WriteString(fmt.Sprintf("%4d | %s\n", i, lines[i-1]))
		}

		return sb.String(), nil

	case "read_file":
		path, ok := args["path"].(string)
		if !ok {
			return "", fmt.Errorf("argumen 'path' tidak valid")
		}
		// Strip [file:/path] / [image:/path] wrappers if user/agent passed
		// the raw attachment token. Same convention as analyze_image.
		path = stripAttachmentWrapper(path)
		progress("tool_progress", "Membaca file.", map[string]interface{}{"path": path})
		content, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("gagal membaca file: %w", err)
		}
		progress("tool_progress", "File terbaca, memeriksa tipe konten.", map[string]interface{}{"path": path, "bytes": len(content)})
		// Guard against binary files. Dumping raw bytes (PDF, images,
		// archives) into the LLM context corrupts encoding and can crash
		// upstream providers. Steer the agent to the right tool instead.
		if kind, isBinary := detectBinaryKind(path, content); isBinary {
			tool := "read_document"
			if kind == "image" {
				tool = "analyze_image"
			}
			return "", fmt.Errorf("file %s adalah file biner (%s) — pakai %s untuk mengekstrak isinya, jangan read_file", filepath.Base(path), kind, tool)
		}
		return string(content), nil

	case "write_file":
		path, ok := args["path"].(string)
		if !ok {
			return "", fmt.Errorf("argumen 'path' tidak valid")
		}
		content, ok := args["content"].(string)
		if !ok {
			return "", fmt.Errorf("argumen 'content' tidak valid")
		}
		progress("tool_progress", "Menyiapkan penulisan file.", map[string]interface{}{"path": path, "bytes": len(content)})

		// Pastikan direktori ada
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("gagal membuat direktori: %w", err)
		}
		progress("tool_progress", "Direktori output siap.", map[string]interface{}{"dir": dir})

		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("gagal menulis file: %w", err)
		}
		progress("tool_verify", "File berhasil ditulis.", map[string]interface{}{"path": path, "bytes": len(content)})
		return fmt.Sprintf("File %s berhasil ditulis.", path), nil

	case "delete_file":
		path, ok := args["path"].(string)
		if !ok {
			return "", fmt.Errorf("argumen 'path' tidak valid")
		}
		progress("tool_progress", "Menghapus file.", map[string]interface{}{"path": path})
		if err := os.Remove(path); err != nil {
			return "", fmt.Errorf("gagal menghapus file: %w", err)
		}
		return fmt.Sprintf("File %s berhasil dihapus.", path), nil

	case "list_dir":
		path, ok := args["path"].(string)
		if !ok {
			return "", fmt.Errorf("argumen 'path' tidak valid")
		}
		progress("tool_progress", "Membaca isi direktori.", map[string]interface{}{"path": path})

		entries, err := os.ReadDir(path)
		if err != nil {
			return "", fmt.Errorf("gagal membaca direktori: %w", err)
		}
		progress("tool_progress", "Direktori terbaca.", map[string]interface{}{"path": path, "entries": len(entries)})

		var result string
		for _, entry := range entries {
			if entry.IsDir() {
				result += fmt.Sprintf("[DIR]  %s\n", entry.Name())
			} else {
				info, _ := entry.Info()
				size := int64(0)
				if info != nil {
					size = info.Size()
				}
				result += fmt.Sprintf("[FILE] %s (%d bytes)\n", entry.Name(), size)
			}
		}

		if result == "" {
			return fmt.Sprintf("Direktori '%s' kosong.", path), nil
		}
		return result, nil

	case "analyze_workspace":
		depth := 2
		if d, ok := args["depth"].(float64); ok {
			depth = int(d)
		}
		progress("tool_progress", "Menganalisis struktur workspace.", map[string]interface{}{"depth": depth})

		var summary strings.Builder
		summary.WriteString("### Workspace Analysis Summary\n\n")

		cwd, _ := os.Getwd()
		summary.WriteString(fmt.Sprintf("**Working Directory:** %s\n\n", cwd))

		summary.WriteString("**Directory Structure:**\n")
		err := filepath.Walk(cwd, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			rel, _ := filepath.Rel(cwd, path)
			if rel == "." {
				return nil
			}

			// Skip hidden dirs and node_modules
			if strings.HasPrefix(rel, ".") || strings.Contains(rel, "node_modules") || strings.Contains(rel, "vendor") {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			level := strings.Count(rel, string(os.PathSeparator))
			if level >= depth {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			indent := strings.Repeat("  ", level)
			if info.IsDir() {
				summary.WriteString(fmt.Sprintf("%s- 📁 %s/\n", indent, info.Name()))
			} else {
				summary.WriteString(fmt.Sprintf("%s- 📄 %s\n", indent, info.Name()))
			}
			return nil
		})

		if err != nil {
			return "", err
		}

		return summary.String(), nil

	case "edit_file":
		path, _ := args["path"].(string)
		oldContent, _ := args["old_content"].(string)
		newContent, _ := args["new_content"].(string)
		if oldContent == "" {
			return "", fmt.Errorf("old_content tidak boleh kosong. Gunakan view_file untuk mengambil teks yang tepat.")
		}
		progress("tool_progress", "Membaca file target untuk edit.", map[string]interface{}{"path": path, "old_chars": len(oldContent), "new_chars": len(newContent)})

		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("gagal membaca file: %w", err)
		}
		progress("tool_progress", "File target terbaca, mencari old_content.", map[string]interface{}{"path": path, "bytes": len(data)})

		lines := strings.Split(string(data), "\n")
		startLine := 1
		hasStartLine := false
		if sl, ok := args["start_line"].(float64); ok && sl > 0 {
			startLine = int(sl)
			hasStartLine = true
		}
		endLine := len(lines)
		hasEndLine := false
		if el, ok := args["end_line"].(float64); ok && el > 0 {
			endLine = int(el)
			hasEndLine = true
		}

		// Jika start_line/end_line diberikan, cari hanya di range tersebut
		content := string(data)
		if hasStartLine || hasEndLine {
			if startLine < 1 {
				startLine = 1
			}
			if endLine > len(lines) {
				endLine = len(lines)
			}
			if startLine > len(lines) {
				return "", fmt.Errorf("start_line %d melebihi jumlah baris file (%d). Pakai view_file untuk verifikasi.", startLine, len(lines))
			}
			if endLine < startLine {
				return "", fmt.Errorf("range tidak valid: end_line (%d) < start_line (%d)", endLine, startLine)
			}

			subContent := strings.Join(lines[startLine-1:endLine], "\n")
			if !strings.Contains(subContent, oldContent) {
				count := strings.Count(content, oldContent)
				if count == 1 {
					newContentStr := strings.Replace(content, oldContent, newContent, 1)
					err = os.WriteFile(path, []byte(newContentStr), 0644)
					if err != nil {
						return "", fmt.Errorf("gagal menulis file: %w", err)
					}
					progress("tool_verify", "Edit diterapkan pada kemunculan unik di luar range.", map[string]interface{}{"path": path, "start_line": startLine, "end_line": endLine})
					return fmt.Sprintf("File %s berhasil diperbarui. Catatan: range baris %d-%d tidak cocok, jadi Smara mengganti satu kemunculan unik old_content di lokasi aktual file.", path, startLine, endLine), nil
				}
				if count > 1 {
					return "", fmt.Errorf("teks 'old_content' tidak ditemukan di baris %d-%d, tetapi muncul %d kali di luar range tersebut. Gunakan view_file untuk memilih start_line dan end_line yang tepat.", startLine, endLine, count)
				}
				return "", fmt.Errorf("teks 'old_content' tidak ditemukan di baris %d-%d atau di seluruh file. Gunakan view_file untuk verifikasi isi terbaru.", startLine, endLine)
			}

			// Lakukan penggantian hanya di bagian tersebut
			newSubContent := strings.Replace(subContent, oldContent, newContent, 1)

			// Gabungkan kembali
			finalLines := append(lines[:startLine-1], strings.Split(newSubContent, "\n")...)
			finalLines = append(finalLines, lines[endLine:]...)
			finalContent := strings.Join(finalLines, "\n")

			err = os.WriteFile(path, []byte(finalContent), 0644)
			if err != nil {
				return "", fmt.Errorf("gagal menulis file: %w", err)
			}
			progress("tool_verify", "Edit diterapkan pada range baris.", map[string]interface{}{"path": path, "start_line": startLine, "end_line": endLine})
			return fmt.Sprintf("File %s berhasil diperbarui di baris %d-%d.", path, startLine, endLine), nil
		}

		// Fallback ke pencarian global jika baris tidak diberikan
		if !strings.Contains(content, oldContent) {
			return "", fmt.Errorf("teks 'old_content' tidak ditemukan di dalam file. Pastikan teks sama persis termasuk spasi.")
		}

		count := strings.Count(content, oldContent)
		if count > 1 {
			return "", fmt.Errorf("teks 'old_content' muncul %d kali. Gunakan start_line dan end_line untuk spesifikasi lokasi.", count)
		}

		newContentStr := strings.Replace(content, oldContent, newContent, 1)
		err = os.WriteFile(path, []byte(newContentStr), 0644)
		if err != nil {
			return "", fmt.Errorf("gagal menulis file: %w", err)
		}
		progress("tool_verify", "Edit diterapkan pada satu kemunculan global.", map[string]interface{}{"path": path})

		return fmt.Sprintf("File %s berhasil diperbarui.", path), nil

	case "grep_search":
		query, _ := args["query"].(string)
		searchPathStr := "."
		if p, ok := args["path"].(string); ok {
			searchPathStr = p
		}
		progress("tool_progress", "Menjalankan pencarian teks rekursif.", map[string]interface{}{"query": query, "path": searchPathStr})

		// Gunakan grep -r -n untuk hasil rekursif dengan nomor baris
		cmd := exec.Command("grep", "-r", "-n", "--exclude-dir=.git", "--exclude-dir=node_modules", query, searchPathStr)
		output, _ := cmd.CombinedOutput() // Grep returns exit code 1 if no matches

		res := string(output)
		if res == "" {
			return "Tidak ada hasil ditemukan.", nil
		}

		// Batasi output agar tidak terlalu besar
		lines := strings.Split(res, "\n")
		if len(lines) > 50 {
			res = strings.Join(lines[:50], "\n") + "\n... (output dipotong karena terlalu panjang)"
		}

		return res, nil

	case "search_path":
		query, _ := args["query"].(string)
		root, _ := args["root"].(string)
		progress("tool_progress", "Mencari path di workspace.", map[string]interface{}{"query": query, "root": root})
		return searchPath(query, root, logCallback)

	case "get_cwd":
		progress("tool_progress", "Membaca direktori kerja saat ini.", nil)
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("gagal mendapatkan working directory: %w", err)
		}
		if cwd == "" {
			return "Gagal mendapatkan path (kosong).", nil
		}
		return cwd, nil

	case "web_search":
		query, _ := args["query"].(string)
		progress("tool_progress", "Menjalankan pencarian web.", map[string]interface{}{"query": query})
		return searchWeb(query)

	case "web_fetch":
		rawURL, _ := args["url"].(string)
		maxChars := 20000
		if mc, ok := args["max_chars"].(float64); ok && mc > 0 {
			maxChars = int(mc)
		}
		render := strings.ToLower(getStr(args, "render"))
		if render == "" {
			render = "auto"
		}
		waitMS := 5000
		if wm, ok := args["wait_ms"].(float64); ok && wm > 0 {
			waitMS = int(wm)
		}
		progress("tool_progress", "Mengambil halaman web.", map[string]interface{}{"url": rawURL, "render": render, "max_chars": maxChars})

		// Route based on render policy.
		if render == "always" {
			progress("tool_progress", "Menggunakan headless Chromium.", map[string]interface{}{"wait_ms": waitMS})
			return fetchHeadless(rawURL, maxChars, waitMS)
		}

		// Normal HTTP fetch first.
		result, err := fetchWebPage(rawURL, maxChars)
		if err != nil {
			// If it's a 4xx/5xx and render=auto, fall through to headless.
			if render == "auto" && isBlockingError(err) {
				progress("tool_progress", "HTTP fetch gagal, mencoba fallback headless.", map[string]interface{}{"error": err.Error(), "wait_ms": waitMS})
				if headlessResult, hErr := fetchHeadless(rawURL, maxChars, waitMS); hErr == nil {
					return "⚠ HTTP fetch gagal (" + err.Error() + ") — auto-switched ke headless Chromium.\n\n" + headlessResult, nil
				}
			}
			return "", err
		}

		// If HTTP succeeded but body looks like a challenge page, auto-retry
		// with headless when render=auto.
		if render == "auto" && looksLikeChallenge(result) {
			progress("tool_progress", "Halaman terlihat seperti challenge, mencoba headless.", map[string]interface{}{"wait_ms": waitMS})
			if headlessResult, hErr := fetchHeadless(rawURL, maxChars, waitMS); hErr == nil {
				return "⚠ Halaman terdeteksi anti-bot challenge — di-retry via headless Chromium.\n\n" + headlessResult, nil
			}
		}
		return result, nil

	case "export_data":
		progress("tool_progress", "Mengekspor data terstruktur.", map[string]interface{}{"format": getStr(args, "format"), "path": getStr(args, "path")})
		return exportData(args)

	case "ssh_exec":
		hostArg, _ := args["host"].(string)
		command, _ := args["command"].(string)
		if hostArg == "" || command == "" {
			return "", fmt.Errorf("argumen 'host' dan 'command' wajib diisi")
		}
		progress("tool_progress", "Menyiapkan eksekusi SSH.", map[string]interface{}{"host": hostArg, "command": command})

		host, err := resolveHost(hostArg)
		if err != nil {
			return "", err
		}
		progress("tool_progress", "Menghubungkan SSH.", map[string]interface{}{"host": host.Name, "address": host.Address, "user": host.User})

		client, err := smarassh.Connect(host)
		if err != nil {
			return "", fmt.Errorf("gagal koneksi SSH: %w", err)
		}
		defer client.Close()
		progress("tool_progress", "SSH terhubung, menjalankan command.", map[string]interface{}{"host": host.Name})

		stdout, stderr, err := client.Exec(command)
		var result strings.Builder
		if stdout != "" {
			result.WriteString("stdout:\n" + stdout + "\n")
		}
		if stderr != "" {
			result.WriteString("stderr:\n" + stderr + "\n")
		}
		if err != nil {
			result.WriteString(fmt.Sprintf("error: %v\n", err))
			return result.String(), nil // return partial output even on error
		}
		if result.Len() == 0 {
			return "Perintah berhasil dieksekusi tanpa output.", nil
		}
		return result.String(), nil

	case "ssh_view_file":
		hostArg, _ := args["host"].(string)
		path, _ := args["path"].(string)
		if hostArg == "" || path == "" {
			return "", fmt.Errorf("argumen 'host' dan 'path' wajib diisi")
		}
		progress("tool_progress", "Menyiapkan baca file via SSH.", map[string]interface{}{"host": hostArg, "path": path})

		host, err := resolveHost(hostArg)
		if err != nil {
			return "", err
		}

		client, err := smarassh.Connect(host)
		if err != nil {
			return "", fmt.Errorf("gagal koneksi SSH: %w", err)
		}
		defer client.Close()
		progress("tool_progress", "SSH terhubung, membaca file remote.", map[string]interface{}{"host": host.Name, "path": path})

		stdout, stderr, err := client.Exec("cat " + path)
		if err != nil {
			return "", fmt.Errorf("gagal membaca file: %w\nstderr: %s", err, stderr)
		}
		if stderr != "" {
			return stdout + "\n(stderr: " + stderr + ")", nil
		}
		return stdout, nil

	case "ssh_list_dir":
		hostArg, _ := args["host"].(string)
		path := "/home/"
		if p, ok := args["path"].(string); ok && p != "" {
			path = p
		}
		if hostArg == "" {
			return "", fmt.Errorf("argumen 'host' wajib diisi")
		}
		progress("tool_progress", "Menyiapkan list direktori via SSH.", map[string]interface{}{"host": hostArg, "path": path})

		host, err := resolveHost(hostArg)
		if err != nil {
			return "", err
		}

		client, err := smarassh.Connect(host)
		if err != nil {
			return "", fmt.Errorf("gagal koneksi SSH: %w", err)
		}
		defer client.Close()
		progress("tool_progress", "SSH terhubung, membaca direktori remote.", map[string]interface{}{"host": host.Name, "path": path})

		stdout, stderr, err := client.Exec("ls -la " + path)
		if err != nil {
			return "", fmt.Errorf("gagal list direktori: %w\nstderr: %s", err, stderr)
		}
		if stderr != "" {
			return stdout + "\n(stderr: " + stderr + ")", nil
		}
		return stdout, nil

	case "ssh_upload":
		hostArg, _ := args["host"].(string)
		localPath, _ := args["local_path"].(string)
		remotePath, _ := args["remote_path"].(string)
		method := "sftp"
		if m, ok := args["method"].(string); ok && m != "" {
			method = m
		}
		if hostArg == "" || localPath == "" || remotePath == "" {
			return "", fmt.Errorf("argumen 'host', 'local_path', dan 'remote_path' wajib diisi")
		}
		progress("tool_progress", "Menyiapkan upload SSH.", map[string]interface{}{"host": hostArg, "local_path": localPath, "remote_path": remotePath, "method": method})

		host, err := resolveHost(hostArg)
		if err != nil {
			return "", err
		}

		client, err := smarassh.Connect(host)
		if err != nil {
			return "", fmt.Errorf("gagal koneksi SSH: %w", err)
		}
		defer client.Close()
		progress("tool_progress", "SSH terhubung, mulai upload.", map[string]interface{}{"host": host.Name, "method": method})

		var res *smarassh.TransferResult
		if method == "scp" {
			res, err = smarassh.UploadFileSCP(client, localPath, remotePath, false)
		} else {
			res, err = smarassh.UploadFile(client, localPath, remotePath, false)
		}
		if err != nil {
			return "", fmt.Errorf("upload gagal: %w", err)
		}
		return fmt.Sprintf("Upload berhasil: %s -> %s (%d bytes, %v)", res.LocalPath, res.RemotePath, res.Bytes, res.Duration), nil

	case "ssh_download":
		hostArg, _ := args["host"].(string)
		remotePath, _ := args["remote_path"].(string)
		localPath := ""
		if lp, ok := args["local_path"].(string); ok {
			localPath = lp
		}
		if localPath == "" {
			localPath = filepath.Base(remotePath)
		}
		method := "sftp"
		if m, ok := args["method"].(string); ok && m != "" {
			method = m
		}
		if hostArg == "" || remotePath == "" {
			return "", fmt.Errorf("argumen 'host' dan 'remote_path' wajib diisi")
		}
		progress("tool_progress", "Menyiapkan download SSH.", map[string]interface{}{"host": hostArg, "remote_path": remotePath, "local_path": localPath, "method": method})

		host, err := resolveHost(hostArg)
		if err != nil {
			return "", err
		}

		client, err := smarassh.Connect(host)
		if err != nil {
			return "", fmt.Errorf("gagal koneksi SSH: %w", err)
		}
		defer client.Close()
		progress("tool_progress", "SSH terhubung, mulai download.", map[string]interface{}{"host": host.Name, "method": method})

		var res *smarassh.TransferResult
		if method == "scp" {
			res, err = smarassh.DownloadFileSCP(client, remotePath, localPath, false)
		} else {
			res, err = smarassh.DownloadFile(client, remotePath, localPath, false)
		}
		if err != nil {
			return "", fmt.Errorf("download gagal: %w", err)
		}
		return fmt.Sprintf("Download berhasil: %s -> %s (%d bytes, %v)", res.RemotePath, res.LocalPath, res.Bytes, res.Duration), nil

	case "ssh_manage":
		action, _ := args["action"].(string)
		progress("tool_progress", "Mengelola konfigurasi SSH.", map[string]interface{}{"action": action, "name": getStr(args, "name")})
		switch action {
		case "add":
			name, _ := args["name"].(string)
			address, _ := args["address"].(string)
			user, _ := args["user"].(string)
			if name == "" || address == "" {
				return "", fmt.Errorf("'name' dan 'address' wajib diisi untuk add")
			}
			if user == "" {
				user = "root"
			}
			host := smarassh.Host{
				Name:     name,
				Address:  address,
				User:     user,
				Port:     "22",
				KeyPath:  getStr(args, "key_path"),
				Password: getStr(args, "password"),
			}
			if p, ok := args["port"].(string); ok && p != "" {
				host.Port = p
			}
			if err := smarassh.SaveHost(host); err != nil {
				return "", fmt.Errorf("gagal menyimpan host: %w", err)
			}
			return fmt.Sprintf("Host '%s' (%s@%s) berhasil ditambahkan.", name, user, address), nil

		case "remove":
			name, _ := args["name"].(string)
			if name == "" {
				return "", fmt.Errorf("'name' wajib diisi untuk remove")
			}
			if err := smarassh.RemoveHost(name); err != nil {
				return "", fmt.Errorf("gagal menghapus host: %w", err)
			}
			return fmt.Sprintf("Host '%s' berhasil dihapus.", name), nil

		case "list":
			hosts, err := smarassh.LoadHosts()
			if err != nil {
				return "", fmt.Errorf("gagal membaca host: %w", err)
			}
			if len(hosts) == 0 {
				return "Belum ada host SSH tersimpan.", nil
			}
			var sb strings.Builder
			sb.WriteString("Daftar host SSH:\n")
			for _, h := range hosts {
				sb.WriteString(fmt.Sprintf("- %s: %s@%s:%s\n", h.Name, h.User, h.Address, h.Port))
			}
			return sb.String(), nil

		default:
			return "", fmt.Errorf("aksi '%s' tidak dikenali (pilih: add, remove, list)", action)
		}

	case "user_model":
		action := getStr(args, "action")
		progress("tool_progress", "Mengakses user model.", map[string]interface{}{"action": action})
		if BuiltinDB == nil {
			return "", fmt.Errorf("database belum tersedia")
		}
		if action == "read" {
			profile, err := LoadProfile(BuiltinDB)
			if err != nil {
				return "", fmt.Errorf("gagal membaca profil: %w", err)
			}
			return profile.ToContext(), nil
		}
		if action == "update" {
			key := getStr(args, "key")
			value := getStr(args, "value")
			if key == "" {
				return "", fmt.Errorf("argumen 'key' wajib diisi untuk action=update")
			}
			if err := UpdateFromPreference(BuiltinDB, key, value); err != nil {
				return "", fmt.Errorf("gagal update profil: %w", err)
			}
			return fmt.Sprintf("Profil diupdate: %s = %s", key, value), nil
		}
		return "", fmt.Errorf("aksi '%s' tidak dikenali (pilih: read, update)", action)

	case "skill_run":
		name := getStr(args, "skill_name")
		if name == "" {
			return "", fmt.Errorf("argumen 'skill_name' wajib diisi")
		}
		progress("tool_progress", "Memuat skill.", map[string]interface{}{"skill_name": name})
		sk, err := skill.Load(name)
		if err != nil {
			return "", fmt.Errorf("skill '%s' tidak ditemukan: %w", name, err)
		}
		res, err := sk.Run(func(toolName string, toolArgs map[string]interface{}) (string, error) {
			emitBuiltinProgress(logCallback, "skill_run", "tool_progress", "Menjalankan step skill.", map[string]interface{}{"skill_name": name, "step_tool": toolName})
			return ExecuteBuiltinTool(toolName, toolArgs, logCallback)
		})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Skill '%s' dijalankan. Sukses=%v. %s", name, res.Success, res.Summary), nil

	case "skill_instructions":
		name := getStr(args, "skill_name")
		instructions := strings.TrimSpace(getStr(args, "instructions"))
		if instructions == "" {
			return "", fmt.Errorf("instruksi skill kosong")
		}
		progress("tool_progress", "Memuat instruksi skill.", map[string]interface{}{"skill_name": name, "chars": len(instructions)})
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Instruksi skill '%s' dimuat.\n", name))
		if trigger := strings.TrimSpace(getStr(args, "trigger")); trigger != "" {
			sb.WriteString("Trigger: " + trigger + "\n")
		}
		if skillDir := strings.TrimSpace(getStr(args, "skill_dir")); skillDir != "" {
			sb.WriteString("Direktori skill: " + skillDir + "\n")
		}
		sb.WriteString("\nIkuti instruksi berikut dan lanjutkan dengan tool Smara yang tersedia bila perlu:\n\n")
		sb.WriteString(instructions)
		return sb.String(), nil

	case "skill_create":
		progress("tool_progress", "Membuat skill baru.", map[string]interface{}{"name": getStr(args, "name")})
		return createSkillFromArgs(args)

	case "skill_list":
		progress("tool_progress", "Membaca daftar skill.", nil)
		names, err := skill.List()
		if err != nil {
			return "", fmt.Errorf("gagal membaca daftar skill: %w", err)
		}
		if len(names) == 0 {
			return "Belum ada skill tersimpan di ~/.smara/skills/.", nil
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Skill tersimpan (%d):\n", len(names)))
		for _, n := range names {
			sk, err := skill.Load(n)
			if err != nil {
				sb.WriteString(fmt.Sprintf("  • %s (gagal load: %v)\n", n, err))
				continue
			}
			desc := sk.Description
			if len(desc) > 80 {
				desc = desc[:80] + "…"
			}
			sb.WriteString(fmt.Sprintf("  • %s — %s (v%d, %d steps)\n", sk.Name, desc, sk.Version, len(sk.Steps)))
		}
		return sb.String(), nil

	case "skill_install":
		source := strings.TrimSpace(getStr(args, "source"))
		if source == "" {
			return "", fmt.Errorf("argumen 'source' wajib diisi")
		}
		progress("tool_progress", "Menginstal skill.", map[string]interface{}{"source": source, "alias": getStr(args, "alias")})
		normalized, err := skill.NormalizePluginSource([]string{source})
		if err != nil {
			return "", err
		}
		overwrite := false
		if v, ok := args["overwrite"].(bool); ok {
			overwrite = v
		}
		installed, err := skill.InstallFromPluginSource(skill.PluginInstallOptions{
			Source:    normalized,
			Alias:     strings.TrimSpace(getStr(args, "alias")),
			Overwrite: overwrite,
		})
		if err != nil {
			return "", fmt.Errorf("gagal install skill: %w", err)
		}
		var names []string
		for _, sk := range installed {
			names = append(names, sk.Name)
		}
		return fmt.Sprintf("Berhasil install %d skill: %s", len(installed), strings.Join(names, ", ")), nil

	case "skill_delete":
		name := getStr(args, "skill_name")
		if name == "" {
			return "", fmt.Errorf("argumen 'skill_name' wajib diisi")
		}
		progress("tool_progress", "Menghapus skill.", map[string]interface{}{"skill_name": name})
		if _, err := skill.Load(name); err != nil {
			return "", fmt.Errorf("skill '%s' tidak ditemukan: %w", name, err)
		}
		if err := skill.Delete(name, BuiltinDB); err != nil {
			return "", fmt.Errorf("gagal menghapus skill: %w", err)
		}
		return fmt.Sprintf("Skill '%s' dihapus.", name), nil

	case "planning_template":
		progress("tool_progress", "Membangun template rencana.", map[string]interface{}{"kind": getStr(args, "kind")})
		return buildPlanningTemplate(getStr(args, "kind"), getStr(args, "goal"), getStr(args, "context"))

	case "schedule_reminder":
		promptText := getStr(args, "prompt_text")
		when := getStr(args, "when")
		progress("tool_progress", "Membuat reminder.", map[string]interface{}{"when": when})
		if BuiltinDB == nil {
			return "", fmt.Errorf("database belum tersedia")
		}
		var nextRun *time.Time
		if when != "" {
			n, err := nudge.SimpleCron(when, time.Now())
			if err == nil {
				nextRun = n
			}
		}
		_, err := nudge.CreateSchedule(BuiltinDB, promptText, when, nextRun)
		if err != nil {
			return "", fmt.Errorf("gagal membuat reminder: %w", err)
		}
		return fmt.Sprintf("Reminder tersimpan: '%s' (when: %s)", promptText, when), nil

	// ─── Reverse Engineering Tools ────────────────────────────
	case "analyze_binary":
		filePath := getStr(args, "file_path")
		if filePath == "" {
			return "", fmt.Errorf("argumen 'file_path' wajib diisi")
		}
		progress("tool_progress", "Menganalisis binary.", map[string]interface{}{"file_path": filePath})
		return analyzeBinaryFile(filePath)

	case "extract_strings":
		filePath := getStr(args, "file_path")
		if filePath == "" {
			return "", fmt.Errorf("argumen 'file_path' wajib diisi")
		}
		minLen := 4
		if ml, ok := args["min_length"].(float64); ok {
			minLen = int(ml)
		}
		maxResults := 500
		if mr, ok := args["max_results"].(float64); ok {
			maxResults = int(mr)
		}
		if maxResults > 2000 {
			maxResults = 2000
		}
		progress("tool_progress", "Mengekstrak strings dari file.", map[string]interface{}{"file_path": filePath, "min_length": minLen, "max_results": maxResults})
		return extractStringsFromFile(filePath, minLen, maxResults)

	case "scan_signature":
		filePath := getStr(args, "file_path")
		if filePath == "" {
			return "", fmt.Errorf("argumen 'file_path' wajib diisi")
		}
		patternsRaw, ok := args["patterns"].([]interface{})
		if !ok || len(patternsRaw) == 0 {
			return "", fmt.Errorf("argumen 'patterns' wajib berupa array non-kosong")
		}
		patterns := make([]string, 0, len(patternsRaw))
		for _, p := range patternsRaw {
			if s, ok := p.(string); ok {
				patterns = append(patterns, s)
			}
		}
		progress("tool_progress", "Menjalankan signature scan.", map[string]interface{}{"file_path": filePath, "patterns": len(patterns)})
		return scanSignature(filePath, patterns)

	case "analyze_dependencies":
		sourcePath := getStr(args, "source_path")
		if sourcePath == "" {
			return "", fmt.Errorf("argumen 'source_path' wajib diisi")
		}
		lang := getStr(args, "language")
		if lang == "" {
			lang = "auto"
		}
		progress("tool_progress", "Menganalisis dependency source.", map[string]interface{}{"source_path": sourcePath, "language": lang})
		return analyzeDependencies(sourcePath, lang)

	case "generate_call_graph":
		sourcePath := getStr(args, "source_path")
		if sourcePath == "" {
			return "", fmt.Errorf("argumen 'source_path' wajib diisi")
		}
		lang := getStr(args, "language")
		if lang == "" {
			lang = "auto"
		}
		maxDepth := 3
		if md, ok := args["max_depth"].(float64); ok {
			maxDepth = int(md)
		}
		progress("tool_progress", "Membangun call graph.", map[string]interface{}{"source_path": sourcePath, "language": lang, "max_depth": maxDepth})
		return generateCallGraph(sourcePath, lang, maxDepth)

	case "serve_project":
		progress("tool_progress", "Menyiapkan preview server project.", map[string]interface{}{"project_dir": getStr(args, "project_dir"), "port": args["port"]})
		result, err := serveProject(args)
		return result, err

	case "graphify_init":
		path := getStr(args, "path")
		if path == "" {
			path = "."
		}
		name := getStr(args, "name")
		if name == "" {
			name = filepath.Base(path)
		}
		if BuiltinDB == nil {
			return "", fmt.Errorf("database tidak tersedia")
		}
		progress("tool_progress", "Membangun graphify graph.", map[string]interface{}{"path": path, "name": name})
		g, err := graphify.ParseGoCodebase(path, name)
		if err != nil {
			return "", fmt.Errorf("gagal parse: %w", err)
		}
		g.AssignCommunities()
		gs, err := graphify.NewGraphStore(BuiltinDB)
		if err != nil {
			return "", fmt.Errorf("gagal buat graph store: %w", err)
		}
		if err := gs.SaveGraph(g); err != nil {
			return "", fmt.Errorf("gagal simpan graph: %w", err)
		}
		progress("tool_verify", "Graphify graph tersimpan.", map[string]interface{}{"name": name, "nodes": g.NodeCount(), "edges": g.EdgeCount()})
		return fmt.Sprintf("Graph '%s' dibuat: %d nodes, %d edges", name, g.NodeCount(), g.EdgeCount()), nil

	case "graphify_query":
		query := getStr(args, "query")
		if query == "" {
			return "", fmt.Errorf("argumen 'query' wajib diisi")
		}
		graphName := getStr(args, "graph_name")
		if graphName == "" {
			return "", fmt.Errorf("argumen 'graph_name' wajib diisi")
		}
		depth := 2
		if d, ok := args["depth"].(float64); ok {
			depth = int(d)
		}
		if BuiltinDB == nil {
			return "", fmt.Errorf("database tidak tersedia")
		}
		progress("tool_progress", "Menjalankan query graphify.", map[string]interface{}{"query": query, "graph_name": graphName, "depth": depth})
		gs, err := graphify.NewGraphStore(BuiltinDB)
		if err != nil {
			return "", fmt.Errorf("gagal buat graph store: %w", err)
		}
		g, err := gs.LoadGraph(graphName)
		if err != nil {
			return "", fmt.Errorf("graph '%s' tidak ditemukan: %w", graphName, err)
		}
		result := g.Query(query, depth)
		var b strings.Builder
		b.WriteString(fmt.Sprintf("Hasil query '%s' (graph: %s):\n\n", query, graphName))
		b.WriteString(fmt.Sprintf("Nodes (%d):\n", len(result.Nodes)))
		for _, n := range result.Nodes {
			b.WriteString(fmt.Sprintf("- %s (%s) %s:%d\n", n.Label, n.Type, n.SourceFile, n.SourceLine))
		}
		b.WriteString(fmt.Sprintf("\nEdges (%d):\n", len(result.Edges)))
		for _, e := range result.Edges {
			b.WriteString(fmt.Sprintf("- %s --[%s]--> %s\n", e.Source, e.Relation, e.Target))
		}
		return b.String(), nil

	case "analyze_image":
		path := getStr(args, "path")
		if path == "" {
			return "", fmt.Errorf("argumen 'path' wajib diisi")
		}
		// Strip [image:...] wrapper if user/agent passed the raw token.
		path = strings.TrimSpace(path)
		if strings.HasPrefix(path, "[image:") && strings.HasSuffix(path, "]") {
			path = strings.TrimSuffix(strings.TrimPrefix(path, "[image:"), "]")
		}
		ocrLang := getStr(args, "ocr_lang")
		if ocrLang == "" {
			ocrLang = "eng+ind"
		}
		includeMeta := true
		if v, ok := args["include_metadata"].(bool); ok {
			includeMeta = v
		}
		progress("tool_progress", "Menganalisis gambar.", map[string]interface{}{"path": path, "ocr_lang": ocrLang, "include_metadata": includeMeta})
		return analyzeImageFile(path, ocrLang, includeMeta)

	case "clip_paste_image":
		progress("tool_progress", "Membaca gambar dari clipboard.", nil)
		res, err := clipboard.ReadImage()
		if err != nil {
			return "", fmt.Errorf("paste image gagal: %w", err)
		}
		return fmt.Sprintf("✓ Image disimpan: %s\n  size: %d bytes\n  source: %s\n  Pakai analyze_image dengan path tersebut untuk analisa lebih lanjut.",
			res.Path, res.Size, res.Source), nil

	case "clip_copy_image":
		path := getStr(args, "path")
		if path == "" {
			return "", fmt.Errorf("argumen 'path' wajib diisi")
		}
		progress("tool_progress", "Menyalin gambar ke clipboard.", map[string]interface{}{"path": path})
		if err := clipboard.WriteImage(path); err != nil {
			return "", fmt.Errorf("copy image gagal: %w", err)
		}
		return fmt.Sprintf("✓ Image %s sudah masuk clipboard sistem", path), nil

	case "read_document":
		path := getStr(args, "path")
		if path == "" {
			return "", fmt.Errorf("argumen 'path' wajib diisi")
		}
		path = stripAttachmentWrapper(path)
		maxChars := 20000
		if v, ok := args["max_chars"].(float64); ok && v > 0 {
			maxChars = int(v)
		}
		progress("tool_progress", "Mengekstrak teks dokumen.", map[string]interface{}{"path": path, "max_chars": maxChars})
		text, source, err := extractDocumentText(path)
		if err != nil {
			return "", err
		}
		truncated := false
		if len(text) > maxChars {
			text = text[:maxChars]
			truncated = true
		}
		st, _ := os.Stat(path)
		var sizeStr string
		if st != nil {
			sizeStr = fmt.Sprintf(" · %d KB", st.Size()/1024)
		}
		header := fmt.Sprintf("✓ %s%s · ekstrak: %s · %d karakter", filepath.Base(path), sizeStr, source, len(text))
		if truncated {
			header += fmt.Sprintf(" (truncated dari aslinya, panggil ulang dengan max_chars lebih besar bila perlu)")
		}
		return header + "\n\n" + text, nil

	default:
		return "", fmt.Errorf("tool built-in '%s' tidak dikenali", toolName)
	}
}

func getStr(args map[string]interface{}, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

// skillNameRe validates the skill name: letters/digits/hyphen/underscore only.
var skillNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// createSkillFromArgs builds and persists a skill from the JSON arguments
// supplied by the LLM via the skill_create builtin tool.
func createSkillFromArgs(args map[string]interface{}) (string, error) {
	name := strings.TrimSpace(getStr(args, "name"))
	description := strings.TrimSpace(getStr(args, "description"))

	if name == "" {
		return "", fmt.Errorf("argumen 'name' wajib diisi")
	}
	if !skillNameRe.MatchString(name) {
		return "", fmt.Errorf("nama skill hanya boleh huruf, angka, '-', dan '_' (dapat: %q)", name)
	}
	if len(name) > 60 {
		return "", fmt.Errorf("nama skill terlalu panjang (max 60 karakter)")
	}
	if description == "" {
		return "", fmt.Errorf("argumen 'description' wajib diisi")
	}

	rawSteps, ok := args["steps"].([]interface{})
	if !ok || len(rawSteps) == 0 {
		return "", fmt.Errorf("argumen 'steps' wajib berupa array minimal 1 elemen")
	}

	steps := make([]skill.Step, 0, len(rawSteps))
	for i, raw := range rawSteps {
		stepMap, ok := raw.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("steps[%d] bukan object", i)
		}
		tool := strings.TrimSpace(getStr(stepMap, "tool"))
		if tool == "" {
			return "", fmt.Errorf("steps[%d].tool wajib diisi", i)
		}
		var stepArgs map[string]interface{}
		if a, ok := stepMap["args"].(map[string]interface{}); ok {
			stepArgs = a
		} else {
			stepArgs = map[string]interface{}{}
		}
		steps = append(steps, skill.Step{Tool: tool, Args: stepArgs})
	}

	var tags []string
	if rawTags, ok := args["tags"].([]interface{}); ok {
		for _, t := range rawTags {
			if s, ok := t.(string); ok && s != "" {
				tags = append(tags, s)
			}
		}
	}

	var params []skill.ParamDef
	if rawParams, ok := args["params"].([]interface{}); ok {
		for _, p := range rawParams {
			pm, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			pd := skill.ParamDef{
				Name:        getStr(pm, "name"),
				Type:        getStr(pm, "type"),
				Description: getStr(pm, "description"),
			}
			if req, ok := pm["required"].(bool); ok {
				pd.Required = req
			}
			if def, ok := pm["default"]; ok {
				pd.Default = def
			}
			if pd.Type == "" {
				pd.Type = "string"
			}
			if pd.Name != "" {
				params = append(params, pd)
			}
		}
	}

	overwrite := false
	if o, ok := args["overwrite"].(bool); ok {
		overwrite = o
	}

	parent := strings.TrimSpace(getStr(args, "parent"))
	// Accept either "parent" or "parent_id" for convenience.
	if parent == "" {
		parent = strings.TrimSpace(getStr(args, "parent_id"))
	}
	if parent != "" {
		if _, err := skill.Load(parent); err != nil {
			return "", fmt.Errorf("skill induk '%s' tidak ditemukan — buat skill induk lebih dulu atau kosongkan field parent", parent)
		}
		if parent == name {
			return "", fmt.Errorf("skill tidak bisa menjadi induk dirinya sendiri")
		}
	}

	var categoryPath []string
	if rawCat, ok := args["category_path"].([]interface{}); ok {
		for _, c := range rawCat {
			if s, ok := c.(string); ok && s != "" {
				categoryPath = append(categoryPath, s)
			}
		}
	}

	var dependencies []string
	if rawDeps, ok := args["dependencies"].([]interface{}); ok {
		for _, d := range rawDeps {
			if s, ok := d.(string); ok && s != "" {
				dependencies = append(dependencies, s)
			}
		}
	}

	// Conflict check + lineage preservation on overwrite.
	var priorSkill *skill.Skill
	if existing, err := skill.Load(name); err == nil {
		if !overwrite {
			return "", fmt.Errorf("skill '%s' sudah ada. Gunakan overwrite=true untuk menimpa atau pilih nama lain.", name)
		}
		priorSkill = existing
	}

	// Version: new skill starts at 1; overwrite bumps existing
	version := 1
	if priorSkill != nil {
		version = priorSkill.Version + 1
	}

	sk := &skill.Skill{
		Name:         name,
		Description:  description,
		Steps:        steps,
		Version:      version,
		Tags:         tags,
		Params:       params,
		ParentID:     parent,
		CategoryPath: categoryPath,
		Dependencies: dependencies,
		AutoSkill: &skill.AutoSkillMetadata{
			Enabled:                  true,
			Mode:                     "aggressive",
			CreatePolicy:             "always",
			MinimumToolActions:       0,
			RepeatedWorkflowRequired: false,
			UpgradePolicy:            "auto",
			ExecuteAfterCreate:       true,
			ExecuteAfterUpgrade:      true,
			ApprovalRequired:         false,
			LineageBackup:            true,
			MaxAutoUpgradeRetries:    2,
		},
		Risk: &skill.RiskMetadata{
			RequiresApproval: false,
		},
	}

	// Preserve ancestry if we're overwriting an older version.
	if priorSkill != nil {
		skill.AttachLineage(sk, priorSkill, "manual")
		// Inherit relational fields if LLM did not specify them on the
		// new version, so overwriting does not accidentally flatten the
		// hierarchy.
		if sk.ParentID == "" {
			sk.ParentID = priorSkill.ParentID
		}
		if len(sk.CategoryPath) == 0 {
			sk.CategoryPath = priorSkill.CategoryPath
		}
		if len(sk.Dependencies) == 0 {
			sk.Dependencies = priorSkill.Dependencies
		}
	}

	if err := sk.Validate(); err != nil {
		return "", fmt.Errorf("skill tidak valid: %w", err)
	}

	if err := skill.Save(sk, BuiltinDB); err != nil {
		return "", fmt.Errorf("gagal menyimpan skill: %w", err)
	}

	lessonType := "skill-created"
	summary := fmt.Sprintf("Skill %s v%d dibuat dengan mode AutoSkill agresif.", sk.Name, sk.Version)
	if priorSkill != nil {
		lessonType = "skill-upgrade"
		summary = fmt.Sprintf("Skill %s di-upgrade dari v%d ke v%d dan harus langsung dipakai tanpa approval.", sk.Name, priorSkill.Version, sk.Version)
	}
	_, _ = SaveSelfImprovementMemory(BuiltinDB, SelfImprovementMemory{
		Type:       lessonType,
		Scope:      "skill:" + sk.Name,
		Summary:    summary,
		Lesson:     fmt.Sprintf("Saat task relevan dengan %s, prioritaskan skill ini; jika gagal/kurang sesuai, auto-upgrade dengan overwrite=true lalu eksekusi ulang tanpa approval.", sk.Name),
		AppliesTo:  append([]string{"skill", "auto-skill"}, sk.Tags...),
		Confidence: 0.95,
		AutoApply:  true,
	})

	extra := ""
	if parent != "" {
		extra += fmt.Sprintf(" parent=%s", parent)
	}
	if len(categoryPath) > 0 {
		extra += fmt.Sprintf(" category=%s", strings.Join(categoryPath, "/"))
	}
	if len(dependencies) > 0 {
		extra += fmt.Sprintf(" deps=%v", dependencies)
	}
	return fmt.Sprintf("Skill '%s' v%d tersimpan di ~/.smara/skills/%s.json (%d steps, tags=%v%s). AutoSkill=aggressive, approval=false. Jalankan dengan skill_run.",
		sk.Name, sk.Version, sk.Name, len(sk.Steps), sk.Tags, extra), nil
}

// resolveHost attempts to find a host by exact name, then fuzzy search, then raw user@address.
func resolveHost(hostArg string) (*smarassh.Host, error) {
	// 1. Exact match from saved config
	host, err := smarassh.GetHost(hostArg)
	if err == nil {
		return host, nil
	}

	// 2. Fuzzy/partial match
	found, matches, err := smarassh.FindHost(hostArg)
	if err == nil && found != nil {
		return found, nil
	}
	if len(matches) > 0 {
		var sb strings.Builder
		for _, m := range matches {
			sb.WriteString(fmt.Sprintf("- %s (%s@%s:%s)\n", m.Name, m.User, m.Address, m.Port))
		}
		return nil, fmt.Errorf("multiple hosts cocok dengan '%s':\n%sGunakan nama exact host.", hostArg, sb.String())
	}

	// 3. Raw user@address
	if strings.Contains(hostArg, "@") {
		parts := strings.SplitN(hostArg, "@", 2)
		return &smarassh.Host{
			Name:    hostArg,
			User:    parts[0],
			Address: parts[1],
			Port:    "22",
		}, nil
	}

	return nil, fmt.Errorf("host '%s' tidak ditemukan. Gunakan 'ssh_manage' dengan action=list untuk melihat host tersimpan, atau gunakan format user@address.", hostArg)
}

func searchPath(query, root string, logFn func(string, string)) (string, error) {
	if logFn != nil {
		logFn("system", fmt.Sprintf("Mencari '%s' mulai dari '%s'...", query, root))
	}

	if root == "" {
		root = "."
	}

	matcher := workspace.NewIgnoreMatcher(root)

	var results []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if matcher.IsIgnored(path, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.Contains(strings.ToLower(info.Name()), strings.ToLower(query)) {
			results = append(results, path)
		}

		// Limit results to 50
		if len(results) >= 50 {
			return io.EOF
		}
		return nil
	})

	if err != nil && err != io.EOF {
		return "", err
	}

	if len(results) == 0 {
		return fmt.Sprintf("Tidak ada hasil yang ditemukan untuk '%s' di '%s'.", query, root), nil
	}

	return fmt.Sprintf("Hasil pencarian untuk '%s':\n- %s", query, strings.Join(results, "\n- ")), nil
}

func searchWeb(query string) (string, error) {
	searchURL := fmt.Sprintf("https://duckduckgo.com/html/?q=%s", url.QueryEscape(query))

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return "", err
	}

	// Gunakan User-Agent stealth untuk menghindari blokir
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gagal menghubungi search engine: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("search engine mengembalikan status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	html := string(body)

	// Regex sederhana untuk mengekstrak hasil (title, link, snippet)
	// Struktur: <a class="result__a" href="URL">TITLE</a> ... <a class="result__snippet">SNIPPET</a>
	re := regexp.MustCompile(`(?s)<div class="result__body">.*?<a class="result__a" href="(.*?)">(.*?)</a>.*?<a class="result__snippet".*?>(.*?)</a>`)
	matches := re.FindAllStringSubmatch(html, 10)

	if len(matches) == 0 {
		return "Tidak ada hasil pencarian yang ditemukan atau format halaman berubah.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### Hasil Pencarian Internet untuk: '%s'\n\n", query))

	for i, m := range matches {
		link := m[1]
		// Bersihkan link jika melalui proxy DDG
		if strings.Contains(link, "uddg=") {
			parts := strings.Split(link, "uddg=")
			if len(parts) > 1 {
				decoded, _ := url.QueryUnescape(strings.Split(parts[1], "&")[0])
				link = decoded
			}
		}

		title := cleanHTML(m[2])
		snippet := cleanHTML(m[3])

		sb.WriteString(fmt.Sprintf("%d. **%s**\n   - Link: %s\n   - %s\n\n", i+1, title, link, snippet))
	}

	return sb.String(), nil
}

func cleanHTML(s string) string {
	// Hapus tag HTML dasar dan decode entitas umum
	s = regexp.MustCompile(`<.*?>`).ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	return strings.TrimSpace(s)
}

// ─── Reverse Engineering Helpers ──────────────────────────

// analyzeBinaryFile performs read-only static analysis on a binary file.
func analyzeBinaryFile(filePath string) (string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("gagal mengakses file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("path adalah direktori, bukan file")
	}

	const maxSize = 50 * 1024 * 1024 // 50 MB cap
	if info.Size() > maxSize {
		return "", fmt.Errorf("file terlalu besar (max 50 MB)")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### Binary Analysis Report: %s\n\n", filepath.Base(filePath)))
	sb.WriteString(fmt.Sprintf("- **Path:** %s\n", filePath))
	sb.WriteString(fmt.Sprintf("- **Size:** %d bytes (%.2f MB)\n", info.Size(), float64(info.Size())/(1024*1024)))

	// Attempt 'file' command first
	if out, err := exec.Command("file", filePath).CombinedOutput(); err == nil {
		sb.WriteString(fmt.Sprintf("- **File Command:** %s\n", strings.TrimSpace(string(out))))
	} else {
		sb.WriteString("- **File Command:** not available (fallback to magic bytes)\n")
	}

	// Read first 64 bytes for magic bytes
	f, err := os.Open(filePath)
	if err != nil {
		return sb.String(), nil
	}
	defer f.Close()

	magic := make([]byte, 64)
	n, _ := f.Read(magic)
	magic = magic[:n]

	// Identify common formats by magic bytes
	format, arch := identifyFormatByMagic(magic)
	sb.WriteString(fmt.Sprintf("- **Detected Format:** %s\n", format))
	if arch != "" {
		sb.WriteString(fmt.Sprintf("- **Architecture Hint:** %s\n", arch))
	}

	// Entropy of first 8KB
	entBuf := make([]byte, 8192)
	f.Seek(0, 0)
	en, _ := f.Read(entBuf)
	if en > 0 {
		entropy := calculateEntropy(entBuf[:en])
		sb.WriteString(fmt.Sprintf("- **Entropy (first 8KB):** %.4f", entropy))
		if entropy > 7.5 {
			sb.WriteString(" (high — possible encryption/packing/compression)")
		} else if entropy > 6.5 {
			sb.WriteString(" (moderate — mixed content)")
		} else {
			sb.WriteString(" (low — structured/plain)")
		}
		sb.WriteString("\n")
	}

	// Packer indicators from strings
	f.Seek(0, 0)
	packers := detectPackerIndicators(f)
	if len(packers) > 0 {
		sb.WriteString(fmt.Sprintf("- **Packer/Compiler Indicators:** %s\n", strings.Join(packers, ", ")))
	}

	return sb.String(), nil
}

func identifyFormatByMagic(magic []byte) (format, arch string) {
	if len(magic) < 4 {
		return "unknown", ""
	}
	switch {
	case magic[0] == 0x7f && magic[1] == 'E' && magic[2] == 'L' && magic[3] == 'F':
		format = "ELF (Executable and Linkable Format)"
		if len(magic) > 18 {
			switch magic[4] {
			case 1:
				arch = "32-bit"
			case 2:
				arch = "64-bit"
			}
			// e_machine at offset 18 (32-bit) or 18 (64-bit same for first field)
			if len(magic) > 19 {
				machine := uint16(magic[18]) | uint16(magic[19])<<8
				switch machine {
				case 0x03:
					arch += ", x86"
				case 0x3E:
					arch += ", x86-64"
				case 0xB7:
					arch += ", AArch64"
				case 0x28:
					arch += ", ARM"
				}
			}
		}
	case magic[0] == 'M' && magic[1] == 'Z':
		format = "PE (Portable Executable) / DOS executable"
		arch = "Windows"
	case magic[0] == 0xCF && magic[1] == 0xFA && magic[2] == 0xED && magic[3] == 0xFE:
		format = "Mach-O (64-bit)"
		arch = "macOS/iOS"
	case magic[0] == 0xFE && magic[1] == 0xED && magic[2] == 0xFA && magic[3] == 0xCF:
		format = "Mach-O (64-bit, reversed)"
		arch = "macOS/iOS"
	case magic[0] == 0xCA && magic[1] == 0xFE && magic[2] == 0xBA && magic[3] == 0xBE:
		format = "Mach-O Universal Binary"
		arch = "macOS/iOS"
	case magic[0] == 'P' && magic[1] == 'K' && magic[2] == 0x03 && magic[3] == 0x04:
		format = "ZIP / JAR / APK / DOCX (PKZIP)"
	case magic[0] == 0x1F && (magic[1] == 0x8B || magic[1] == 0x9E):
		format = "GZIP compressed"
	case magic[0] == 'B' && magic[1] == 'Z' && magic[2] == 'h':
		format = "BZIP2 compressed"
	case magic[0] == 0xFD && magic[1] == 0x37 && magic[2] == 0x7A && magic[3] == 0x58 && magic[4] == 0x5A && magic[5] == 0x00:
		format = "XZ compressed"
	case len(magic) > 7 && string(magic[:8]) == "\x89PNG\r\n\x1a\n":
		format = "PNG image"
	case len(magic) > 2 && magic[0] == 0xFF && magic[1] == 0xD8 && magic[2] == 0xFF:
		format = "JPEG image"
	default:
		format = "unknown / raw binary"
	}
	return format, arch
}

func calculateEntropy(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}
	var freq [256]int
	for _, b := range data {
		freq[b]++
	}
	var entropy float64
	ln2 := 1.4426950408889634 // 1/log(2)
	for _, count := range freq {
		if count == 0 {
			continue
		}
		p := float64(count) / float64(len(data))
		entropy -= p * math.Log(p) * ln2
	}
	return entropy
}

func detectPackerIndicators(r io.Reader) []string {
	var indicators []string
	scanner := bufio.NewScanner(r)
	scanner.Split(bufio.ScanBytes)
	var buf []byte
	for scanner.Scan() {
		b := scanner.Bytes()[0]
		if b >= 32 && b <= 126 {
			buf = append(buf, b)
		} else {
			if len(buf) >= 4 {
				s := string(buf)
				lower := strings.ToLower(s)
				switch {
				case strings.Contains(lower, "upx"):
					indicators = append(indicators, "UPX")
				case strings.Contains(lower, "aspack"):
					indicators = append(indicators, "ASPack")
				case strings.Contains(lower, "petite"):
					indicators = append(indicators, "PEtite")
				case strings.Contains(lower, "vmprotect"):
					indicators = append(indicators, "VMProtect")
				case strings.Contains(lower, "themida"):
					indicators = append(indicators, "Themida")
				case strings.Contains(lower, "enigma"):
					indicators = append(indicators, "Enigma")
				case strings.Contains(lower, "mingw"):
					indicators = append(indicators, "MinGW")
				case strings.Contains(lower, "visual c++"):
					indicators = append(indicators, "MSVC")
				case strings.Contains(lower, "gcc"):
					indicators = append(indicators, "GCC")
				case strings.Contains(lower, "go.build") || strings.Contains(lower, "runtime.go"):
					indicators = append(indicators, "Go binary")
				case strings.Contains(lower, "rust"):
					indicators = append(indicators, "Rust binary")
				}
			}
			if len(indicators) >= 5 {
				break
			}
			buf = buf[:0]
		}
	}
	return dedupStrings(indicators)
}

func dedupStrings(ss []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// extractStringsFromFile extracts printable ASCII strings from a file.
func extractStringsFromFile(filePath string, minLen, maxResults int) (string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("gagal mengakses file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("path adalah direktori")
	}
	const maxSize = 50 * 1024 * 1024
	if info.Size() > maxSize {
		return "", fmt.Errorf("file terlalu besar (max 50 MB)")
	}

	// Try 'strings' command first
	if _, err := exec.LookPath("strings"); err == nil {
		cmd := exec.Command("strings", "-n", fmt.Sprintf("%d", minLen), filePath)
		out, err := cmd.CombinedOutput()
		if err == nil {
			lines := strings.Split(string(out), "\n")
			if len(lines) > maxResults {
				lines = lines[:maxResults]
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("### Extracted Strings from %s (min_length=%d, top %d results via 'strings' CLI)\n\n", filepath.Base(filePath), minLen, len(lines)))
			for i, line := range lines {
				if line == "" {
					continue
				}
				sb.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, line))
			}
			return sb.String(), nil
		}
	}

	// Fallback: pure Go implementation
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var results []string
	var buf []byte
	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanBytes)
	for scanner.Scan() {
		b := scanner.Bytes()[0]
		if (b >= 32 && b <= 126) || (b >= 0xC0 && b <= 0xFD) { // printable ASCII + UTF-8 lead bytes
			buf = append(buf, b)
		} else {
			if len(buf) >= minLen {
				results = append(results, string(buf))
				if len(results) >= maxResults {
					break
				}
			}
			buf = buf[:0]
		}
	}
	if len(buf) >= minLen && len(results) < maxResults {
		results = append(results, string(buf))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### Extracted Strings from %s (min_length=%d, top %d results, pure-Go fallback)\n\n", filepath.Base(filePath), minLen, len(results)))
	for i, s := range results {
		sb.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, s))
	}
	return sb.String(), nil
}

// scanSignature performs pattern matching against file bytes.
func scanSignature(filePath string, patterns []string) (string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("gagal mengakses file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("path adalah direktori")
	}
	const maxSize = 50 * 1024 * 1024
	if info.Size() > maxSize {
		return "", fmt.Errorf("file terlalu besar (max 50 MB)")
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("gagal membaca file: %w", err)
	}

	type match struct {
		pattern     string
		occurrences int
		offsets     []int
		confidence  string
	}

	var matches []match

	for _, pat := range patterns {
		m := match{pattern: pat, confidence: "medium"}
		if strings.HasPrefix(pat, "regex:") {
			reStr := strings.TrimPrefix(pat, "regex:")
			re, err := regexp.Compile(reStr)
			if err != nil {
				m.confidence = "invalid-pattern"
				matches = append(matches, m)
				continue
			}
			for _, loc := range re.FindAllIndex(data, -1) {
				m.occurrences++
				if len(m.offsets) < 5 {
					m.offsets = append(m.offsets, loc[0])
				}
			}
			m.confidence = "regex-match"
		} else {
			// Hex pattern or plain string
			hexPat := strings.ReplaceAll(pat, " ", "")
			var search []byte
			if regexp.MustCompile(`^[0-9A-Fa-f]+$`).MatchString(hexPat) && len(hexPat)%2 == 0 {
				search, err = hexDecodeString(hexPat)
				if err != nil {
					search = []byte(pat)
				}
				m.confidence = "hex-match"
			} else {
				search = []byte(pat)
				m.confidence = "plain-match"
			}
			for i := 0; i <= len(data)-len(search); i++ {
				if bytesEqual(data[i:i+len(search)], search) {
					m.occurrences++
					if len(m.offsets) < 5 {
						m.offsets = append(m.offsets, i)
					}
				}
			}
		}
		matches = append(matches, m)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### Signature Scan Report: %s\n\n", filepath.Base(filePath)))
	sb.WriteString(fmt.Sprintf("- **File Size:** %d bytes\n", len(data)))
	sb.WriteString(fmt.Sprintf("- **Patterns Scanned:** %d\n\n", len(patterns)))

	foundCount := 0
	for _, m := range matches {
		if m.occurrences > 0 {
			foundCount++
		}
		sb.WriteString(fmt.Sprintf("**Pattern:** `%s`\n", m.pattern))
		sb.WriteString(fmt.Sprintf("- Confidence: %s\n", m.confidence))
		sb.WriteString(fmt.Sprintf("- Occurrences: %d\n", m.occurrences))
		if len(m.offsets) > 0 {
			offStrs := make([]string, 0, len(m.offsets))
			for _, off := range m.offsets {
				offStrs = append(offStrs, fmt.Sprintf("0x%08X", off))
			}
			sb.WriteString(fmt.Sprintf("- Sample Offsets: %s\n", strings.Join(offStrs, ", ")))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("**Summary:** %d/%d patterns matched.\n", foundCount, len(patterns)))
	return sb.String(), nil
}

func hexDecodeString(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("invalid hex string length")
	}
	b := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		v, err := strconv.ParseUint(s[i:i+2], 16, 8)
		if err != nil {
			return nil, err
		}
		b[i/2] = byte(v)
	}
	return b, nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// analyzeDependencies maps imports and package dependencies in source code.
func analyzeDependencies(sourcePath, lang string) (string, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", fmt.Errorf("gagal mengakses path: %w", err)
	}

	// Auto-detect language by file extensions
	if lang == "auto" || lang == "" {
		lang = detectLanguage(sourcePath, info.IsDir())
	}

	var files []string
	if info.IsDir() {
		err = filepath.Walk(sourcePath, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if fi.IsDir() {
				if strings.HasPrefix(fi.Name(), ".") || fi.Name() == "node_modules" || fi.Name() == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			switch lang {
			case "go":
				if ext == ".go" {
					files = append(files, path)
				}
			case "javascript":
				if ext == ".js" || ext == ".mjs" || ext == ".cjs" {
					files = append(files, path)
				}
			case "typescript":
				if ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".jsx" {
					files = append(files, path)
				}
			case "python":
				if ext == ".py" {
					files = append(files, path)
				}
			default:
				files = append(files, path)
			}
			if len(files) > 500 {
				return io.EOF
			}
			return nil
		})
		if err != nil && err != io.EOF {
			return "", err
		}
	} else {
		files = append(files, sourcePath)
		if lang == "auto" {
			lang = detectLanguage(sourcePath, false)
		}
	}

	internalSet := make(map[string]bool)
	externalSet := make(map[string]bool)
	internalPrefix := ""
	if lang == "go" {
		internalPrefix = guessGoModulePrefix(sourcePath)
	}

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			switch lang {
			case "go":
				if strings.HasPrefix(line, "import (") || strings.HasPrefix(line, "import ") {
					imp := extractGoImport(line, string(data))
					if imp != "" {
						if internalPrefix != "" && strings.HasPrefix(imp, internalPrefix) {
							internalSet[imp] = true
						} else {
							externalSet[imp] = true
						}
					}
				}
			case "javascript", "typescript":
				if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "require(") || strings.HasPrefix(line, "from ") {
					imp := extractJSImport(line)
					if imp != "" {
						if strings.HasPrefix(imp, ".") || strings.HasPrefix(imp, "@/") || strings.HasPrefix(imp, "~/") {
							internalSet[imp] = true
						} else {
							externalSet[imp] = true
						}
					}
				}
			case "python":
				if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "from ") {
					imp := extractPyImport(line)
					if imp != "" {
						if strings.HasPrefix(imp, ".") {
							internalSet[imp] = true
						} else {
							externalSet[imp] = true
						}
					}
				}
			default:
				// Generic regex-based import extraction
				re := regexp.MustCompile(`(?:import|require|from)\s+["']([^"']+)["']`)
				m := re.FindStringSubmatch(line)
				if len(m) > 1 {
					externalSet[m[1]] = true
				}
			}
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### Dependency Analysis: %s (%s, %d files scanned)\n\n", filepath.Base(sourcePath), lang, len(files)))

	internals := sortedKeys(internalSet)
	if len(internals) > 0 {
		sb.WriteString(fmt.Sprintf("**Internal Dependencies (%d):**\n", len(internals)))
		for _, imp := range internals {
			sb.WriteString(fmt.Sprintf("- `%s`\n", imp))
		}
		sb.WriteString("\n")
	}

	externals := sortedKeys(externalSet)
	if len(externals) > 0 {
		sb.WriteString(fmt.Sprintf("**External Dependencies (%d):**\n", len(externals)))
		for _, imp := range externals {
			sb.WriteString(fmt.Sprintf("- `%s`\n", imp))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("**Summary:** %d internal, %d external dependencies across %d files.\n", len(internals), len(externals), len(files)))
	return sb.String(), nil
}

func detectLanguage(path string, isDir bool) string {
	if !isDir {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".go":
			return "go"
		case ".js", ".mjs", ".cjs":
			return "javascript"
		case ".ts", ".tsx":
			return "typescript"
		case ".py":
			return "python"
		default:
			return "unknown"
		}
	}
	// Heuristic: count file extensions in directory
	exts := make(map[string]int)
	filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		exts[strings.ToLower(filepath.Ext(p))]++
		return nil
	})
	type extCount struct {
		ext   string
		count int
	}
	var ecs []extCount
	for e, c := range exts {
		ecs = append(ecs, extCount{e, c})
	}
	sort.Slice(ecs, func(i, j int) bool { return ecs[i].count > ecs[j].count })
	if len(ecs) == 0 {
		return "unknown"
	}
	switch ecs[0].ext {
	case ".go":
		return "go"
	case ".js", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".py":
		return "python"
	default:
		return "unknown"
	}
}

func guessGoModulePrefix(sourcePath string) string {
	modPath := filepath.Join(sourcePath, "go.mod")
	if _, err := os.Stat(modPath); err == nil {
		data, _ := os.ReadFile(modPath)
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "module ") {
				mod := strings.TrimSpace(strings.TrimPrefix(line, "module "))
				return mod
			}
		}
	}
	return ""
}

func extractGoImport(line, fullData string) string {
	// Handle single-line import: import "path" or import alias "path"
	re := regexp.MustCompile(`import\s+(?:\S+\s+)?"([^"]+)"`)
	m := re.FindStringSubmatch(line)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func extractJSImport(line string) string {
	re := regexp.MustCompile(`(?:import\s+.*?\s+from\s+["']([^"']+)["']|require\s*\(\s*["']([^"']+)["']\s*\)|from\s+["']([^"']+)["'])`)
	m := re.FindStringSubmatch(line)
	for i := 1; i < len(m); i++ {
		if m[i] != "" {
			return m[i]
		}
	}
	return ""
}

func extractPyImport(line string) string {
	re := regexp.MustCompile(`(?:import|from)\s+([\w.]+)`)
	m := re.FindStringSubmatch(line)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// generateCallGraph creates a simple static call-graph outline from source code.
func generateCallGraph(sourcePath, lang string, maxDepth int) (string, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", fmt.Errorf("gagal mengakses path: %w", err)
	}

	if lang == "auto" || lang == "" {
		lang = detectLanguage(sourcePath, info.IsDir())
	}

	var files []string
	if info.IsDir() {
		err = filepath.Walk(sourcePath, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if fi.IsDir() {
				if strings.HasPrefix(fi.Name(), ".") || fi.Name() == "node_modules" || fi.Name() == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			switch lang {
			case "go":
				if ext == ".go" {
					files = append(files, path)
				}
			case "javascript":
				if ext == ".js" || ext == ".mjs" || ext == ".cjs" {
					files = append(files, path)
				}
			case "typescript":
				if ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".jsx" {
					files = append(files, path)
				}
			case "python":
				if ext == ".py" {
					files = append(files, path)
				}
			default:
				files = append(files, path)
			}
			if len(files) > 500 {
				return io.EOF
			}
			return nil
		})
		if err != nil && err != io.EOF {
			return "", err
		}
	} else {
		files = append(files, sourcePath)
	}

	functions := make(map[string][]string) // function name -> callers
	var funcNames []string
	var funcRegex *regexp.Regexp

	switch lang {
	case "go":
		funcRegex = regexp.MustCompile(`^func\s+(?:\([^)]*\)\s+)?(\w+)`)
	case "javascript", "typescript":
		funcRegex = regexp.MustCompile(`(?:function\s+(\w+)|(?:const|let|var)\s+(\w+)\s*=\s*(?:function|.*?=>))`)
	case "python":
		funcRegex = regexp.MustCompile(`^def\s+(\w+)\s*\(`)
	default:
		funcRegex = regexp.MustCompile(`(?:function|func|def|void|int|String)\s+(\w+)\s*\(`)
	}

	// Collect function definitions
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			m := funcRegex.FindStringSubmatch(line)
			for i := 1; i < len(m); i++ {
				if m[i] != "" {
					name := m[i]
					if _, exists := functions[name]; !exists {
						functions[name] = []string{}
						funcNames = append(funcNames, name)
					}
				}
			}
		}
	}

	// Find callers (naive regex scan)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		content := string(data)
		for _, fn := range funcNames {
			// Simple caller search: fnName( or fnName ( with preceding non-word char
			re := regexp.MustCompile(`\b` + regexp.QuoteMeta(fn) + `\s*\(`)
			if re.MatchString(content) {
				// Avoid self-count from definition line
				count := len(re.FindAllString(content, -1))
				if count > 0 {
					functions[fn] = append(functions[fn], fmt.Sprintf("%s (%dx)", filepath.Base(f), count))
				}
			}
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### Static Call Graph Outline: %s (%s, %d files, depth=%d)\n\n", filepath.Base(sourcePath), lang, len(files), maxDepth))

	for _, fn := range funcNames {
		callers := functions[fn]
		sb.WriteString(fmt.Sprintf("**%s()**\n", fn))
		if len(callers) == 0 {
			sb.WriteString("- (no external callers detected — may be entry point / unused)\n")
		} else {
			// Deduplicate callers
			seen := make(map[string]bool)
			var uniq []string
			for _, c := range callers {
				if !seen[c] {
					seen[c] = true
					uniq = append(uniq, c)
				}
			}
			for _, c := range uniq {
				sb.WriteString(fmt.Sprintf("- Called from: %s\n", c))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("**Summary:** %d functions analyzed across %d source files.\n", len(funcNames), len(files)))
	return sb.String(), nil
}

// serveProject detects project type and starts an appropriate background HTTP server.
func serveProject(args map[string]interface{}) (string, error) {
	projectDir, _ := args["project_dir"].(string)
	if projectDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("gagal mendapatkan cwd: %w", err)
		}
		projectDir = cwd
	}
	// Pastikan direktori ada
	if fi, err := os.Stat(projectDir); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("direktori '%s' tidak ditemukan", projectDir)
	}

	// Cek apakah sudah ada server aktif untuk direktori ini
	activeServersMu.Lock()
	if cmd, ok := activeServers[projectDir]; ok && cmd.Process != nil {
		activeServersMu.Unlock()
		return fmt.Sprintf("Server sudah berjalan untuk '%s'.", projectDir), nil
	}
	activeServersMu.Unlock()

	// Tentukan port
	port := 0
	if p, ok := args["port"].(float64); ok {
		port = int(p)
	}
	if port <= 0 {
		for p := 8000; p <= 8999; p++ {
			ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
			if err == nil {
				ln.Close()
				port = p
				break
			}
		}
	}
	if port <= 0 {
		return "", fmt.Errorf("tidak dapat menemukan port kosong di range 8000-8999")
	}

	// Auto-detect project type
	type serveInfo struct {
		cmd       *exec.Cmd
		entryFile string
		portHint  int
	}
	var detected *serveInfo

	// Helpers
	fileExists := func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}
	readFile := func(path string) string {
		b, _ := os.ReadFile(path)
		return string(b)
	}
	extractPortFromContent := func(content string) int {
		// Common patterns: :3000, :8080, :5000, process.env.PORT || 3000, ListenAndServe(":8080"
		re := regexp.MustCompile(`(?i)(?:listen.*?:|\bport\b.*?=|env\.PORT.*?\|\|).*?(\d{4,5})`)
		m := re.FindStringSubmatch(content)
		if len(m) > 1 {
			if p, _ := strconv.Atoi(m[1]); p > 0 {
				return p
			}
		}
		return 0
	}

	switch {
	// Node.js (server.js / app.js / index.js)
	case fileExists(filepath.Join(projectDir, "server.js")):
		detected = &serveInfo{entryFile: "server.js", cmd: exec.Command("node", "server.js")}
	case fileExists(filepath.Join(projectDir, "app.js")):
		detected = &serveInfo{entryFile: "app.js", cmd: exec.Command("node", "app.js")}
	case fileExists(filepath.Join(projectDir, "index.js")):
		detected = &serveInfo{entryFile: "index.js", cmd: exec.Command("node", "index.js")}

	// Bun (bun.lockb, bun run)
	case fileExists(filepath.Join(projectDir, "bun.lockb")) || fileExists(filepath.Join(projectDir, "bun.lock")):
		if fileExists(filepath.Join(projectDir, "index.ts")) {
			detected = &serveInfo{entryFile: "index.ts", cmd: exec.Command("bun", "run", "index.ts")}
		} else if fileExists(filepath.Join(projectDir, "server.ts")) {
			detected = &serveInfo{entryFile: "server.ts", cmd: exec.Command("bun", "run", "server.ts")}
		} else if fileExists(filepath.Join(projectDir, "package.json")) {
			detected = &serveInfo{entryFile: "package.json", cmd: exec.Command("bun", "run", "start")}
		}

	// TypeScript Node (server.ts / index.ts)
	case fileExists(filepath.Join(projectDir, "server.ts")):
		if fileExists(filepath.Join(projectDir, "package.json")) {
			pj := readFile(filepath.Join(projectDir, "package.json"))
			if strings.Contains(pj, "tsx") || strings.Contains(pj, "ts-node") {
				detected = &serveInfo{entryFile: "server.ts", cmd: exec.Command("npx", "tsx", "server.ts")}
			}
		}
		if detected == nil {
			detected = &serveInfo{entryFile: "server.ts", cmd: exec.Command("npx", "ts-node", "server.ts")}
		}
	case fileExists(filepath.Join(projectDir, "index.ts")):
		if fileExists(filepath.Join(projectDir, "package.json")) {
			pj := readFile(filepath.Join(projectDir, "package.json"))
			if strings.Contains(pj, "tsx") || strings.Contains(pj, "ts-node") {
				detected = &serveInfo{entryFile: "index.ts", cmd: exec.Command("npx", "tsx", "index.ts")}
			}
		}
		if detected == nil {
			detected = &serveInfo{entryFile: "index.ts", cmd: exec.Command("npx", "ts-node", "index.ts")}
		}

	// Go
	case fileExists(filepath.Join(projectDir, "go.mod")) || fileExists(filepath.Join(projectDir, "main.go")):
		if fileExists(filepath.Join(projectDir, "main.go")) {
			detected = &serveInfo{entryFile: "main.go", cmd: exec.Command("go", "run", "main.go")}
		} else {
			detected = &serveInfo{entryFile: "go.mod", cmd: exec.Command("go", "run", ".")}
		}

	// PHP
	case fileExists(filepath.Join(projectDir, "index.php")):
		detected = &serveInfo{entryFile: "index.php", cmd: exec.Command("php", "-S", fmt.Sprintf("0.0.0.0:%d", port), "index.php")}
	case fileExists(filepath.Join(projectDir, "server.php")):
		detected = &serveInfo{entryFile: "server.php", cmd: exec.Command("php", "-S", fmt.Sprintf("0.0.0.0:%d", port), "server.php")}
	}

	// Override port for PHP (already bound in cmd above) or detect from entry file
	if detected != nil && detected.entryFile != "" && !strings.HasPrefix(detected.entryFile, "package.json") {
		content := readFile(filepath.Join(projectDir, detected.entryFile))
		if p := extractPortFromContent(content); p > 0 {
			detected.portHint = p
		}
	}

	// If no backend runtime detected → fallback to static file server
	if detected == nil {
		detected = &serveInfo{entryFile: "static", cmd: exec.Command("python3", "-m", "http.server", strconv.Itoa(port))}
	} else {
		// Non-static runtime detected
		// Override detected port hint via environment variable so the app binds to our free port
		if detected.portHint > 0 {
			// The app might read PORT env var or hardcode. We'll set both PORT and commonly used alternatives.
			detected.cmd.Env = append(os.Environ(),
				fmt.Sprintf("PORT=%d", port),
				fmt.Sprintf("SERVER_PORT=%d", port),
				fmt.Sprintf("HTTP_PORT=%d", port),
				fmt.Sprintf("APP_PORT=%d", port),
			)
		} else {
			detected.cmd.Env = append(os.Environ(), fmt.Sprintf("PORT=%d", port))
		}
	}

	detected.cmd.Dir = projectDir
	setProcessGroup(detected.cmd)
	// Redirect output to avoid hanging if pipes fill
	detected.cmd.Stdout = nil
	detected.cmd.Stderr = nil
	if err := detected.cmd.Start(); err != nil {
		return "", fmt.Errorf("gagal memulai server [%s]: %w", detected.entryFile, err)
	}

	activeServersMu.Lock()
	activeServers[projectDir] = detected.cmd
	activeServersMu.Unlock()

	// Deteksi IP publik
	publicIP := "VPS_IP"
	if out, err := exec.Command("curl", "-s", "-4", "ifconfig.me").CombinedOutput(); err == nil {
		ip := strings.TrimSpace(string(out))
		if ip != "" {
			publicIP = ip
		}
	}

	url := fmt.Sprintf("http://%s:%d", publicIP, port)
	return fmt.Sprintf(
		"✅ Server %s berjalan\n📂 Project: %s\n🚀 Entry: %s\n🔗 URL akses: %s\n\nBuka di browser PC kamu:\n%s",
		detected.entryFile, projectDir, detected.entryFile, url, url,
	), nil
}
