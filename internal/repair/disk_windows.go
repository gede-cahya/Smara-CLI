//go:build windows

package repair


func checkDiskSpace(dir string) CheckResult {
	return CheckResult{
		Module:  "disk",
		Status:  StatusOK,
		Message: "Disk space check skipped on Windows",
		Fixable: false,
	}
}
