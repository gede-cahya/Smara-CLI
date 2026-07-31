package agent

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"html"
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

	"github.com/gede-cahya/Smara-CLI/pkg/llm"
)

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
			Name:        "respond",
			Description: "Mengirim respons teks langsung dari skill kepada pengguna. Berguna untuk skill instruksional hasil konversi plugin Claude/Obsidian.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"message": map[string]interface{}{
						"type":        "string",
						"description": "Pesan yang akan ditampilkan kepada pengguna",
					},
				},
				"required": []string{"message"},
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
			Name:        "skill_create",
			Description: "Membuat atau meng-upgrade skill otomasi (resep tool calls) ke ~/.smara/skills/.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":        map[string]interface{}{"type": "string"},
					"description": map[string]interface{}{"type": "string"},
					"steps": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"tool": map[string]interface{}{"type": "string"},
								"args": map[string]interface{}{"type": "object"},
							},
							"required": []string{"tool", "args"},
						},
					},
					"tags":      map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
					"params":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "object"}},
					"overwrite": map[string]interface{}{"type": "boolean"},
				},
				"required": []string{"name", "description", "steps"},
			},
		},
		{
			Name:        "skill_list",
			Description: "Daftar semua skill yang tersimpan di ~/.smara/skills/.",
			Parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
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
	}
}

// ExecuteBuiltinTool eksekusi fungsi tool built-in tanpa harus melewati koneksi MCP
func ExecuteBuiltinTool(toolName string, args map[string]interface{}, logCallback func(role, content string)) (string, error) {
	switch toolName {
	case "generate_image":
		return executeGenerateImageTool(args)
	case "edit_image":
		return executeEditImageTool(args)

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

		if startLine < 1 {
			startLine = 1
		}
		if endLine > len(lines) {
			endLine = len(lines)
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
		if oldContent == "" {
			return "", fmt.Errorf("old_content tidak boleh kosong. Gunakan view_file untuk mengambil teks yang tepat.")
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("gagal membaca file: %w", err)
		}

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
		if p, ok := args["path"].(string); ok && p != "" {
			searchPathStr = p
		}

		cmd := exec.Command("grep", "-r", "-n",
			"--exclude-dir=.git",
			"--exclude-dir=node_modules",
			"--exclude-dir=.smara",
			"--exclude-dir=dist",
			"--exclude-dir=build",
			"--exclude-dir=graphify-out",
			"--exclude-dir=.agents",
			"--exclude-dir=smara-backup*",
			"--exclude=*.test",
			"--exclude=*.png",
			"--exclude=*.jpg",
			"--exclude=*.zip",
			"--exclude=*.tar.gz",
			"--exclude=*.db",
			"--exclude=*.log",
			"--binary-files=without-match",
			query, searchPathStr)
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

	default:
		return "", fmt.Errorf("tool built-in '%s' tidak dikenali", toolName)
	}
}

func searchPath(query, root string, logFn func(string, string)) (string, error) {
	if logFn != nil {
		logFn("system", fmt.Sprintf("Mencari '%s' mulai dari '%s'...", query, root))
	}

	if root == "" {
		root = "."
	}

	var results []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip hidden directories like .git
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") && info.Name() != "." && info.Name() != ".." {
			return filepath.SkipDir
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
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("query pencarian kosong")
	}

	providers := []struct {
		name  string
		url   string
		parse func(string, string) (string, bool)
	}{
		{name: "DuckDuckGo", url: fmt.Sprintf("https://duckduckgo.com/html/?q=%s", url.QueryEscape(query)), parse: parseDuckDuckGoResults},
		{name: "DuckDuckGo Lite", url: fmt.Sprintf("https://lite.duckduckgo.com/lite/?q=%s", url.QueryEscape(query)), parse: parseDuckDuckGoResults},
		{name: "Bing RSS", url: fmt.Sprintf("https://www.bing.com/search?format=rss&q=%s&setlang=id-ID&count=10", url.QueryEscape(query)), parse: parseBingRSSResults},
		{name: "Bing", url: fmt.Sprintf("https://www.bing.com/search?q=%s&setlang=id-ID&count=10", url.QueryEscape(query)), parse: parseBingResults},
		{name: "Google", url: fmt.Sprintf("https://www.google.com/search?q=%s&hl=id&num=10", url.QueryEscape(query)), parse: parseGoogleResults},
	}

	client := &http.Client{Timeout: 15 * time.Second}
	errs := []string{}
	for _, provider := range providers {
		htmlBody, err := fetchSearchHTML(client, provider.url)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", provider.name, err))
			continue
		}
		if result, ok := provider.parse(query, htmlBody); ok {
			if provider.name != "DuckDuckGo" && provider.name != "DuckDuckGo Lite" {
				return fmt.Sprintf("⚠ DuckDuckGo tidak tersedia/hasilnya tidak terparse, memakai fallback %s.\n\n%s", provider.name, result), nil
			}
			return result, nil
		}
		if result, ok := parseGenericSearchResults(query, htmlBody); ok {
			return fmt.Sprintf("⚠ Parser khusus %s tidak menemukan hasil, memakai parser generic.\n\n%s", provider.name, result), nil
		}
		errs = append(errs, provider.name+": tidak ada hasil terparse")
	}

	// Jangan biarkan tool benar-benar buntu ketika search engine memblokir,
	// mengubah HTML, atau jaringan sedang membatasi TLS/anti-bot. Kembalikan
	// tautan pencarian langsung supaya agent tetap bisa membantu user dan bisa
	// lanjut dengan web_fetch pada URL yang diberikan user.
	return buildSearchFallback(query, errs), nil
}

