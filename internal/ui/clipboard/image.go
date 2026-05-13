// Package clipboard — image support.
//
// Reads an image from the system clipboard, writes it to a temp file,
// and returns the path. Used by the TUI when the user presses Ctrl+V
// and the clipboard happens to contain an image (rather than text).
//
// Supported platforms:
//   - Linux X11    : xclip
//   - Linux Wayland: wl-paste
//   - macOS        : osascript (built-in)
//   - Windows      : powershell Get-Clipboard
//
// Each helper is a separate exec.Command call. We prefer image/png because
// every platform's native clipboard format includes it.
package clipboard

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ErrNoImage is returned when the system clipboard does not contain an image.
var ErrNoImage = errors.New("tidak ada gambar di clipboard")

// ImagePasteResult describes the file written from the clipboard.
type ImagePasteResult struct {
	Path   string // absolute path to the temp PNG file
	Size   int64  // bytes
	Source string // platform: "x11", "wayland", "macos", "windows"
}

// ReadImage attempts to read an image from the system clipboard and write
// it to a temp file. Returns ErrNoImage if the clipboard has no image.
//
// Caller is responsible for the temp file lifecycle (remove when done).
// Use SmaraTempImagePath to get a stable directory under ~/.smara/clip-images.
func ReadImage() (*ImagePasteResult, error) {
	switch runtime.GOOS {
	case "linux":
		return readImageLinux()
	case "darwin":
		return readImageMacOS()
	case "windows":
		return readImageWindows()
	default:
		return nil, fmt.Errorf("paste image belum didukung di %s", runtime.GOOS)
	}
}

// SmaraTempImagePath returns a stable directory used to keep clipboard
// images so attached files survive across the prompt lifecycle.
// Files are pruned (>1h old, max 50 files) on each call.
func SmaraTempImagePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".smara", "clip-images")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	pruneOldClipImages(dir)
	return dir, nil
}

// pruneOldClipImages removes files older than 1h, keeps newest 50.
func pruneOldClipImages(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type entry struct {
		path string
		mod  time.Time
	}
	var files []entry
	cutoff := time.Now().Add(-1 * time.Hour)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(p)
			continue
		}
		files = append(files, entry{p, info.ModTime()})
	}
	if len(files) > 50 {
		// Sort newest first; remove oldest beyond 50.
		// Simple selection sort is fine for small N.
		for i := 0; i < len(files); i++ {
			max := i
			for j := i + 1; j < len(files); j++ {
				if files[j].mod.After(files[max].mod) {
					max = j
				}
			}
			files[i], files[max] = files[max], files[i]
		}
		for _, f := range files[50:] {
			_ = os.Remove(f.path)
		}
	}
}

// ─────────────────── Linux ───────────────────

func readImageLinux() (*ImagePasteResult, error) {
	// Wayland first if WAYLAND_DISPLAY is set.
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if res, err := readImageWlPaste(); err == nil {
			return res, nil
		}
	}
	// Fallback to xclip (works on X11 and XWayland).
	return readImageXclip()
}

func readImageWlPaste() (*ImagePasteResult, error) {
	if _, err := exec.LookPath("wl-paste"); err != nil {
		return nil, fmt.Errorf("wl-paste tidak terinstall (sudo pacman -S wl-clipboard / apt install wl-clipboard)")
	}
	// Check available types; if "image/" is not present, no image.
	types, err := runCmdString(exec.Command("wl-paste", "--list-types"))
	if err != nil {
		return nil, err
	}
	if !strings.Contains(types, "image/") {
		return nil, ErrNoImage
	}
	return writeClipImage("wayland", exec.Command("wl-paste", "--type", "image/png"))
}

func readImageXclip() (*ImagePasteResult, error) {
	if _, err := exec.LookPath("xclip"); err != nil {
		return nil, fmt.Errorf("xclip tidak terinstall (sudo pacman -S xclip / apt install xclip)")
	}
	// Check available targets.
	targets, err := runCmdString(exec.Command("xclip", "-selection", "clipboard", "-t", "TARGETS", "-o"))
	if err != nil {
		return nil, fmt.Errorf("xclip targets gagal: %w", err)
	}
	if !strings.Contains(targets, "image/") {
		return nil, ErrNoImage
	}
	return writeClipImage("x11", exec.Command("xclip", "-selection", "clipboard", "-t", "image/png", "-o"))
}

// ─────────────────── macOS ───────────────────

