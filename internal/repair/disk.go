package repair

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/gede-cahya/Smara-CLI/internal/config"
)

// CheckDiskHealth checks directory writability and available disk space.
func CheckDiskHealth() []CheckResult {
	var results []CheckResult

	// Check .smara directory
	smaraDir := config.SmaraDir()
	res := checkDirWritable(smaraDir)
	results = append(results, res)

	// Check disk space
	res2 := checkDiskSpace(smaraDir)
	results = append(results, res2)

	return results
}

func checkDirWritable(dir string) CheckResult {
	res := CheckResult{
		Module:  "disk",
		Status:  StatusOK,
		Message: fmt.Sprintf("Direktori %s writable", dir),
		Fixable: false,
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		res.Status = StatusFail
		res.Message = fmt.Sprintf("Gagal membuat/mengakses %s: %v", dir, err)
		return res
	}

	testFile := filepath.Join(dir, ".write_test")
	if f, err := os.Create(testFile); err != nil {
		res.Status = StatusFail
		res.Message = fmt.Sprintf("Direktori %s tidak writable: %v", dir, err)
	} else {
		f.Close()
		_ = os.Remove(testFile)
	}

	return res
}

func checkDiskSpace(dir string) CheckResult {
	res := CheckResult{
		Module:  "disk",
		Status:  StatusOK,
		Message: "Disk space cukup",
		Fixable: false,
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil {
		res.Status = StatusWarn
		res.Message = fmt.Sprintf("Gagal cek disk space: %v", err)
		return res
	}

	// Available bytes
	avail := stat.Bavail * uint64(stat.Bsize)
	const minBytes = 100 * 1024 * 1024 // 100 MB

	if avail < minBytes {
		res.Status = StatusWarn
		res.Message = fmt.Sprintf("Disk space rendah: %d MB tersisa", avail/(1024*1024))
		res.Suggestion = "Bersihkan disk atau pindahkan DB"
	} else {
		res.Message = fmt.Sprintf("Disk space OK: %d MB tersisa", avail/(1024*1024))
	}

	return res
}

// CountFiles returns file count in a directory (non-recursive).
func CountFiles(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			count++
		}
	}
	return count, nil
}
