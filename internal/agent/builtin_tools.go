package agent

import (
	"bufio"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/nudge"
	"github.com/gede-cahya/Smara-CLI/internal/skill"
	smarassh "github.com/gede-cahya/Smara-CLI/internal/ssh"
	"github.com/gede-cahya/Smara-CLI/internal/workspace"
)

// BuiltinDB is set by the Supervisor so built-in tools can access SQLite.
var BuiltinDB *sql.DB

const builtinMCPServerName = "builtin"

// GetBuiltinTools returns the standard OS and file manipulation tools
func GetBuiltinTools() []llm.ToolFunction {
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
				"type": "object",
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
						"items": map[string]interface{}{"type": "string"},
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
	}
}

// ExecuteBuiltinTool eksekusi fungsi tool built-in tanpa harus melewati koneksi MCP
func ExecuteBuiltinTool(toolName string, args map[string]interface{}, logCallback func(role, content string)) (string, error) {
	switch toolName {
	case "run_command":
		cmdStr, ok := args["command"].(string)
		if !ok {
			return "", fmt.Errorf("argumen 'command' tidak valid")
		}

		cmd := exec.Command("sh", "-c", cmdStr)
		
		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()
		
		var fullOutput strings.Builder
		var mu sync.Mutex
		
		if err := cmd.Start(); err != nil {
			return "", fmt.Errorf("gagal memulai perintah: %w", err)
		}

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

		// Tunggu sampai kedua pipe ditutup (EOF)
		wg.Wait()

		if err := cmd.Wait(); err != nil {
			output := fullOutput.String()
			if output == "" {
				output = "(tidak ada output)"
			}
			return output, fmt.Errorf("eksekusi gagal: %w\nOutput: %s", err, output)
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
		
		if startLine < 1 { startLine = 1 }
		if endLine > len(lines) { endLine = len(lines) }
		
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
		content, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("gagal membaca file: %w", err)
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

		// Pastikan direktori ada
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("gagal membuat direktori: %w", err)
		}

		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("gagal menulis file: %w", err)
		}
		return fmt.Sprintf("File %s berhasil ditulis.", path), nil

	case "delete_file":
		path, ok := args["path"].(string)
		if !ok {
			return "", fmt.Errorf("argumen 'path' tidak valid")
		}
		if err := os.Remove(path); err != nil {
			return "", fmt.Errorf("gagal menghapus file: %w", err)
		}
		return fmt.Sprintf("File %s berhasil dihapus.", path), nil

	case "list_dir":
		path, ok := args["path"].(string)
		if !ok {
			return "", fmt.Errorf("argumen 'path' tidak valid")
		}
		
		entries, err := os.ReadDir(path)
		if err != nil {
			return "", fmt.Errorf("gagal membaca direktori: %w", err)
		}

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

		// Jika start_line/end_line diberikan, cari hanya di range tersebut
		content := string(data)
		if okStart, okEnd := args["start_line"] != nil, args["end_line"] != nil; okStart || okEnd {
			if startLine < 1 { startLine = 1 }
			if endLine > len(lines) { endLine = len(lines) }
			
			subContent := strings.Join(lines[startLine-1:endLine], "\n")
			if !strings.Contains(subContent, oldContent) {
				return "", fmt.Errorf("teks 'old_content' tidak ditemukan di baris %d-%d. Gunakan view_file untuk verifikasi.", startLine, endLine)
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

		return fmt.Sprintf("File %s berhasil diperbarui.", path), nil

	case "grep_search":
		query, _ := args["query"].(string)
		searchPathStr := "."
		if p, ok := args["path"].(string); ok {
			searchPathStr = p
		}

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
		return searchPath(query, root, logCallback)

	case "get_cwd":
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
		return searchWeb(query)

	case "ssh_exec":
		hostArg, _ := args["host"].(string)
		command, _ := args["command"].(string)
		if hostArg == "" || command == "" {
			return "", fmt.Errorf("argumen 'host' dan 'command' wajib diisi")
		}

		host, err := resolveHost(hostArg)
		if err != nil {
			return "", err
		}

		client, err := smarassh.Connect(host)
		if err != nil {
			return "", fmt.Errorf("gagal koneksi SSH: %w", err)
		}
		defer client.Close()

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

		host, err := resolveHost(hostArg)
		if err != nil {
			return "", err
		}

		client, err := smarassh.Connect(host)
		if err != nil {
			return "", fmt.Errorf("gagal koneksi SSH: %w", err)
		}
		defer client.Close()

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

		host, err := resolveHost(hostArg)
		if err != nil {
			return "", err
		}

		client, err := smarassh.Connect(host)
		if err != nil {
			return "", fmt.Errorf("gagal koneksi SSH: %w", err)
		}
		defer client.Close()

		stdout, stderr, err := client.Exec("ls -la " + path)
		if err != nil {
			return "", fmt.Errorf("gagal list direktori: %w\nstderr: %s", err, stderr)
		}
		if stderr != "" {
			return stdout + "\n(stderr: " + stderr + ")", nil
		}
		return stdout, nil

	case "ssh_manage":
		action, _ := args["action"].(string)
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
		sk, err := skill.Load(name)
		if err != nil {
			return "", fmt.Errorf("skill '%s' tidak ditemukan: %w", name, err)
		}
		res, err := sk.Run(func(toolName string, toolArgs map[string]interface{}) (string, error) {
			return ExecuteBuiltinTool(toolName, toolArgs, logCallback)
		})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Skill '%s' dijalankan. Sukses=%v. %s", name, res.Success, res.Summary), nil

	case "schedule_reminder":
		promptText := getStr(args, "prompt_text")
		when := getStr(args, "when")
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
