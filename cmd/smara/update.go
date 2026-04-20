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
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

var updateVersion string

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
	},
}

func init() {
	updateCmd.Flags().StringVarP(&updateVersion, "version", "V", "", "Versi spesifik yang ingin diinstal (contoh: 1.3.0)")
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

	ext := ".tar.gz"
	if osName == "windows" {
		ext = ".zip"
	}

	// Try exact match: smara-VERSION-OS-ARCH.ext
	expectedPrefix := fmt.Sprintf("smara-%s-%s-%s", strings.TrimPrefix(release.TagName, "v"), osName, archName)
	for _, a := range release.Assets {
		if strings.HasPrefix(a.Name, expectedPrefix) && strings.HasSuffix(a.Name, ext) {
			return &a, nil
		}
	}

	// Fallback: any asset containing OS-ARCH
	osArch := fmt.Sprintf("%s-%s", osName, archName)
	for _, a := range release.Assets {
		if strings.Contains(a.Name, osArch) && strings.HasSuffix(a.Name, ext) {
			return &a, nil
		}
	}

	return nil, fmt.Errorf("tidak dapat menemukan binary yang sesuai untuk sistem Anda (%s/%s)", osName, archName)
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
		return fmt.Errorf("format arsip tidak didukung: %s", filename)
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