// readImageMacOS uses AppleScript to query and write the clipboard image.
// pbpaste does NOT support image clipboard data on macOS, so we go through
// osascript which has a `the clipboard as «class PNGf»` trick.
func readImageMacOS() (*ImagePasteResult, error) {
	dir, err := SmaraTempImagePath()
	if err != nil {
		return nil, err
	}
	out := filepath.Join(dir, fmt.Sprintf("clip-%d.png", time.Now().UnixNano()))

	script := fmt.Sprintf(`
		try
			set imgData to the clipboard as «class PNGf»
		on error
			return "NOIMAGE"
		end try
		set fp to open for access POSIX file "%s" with write permission
		set eof of fp to 0
		write imgData to fp
		close access fp
		return "OK"
	`, out)

	cmd := exec.Command("osascript", "-e", script)
	res, err := runCmdString(cmd)
	if err != nil {
		return nil, fmt.Errorf("osascript gagal: %w", err)
	}
	if strings.TrimSpace(res) == "NOIMAGE" {
		return nil, ErrNoImage
	}
	st, err := os.Stat(out)
	if err != nil {
		return nil, fmt.Errorf("file PNG tidak terbentuk: %w", err)
	}
	return &ImagePasteResult{Path: out, Size: st.Size(), Source: "macos"}, nil
}

// ─────────────────── Windows ───────────────────

func readImageWindows() (*ImagePasteResult, error) {
	if _, err := exec.LookPath("powershell"); err != nil {
		return nil, fmt.Errorf("powershell tidak ditemukan")
	}
	dir, err := SmaraTempImagePath()
	if err != nil {
		return nil, err
	}
	out := filepath.Join(dir, fmt.Sprintf("clip-%d.png", time.Now().UnixNano()))

	script := fmt.Sprintf(`
$ErrorActionPreference='Stop'
Add-Type -AssemblyName System.Windows.Forms
$img = [System.Windows.Forms.Clipboard]::GetImage()
if ($null -eq $img) { Write-Output "NOIMAGE"; exit 0 }
$img.Save("%s",[System.Drawing.Imaging.ImageFormat]::Png)
Write-Output "OK"
`, strings.ReplaceAll(out, `\`, `\\`))

	cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
	res, err := runCmdString(cmd)
	if err != nil {
		return nil, fmt.Errorf("powershell gagal: %w", err)
	}
	if strings.Contains(res, "NOIMAGE") {
		return nil, ErrNoImage
	}
	st, err := os.Stat(out)
	if err != nil {
		return nil, fmt.Errorf("file PNG tidak terbentuk: %w", err)
	}
	return &ImagePasteResult{Path: out, Size: st.Size(), Source: "windows"}, nil
}

// ─────────────────── Helpers ───────────────────

// writeClipImage runs cmd, expecting binary PNG on stdout, and writes it
// to a temp file inside ~/.smara/clip-images.
func writeClipImage(source string, cmd *exec.Cmd) (*ImagePasteResult, error) {
	dir, err := SmaraTempImagePath()
	if err != nil {
		return nil, err
	}
	out := filepath.Join(dir, fmt.Sprintf("clip-%d.png", time.Now().UnixNano()))

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	f, err := os.Create(out)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	defer f.Close()

	n, copyErr := io.Copy(f, stdout)
	waitErr := cmd.Wait()
	if copyErr != nil {
		_ = os.Remove(out)
		return nil, copyErr
	}
	if waitErr != nil {
		_ = os.Remove(out)
		return nil, waitErr
	}
	if n < 256 {
		// Suspiciously small for a PNG; treat as no image.
		_ = os.Remove(out)
		return nil, ErrNoImage
	}
	return &ImagePasteResult{Path: out, Size: n, Source: source}, nil
}

// WriteImage copies a file (typically PNG) into the system clipboard.
// Reverse of ReadImage. Useful when the agent generates an image and
// wants to make it available for paste in other apps.
func WriteImage(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "linux":
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			if _, err := exec.LookPath("wl-copy"); err == nil {
				return runStdinFile(exec.Command("wl-copy", "--type", "image/png"), path)
			}
		}
		if _, err := exec.LookPath("xclip"); err == nil {
			return runStdinFile(exec.Command("xclip", "-selection", "clipboard", "-t", "image/png", "-i"), path)
		}
		return fmt.Errorf("install xclip atau wl-clipboard untuk copy image")
	case "darwin":
		// Use osascript to set PNG clipboard.
		script := fmt.Sprintf(`set the clipboard to (read (POSIX file "%s") as «class PNGf»)`, path)
		_, err := runCmdString(exec.Command("osascript", "-e", script))
		return err
	case "windows":
		// PowerShell loads image and sets clipboard.
		script := fmt.Sprintf(`
$img = [System.Drawing.Image]::FromFile("%s")
[System.Windows.Forms.Clipboard]::SetImage($img)
`, strings.ReplaceAll(path, `\`, `\\`))
		_, err := runCmdString(exec.Command("powershell", "-NoProfile", "-Command", script))
		return err
	}
	return fmt.Errorf("copy image belum didukung di %s", runtime.GOOS)
}

func runCmdString(cmd *exec.Cmd) (string, error) {
	out, err := cmd.Output()
	return string(out), err
}

func runStdinFile(cmd *exec.Cmd, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	cmd.Stdin = f
	return cmd.Run()
}
