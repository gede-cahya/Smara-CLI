//go:build !windows

package repair

import (
	"fmt"
	"syscall"
)

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
