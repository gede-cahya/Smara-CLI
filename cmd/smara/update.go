package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

var (
	updateVersion   string
	updateNoRestart bool
)

var updateCmd = &cobra.Command{
	Use:   "update [version]",
	Short: "Perbarui Smara CLI ke versi terbaru atau versi spesifik",
	Long:  `Mengunduh dan menginstal pembaruan Smara CLI langsung dari GitHub Releases. Jika versi tidak disertakan, akan menggunakan versi terbaru.`,
	Run: func(cmd *cobra.Command, args []string) {
		targetVersion := updateVersion
		if targetVersion == "" && len(args) > 0 {
			targetVersion = args[0]
		}
		// Strip 'v' prefix if user typed it
		targetVersion = strings.TrimPrefix(targetVersion, "v")

		fmt.Println("🌀 Memeriksa pembaruan Smara...")

		releaseInfo, err := getGitHubRelease(targetVersion)
		if err != nil {
			fmt.Printf("❌ Gagal mendapatkan informasi rilis: %v\n", err)
			os.Exit(1)
		}

		remoteVersion := strings.TrimPrefix(releaseInfo.TagName, "v")

		// Check if already on latest version (only when no specific version requested)
		if targetVersion == "" && remoteVersion == version {
			fmt.Printf("✅ Anda sudah menggunakan versi terbaru: v%s\n", version)
			return
		}

		fmt.Printf("📦 Versi saat ini: v%s → Versi target: v%s\n", version, remoteVersion)

		asset, err := findMatchingAsset(releaseInfo)
		if err != nil {
			fmt.Printf("❌ %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("📥 Mengunduh %s...\n", asset.Name)
		tmpFile, err := downloadAsset(asset.BrowserDownloadURL)
		if err != nil {
			fmt.Printf("❌ Gagal mengunduh: %v\n", err)
			os.Exit(1)
		}
		defer os.Remove(tmpFile)

		fmt.Println("🚀 Mengekstrak dan menerapkan pembaruan...")
		err = extractAndApply(tmpFile, asset.Name)
		if err != nil {
			fmt.Printf("❌ Gagal menerapkan pembaruan: %v\n", err)
			if os.IsPermission(err) {
				fmt.Println("💡 Coba jalankan kembali dengan sudo (contoh: sudo smara update)")
			}
			os.Exit(1)
		}

		fmt.Printf("✅ Pembaruan berhasil! Smara diperbarui dari v%s ke v%s\n", version, remoteVersion)
		if !updateNoRestart {
			if err := activateUpdatedBinary(); err != nil {
				fmt.Printf("⚠️  Binary sudah diperbarui, tapi aktivasi service otomatis gagal: %v\n", err)
				fmt.Println("💡 Jalankan manual: sudo systemctl restart smara")
			}
		}
	},
}

func init() {
	updateCmd.Flags().StringVarP(&updateVersion, "version", "V", "", "Versi spesifik yang ingin diinstal (contoh: 1.4.0)")
	updateCmd.Flags().BoolVar(&updateNoRestart, "no-restart", false, "Jangan restart otomatis service systemd yang memakai binary Smara")
}

type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func getGitHubRelease(version string) (*Release, error) {
	url := "https://api.github.com/repos/gede-cahya/Smara-CLI/releases/latest"
	if version != "" {
		url = fmt.Sprintf("https://api.github.com/repos/gede-cahya/Smara-CLI/releases/tags/v%s", version)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API mengembalikan status: %s", resp.Status)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

func findMatchingAsset(release *Release) (*Asset, error) {
	osName := runtime.GOOS
	archName := runtime.GOARCH

	versionPart := strings.TrimPrefix(release.TagName, "v")
	expectedBase := fmt.Sprintf("smara-%s-%s-%s", versionPart, osName, archName)
	expectedBaseWithV := fmt.Sprintf("smara-v%s-%s-%s", versionPart, osName, archName)

	// Current releases may publish raw executable assets without .tar.gz/.zip,
	// while older updater versions expected archives. Accept both formats.
	validName := func(name string) bool {
		if name == expectedBase || name == expectedBaseWithV {
			return true
		}
		if osName == "windows" && (name == expectedBase+".exe" || name == expectedBaseWithV+".exe") {
			return true
		}
		for _, ext := range []string{".tar.gz", ".zip"} {
			if name == expectedBase+ext || name == expectedBaseWithV+ext {
				return true
			}
		}
		return false
	}

	for _, a := range release.Assets {
		if validName(a.Name) {
			return &a, nil
		}
	}

	// Fallback: any installable asset containing OS-ARCH, excluding checksum files.
	osArch := fmt.Sprintf("%s-%s", osName, archName)
	for _, a := range release.Assets {
		name := a.Name
		if strings.Contains(name, osArch) && !strings.Contains(strings.ToLower(name), "sha256") && !strings.HasSuffix(name, ".txt") {
			return &a, nil
		}
	}

	var names []string
	for _, a := range release.Assets {
		names = append(names, a.Name)
	}
	return nil, fmt.Errorf("tidak dapat menemukan binary yang sesuai untuk sistem Anda (%s/%s). Asset tersedia: %s", osName, archName, strings.Join(names, ", "))
}
func downloadAsset(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status: %s", resp.Status)
	}

	tmpFile, err := os.CreateTemp("", "smara-update-*")
	if err != nil {
		return "", err
	}

	_, err = io.Copy(tmpFile, resp.Body)
	tmpFile.Close()

	if err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

func activateUpdatedBinary() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	services, err := findSystemdServicesUsing(exePath)
	if err != nil {
		return err
	}
	if len(services) == 0 {
		return nil
	}
	fmt.Printf("🔄 Menjadwalkan restart service: %s\n", strings.Join(services, ", "))
	if err := scheduleSystemdRestart(services); err != nil {
		return err
	}
	fmt.Println("✅ Service akan restart otomatis dalam beberapa detik agar binary baru aktif")
	return nil
}

func findSystemdServicesUsing(exePath string) ([]string, error) {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return nil, nil
	}
	resolvedExe := exePath
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		resolvedExe = resolved
	}
	out, err := exec.Command("systemctl", "list-units", "--type=service", "--state=running", "--no-legend", "--no-pager").Output()
	if err != nil {
		return nil, nil
	}
	seen := map[string]bool{}
	var services []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.HasSuffix(fields[0], ".service") {
			continue
		}
		unit := fields[0]
		execStart, err := exec.Command("systemctl", "show", unit, "--property=ExecStart", "--value").Output()
		if err != nil {
			continue
		}
		if !systemdExecUsesSmaraBinary(string(execStart), exePath, resolvedExe) || seen[unit] {
			continue
		}
		seen[unit] = true
		services = append(services, unit)
	}
	return services, nil
}