func buildSearchFallback(query string, errs []string) string {
	encoded := url.QueryEscape(strings.TrimSpace(query))
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### Pencarian web untuk: '%s'\n\n", query))
	sb.WriteString("⚠ Semua provider search gagal atau hasilnya tidak dapat diparse otomatis. Berikut tautan pencarian langsung yang bisa dibuka/difetch ulang:\n\n")
	sb.WriteString(fmt.Sprintf("1. DuckDuckGo: https://duckduckgo.com/?q=%s\n", encoded))
	sb.WriteString(fmt.Sprintf("2. DuckDuckGo Lite: https://lite.duckduckgo.com/lite/?q=%s\n", encoded))
	sb.WriteString(fmt.Sprintf("3. Bing: https://www.bing.com/search?q=%s\n", encoded))
	sb.WriteString(fmt.Sprintf("4. Google: https://www.google.com/search?q=%s\n", encoded))
	if len(errs) > 0 {
		maxErrs := len(errs)
		if maxErrs > 5 {
			maxErrs = 5
		}
		sb.WriteString("\nDetail kegagalan provider:\n")
		for i := 0; i < maxErrs; i++ {
			sb.WriteString(fmt.Sprintf("- %s\n", errs[i]))
		}
		if len(errs) > maxErrs {
			sb.WriteString(fmt.Sprintf("- ... dan %d error lain\n", len(errs)-maxErrs))
		}
	}
	return sb.String()
}

func fetchSearchHTML(client *http.Client, searchURL string) (string, error) {
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,application/rss+xml,application/xml;q=0.8,*/*;q=0.7")
	req.Header.Set("Accept-Language", "id-ID,id;q=0.9,en-US;q=0.8,en;q=0.7")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gagal menghubungi search engine: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("search engine mengembalikan status: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func parseDuckDuckGoResults(query, htmlBody string) (string, bool) {
	patterns := []*regexp.Regexp{
		// DuckDuckGo HTML. Attribute order/classes can change, so keep this tolerant.
		regexp.MustCompile(`(?is)<div[^>]+class=["'][^"']*result__body[^"']*["'][^>]*>.*?<a[^>]+class=["'][^"']*result__a[^"']*["'][^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>.*?<(?:a|div)[^>]+class=["'][^"']*result__snippet[^"']*["'][^>]*>(.*?)</(?:a|div)>`),
		regexp.MustCompile(`(?is)<div[^>]+class=["'][^"']*result__body[^"']*["'][^>]*>.*?<a[^>]+href=["']([^"']+)["'][^>]*class=["'][^"']*result__a[^"']*["'][^>]*>(.*?)</a>.*?<(?:a|div)[^>]+class=["'][^"']*result__snippet[^"']*["'][^>]*>(.*?)</(?:a|div)>`),
		// DuckDuckGo Lite.
		regexp.MustCompile(`(?is)<a[^>]+class=["'][^"']*result-link[^"']*["'][^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>.*?<td[^>]+class=["'][^"']*result-snippet[^"']*["'][^>]*>(.*?)</td>`),
		regexp.MustCompile(`(?is)<a[^>]+href=["']([^"']+)["'][^>]*class=["'][^"']*result-link[^"']*["'][^>]*>(.*?)</a>.*?<td[^>]+class=["'][^"']*result-snippet[^"']*["'][^>]*>(.*?)</td>`),
	}
	for _, re := range patterns {
		if result, ok := formatSearchMatches(query, re.FindAllStringSubmatch(htmlBody, 10)); ok {
			return result, true
		}
	}
	return "", false
}

func parseGoogleResults(query, htmlBody string) (string, bool) {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?is)<a\s+href=["']/url\?q=([^"'&]+)[^"']*["'][^>]*>(.*?)</a>`),
		regexp.MustCompile(`(?is)<a\s+href=["'](https?://[^"']+)["'][^>]*>\s*<br\s*/?>?\s*<h3[^>]*>(.*?)</h3>`),
		regexp.MustCompile(`(?is)<a\s+href=["'](https?://[^"']+)["'][^>]*>.*?<h3[^>]*>(.*?)</h3>.*?</a>`),
	}
	for _, re := range patterns {
		if result, ok := formatSearchMatches(query, re.FindAllStringSubmatch(htmlBody, 10)); ok {
			return result, true
		}
	}
	return "", false
}

func parseBingResults(query, htmlBody string) (string, bool) {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?is)<li[^>]+class=["'][^"']*b_algo[^"']*["'][^>]*>.*?<h2[^>]*>.*?<a\s+href=["'](https?://[^"']+)["'][^>]*>(.*?)</a>.*?</h2>.*?<p[^>]*>(.*?)</p>`),
		regexp.MustCompile(`(?is)<li[^>]+class=["'][^"']*b_algo[^"']*["'][^>]*>.*?<a\s+href=["'](https?://[^"']+)["'][^>]*>(.*?)</a>`),
		regexp.MustCompile(`(?is)<h2[^>]*>\s*<a\s+href=["'](https?://[^"']+)["'][^>]*>(.*?)</a>\s*</h2>`),
	}
	for _, re := range patterns {
		if result, ok := formatSearchMatches(query, re.FindAllStringSubmatch(htmlBody, 10)); ok {
			return result, true
		}
	}
	return "", false
}

