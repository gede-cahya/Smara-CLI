package repair

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BackupFile creates a timestamped backup of src.
// Format: <src>.backup.<RFC3339>.
// Returns the backup path.
func BackupFile(src string) (string, error) {
	timestamp := time.Now().UTC().Format(time.RFC3339)
	ext := filepath.Ext(src)
	base := strings.TrimSuffix(src, ext)
	backupPath := fmt.Sprintf("%s.backup.%s%s", base, timestamp, ext)

	if err := copyFile(src, backupPath); err != nil {
		return "", fmt.Errorf("gagal backup %s: %w", src, err)
	}

	// Secure permission if file may contain secrets
	_ = os.Chmod(backupPath, 0o600)

	// Cleanup old backups (keep last 5)
	if err := cleanupOldBackups(base, ext); err != nil {
		// non-fatal: just log if we could
		_ = err
	}

	return backupPath, nil
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func cleanupOldBackups(base, ext string) error {
	dir := filepath.Dir(base)
	prefix := filepath.Base(base) + ".backup."

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var backups []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ext) {
			backups = append(backups, filepath.Join(dir, name))
		}
	}

	if len(backups) <= 5 {
		return nil
	}

	// Sort oldest first (by name which includes timestamp)
	sort.Strings(backups)

	// Remove oldest, keep last 5
	for i := 0; i < len(backups)-5; i++ {
		_ = os.Remove(backups[i])
	}
	return nil
}
