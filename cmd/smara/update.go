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
	"path/filepath"
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

		fmt.Printf("📦 Ditemukan versi: %s\n", releaseInfo.TagName)

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

		fmt.Println("✅ Pembaruan berhasil! Smara sekarang berada pada versi", releaseInfo.TagName)
	},
}

func init() {
	updateCmd.Flags().StringVarP(&updateVersion, "version", "V", "", "Versi spesifik yang ingin diinstal (contoh: 1.2.0)")
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

	// Map architectures based on install.sh
	if archName == "amd64" {
		// x86_64 or amd64 is usually just amd64 in GOARCH
	} else if archName == "arm64" {
		// arm64/aarch64 is arm64 in GOARCH
	}

	ext := ".tar.gz"
	if osName == "windows" {
		ext = ".zip"
	}

	expectedPrefix := fmt.Sprintf("smara-%s-%s-%s", strings.TrimPrefix(release.TagName, "v"), osName, archName)
	// fallback matching
	for _, a := range release.Assets {
		if strings.HasPrefix(a.Name, expectedPrefix) && strings.HasSuffix(a.Name, ext) {
			return &a, nil
		}
	}

	// Another fallback: try without version in case format varies
	expectedPattern := fmt.Sprintf("smara-%s-%s%s", osName, archName, ext)
	for _, a := range release.Assets {
		if strings.Contains(a.Name, expectedPattern) || strings.Contains(a.Name, fmt.Sprintf("%s-%s", osName, archName)) && strings.HasSuffix(a.Name, ext) {
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
		if err := extractZip(archivePath, tmpExtractPath); err != nil {
			return err
		}
	} else if strings.HasSuffix(filename, ".tar.gz") {
		if err := extractTarGz(archivePath, tmpExtractPath); err != nil {
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

func extractTarGz(tarGzPath, outPath string) error {
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

		if header.Typeflag == tar.TypeReg {
			// Find the binary file (usually named "smara" or "smara.exe" inside the archive)
			if filepath.Base(header.Name) == "smara" || filepath.Base(header.Name) == "smara.exe" {
				outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
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
	}

	return fmt.Errorf("file binary smara tidak ditemukan di dalam arsip tar.gz")
}

func extractZip(zipPath, outPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if filepath.Base(f.Name) == "smara" || filepath.Base(f.Name) == "smara.exe" {
			rc, err := f.Open()
			if err != nil {
				return err
			}

			outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, f.Mode())
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

	return fmt.Errorf("file binary smara.exe tidak ditemukan di dalam arsip zip")
}
