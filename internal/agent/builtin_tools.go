package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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
	}
}

// executeBuiltinTool eksekusi fungsi tool built-in tanpa harus melewati koneksi MCP
func executeBuiltinTool(toolName string, args map[string]interface{}) (string, error) {
	switch toolName {
	case "run_command":
		cmdStr, ok := args["command"].(string)
		if !ok {
			return "", fmt.Errorf("argumen 'command' tidak valid")
		}

		// Jalankan command menggunakan shell default bash/sh
		cmd := exec.Command("sh", "-c", cmdStr)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return string(out), fmt.Errorf("eksekusi gagal: %w\nOutput: %s", err, string(out))
		}
		
		result := string(out)
		if result == "" {
			result = "Perintah berhasil dieksekusi tanpa output."
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

	default:
		return "", fmt.Errorf("tool built-in '%s' tidak dikenali", toolName)
	}
}
