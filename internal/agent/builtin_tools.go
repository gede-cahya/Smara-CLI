package agent

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gede-cahya/Smara-CLI/internal/llm"
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
			Name:        "read_file",
			Description: "Membaca isi file di sistem lokal.",
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
			Description: "Mengubah bagian spesifik dari sebuah file dengan mencari teks lama dan menggantinya dengan teks baru. Sangat berguna untuk file besar.",
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
	}
}

// executeBuiltinTool eksekusi fungsi tool built-in tanpa harus melewati koneksi MCP
func executeBuiltinTool(toolName string, args map[string]interface{}, logCallback func(role, content string)) (string, error) {
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
			return "Direktori kosong.", nil
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

		content := string(data)
		if !strings.Contains(content, oldContent) {
			return "", fmt.Errorf("teks 'old_content' tidak ditemukan di dalam file. Pastikan teks sama persis termasuk spasi.")
		}

		// Hitung berapa kali muncul untuk keamanan
		count := strings.Count(content, oldContent)
		if count > 1 {
			return "", fmt.Errorf("teks 'old_content' muncul %d kali. Mohon berikan blok teks yang lebih unik.", count)
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
			return "", err
		}
		return cwd, nil

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