func systemdExecUsesSmaraBinary(execStart, exePath, resolvedExe string) bool {
	if execStart == "" {
		return false
	}
	if strings.Contains(execStart, exePath) || strings.Contains(execStart, resolvedExe) {
		return true
	}
	base := filepath.Base(exePath)
	return base == "smara" && strings.Contains(execStart, "/smara") && strings.Contains(execStart, " serve")
}

func scheduleSystemdRestart(services []string) error {
	if len(services) == 0 {
		return nil
	}
	unitName := fmt.Sprintf("smara-post-update-restart-%d", os.Getpid())
	args := []string{"--unit", unitName, "--on-active=2s", "/bin/systemctl", "restart"}
	args = append(args, services...)
	out, err := exec.Command("systemd-run", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("gagal menjadwalkan restart service: %w — %s", err, strings.TrimSpace(string(out)))
	}
	if strings.TrimSpace(string(out)) != "" {
		fmt.Print(string(out))
	}
	return nil
}

func extractAndApply(archivePath, filename string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("gagal menemukan lokasi eksekusi: %w", err)
	}

	tmpExtractPath := exePath + ".new"

	if strings.HasSuffix(filename, ".zip") {
		if err := extractFirstFileFromZip(archivePath, tmpExtractPath); err != nil {
			return err
		}
	} else if strings.HasSuffix(filename, ".tar.gz") {
		if err := extractFirstFileFromTarGz(archivePath, tmpExtractPath); err != nil {
			return err
		}
	} else {
		// Raw binary release asset (current release format).
		if err := copyFile(archivePath, tmpExtractPath); err != nil {
			return fmt.Errorf("gagal menyalin binary release: %w", err)
		}
	}

	// Make the extracted binary executable
	if err := os.Chmod(tmpExtractPath, 0755); err != nil {
		os.Remove(tmpExtractPath)
		return fmt.Errorf("gagal mengatur izin eksekusi: %w", err)
	}

	// Replace the old executable
	if err := os.Rename(tmpExtractPath, exePath); err != nil {
		os.Remove(tmpExtractPath)
		return err
	}

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// extractFirstFileFromTarGz extracts the first regular file from a .tar.gz archive.
// This is simpler and more robust than trying to match filenames, since our
// release archives contain exactly one binary file.
func extractFirstFileFromTarGz(tarGzPath, outPath string) error {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Accept any regular file (TypeReg='0' or TypeRegA='\x00')
		if header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA {
			outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
			return nil
		}
	}

	return fmt.Errorf("tidak ada file ditemukan di dalam arsip tar.gz")
}

// extractFirstFileFromZip extracts the first regular file from a .zip archive.
func extractFirstFileFromZip(zipPath, outPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if !f.FileInfo().IsDir() && f.FileInfo().Size() > 0 {
			rc, err := f.Open()
			if err != nil {
				return err
			}

			outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
			if err != nil {
				rc.Close()
				return err
			}

			_, err = io.Copy(outFile, rc)
			outFile.Close()
			rc.Close()

			if err != nil {
				return err
			}
			return nil
		}
	}

	return fmt.Errorf("tidak ada file ditemukan di dalam arsip zip")
}