func parseBingRSSResults(query, xmlBody string) (string, bool) {
	re := regexp.MustCompile(`(?is)<item\b[^>]*>.*?<title>(.*?)</title>.*?<link>(.*?)</link>.*?(?:<description>|<content:encoded>)(.*?)(?:</description>|</content:encoded>).*?</item>`)
	matches := re.FindAllStringSubmatch(xmlBody, 10)
	if len(matches) == 0 {
		return "", false
	}
	converted := make([][]string, 0, len(matches))
	for _, m := range matches {
		converted = append(converted, []string{m[0], m[2], m[1], m[3]})
	}
	return formatSearchMatches(query, converted)
}

func parseGenericSearchResults(query, htmlBody string) (string, bool) {
	// Generic fallback for frequently changing search-result HTML. Prefer
	// anchors that expose an h3 title, then fall back to a tolerant anchor
	// scanner. Search engines often alter wrappers/classes while keeping links.
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?is)<a\b[^>]*\bhref=["']([^"']+)["'][^>]*>\s*<h3[^>]*>(.*?)</h3>.*?</a>`),
		regexp.MustCompile(`(?is)<h3[^>]*>\s*<a\b[^>]*\bhref=["']([^"']+)["'][^>]*>(.*?)</a>\s*</h3>`),
		regexp.MustCompile(`(?is)<a\b[^>]*\bhref=["'](https?://[^"']+)["'][^>]*>(.*?)</a>`),
		regexp.MustCompile(`(?is)<a\b[^>]*\bhref=["'](/url\?q=[^"']+)["'][^>]*>(.*?)</a>`),
	}
	for _, re := range patterns {
		if result, ok := formatSearchMatches(query, re.FindAllStringSubmatch(htmlBody, 30)); ok {
			return result, true
		}
	}
	return formatSearchMatches(query, extractSearchAnchors(htmlBody, 40))
}

func extractSearchAnchors(htmlBody string, limit int) [][]string {
	if limit <= 0 {
		limit = 40
	}
	anchorRe := regexp.MustCompile(`(?is)<a\b([^>]*)>(.*?)</a>`)
	hrefRe := regexp.MustCompile(`(?is)\bhref\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
	matches := anchorRe.FindAllStringSubmatch(htmlBody, -1)
	out := make([][]string, 0, limit)
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		h := hrefRe.FindStringSubmatch(m[1])
		if len(h) == 0 {
			continue
		}
		rawHref := firstNonEmpty(h[1:]...)
		title := cleanHTML(m[2])
		if rawHref == "" || title == "" || len([]rune(title)) < 3 {
			continue
		}
		// Drop obvious navigation/noise anchors before formatting.
		lowerTitle := strings.ToLower(title)
		if lowerTitle == "images" || lowerTitle == "videos" || lowerTitle == "news" || lowerTitle == "maps" || lowerTitle == "login" || lowerTitle == "sign in" || lowerTitle == "next" {
			continue
		}
		out = append(out, []string{m[0], rawHref, title})
		if len(out) >= limit {
			break
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func formatSearchMatches(query string, matches [][]string) (string, bool) {
	if len(matches) == 0 {
		return "", false
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### Hasil Pencarian Internet untuk: '%s'\n\n", query))
	seen := map[string]bool{}
	count := 0

	for _, m := range matches {
		if len(m) < 3 {
			continue
		}

		link := normalizeSearchLink(m[1])
		title := cleanHTML(m[2])
		snippet := ""
		if len(m) > 3 {
			snippet = cleanHTML(m[3])
		}

		if title == "" || link == "" || isSearchInternalLink(link) || seen[link] {
			continue
		}
		seen[link] = true
		count++

		if snippet != "" {
			sb.WriteString(fmt.Sprintf("%d. **%s**\n   - Link: %s\n   - %s\n\n", count, title, link, snippet))
		} else {
			sb.WriteString(fmt.Sprintf("%d. **%s**\n   - Link: %s\n\n", count, title, link))
		}
		if count >= 10 {
			break
		}
	}

	if count == 0 {
		return "", false
	}
	return sb.String(), true
}

func normalizeSearchLink(link string) string {
	link = strings.TrimSpace(html.UnescapeString(link))
	if link == "" {
		return ""
	}

	// Google redirect: /url?q=https://example.com or full URL with q parameter.
	if strings.HasPrefix(link, "/url?") || strings.Contains(link, "?q=") || strings.Contains(link, "&q=") {
		if u, err := url.Parse(link); err == nil {
			if q := u.Query().Get("q"); q != "" {
				link = q
			}
		}
	}

	// DuckDuckGo redirect: /l/?uddg=https%3A%2F%2Fexample.com
	if strings.Contains(link, "uddg=") {
		if u, err := url.Parse(link); err == nil {
			if uddg := u.Query().Get("uddg"); uddg != "" {
				link = uddg
			}
		} else {
			parts := strings.Split(link, "uddg=")
			if len(parts) > 1 {
				decoded, _ := url.QueryUnescape(strings.Split(parts[1], "&")[0])
				link = decoded
			}
		}
	}

	// Bing redirect: https://www.bing.com/ck/a?...&u=a1aHR0cHM6Ly9leGFtcGxlLmNvbQ&...
	if strings.Contains(link, "bing.com/ck/") {
		if u, err := url.Parse(link); err == nil {
			if target := decodeBingUParam(u.Query().Get("u")); target != "" {
				link = target
			}
		}
	}

	if decoded, err := url.QueryUnescape(link); err == nil {
		link = decoded
	}
	link = strings.TrimSpace(link)
	if u, err := url.Parse(link); err == nil && u.Scheme != "" && u.Host != "" {
		return u.String()
	}
	return link
}

func decodeBingUParam(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	// Bing commonly prefixes the URL-safe base64 payload with "a1".
	if strings.HasPrefix(value, "a1") && len(value) > 2 {
		value = value[2:]
	}

	// Add missing base64 padding when Bing omits it.
	for len(value)%4 != 0 {
		value += "="
	}

	decoded, err := base64.URLEncoding.DecodeString(value)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(value)
	}
	if err != nil {
		return ""
	}
	out := strings.TrimSpace(string(decoded))
	if u, err := url.Parse(out); err == nil && u.Scheme != "" && u.Host != "" {
		return u.String()
	}
	return ""
}
func cleanHTML(s string) string {
	if s == "" {
		return ""
	}
	// Remove tags/scripts that may appear inside titles/snippets, decode entities,
	// and collapse whitespace so parser output is stable across providers.
	s = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = decodeEntities(s)
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func isSearchInternalLink(link string) bool {
	link = strings.TrimSpace(strings.ToLower(link))
	if link == "" || strings.HasPrefix(link, "#") || strings.HasPrefix(link, "javascript:") || strings.HasPrefix(link, "mailto:") {
		return true
	}
	u, err := url.Parse(link)
	if err != nil {
		return true
	}
	if u.Scheme == "" || u.Host == "" {
		return true
	}
	host := strings.TrimPrefix(u.Hostname(), "www.")
	path := strings.ToLower(u.EscapedPath())

	// Exclude search/navigation/redirect-only URLs so generic fallback does not
	// return provider chrome instead of actual web results.
	if strings.Contains(host, "google.") {
		return strings.HasPrefix(path, "/search") || strings.HasPrefix(path, "/preferences") || strings.HasPrefix(path, "/advanced_search") || strings.HasPrefix(path, "/sorry") || strings.HasPrefix(path, "/setprefs")
	}
	if strings.Contains(host, "bing.com") {
		return strings.HasPrefix(path, "/search") || strings.HasPrefix(path, "/images") || strings.HasPrefix(path, "/videos") || strings.HasPrefix(path, "/maps") || strings.HasPrefix(path, "/news") || strings.HasPrefix(path, "/account")
	}
	if strings.Contains(host, "duckduckgo.com") {
		return strings.HasPrefix(path, "/html") || strings.HasPrefix(path, "/lite") || strings.HasPrefix(path, "/settings") || strings.HasPrefix(path, "/params")
	}
	return false
}
